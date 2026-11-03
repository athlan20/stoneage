package character

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// push 读一条消息，断言它是类型为 typ 的推送，返回它的 data。
func (c *client) push(typ string) map[string]any {
	c.t.Helper()
	m := c.recv()
	if m["type"] != typ || m["id"] != nil {
		c.t.Fatalf("期望推送 %s，实际 %v", typ, m)
	}
	d, _ := m["data"].(map[string]any)
	return d
}

// callp 发一个请求，读到它的响应为止；响应之前到达的推送一并返回
// （改变角色的请求，推送总是在响应之前入队）。
func (c *client) callp(typ, data string) (map[string]any, []map[string]any) {
	c.t.Helper()
	id := c.send(typ, data)
	var pushes []map[string]any
	for {
		m := c.recv()
		if got, ok := m["id"].(float64); ok && int(got) == id {
			return m, pushes
		}
		pushes = append(pushes, m)
	}
}

// okp 同 callp，要求响应成功，返回响应的 data。
func (c *client) okp(typ, data string) (map[string]any, []map[string]any) {
	c.t.Helper()
	m, pushes := c.callp(typ, data)
	if m["ok"] != true {
		c.t.Fatalf("%s 应成功: %v", typ, m)
	}
	d, _ := m["data"].(map[string]any)
	return d, pushes
}

// wantPushes 断言推送列表的类型依次是 types。
func wantPushes(t *testing.T, pushes []map[string]any, types ...string) {
	t.Helper()
	var got []string
	for _, m := range pushes {
		got = append(got, fmt.Sprint(m["type"]))
	}
	if fmt.Sprint(got) != fmt.Sprint(types) {
		t.Fatalf("响应前收到的推送 %v，期望 %v", got, types)
	}
}

func (c *client) noMessage() {
	c.t.Helper()
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			return
		}
		var m map[string]any
		_ = json.Unmarshal(line, &m)
		if !isAgentPanelPush(m) {
			c.t.Fatalf("不应收到消息，实际 %s", line)
		}
		c.agent = append(c.agent, m) // agent 面板的推送不算在内（STAGE-119）
	}
}

func (c *client) control(controller, you, reason string) {
	c.t.Helper()
	d := c.push("control_changed")
	if d["controller"] != controller || d["you"] != you || d["reason"] != reason {
		c.t.Fatalf("control_changed 不对：%v，期望 controller=%s you=%s reason=%s", d, controller, you, reason)
	}
}

func (c *client) disconnectReason(reason string) {
	c.t.Helper()
	if d := c.push("disconnect"); d["reason"] != reason {
		c.t.Fatalf("断开原因应是 %s，实际 %v", reason, d)
	}
	if !c.closed() {
		c.t.Fatal("连接应被断开")
	}
}

