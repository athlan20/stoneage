package character

// 真人同场战斗结束后（STAGE-275 第三轮，路径 6）：队长的自动遇敌接着开下一场，下一场仍是真人队（队员随队同场），
// 队员不会各自开自动遇敌（队员的遇敌开关没开，也不会自己触发）。

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestPartyLeaderEncounterContinuesAfterWin(t *testing.T) {
	_, a, b, _ := realPartyEnvWith(t, rand.New(rand.NewPCG(21, 43)).IntN, 60*time.Millisecond, nil)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")

	endA, _ := fightTogether(t, a, b, 60, 1, pickAttack, pickAttack)
	if endA["result"] != "win" {
		t.Fatalf("应打赢（结果 %v）", endA["result"])
	}

	next := a.drainTo(t, "battle_start") // 队长的遇敌开关还开着，间隔过后自动开下一场
	if next["kind"] != "party" || next["you"] != float64(0) {
		t.Fatalf("队长的下一场应是真人队的战斗（you=0）: %v", next)
	}
	mate := b.drainTo(t, "battle_start") // 队员随队进入同一场，而不是自己开一场
	if mate["kind"] != "party" || mate["you"] != float64(1) || mate["battle_id"] != next["battle_id"] {
		t.Fatalf("队员应进入队长的同一场真人队战斗: 队长 %v 队员 %v", next, mate)
	}
}
