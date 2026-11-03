package character

// 真人同场战斗的逃跑（STAGE-275 第三轮，路径 1、2）：队员逃跑只让他自己离场，队长逃跑解散全队，
// 留下的人继续打到结束。逃跑成功由 escapeRnd 保证（同单人逃跑用例）；遇敌间隔拉长，战斗结束后不自动开下一场。

import (
	"testing"
	"time"
)

// 队员逃跑成功：队员收到 battle_end(escaped) 和 party_changed（battle_escape，party 为空）；
// 队长收到 party_changed，不收到 battle_leave（逃跑不推 battle_leave，10.14）；两人队只剩队长，队伍解散，队长继续打完。
func TestPartyMateEscapeLeavesAndLeaderFights(t *testing.T) {
	_, a, b, _ := realPartyEnvWith(t, escapeRnd, time.Hour, nil)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")

	submitNeeds(t, a, 1, func(string) string { return "guard" })
	submitNeeds(t, b, 1, func(actor string) string {
		if actor == "player" {
			return "escape"
		}
		return "guard"
	})

	round := b.drainTo(t, "battle_round")
	if ev := eventOf(round, "escape"); ev == nil || ev["ok"] != true || ev["actor"] != float64(1) {
		t.Fatalf("队员应有一条成功的 escape 事件（actor=1）: %v", round["events"])
	}
	if end := b.drainTo(t, "battle_end"); end["result"] != "escaped" {
		t.Fatalf("逃跑的队员应收到 escaped: %v", end)
	}
	if left := b.drainTo(t, "party_changed"); left["reason"] != "left" || left["cause"] != "battle_escape" || left["party"] != nil {
		t.Fatalf("逃跑的队员应收到 left（battle_escape，party 为空）: %v", left)
	}

	a.drainTo(t, "battle_round")
	msgs := collectUntil(t, a, "party_changed")
	if hasType(msgs, "battle_leave") {
		t.Fatal("逃跑不应推 battle_leave")
	}
	if msgs[len(msgs)-1]["data"].(map[string]any)["cause"] != "battle_escape" {
		t.Fatalf("队长的 party_changed 应是 battle_escape: %v", msgs[len(msgs)-1])
	}
	a.roundtrip(t, "battle_ack", `{"turn":1}`, "")
	end := fightAlone(t, a, 40, pickAttack)
	if end["result"] == "escaped" || end["result"] == "forced_out" {
		t.Fatalf("队长应正常打完，结果 %v", end["result"])
	}
}

// 队长逃跑成功：队长收到 battle_end(escaped) 和 party_changed（left），队员收到 party_changed（disbanded）；
// 队员留在战斗里，继续交指令打到结束。
func TestPartyLeaderEscapeDisbandsButMateFights(t *testing.T) {
	_, a, b, _ := realPartyEnvWith(t, escapeRnd, time.Hour, nil)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")

	submitNeeds(t, a, 1, func(actor string) string {
		if actor == "player" {
			return "escape"
		}
		return "guard"
	})
	submitNeeds(t, b, 1, func(string) string { return "guard" })

	round := a.drainTo(t, "battle_round")
	if ev := eventOf(round, "escape"); ev == nil || ev["ok"] != true || ev["actor"] != float64(0) {
		t.Fatalf("队长应有一条成功的 escape 事件（actor=0）: %v", round["events"])
	}
	if end := a.drainTo(t, "battle_end"); end["result"] != "escaped" {
		t.Fatalf("逃跑的队长应收到 escaped: %v", end)
	}
	if left := a.drainTo(t, "party_changed"); left["reason"] != "left" || left["cause"] != "battle_escape" {
		t.Fatalf("队长应收到 left（battle_escape）: %v", left)
	}
	if d := b.drainTo(t, "party_changed"); d["reason"] != "disbanded" || d["cause"] != "battle_escape" {
		t.Fatalf("队员应收到 disbanded（battle_escape）: %v", d)
	}

	b.roundtrip(t, "battle_ack", `{"turn":1}`, "")
	end := fightAlone(t, b, 40, pickAttack)
	if end["result"] == "escaped" || end["result"] == "forced_out" {
		t.Fatalf("队员应正常打完，结果 %v", end["result"])
	}
}
