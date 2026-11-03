package character

// talk / npc_choose / npc_close（STAGE-69，docs/protocol.md 5.33–5.36）：村长家 talk
// 厨师弹窗、编号与地图与类型校验、战斗中和观察方被拒、窗口随传送和战斗关闭、观察方
// 可以关窗、agent 操控时两边连接看到同一个窗口。

import (
	"testing"
)

// talkChef 在村长家里 talk 村长家的厨师（225），返回推送 npc_window 的 data。
func talkChef(t *testing.T, c *client) map[string]any {
	t.Helper()
	m, pushes := c.callp("talk", `{"npc":225}`)
	if m["ok"] != true {
		t.Fatalf("talk 应成功: %v", m)
	}
	if len(pushes) != 1 || pushes[0]["type"] != "npc_window" {
		t.Fatalf("talk 后应推一个 npc_window，实际 %v", pushes)
	}
	d, _ := pushes[0]["data"].(map[string]any)
	return d
}

func TestTalkChefAtHome(t *testing.T) {
	e := newEnv(t, func(s *Service) { // 固定随机源：多句里取最后一句，结果可断言
		s.opts.NpcTalkRnd = func(n int) int { return n - 1 }
	})
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	d := talkChef(t, c)
	if d["npc"] != float64(225) || d["name"] != "村长家的厨师" || d["type"] != "对话" {
		t.Errorf("npc_window 的 npc/name/type 不对: %v", d)
	}
	if text, _ := d["text"].(string); text == "" {
		t.Errorf("npc_window 应带正文: %v", d)
	}
	if opts, ok := d["options"].([]any); !ok || len(opts) != 0 {
		t.Errorf("本期对话没有选项，options 应是空数组: %v", d["options"])
	}

	// npc_choose 没有可用的选项：窗口开着也回 NPC_BAD_CHOICE（本期对话都没有选项）。
	if m := c.call("npc_choose", `{"no":1}`); m["ok"] != false {
		t.Errorf("没有选项的窗口 choose 应失败: %v", m)
	}
	// npc_close 关掉：推 npc_window_closed（CLOSED），再关一次没有动静（幂等）。
	if m, pushes := c.callp("npc_close", `{}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("npc_close 应成功并推一次关闭: %v %v", m, pushes)
	} else if pushes[0]["data"].(map[string]any)["reason"] != WindowReasonClosed {
		t.Errorf("关闭原因应是 CLOSED: %v", pushes[0])
	}
	c.noMessage()
}

func TestTalkValidation(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	c.wantErr(t, "talk", `{}`, "BAD_REQUEST")
	c.wantErr(t, "talk", `{"npc":99999}`, "NPC_NOT_FOUND") // 编号不存在
	c.wantErr(t, "talk", `{"npc":198}`, "NPC_NOT_HERE")    // 特产品贩卖员在萨姆吉尔村（1000），村长家（1006）没有
	c.wantErr(t, "talk", `{"npc":0}`, "NPC_NOT_FOUND")
	c.noMessage()

	// warp 到萨姆吉尔村：196 村庄青年可以 talk（对话）。
	// 村里各类 NPC 现在都有对话逻辑（商店/护士/存档点/传送员各自走 5.38–5.42 的窗口），
	// 「不支持类型回 NPC_NOT_TALKABLE」由依赖任务条件的传送员覆盖（warpman_test）。
	c.okp("warp", `{"place":1}`)
	c.wantErr(t, "talk", `{"npc":225}`, "NPC_NOT_HERE") // 厨师留在村长家
	m, pushes := c.callp("talk", `{"npc":196}`)
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 村庄青年应成功: %v %v", m, pushes)
	}
}

func TestTalkBlockedInBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 先在村长家正常对话一次；开战后同样的 talk 回 IN_BATTLE。
	talkChef(t, c)
	if m, _ := c.callp("npc_close", `{}`); m["ok"] != true { // npc_close 自带关闭推送，用 callp 消费
		t.Fatalf("npc_close 应成功: %v", m)
	}
	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "talk", `{"npc":225}`, "IN_BATTLE")
	c.drainToEnd()
}

func TestTalkWhileObserving(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// agent 接管后，玩家连接（观察方）talk 被拒；agent 自己可以对话，
	// 并且玩家连接同样收到 npc_window（两边看到同一个窗口）。
	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed")
	c.recv() // 玩家连接的控制权变化
	c.drainToEnd()

	c.wantErr(t, "talk", `{"npc":225}`, "CONTROLLED_BY_AGENT")
	m, pushes := agent.callp("talk", `{"npc":225}`)
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("agent 应能对话: %v %v", m, pushes)
	}
	d := c.push("npc_window") // 玩家连接看到 agent 打开的同一个窗口
	if d["npc"] != float64(225) {
		t.Errorf("玩家连接应看到同一个 npc_window: %v", d)
	}

	// 观察方（玩家连接）可以关窗（npc_close 是确认类）；关掉后 agent 连接也收到推送。
	if m, _ := c.callp("npc_close", `{}`); m["ok"] != true {
		t.Fatalf("观察方应能关闭窗口: %v", m)
	}
	if d := agent.push("npc_window_closed"); d["reason"] != WindowReasonClosed {
		t.Errorf("agent 连接应收到窗口关闭: %v", d)
	}

	// 没有窗口时 choose 回 WRONG_STATE（操控方 agent 发）；close 仍然成功（幂等）。
	c.wantErr(t, "npc_choose", `{"no":1}`, "CONTROLLED_BY_AGENT") // 观察方的操作类先被控制权拦下
	agent.wantErr(t, "npc_choose", `{"no":1}`, "WRONG_STATE")
	agent.ok("npc_close", `{}`)
	agent.noMessage()
}

func TestWindowClosesOnWarpAndBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	talkChef(t, c)
	// warp 换地图：position_changed 之外还应推 npc_window_closed（MOVED）。
	if m, pushes := c.callp("warp", `{"place":1}`); m["ok"] != true {
		t.Fatalf("warp 应成功: %v", m)
	} else {
		want := []string{"position_changed", "npc_window_closed"}
		if len(pushes) != len(want) {
			t.Fatalf("warp 应推 %v，实际 %v", want, pushes)
		}
		for i, typ := range want {
			if pushes[i]["type"] != typ {
				t.Fatalf("warp 应推 %v，实际 %v", want, pushes)
			}
		}
		if pushes[1]["data"].(map[string]any)["reason"] != WindowReasonMoved {
			t.Errorf("关闭原因应是 MOVED: %v", pushes[1])
		}
	}
	// 关掉之后 warp 不再推 npc_window_closed。
	if _, pushes := c.okp("warp", `{"place":2}`); len(pushes) != 1 {
		t.Errorf("没有窗口时 warp 只应推 position_changed，实际 %v", pushes)
	}

	// 战斗开始时窗口给战场让路（BATTLE）。先挪回村长家才能再和厨师说话。
	move(t, e.chars.LiveOf(e.session("alice")), 1006, 15, 22, 44)
	talkChef(t, c)
	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	d := c.push("npc_window_closed")
	if d["reason"] != WindowReasonBattle {
		t.Errorf("战斗开始应推 BATTLE 关窗: %v", d)
	}
	c.drainTo(t, "battle_turn")
}

// wantErr 发一个请求并断言失败码（talk 的各种校验都走这里）。
func (c *client) wantErr(t *testing.T, typ, data, code string) {
	t.Helper()
	m := c.call(typ, data)
	if m["ok"] != false {
		t.Fatalf("%s %s 应失败: %v", typ, data, m)
	}
	errObj, _ := m["error"].(map[string]any)
	if errObj["code"] != code {
		t.Fatalf("%s %s 应回 %s，实际 %v", typ, data, code, errObj)
	}
}
