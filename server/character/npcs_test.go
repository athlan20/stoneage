package character

// npcs 请求（STAGE-68，docs/protocol.md 5.32）：村长家有对话 NPC；把角色挪到没有 NPC
// 的地图后返回空数组（不是 null）；观察方（agent 连接）也能查看。

import (
	"testing"
)

func TestNpcsFollowsCurrentFloor(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 村长家（1006）：村长家的厨师（对话），还有萨姆吉尔的村长（任务，STAGE-90 收录；脚本没放开，不能对话）。
	m := c.ok("npcs", `{}`)
	list, ok := m["npcs"].([]any)
	if !ok {
		t.Fatalf("npcs 应是数组: %v", m)
	}
	if len(list) != 2 {
		t.Fatalf("村长家应有 2 个 NPC，实际 %d 个: %v", len(list), list)
	}
	if cook := list[0].(map[string]any); cook["name"] != "村长家的厨师" || cook["type"] != "对话" {
		t.Errorf("村长家的厨师应是对话，实际 %v", cook)
	}
	if chief := list[1].(map[string]any); chief["name"] != "萨姆吉尔的村长" || chief["type"] != "任务" {
		t.Errorf("村长家的村长应是任务，实际 %v", chief)
	}

	// 挪到没有 NPC 的地图：返回空数组，不报错。
	move(t, e.chars.LiveOf(e.session("alice")), 5555, 10, 10, 44)
	m = c.ok("npcs", `{}`)
	list, ok = m["npcs"].([]any)
	if !ok {
		t.Fatalf("没有 NPC 时 npcs 也应是数组（不能是 null）: %v", m)
	}
	if len(list) != 0 {
		t.Errorf("没有 NPC 的地图应返回空列表，实际 %v", list)
	}
}

func TestNpcsVisibleToObserver(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed") // agent 进入即接管控制权，玩家连接变成观察方

	// 观察方（agent 操控时，玩家的连接）可以查看；先消费掉控制权变化的推送。
	c.recv()
	m := c.ok("npcs", `{}`)
	if list, ok := m["npcs"].([]any); !ok || len(list) != 2 {
		t.Errorf("观察方应能查看 NPC 列表，实际 %v", m)
	}
	// agent 自己也可以查看。
	m = agent.ok("npcs", `{}`)
	if list, ok := m["npcs"].([]any); !ok || len(list) != 2 {
		t.Errorf("agent 应能查看 NPC 列表，实际 %v", m)
	}
}