// 双方都在世界中的完整控制权流程：agent 进入接管、玩家收回、交还，以及各方发“操作”和“查看”请求的结果。
// 约定：改变角色的请求，推送先到、响应后到；推送发给世界中的每一条连接（含发起方）。
func TestControlFlow(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	if m := player.ok("status", `{}`); m["control"].(map[string]any)["you"] != "controller" {
		t.Fatalf("只有玩家在世界中，玩家应是操控方: %v", m)
	}
	if _, pushes := player.okp("turn", `{"direction":2}`); len(pushes) != 1 {
		t.Fatalf("玩家操控时 turn 应成功并收到 position_changed: %v", pushes)
	}

	// agent 进入同一个角色：接管控制权，两边都收到 control_changed。
	agent := e.agentFor(player)
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent)

	// 玩家是观察方：操作类请求被拒，查看正常。
	if m, pushes := player.callp("turn", `{"direction":3}`); errCode(m) != protocol.CodeControlledByAgent || len(pushes) != 0 {
		t.Errorf("观察方 turn 应回复 CONTROLLED_BY_AGENT: %v %v", m, pushes)
	}
	if m := player.ok("status", `{}`); m["control"].(map[string]any)["you"] != "observer" {
		t.Errorf("status 应显示观察方: %v", m)
	}
	// agent 操控：turn 成功，两边都收到 position_changed。
	_, pushes := agent.okp("turn", `{"direction":5}`)
	wantPushes(t, pushes, "position_changed")
	if d := player.push("position_changed"); d["direction"].(float64) != 5 {
		t.Errorf("玩家应看到 agent 的变化: %v", d)
	}

	// 玩家收回控制权。
	_, pushes = player.okp("take_control", `{}`)
	wantPushes(t, pushes, "control_changed")
	agent.control("player", "observer", ReasonTakenByPlayer)
	if m, _ := agent.callp("turn", `{"direction":1}`); errCode(m) != protocol.CodeControlledByPlayer {
		t.Errorf("观察方 agent turn 应回复 CONTROLLED_BY_PLAYER: %v", m)
	}
	if m := agent.ok("status", `{}`); m["control"].(map[string]any)["you"] != "observer" {
		t.Errorf("agent 的 status 应显示观察方: %v", m)
	}
	_, pushes = player.okp("turn", `{"direction":6}`)
	wantPushes(t, pushes, "position_changed")
	agent.push("position_changed")
	// 重复收回：什么都不变，不推送。
	_, pushes = player.okp("take_control", `{}`)
	wantPushes(t, pushes)
	agent.noMessage()

	// 交还。
	_, pushes = player.okp("release_control", `{}`)
	wantPushes(t, pushes, "control_changed")
	agent.control("agent", "controller", ReasonReleasedByPlayer)
	_, pushes = agent.okp("turn", `{"direction":0}`)
	wantPushes(t, pushes, "position_changed")
	player.push("position_changed")
}

// expectAgentEnter 检查 agent 进入世界时的推送：玩家收到 AGENT_ENTERED（变成观察方），
// agent 先收到同样的推送，再收到 enter_world 的响应。agent.send("enter_world") 之后调用。
func (e *env) expectAgentEnter(t *testing.T, player, agent *client) {
	t.Helper()
	player.control("agent", "observer", ReasonAgentEntered)
	agent.control("agent", "controller", ReasonAgentEntered)
	m := agent.recv()
	if m["ok"] != true || m["type"] != "enter_world" {
		t.Fatalf("agent 应收到 enter_world 响应: %v", m)
	}
	if ctl := m["data"].(map[string]any)["control"].(map[string]any); ctl["controller"] != "agent" || ctl["you"] != "controller" {
		t.Errorf("响应里的 control 不对: %v", ctl)
	}
}

// agent 先在世界中，玩家后进入：玩家直接是观察方，agent 的操控不变，也没有控制权变化的推送。
func TestPlayerEntersAsObserver(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	agent := e.agentFor(player)
	agent.ok("enter_world", `{"slot":0}`)

	m := player.ok("enter_world", `{"slot":0}`)
	if ctl := m["control"].(map[string]any); ctl["controller"] != "agent" || ctl["you"] != "observer" {
		t.Fatalf("玩家应以观察方进入: %v", ctl)
	}
	agent.noMessage()
	if r, _ := player.callp("turn", `{"direction":1}`); errCode(r) != protocol.CodeControlledByAgent {
		t.Errorf("观察方 turn 应被拒绝: %v", r)
	}
}

// 操控方离开（主动离开、直接断开）后，控制权自动转给另一方；观察方离开不影响。
func TestControlPassesToPeerWhenControllerLeaves(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	agent := e.agentFor(player)
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent)

	// agent（操控方）主动离开：玩家成为操控方。
	_, pushes := agent.okp("leave_world", `{}`)
	wantPushes(t, pushes)
	player.control("player", "controller", ReasonPeerLeft)
	_, pushes = player.okp("turn", `{"direction":4}`)
	wantPushes(t, pushes, "position_changed")
}

