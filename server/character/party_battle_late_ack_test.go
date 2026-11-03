package character

// STAGE-282 在真人同场战斗上的对应：战斗刚结束，客户端回放时发出的最后一回合确认才到，按成功回复（协议 10.8）。

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"
)

func TestPartyLateAckAfterEndIsSuccess(t *testing.T) {
	_, a, b, _ := realPartyEnvWith(t, rand.New(rand.NewPCG(21, 43)).IntN, time.Hour, nil)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")
	endA, endB := fightTogether(t, a, b, 60, 1, pickAttack, pickAttack)
	if endA["result"] != "win" || endB["result"] != "win" {
		t.Fatalf("应打赢（结果 %v / %v）", endA["result"], endB["result"])
	}
	turn := strconv.Itoa(int(endA["turn"].(float64)))
	a.roundtrip(t, "battle_ack", `{"turn":`+turn+`}`, "")
	b.roundtrip(t, "battle_ack", `{"turn":`+turn+`}`, "")
	a.expectError(t, "battle_ack", `{"turn":`+strconv.Itoa(int(endA["turn"].(float64))+5)+`}`, "NOT_IN_BATTLE")
}
