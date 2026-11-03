package agentmcp

// 会话层测试（STAGE-300 第 5 节的单元测试清单）：握手登录、按 id 配对、推送缓冲、
// 等待推送、战斗确认、压缩行、不可重连的断开、断线重连、令牌不泄露、文档修订号提醒。

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 握手、登录、进世界（connect）；协议请求的字段也一起检查。
func TestConnectHandshakeAndEnterWorld(t *testing.T) {
	fake := newFakeSrv(t)
	srv := newTestServer(t, fake, nil)

	out := callTool(t, srv, "connect", `{}`)
	if out["ok"] != true {
		t.Fatalf("connect 应该成功：%v", out)
	}
	data := out["data"].(map[string]any)
	if _, ok := data["position"]; !ok {
		t.Fatalf("connect 应返回 position：%v", data)
	}
	hello := fake.waitType("hello", time.Second)
	hd := hello["data"].(map[string]any)
	if hd["role"] != "agent" || hd["client"] != clientVersion {
		t.Fatalf("hello 字段不对：%v", hd)
	}
	if c, _ := hd["compress"].([]any); len(c) != 1 || c[0] != "gzip" {
		t.Fatalf("hello 应声明 gzip：%v", hd["compress"])
	}
	if fake.count("characters") != 1 || fake.count("agent_login") != 1 {
		t.Fatalf("应该登录并取角色一次")
	}
	ew := fake.waitType("enter_world", time.Second)
	if ew["data"].(map[string]any)["slot"] != float64(0) {
		t.Fatalf("enter_world 应使用第一个角色的栏位：%v", ew)
	}
}

// 没有调用工具之前不连接（需求第 2 节：启动时不立即连接）。
func TestNoDialUntilFirstCall(t *testing.T) {
	fake := newFakeSrv(t)
	_ = newTestServer(t, fake, nil)
	time.Sleep(50 * time.Millisecond)
	if n := fake.connCount(); n != 0 {
		t.Fatalf("启动时不应连接，实际连了 %d 次", n)
	}
}

// 响应按 id 配对，中间穿插的推送进入 events，取出后清空。
func TestResponsePairingWithPushes(t *testing.T) {
	fake := newFakeSrv(t)
	fake.script = func(fc *fakeConn, req map[string]any) bool {
		if req["type"] != "status" {
			return false
		}
		fc.send(map[string]any{"type": "control_changed", "data": map[string]any{"controller": "player"}})
		fc.send(map[string]any{"type": "agent_message", "data": map[string]any{"text": "hi"}})
		fc.send(okLine(req, map[string]any{"level": 7}))
		return true
	}
	srv := newTestServer(t, fake, nil)
	callTool(t, srv, "connect", `{}`)

	out := callTool(t, srv, "status", `{}`)
	if out["ok"] != true || out["data"].(map[string]any)["level"] != float64(7) {
		t.Fatalf("status 应拿到自己的响应：%v", out)
	}
	events, ok := out["events"].([]any)
	if !ok || len(events) != 2 {
		t.Fatalf("应带上两条推送：%v", out["events"])
	}
	if events[0].(map[string]any)["type"] != "control_changed" {
		t.Fatalf("推送顺序不对：%v", events)
	}
	// 取出后清空
	again := callTool(t, srv, "inventory", `{}`)
	if _, has := again["events"]; has {
		t.Fatalf("上次的推送应已取出清空：%v", again["events"])
	}
}

// 压缩行自动解压（STAGE-246）。
func TestGzipLineIsUnpacked(t *testing.T) {
	fake := newFakeSrv(t)
	big := strings.Repeat("地图", 6000) // 远超 10 KiB 压缩阈值
	fake.script = func(fc *fakeConn, req map[string]any) bool {
		if req["type"] != "status" {
			return false
		}
		fc.sendGzip(map[string]any{"type": "big_push", "data": map[string]any{"text": big}})
		fc.send(okLine(req, map[string]any{}))
		return true
	}
	srv := newTestServer(t, fake, nil)
	callTool(t, srv, "connect", `{}`)
	out := callTool(t, srv, "status", `{}`)
	events, _ := out["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("应解出一条推送：%v", out)
	}
	text := events[0].(map[string]any)["data"].(map[string]any)["text"]
	if text != big {
		t.Fatalf("解压后的内容不对")
	}
}