func TestControlPassesOnDisconnect(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	agent := e.agentFor(player)
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent)

	_ = agent.conn.Close() // agent 直接断线
	player.control("player", "controller", ReasonPeerLeft)

	// STAGE-139：观察方离开只更新在线状态，不改变控制权。
	agent2 := e.agentFor(player)
	agent2.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent2)
	_, pushes := player.okp("take_control", `{}`)
	wantPushes(t, pushes, "control_changed")
	agent2.control("player", "observer", ReasonTakenByPlayer)
	_, pushes = agent2.okp("leave_world", `{}`)
	wantPushes(t, pushes)
	if d := player.push("agent_presence"); d["online"] != false {
		t.Fatalf("agent 离开后应标记离线: %v", d)
	}
	if _, pushes := player.okp("turn", `{"direction":7}`); len(pushes) != 1 {
		t.Errorf("玩家应仍是操控方: %v", pushes)
	}
}

// kick_agent：agent 收到 KICKED_BY_PLAYER 后被断开，令牌仍然有效，玩家恢复操控；agent 不在线时回复 AGENT_OFFLINE。
func TestKickAgent(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	for _, typ := range []string{"kick_agent", "take_control", "release_control"} {
		if m := player.call(typ, `{}`); errCode(m) != protocol.CodeAgentOffline {
			t.Errorf("没有 agent 时 %s 应回复 AGENT_OFFLINE: %v", typ, m)
		}
	}

	tok := player.ok("create_agent_token", `{"label":"x"}`)["token"].(string)
	agent := e.dial("agent")
	agent.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	// agent 登录了但不在世界中：同样回复 AGENT_OFFLINE。
	if m := player.call("kick_agent", `{}`); errCode(m) != protocol.CodeAgentOffline {
		t.Errorf("agent 不在世界中应回复 AGENT_OFFLINE: %v", m)
	}
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent)

	_, pushes := player.okp("kick_agent", `{}`)
	wantPushes(t, pushes, "control_changed") // agent 是操控方，走了之后控制权回到玩家
	if d := pushes[0]["data"].(map[string]any); d["controller"] != "player" || d["reason"] != ReasonPeerLeft {
		t.Errorf("control_changed 不对: %v", d)
	}
	agent.disconnectReason(protocol.ReasonKickedByPlayer)
	_, pushes = player.okp("turn", `{"direction":2}`)
	wantPushes(t, pushes, "position_changed")

	// 令牌仍然有效；agent 连接不能发玩家专用的请求。
	again := e.dial("agent")
	again.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	for _, typ := range []string{"kick_agent", "take_control", "release_control", "create_agent_token", "agent_tokens", "create_character", "delete_character"} {
		if m := again.call(typ, `{}`); errCode(m) != protocol.CodeWrongRole {
			t.Errorf("agent 连接发 %s 应回复 WRONG_ROLE: %v", typ, m)
		}
	}
}

// 吊销令牌：正在使用它的 agent 立刻收到 AGENT_TOKEN_REVOKED 并被断开，玩家恢复操控；令牌再也登录不了。
func TestRevokeTokenKicksAgent(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	made := player.ok("create_agent_token", `{"label":"x"}`)
	tok, id := made["token"].(string), made["token_id"].(string)
	other := player.ok("create_agent_token", `{"label":"y"}`)["token"].(string)

	agent := e.dial("agent")
	agent.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent)

	_, pushes := player.okp("revoke_agent_token", fmt.Sprintf(`{"token_id":%q}`, id))
	wantPushes(t, pushes, "control_changed")
	agent.disconnectReason(protocol.ReasonAgentTokenRevoked)

	late := e.dial("agent")
	if m := late.call("agent_login", fmt.Sprintf(`{"token":%q}`, tok)); errCode(m) != protocol.CodeInvalidToken {
		t.Errorf("吊销后的令牌应回复 INVALID_TOKEN: %v", m)
	}
	if m := late.call("agent_login", fmt.Sprintf(`{"token":%q}`, other)); m["ok"] != true {
		t.Errorf("没吊销的令牌不受影响: %v", m)
	}
}

// 同一账号再来一个 agent 连接：新 agent 登录成功，旧 agent 被挤下线（agent 挤 agent）；玩家不受影响。
func TestSecondAgentKicksFirstAgentOnly(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	tok := player.ok("create_agent_token", `{}`)["token"].(string)
	first := e.dial("agent")
	first.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	first.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, first)

	second := e.dial("agent")
	second.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	first.disconnectReason(protocol.ReasonKickedDuplicateLogin)
	player.control("player", "controller", ReasonPeerLeft)
	if _, pushes := player.okp("turn", `{"direction":1}`); len(pushes) != 1 {
		t.Errorf("玩家连接不应受影响: %v", pushes)
	}
}

