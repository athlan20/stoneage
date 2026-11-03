package character

import (
	"fmt"
	"testing"
)

// STAGE-175 复现：只有 agent 一条连接（玩家没有在线）时，战斗中 agent 断开，
// 重连进入世界后角色不应还在同一场战斗里（协议 10.1：最后一条连接断开即作废）。
func TestAgentOnlyBattleExitsOnDisconnect(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	tok := c.ok("create_agent_token", `{"label":"测试"}`)["token"].(string)
	_ = c.conn.Close() // 玩家离线，只剩 agent

	agent := e.dial("agent")
	agent.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	agent.ok("enter_world", `{"slot":0}`)
	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	agent.drainTo(t, "battle_turn")
	_ = agent.conn.Close() // agent 进程被杀：最后一条连接断开

	c2 := e.loggedIn("alice")
	after, _ := c2.roundtrip(t, "enter_world", `{"slot":0}`, "")
	if _, inBattle := after["battle"]; inBattle {
		t.Fatalf("最后一条连接断开后战斗应已作废，重连不应还在战斗中: %v", after)
	}
}

// 对照：玩家也在线时，agent 断开不算“最后一条连接”，战斗照常由玩家继续（协议 10.12）。
func TestAgentDisconnectKeepsBattleWhilePlayerOnline(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	agent := e.agentFor(c)
	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	_ = agent.conn.Close()

	resp, _ := c.roundtrip(t, "battle", `{}`, "")
	if resp["in_battle"] != true {
		t.Fatalf("玩家还在线，agent 断开后战斗应继续: %v", resp)
	}
}