// 队列满了丢最旧的，并计数（需求第 2 节）。
func TestEventQueueDropsOldest(t *testing.T) {
	q := newEventQueue()
	for i := 0; i < maxEvents+1; i++ {
		q.push(event{Type: "t", Raw: json.RawMessage(`{"i":` + string(rune('0'+i%10)) + `}`)})
	}
	items, dropped := q.drain()
	if len(items) != maxEvents || dropped != 1 {
		t.Fatalf("应保留 %d 条、丢弃 1 条，实际 %d 条、丢弃 %d 条", maxEvents, len(items), dropped)
	}
	if _, dropped := q.drain(); dropped != 0 {
		t.Fatalf("丢弃计数取出后应清零")
	}
}

// 丢弃计数出现在工具结果里。
func TestDroppedEventsInResult(t *testing.T) {
	fake := newFakeSrv(t)
	srv := newTestServer(t, fake, nil)
	for i := 0; i < maxEvents+3; i++ {
		srv.sess.events.push(event{Type: "x", Raw: json.RawMessage(`{"type":"x"}`)})
	}
	out := callTool(t, srv, "inventory", `{}`)
	if out["dropped_events"] != float64(3) {
		t.Fatalf("应带 dropped_events=3：%v", out["dropped_events"])
	}
}

// wait_events：等到匹配的推送就返回；超时返回 matched=false。
func TestWaitEventsMatchAndTimeout(t *testing.T) {
	q := newEventQueue()
	go func() {
		time.Sleep(50 * time.Millisecond)
		q.push(event{Type: "battle_turn", Raw: json.RawMessage(`{"type":"battle_turn"}`)})
		time.Sleep(50 * time.Millisecond)
		q.push(event{Type: "battle_end", Raw: json.RawMessage(`{"type":"battle_end"}`)})
	}()
	start := time.Now()
	if !q.waitFor(context.Background(), []string{"battle_end"}, 2*time.Second) {
		t.Fatal("应等到 battle_end")
	}
	if time.Since(start) > time.Second {
		t.Fatal("等到匹配推送后应尽快返回")
	}
	if q.waitFor(context.Background(), []string{"nothing"}, 100*time.Millisecond) {
		t.Fatal("没有匹配的推送时应超时返回 false")
	}
}

// 联调发现：最后一回合的 battle_end 随出招结果一起交出去了，紧接着的 wait_events 要立刻返回，不能等到超时；
// wait_events 自己交出的推送不记，连续两次 wait_events 第二次照常等待。
func TestWaitEventsSeesAlreadyDelivered(t *testing.T) {
	q := newEventQueue()
	q.push(event{Type: "battle_round"})
	q.push(event{Type: "battle_end"})
	q.drain() // 出招工具的结果把它们交出去了
	if hit := q.takeRecent([]string{"battle_turn", "battle_end"}); len(hit) != 1 || hit[0] != "battle_end" {
		t.Fatalf("应认出已交出的 battle_end：%v", hit)
	}
	q.drain() // wait_events 自己的结果
	if hit := q.takeRecent([]string{"battle_end"}); len(hit) != 0 {
		t.Fatalf("同一条推送不应再算一次：%v", hit)
	}

	q.push(event{Type: "battle_turn"})
	q.takeRecent([]string{"battle_turn"}) // wait_events 开始，等到 battle_turn
	q.drain()                             // wait_events 的结果交出 battle_turn
	if hit := q.takeRecent([]string{"battle_turn"}); len(hit) != 0 {
		t.Fatalf("wait_events 交出的推送不应让下一次 wait_events 立刻返回：%v", hit)
	}
	q.drain()
	q.push(event{Type: "battle_turn"})
	q.drain()
	if hit := q.takeRecent(nil); len(hit) != 0 {
		t.Fatalf("不填 types 时不看已交出的推送：%v", hit)
	}
}

// 收到 battle_round 自动发 battle_ack（协议 10.8），AI 不用管。
func TestBattleRoundAutoAck(t *testing.T) {
	fake := newFakeSrv(t)
	fake.script = func(fc *fakeConn, req map[string]any) bool {
		if req["type"] != "status" {
			return false
		}
		fc.send(map[string]any{"type": "battle_round", "data": map[string]any{"battle_id": "b_1", "turn": 3, "events": []any{}}})
		fc.send(okLine(req, map[string]any{}))
		return true
	}
	srv := newTestServer(t, fake, nil)
	callTool(t, srv, "connect", `{}`)
	callTool(t, srv, "status", `{}`)
	ack := fake.waitType("battle_ack", 2*time.Second)
	if ack["data"].(map[string]any)["turn"] != float64(3) {
		t.Fatalf("battle_ack 的 turn 应为 3：%v", ack)
	}
}

