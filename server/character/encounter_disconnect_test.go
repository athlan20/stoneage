package character

import (
	"testing"
	"time"
)

// STAGE-184 的验收：遇敌开着、战斗进行中直接断开（不发 leave_world），重连后遇敌开关必须是关的，
// 战斗也不能残留，不能一上线就被按着继续打（协议 5.29：离开世界或掉线时开关随角色一起关掉）。
func TestEncounterOffAfterDisconnectAndRelogin(t *testing.T) {
	e, c := encounterEnv(t, 400*time.Millisecond, 9)
	c.warpTo(t, 6) // 玛丽娜丝近郊，有野外敌人
	c.turnOnEncounter(t)

	_ = c.conn.Close() // 不发 leave_world，直接断开，这一场还没结束

	c2 := e.loggedIn("alice")
	after, _ := c2.roundtrip(t, "enter_world", `{"slot":0}`, "")
	if on, _ := after["encounter"].(bool); on {
		t.Fatalf("断线后重连，遇敌开关应已关闭，实际 encounter=true: %v", after)
	}
	if cont, _ := after["encounter_continues"].(bool); cont {
		t.Fatalf("断线后重连，不应还在连场续打: %v", after)
	}
	if _, inBattle := after["battle"]; inBattle {
		t.Fatalf("断线时的战斗应已作废，重连不应残留 battle: %v", after)
	}
}