// 私聊：双方登录就能聊，不要求在世界中；对方不在线回复 PEER_OFFLINE；其他账号收不到；过长或为空回复 BAD_REQUEST。
func TestAgentSay(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	stranger := e.loggedIn("bob_x")
	if m := player.call("agent_say", `{"text":"你好"}`); errCode(m) != protocol.CodePeerOffline {
		t.Errorf("agent 不在线应回复 PEER_OFFLINE: %v", m)
	}
	agent := e.agentFor(player)
	player.ok("agent_say", `{"text":"先去加加村，别打架"}`)
	d := agent.push("agent_message")
	if d["from"] != "player" || d["text"] != "先去加加村，别打架" || d["time_ms"].(float64) < 1e12 {
		t.Errorf("agent_message 不对: %v", d)
	}
	agent.ok("agent_say", `{"text":"收到，出发"}`)
	if d := player.push("agent_message"); d["from"] != "agent" || d["text"] != "收到，出发" {
		t.Errorf("玩家应收到 agent 的消息: %v", d)
	}
	stranger.noMessage()

	for name, data := range map[string]string{
		"空":    `{"text":""}`,
		"全空白":  `{"text":"   "}`,
		"缺字段":  `{}`,
		"过长":   fmt.Sprintf(`{"text":%q}`, repeat("一", 201)),
		"控制字符": `{"text":"a\u0007b"}`,
	} {
		if m := player.call("agent_say", data); errCode(m) != protocol.CodeBadRequest {
			t.Errorf("%s 应回复 BAD_REQUEST: %v", name, m)
		}
	}
	player.ok("agent_say", fmt.Sprintf(`{"text":%q}`, repeat("一", 200))) // 正好 200 个字符
	agent.push("agent_message")
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// 审计：agent 的每个“操作”类请求（含被拒的）和每次控制权变化都有记录，带账号、角色、令牌编号、结果。
func TestAudit(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	made := player.ok("create_agent_token", `{}`)
	tokenID := made["token_id"].(string)
	agent := e.dial("agent")
	agent.ok("agent_login", fmt.Sprintf(`{"token":%q}`, made["token"].(string)))
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, player, agent)
	agent.okp("turn", `{"direction":3}`) // 成功
	player.push("position_changed")
	player.okp("take_control", `{}`)
	agent.push("control_changed")
	agent.callp("turn", `{"direction":4}`) // 被拒绝
	agent.ok("status", `{}`)               // 查看类，不审计
	player.okp("kick_agent", `{}`)

	type row struct{ kind, request, result, role, token string }
	rows, err := e.chars.db.Query(context.Background(),
		`SELECT kind, request, result, role, token_id FROM agent_audit WHERE account = 'alice' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.kind, &r.request, &r.result, &r.role, &r.token); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	want := []row{
		{"control", ReasonAgentEntered, "ok", "agent", tokenID},
		{"request", "enter_world", "ok", "agent", tokenID},
		{"request", "turn", "ok", "agent", tokenID},
		{"control", ReasonTakenByPlayer, "ok", "player", tokenID},
		{"request", "turn", protocol.CodeControlledByPlayer, "agent", tokenID},
		{"control", protocol.ReasonKickedByPlayer, "ok", "player", tokenID},
	}
	if len(got) != len(want) {
		t.Fatalf("审计记录 %d 条，期望 %d 条：%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 条：%v，期望 %v", i+1, got[i], want[i])
		}
	}
}

// 并发压力：反复让 agent 登录进入、玩家同时收回/交还/转向/踢 agent。用 -race 跑；
// 结束后角色只剩玩家一个成员，玩家是操控方，服务端没有死锁。
func TestControlStress(t *testing.T) {
	e := newEnv(t)
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者小明", 0))
	player.ok("enter_world", `{"slot":0}`)
	tok := player.ok("create_agent_token", `{}`)["token"].(string)
	live := e.chars.LiveOf(e.session("alice"))

	quiet := func(c *client) { // 读走这条连接上的全部消息，直到安静
		for {
			_ = c.conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
			if _, err := c.r.ReadBytes('\n'); err != nil {
				return
			}
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		agent := e.dial("agent")
		agent.send("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
		agent.send("enter_world", `{"slot":0}`)
		for j := 0; j < 3; j++ {
			agent.send("turn", `{"direction":1}`)
			player.send("take_control", `{}`)
			player.send("turn", `{"direction":2}`)
			player.send("release_control", `{}`)
		}
		player.send("kick_agent", `{}`)
		_ = agent.conn.Close()
		quiet(player)
	}
	live.p.mu.Lock()
	members, controller := len(live.p.members), live.p.controller
	live.p.mu.Unlock()
	if members != 1 || controller != protocol.RolePlayer {
		t.Fatalf("压力之后应只剩玩家一个成员且玩家是操控方，实际成员 %d 个，操控方 %s", members, controller)
	}
	// 服务端没有卡住，玩家还能正常操作。
	if m, _ := player.callp("turn", `{"direction":0}`); m["ok"] != true {
		t.Errorf("压力之后玩家应能操作: %v", m)
	}
}

// STAGE-311：遇敌开关跟着控制方走。玩家开着遇敌时 agent 接管，开关自动关闭；
// agent 在战斗中重新打开后玩家收回控制权，开关再次关闭，玩家不会被连场锁住。
func TestControlChangeStopsEncounter(t *testing.T) {
	e, player := encounterEnv(t, 50*time.Millisecond, 1)
	player.warpTo(t, 5)       // 萨伊那斯野外，能遇敌
	player.turnOnEncounter(t) // 开关打开，第一场已经开始

	// agent 接管：玩家原来的遇敌开关被自动关掉，当前这一场照常打完。
	agent := e.agentFor(player)
	agent.send("enter_world", `{"slot":0}`)
	player.control("agent", "observer", ReasonAgentEntered)
	if d := player.push("encounter_changed"); d["encounter"] != false || d["reason"] != encounterOffControl {
		t.Fatalf("agent 接管后应关闭遇敌: %v", d)
	}
	if d := player.push("notice"); !strings.Contains(fmt.Sprint(d["text"]), "控制权") {
		t.Fatalf("关闭遇敌应说明原因: %v", d)
	}
	agent.control("agent", "controller", ReasonAgentEntered)
	if d := agent.push("encounter_changed"); d["encounter"] != false || d["reason"] != encounterOffControl {
		t.Fatalf("agent 侧也应关闭遇敌: %v", d)
	}
	agent.push("notice")
	if m := agent.recv(); m["ok"] != true || m["type"] != "enter_world" {
		t.Fatalf("agent 应收到 enter_world 响应: %v", m)
	}
	if m := player.ok("status", `{}`); m["encounter"] != false {
		t.Fatalf("接管后 status 的遇敌开关应是关闭的: %v", m)
	}

	// agent 在战斗中重新打开：这一场还没打完，只开开关，不立刻开新场。
	_, pushes := agent.okp("encounter", `{"set":"on"}`)
	wantPushes(t, pushes, "encounter_changed")
	if d := player.push("encounter_changed"); d["encounter"] != true {
		t.Fatalf("玩家应看到 agent 打开遇敌: %v", d)
	}

	// 玩家收回控制权：开关再次关掉，战后不会再自动开新场。
	_, pushes = player.okp("take_control", `{}`)
	wantPushes(t, pushes, "control_changed", "encounter_changed", "notice")
	agent.control("player", "observer", ReasonTakenByPlayer)
	if d := agent.push("encounter_changed"); d["encounter"] != false || d["reason"] != encounterOffControl {
		t.Fatalf("收回控制权后 agent 侧也应看到关闭: %v", d)
	}
	agent.push("notice")
	if m := player.ok("status", `{}`); m["encounter"] != false {
		t.Fatalf("收回控制权后遇敌应保持关闭: %v", m)
	}
}