// KICKED_BY_PLAYER：不重连，之后的调用直接返回原因。
func TestKickedByPlayerNoReconnect(t *testing.T) {
	fake := newFakeSrv(t)
	fake.script = func(fc *fakeConn, req map[string]any) bool {
		if req["type"] != "status" {
			return false
		}
		fc.send(map[string]any{"type": "disconnect", "data": map[string]any{"reason": "KICKED_BY_PLAYER", "message": "玩家让你下线"}})
		fc.conn.Close()
		return true
	}
	srv := newTestServer(t, fake, nil)
	callTool(t, srv, "connect", `{}`)
	first := callTool(t, srv, "status", `{}`)
	if first["ok"] != false {
		t.Fatalf("断开时这次调用应失败：%v", first)
	}
	if ev, _ := first["events"].([]any); len(ev) == 0 {
		t.Fatalf("disconnect 推送应随结果返回：%v", first)
	}
	second := callTool(t, srv, "status", `{}`)
	if errCode(second) != "KICKED_BY_PLAYER" {
		t.Fatalf("之后的调用应返回断开原因：%v", second)
	}
	msg := second["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "不要重连") {
		t.Fatalf("应带应对说明：%s", msg)
	}
	if n := fake.connCount(); n != 1 {
		t.Fatalf("被踢后不应再拨号，实际连接 %d 次", n)
	}
}

// 普通断线：下一次调用自动重连（重新登录），并补一条 mcp_closed 推送。
func TestReconnectAfterDrop(t *testing.T) {
	fake := newFakeSrv(t)
	var dropped atomic.Bool
	fake.script = func(fc *fakeConn, req map[string]any) bool {
		if req["type"] != "status" || !dropped.CompareAndSwap(false, true) {
			return false
		}
		fc.conn.Close()
		return true
	}
	srv := newTestServer(t, fake, nil)
	callTool(t, srv, "connect", `{}`)
	first := callTool(t, srv, "status", `{}`)
	if first["ok"] != false {
		t.Fatalf("断线时应失败：%v", first)
	}
	if ev, _ := first["events"].([]any); len(ev) != 1 || ev[0].(map[string]any)["type"] != "mcp_closed" {
		t.Fatalf("应补一条 mcp_closed：%v", first["events"])
	}
	out := callTool(t, srv, "status", `{}`)
	if out["ok"] != true {
		t.Fatalf("下一次调用应自动重连成功：%v", out)
	}
	if n := fake.connCount(); n != 2 {
		t.Fatalf("应重连一次，实际连接 %d 次", n)
	}
}

// 连续 3 次连接失败后不再重连。
func TestReconnectLimit(t *testing.T) {
	fake := newFakeSrv(t)
	addr := fake.addr()
	fake.ln.Close() // 服务端不在了
	srv := New(Config{Server: addr, Token: testToken, StatePath: filepath.Join(t.TempDir(), "s.json")})
	t.Cleanup(srv.Close)
	for i := 0; i < maxConnectFailures; i++ {
		out := callTool(t, srv, "status", `{}`)
		if errCode(out) != "CONNECT_FAILED" {
			t.Fatalf("第 %d 次应连接失败：%v", i+1, out)
		}
	}
	out := callTool(t, srv, "status", `{}`)
	if errCode(out) != "RECONNECT_LIMIT" {
		t.Fatalf("连续失败 3 次后应停止重连：%v", out)
	}
}

// request 只在本地拦 MCP 自己管理的四个请求，它们不会发到服务端，也不会建立连接（STAGE-303）。
func TestRequestAllowList(t *testing.T) {
	fake := newFakeSrv(t)
	srv := newTestServer(t, fake, nil)
	for _, typ := range []string{"hello", "agent_login", "ping", "battle_ack"} {
		if c := errCode(callTool(t, srv, "request", `{"type":"`+typ+`"}`)); c != "NOT_ALLOWED" {
			t.Fatalf("%s 应被拒绝，实际 %q", typ, c)
		}
		if n := fake.count(typ); n != 0 {
			t.Fatalf("%s 被本地拦下，不应发到服务端，实际 %d 次", typ, n)
		}
	}
	if n := fake.connCount(); n != 0 {
		t.Fatalf("被本地拦下的请求不应建立连接，实际 %d 次", n)
	}
}

