package character

// 真人同场战斗中队员用道具（STAGE-275 第三轮，路径 5）：道具从队员自己的背包里扣，队长的背包不动。

import (
	"context"
	"strconv"
	"testing"
	"time"
)

const partyTestItem = 1684 // 同 itemuse 用例里的道具

func TestPartyMateItemConsumedFromOwnBag(t *testing.T) {
	pre := func(e *env) {
		if err := GiveItem(context.Background(), e.chars.db, e.chars.opts.ItemNotifyChannel, "小红", partyTestItem, 2); err != nil {
			t.Fatal(err)
		}
	}
	e, a, b, _ := realPartyEnvWith(t, rand2IntN(41, 43), time.Hour, pre)
	slot := -1
	for _, it := range recOf(t, e, "小红").Items {
		if it.ItemID == partyTestItem {
			slot = it.Slot
		}
	}
	if slot < 0 {
		t.Fatal("小红的背包里应有道具")
	}
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")

	submitCmds(t, a, 1, func(string, []int) (string, int) { return "guard", -1 })
	b.roundtrip(t, "battle_command", `{"turn":1,"actor":"player","action":"item","slot":`+strconv.Itoa(slot)+`}`, "")
	b.drainTo(t, "battle_round")
	a.drainTo(t, "battle_round")
	if a.settled(t) != nil || b.settled(t) != nil {
		t.Fatal("第一回合不应结束战斗")
	}
	a.roundtrip(t, "battle_ack", `{"turn":1}`, "")
	b.roundtrip(t, "battle_ack", `{"turn":1}`, "")
	a.drainTo(t, "battle_turn")
	b.drainTo(t, "battle_turn")
	fightTogether(t, a, b, 60, 2, pickAttack, pickAttack)

	count := func(name string) int {
		for _, it := range recOf(t, e, name).Items {
			if it.ItemID == partyTestItem {
				return it.Count
			}
		}
		return 0
	}
	if n := count("小红"); n != 1 {
		t.Fatalf("队员用了一件，背包里应剩 1 件，实际 %d", n)
	}
	if n := count("勇者小明"); n != 0 {
		t.Fatalf("队长的背包不该有这件道具，实际 %d", n)
	}
}
