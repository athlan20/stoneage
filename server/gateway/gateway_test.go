package gateway

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

func TestHelloAndPing(t *testing.T) {
	_, addr := startServer(t, Options{RegisterEnabled: true})
	c := dial(t, addr)
	m := c.hello("player")
	d := m["data"].(map[string]any)
	if d["protocol"] != float64(1) || d["heartbeat_seconds"] != float64(20) || d["idle_timeout_seconds"] != float64(60) || d["register_enabled"] != true {
		t.Errorf("hello 响应不对: %v", d)
	}
	c.send(`{"id":2,"type":"ping","data":{}}`)
	m = c.recv()
	if m["id"] != float64(2) || m["type"] != "ping" || m["ok"] != true || m["data"].(map[string]any)["server_time_ms"] == nil {
		t.Errorf("ping 响应不对: %v", m)
	}
}

func TestVersionMismatch(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.send(`{"id":1,"type":"hello","data":{"protocol":99}}`)
	m := c.recv()
	if errCode(m) != protocol.CodeProtocolVersion {
		t.Fatalf("应回复 PROTOCOL_VERSION: %v", m)
	}
	sup := m["error"].(map[string]any)["data"].(map[string]any)["supported"].([]any)
	if len(sup) != 1 || sup[0] != float64(1) {
		t.Errorf("supported = %v", sup)
	}
	c.expectClosed()
}

func TestRequestBeforeHello(t *testing.T) {
	_, addr := startServer(t, Options{})
	for _, line := range []string{`{"id":1,"type":"ping"}`, `{"id":1,"type":"login","data":{"account":"a","password":"b"}}`, `{"id":1,"type":"nope"}`} {
		c := dial(t, addr)
		c.send(line)
		if m := c.recv(); errCode(m) != protocol.CodeWrongState {
			t.Fatalf("%s: 应回复 WRONG_STATE: %v", line, m)
		}
		c.expectClosed()
	}
}