// STAGE-303：MCP 编译时不存在的请求类型（如 future_feature）原样转给服务端；服务端的成功和错误都原样返回。
func TestRequestForwardsUnknownType(t *testing.T) {
	fake := newFakeSrv(t)
	fake.script = func(fc *fakeConn, req map[string]any) bool {
		switch req["type"] {
		case "future_feature":
			fc.sendLine(okLine(req, map[string]any{"echo": "ok"}))
			return true
		case "create_agent_token":
			fc.sendLine(errLine(req, "WRONG_ROLE", "只能由玩家连接发送"))
			return true
		case "no_such_request":
			fc.sendLine(errLine(req, "UNKNOWN_TYPE", "没有这个请求类型：no_such_request"))
			return true
		}
		return false
	}
	srv := newTestServer(t, fake, nil)

	out := callTool(t, srv, "request", `{"type":"future_feature","data":{"x":1}}`)
	if out["ok"] != true {
		t.Fatalf("未知类型应转给服务端并返回其结果，实际 %v", out)
	}
	if d, _ := out["data"].(map[string]any); d["echo"] != "ok" {
		t.Fatalf("服务端结果应原样返回：%v", out["data"])
	}
	req := fake.waitType("future_feature", time.Second)
	if d, _ := req["data"].(map[string]any); d["x"] != float64(1) {
		t.Fatalf("data 应原样转给服务端：%v", req)
	}

	// 玩家专用请求不再在本地拦，由服务端回 WRONG_ROLE。
	if c := errCode(callTool(t, srv, "request", `{"type":"create_agent_token"}`)); c != "WRONG_ROLE" {
		t.Fatalf("玩家专用请求应由服务端判断，实际 %q", c)
	}
	if n := fake.count("create_agent_token"); n != 1 {
		t.Fatalf("玩家专用请求应发到服务端，实际 %d 次", n)
	}
	if c := errCode(callTool(t, srv, "request", `{"type":"no_such_request"}`)); c != "UNKNOWN_TYPE" {
		t.Fatalf("服务端的 UNKNOWN_TYPE 应原样返回，实际 %q", c)
	}
}

// 令牌不出现在协议日志和工具结果里（需求第 3 节）。
func TestTokenNotLeaked(t *testing.T) {
	fake := newFakeSrv(t)
	var logBuf testWriter
	srv := newTestServer(t, fake, &logBuf)
	out := callTool(t, srv, "connect", `{}`)
	if out["ok"] != true {
		t.Fatalf("connect 应成功：%v", out)
	}
	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte(testToken)) || strings.Contains(logBuf.String(), testToken) {
		t.Fatal("令牌不应出现在工具结果或日志里")
	}
	if !strings.Contains(logBuf.String(), "agent_login") {
		t.Fatal("协议日志应记录请求（脱敏后）")
	}
}

// docs_rev 变了：下一次工具结果带 docs_updated，并记下新的修订号。
func TestDocsUpdatedNotice(t *testing.T) {
	fake := newFakeSrv(t)
	fake.docRev = "rev-new"
	srv := newTestServer(t, fake, nil)
	statePath := srv.cfg.StatePath
	key := docKey(srv.cfg.Server)
	if err := saveDocsState(statePath, map[string]string{key: "rev-old"}); err != nil {
		t.Fatal(err)
	}
	out := callTool(t, srv, "connect", `{}`)
	notice, _ := out["docs_updated"].(string)
	if !strings.Contains(notice, "rev-old") || !strings.Contains(notice, "rev-new") {
		t.Fatalf("应提示新旧修订号：%v", out["docs_updated"])
	}
	raw, err := os.ReadFile(statePath)
	if err != nil || !strings.Contains(string(raw), "rev-new") {
		t.Fatalf("应记下新的修订号：%s %v", raw, err)
	}
	if again := callTool(t, srv, "status", `{}`); again["docs_updated"] != nil {
		t.Fatal("提示只出现一次")
	}
}

// wiki_search：TCP 连接时没给 --wiki 就返回不可用。
func TestWikiUnavailableOnTCP(t *testing.T) {
	fake := newFakeSrv(t)
	srv := newTestServer(t, fake, nil)
	if c := errCode(callTool(t, srv, "wiki_search", `{"q":"加加村"}`)); c != "WIKI_UNAVAILABLE" {
		t.Fatalf("应返回 WIKI_UNAVAILABLE，实际 %q", c)
	}
}