func TestWrongRole(t *testing.T) {
	_, addr := startServer(t, Options{})
	// 验收项：声明 agent 身份却用 login 登录，回复 WRONG_ROLE。
	c := dial(t, addr)
	c.hello("agent")
	c.send(`{"id":2,"type":"login","data":{"account":"alice","password":"secret1"}}`)
	if m := c.recv(); errCode(m) != protocol.CodeWrongRole {
		t.Fatalf("agent 发 login 应回复 WRONG_ROLE: %v", m)
	}
	// 连接保持，可以继续发合法请求。
	c.send(`{"id":3,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("WRONG_ROLE 后连接应保持: %v", m)
	}
	// role 缺省视为 player，玩家发 agent_login 同样被拒绝。
	p := dial(t, addr)
	p.send(`{"id":1,"type":"hello","data":{"protocol":1}}`)
	p.recv()
	p.send(`{"id":2,"type":"agent_login","data":{"token":"sat_x"}}`)
	if m := p.recv(); errCode(m) != protocol.CodeWrongRole {
		t.Fatalf("玩家发 agent_login 应回复 WRONG_ROLE: %v", m)
	}
}

func TestStateAndUnknown(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player")
	cases := []struct{ line, code string }{
		{`{"id":2,"type":"hello","data":{"protocol":1}}`, protocol.CodeWrongState}, // 重复 hello
		{`{"id":3,"type":"characters"}`, protocol.CodeWrongState},                  // 还没登录
		{`{"id":4,"type":"enter_world","data":{"slot":0}}`, protocol.CodeWrongState},
		{`{"id":5,"type":"no_such_type"}`, protocol.CodeUnknownType},
		{`{"id":6,"type":"login","data":{}}`, protocol.CodeUnknownType}, // 已登记但本阶段还没有处理函数
	}
	for _, tt := range cases {
		c.send(tt.line)
		if m := c.recv(); errCode(m) != tt.code {
			t.Errorf("%s: 得到 %v，期望 %s", tt.line, m, tt.code)
		}
	}
	c.send(`{"id":9,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("这些错误都不应断开连接: %v", m)
	}
}

func TestHelloBadFields(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	for _, line := range []string{
		`{"id":1,"type":"hello","data":{}}`,
		`{"id":2,"type":"hello","data":{"protocol":"1"}}`,
		`{"id":3,"type":"hello","data":{"protocol":1,"role":"robot"}}`,
	} {
		c.send(line)
		if m := c.recv(); errCode(m) != protocol.CodeBadRequest {
			t.Errorf("%s: 应回复 BAD_REQUEST: %v", line, m)
		}
	}
	c.hello("player") // 握手失败后连接仍可继续握手
}

func TestBadData(t *testing.T) {
	_, addr := startServer(t, Options{})
	// 无法解析：回复 BAD_REQUEST 后断开。
	c := dial(t, addr)
	c.hello("player")
	c.send(`this is not json`)
	m := c.recv()
	if errCode(m) != protocol.CodeBadRequest || m["id"] != float64(0) || m["type"] != "error" {
		t.Fatalf("坏数据响应不对: %v", m)
	}
	c.expectClosed()
	// 能解析但字段不对：回复 BAD_REQUEST，连接保持。
	c = dial(t, addr)
	c.hello("player")
	for _, line := range []string{`{"id":"x","type":"ping"}`, `{"id":5}`, `[1,2]`, `{"id":6,"type":"ping","data":3}`} {
		c.send(line)
		if m := c.recv(); errCode(m) != protocol.CodeBadRequest {
			t.Errorf("%s: 应回复 BAD_REQUEST: %v", line, m)
		}
	}
	c.send(`{"id":7,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("字段错误不应断开连接: %v", m)
	}
}

func TestLineTooLong(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player")
	c.send(strings.Repeat("a", protocol.MaxLineBytes+1))
	if m := c.recv(); errCode(m) != protocol.CodeBadRequest {
		t.Fatalf("超长行应回复 BAD_REQUEST: %v", m)
	}
	c.expectClosed()
	// 刚好 64 KiB 的一行不算超长（这里是合法 JSON 但缺字段，回复 BAD_REQUEST 且保持连接）。
	c = dial(t, addr)
	c.hello("player")
	pad := strings.Repeat("a", protocol.MaxLineBytes-len(`{"x":""}`))
	c.send(`{"x":"` + pad + `"}`)
	if m := c.recv(); errCode(m) != protocol.CodeBadRequest {
		t.Fatalf("%v", m)
	}
	c.send(`{"id":3,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("64 KiB 的行不应断开连接: %v", m)
	}
}

func TestIdleTimeout(t *testing.T) {
	_, addr := startServer(t, Options{IdleTimeout: 700 * time.Millisecond})
	c := dial(t, addr)
	c.hello("player")
	m := c.recv()
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonIdleTimeout || m["id"] != nil {
		t.Fatalf("应推送 IDLE_TIMEOUT: %v", m)
	}
	c.expectClosed()
}

func TestPingKeepsConnectionAlive(t *testing.T) {
	_, addr := startServer(t, Options{IdleTimeout: 600 * time.Millisecond})
	c := dial(t, addr)
	c.hello("player")
	for i := 0; i < 6; i++ {
		time.Sleep(250 * time.Millisecond)
		c.send(fmt.Sprintf(`{"id":%d,"type":"ping"}`, i+2))
		if m := c.recv(); m["ok"] != true {
			t.Fatalf("第 %d 次心跳应成功: %v", i, m)
		}
	}
}

func TestShutdownPushesDisconnect(t *testing.T) {
	srv, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	m := c.recv()
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonServerShutdown {
		t.Fatalf("应推送 SERVER_SHUTDOWN: %v", m)
	}
	c.expectClosed()
}

// 验收项：30 个连接同时在线互不影响，其中一个发坏数据只断开它自己。
func TestThirtyConnectionsIsolation(t *testing.T) {
	srv, addr := startServer(t, Options{})
	const n = 30
	clients := make([]*testClient, n)
	for i := range clients {
		clients[i] = dial(t, addr)
		clients[i].hello("player")
	}
	if got := srv.Hub().Count(); got != n {
		t.Fatalf("在线连接数 = %d，期望 %d", got, n)
	}
	clients[7].send(`{{{{ garbage`)
	if m := clients[7].recv(); errCode(m) != protocol.CodeBadRequest {
		t.Fatalf("%v", m)
	}
	clients[7].expectClosed()

	var wg sync.WaitGroup
	for i, c := range clients {
		if i == 7 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				c.send(fmt.Sprintf(`{"id":%d,"type":"ping"}`, j+2))
				if m := c.recv(); m["ok"] != true || m["id"] != float64(j+2) {
					t.Errorf("连接 %d 第 %d 次心跳: %v", i, j, m)
					return
				}
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for srv.Hub().Count() != n-1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := srv.Hub().Count(); got != n-1 {
		t.Errorf("只应断开坏数据的那一条，在线数 = %d，期望 %d", got, n-1)
	}
}

func TestHandlerPanicOnlyAffectsRequest(t *testing.T) {
	srv, addr := startServer(t, Options{})
	srv.Handle("characters", func(context.Context, *Session, *protocol.Request) (any, error) { panic("boom") })
	other := dial(t, addr)
	other.hello("player")
	c := dial(t, addr)
	c.hello("player")
	srv.Hub().Sessions() // 触摸一下 Hub
	// 手工把状态推进到已登录，以便调用 characters。
	for _, s := range srv.Hub().Sessions() {
		s.SetState(protocol.StateLoggedIn)
	}
	c.send(`{"id":2,"type":"characters"}`)
	if m := c.recv(); errCode(m) != protocol.CodeInternal {
		t.Fatalf("处理函数崩溃应回复 INTERNAL: %v", m)
	}
	c.send(`{"id":3,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("崩溃后连接仍应可用: %v", m)
	}
	other.send(`{"id":2,"type":"ping"}`)
	if m := other.recv(); m["ok"] != true {
		t.Fatalf("其他连接不受影响: %v", m)
	}
}

func TestHubPublishToAllWatchers(t *testing.T) {
	srv, addr := startServer(t, Options{})
	player, agent, other := dial(t, addr), dial(t, addr), dial(t, addr)
	player.hello("player")
	agent.hello("agent")
	other.hello("player")
	sess := srv.Hub().Sessions()
	sort.Slice(sess, func(i, j int) bool { return sess[i].ID() < sess[j].ID() })
	if len(sess) != 3 {
		t.Fatalf("在线连接数 = %d", len(sess))
	}
	// 玩家连接和 agent 连接同时观看同一个角色，另一个玩家不看。
	srv.Hub().Watch("char:1", sess[0])
	srv.Hub().Watch("char:1", sess[1])
	if n := srv.Hub().Publish("char:1", "notice", map[string]string{"level": "info", "text": "hi"}); n != 2 {
		t.Fatalf("Publish 发给 %d 个连接，期望 2", n)
	}
	for name, c := range map[string]*testClient{"玩家": player, "agent": agent} {
		if m := c.recv(); m["type"] != "notice" {
			t.Errorf("%s 应收到推送: %v", name, m)
		}
	}
	other.send(`{"id":2,"type":"ping"}`)
	if m := other.recv(); m["type"] != "ping" {
		t.Errorf("没有观看的连接不应收到推送: %v", m)
	}
	// 断开后自动取消观看。
	agent.conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for len(srv.Hub().Watchers("char:1")) != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(srv.Hub().Watchers("char:1")); n != 1 {
		t.Errorf("断开后观看者应为 1，实际 %d", n)
	}
}

func TestProtocolLogHidesPasswords(t *testing.T) {
	buf := &syncWriter{}
	_, addr := startServer(t, Options{Trace: buf})
	c := dial(t, addr)
	c.hello("player")
	c.send(`{"id":2,"type":"login","data":{"account":"alice","password":"hunter2-secret"}}`)
	c.recv()
	c.send(`{"id":3,"type":"agent_login","data":{"token":"sat_SECRETTOKEN"}}`)
	c.recv()
	c.send(`{"id":4,"type":"register","data":{"account":"bob","password":"another-secret"`) // 不完整的 JSON
	c.send(`}`)
	time.Sleep(200 * time.Millisecond)
	log := buf.String()
	for _, secret := range []string{"hunter2-secret", "SECRETTOKEN", "another-secret"} {
		if strings.Contains(log, secret) {
			t.Errorf("协议日志里出现了明文 %q:\n%s", secret, log)
		}
	}
	if !strings.Contains(log, `"password":"***"`) || !strings.Contains(log, "alice") {
		t.Errorf("协议日志应保留其他字段并把密码写成 ***:\n%s", log)
	}
}

type syncWriter struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncWriter) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
