// 骑着一只、另一只出战（STAGE-149，docs/ride.md §5）：出战的宠物上格子、有自己的宠物指令，
// 骑着的宠物不上格子，人物的数值照旧与它合成，结算时两只宠物各自拿经验。

package battle

import "testing"

// hasNeed 报告本回合 needs 里有没有这个角色的指令。
func hasNeed(b *Battle, actor string) bool {
	for _, a := range b.Needs() {
		if a == actor {
			return true
		}
	}
	return false
}

// 骑着 0 号栏、出战 1 号栏：出战的宠物站在 5 号格并有宠物指令，骑着的宠物不占格子。
func TestRideWithBattlePetOnGrid(t *testing.T) {
	b := newBattleWithSide(t, riderAndBattleSide(), "win", rollRnd(9999))
	if !b.PlayerUnit().ridden() || b.RiderUnit() == nil || b.RiderUnit().PetSlot != 0 {
		t.Fatal("人物应该骑着 0 号栏的宠物")
	}
	pet := b.PetUnit()
	if pet == nil || pet.PetSlot != 1 || b.Unit(firstPetNo) != pet {
		t.Fatalf("出战宠 = %+v，want 1 号栏站在 5 号格", pet)
	}
	if !hasNeed(b, "player") || !hasNeed(b, "pet") {
		t.Fatalf("needs = %v，want player 和 pet", b.Needs())
	}
}

// 端到端：人物和出战的宠物都下指令打赢；骑着的宠物按 0.6 倍拿经验，出战的宠物拿人物同样的经验。
func TestRideWithBattlePetRewards(t *testing.T) {
	b := newBattleWithSide(t, riderAndBattleSide(), "win", rollRnd(9999))
	var end *EndView
	for turn := 1; turn <= 40 && end == nil; turn++ {
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		if hasNeed(b, "player") {
			if _, perr := b.Submit(b.Turn(), "player", "attack", Cmd{Target: target}); perr != nil {
				t.Fatalf("人物攻击提交失败: %v", perr)
			}
		}
		if hasNeed(b, "pet") {
			if _, perr := b.Submit(b.Turn(), "pet", "attack", Cmd{Target: target}); perr != nil {
				t.Fatalf("宠物攻击提交失败: %v", perr)
			}
		}
		_, end = b.Settle()
	}
	if end == nil || end.Result != "win" {
		t.Fatalf("没有打赢，end = %+v", end)
	}
	var riderGain, battleGain int64 = -1, -1
	for _, e := range end.Rewards.Exp {
		if e.Who != "pet" || e.Slot == nil {
			continue
		}
		switch *e.Slot {
		case 0:
			riderGain = e.Gain
		case 1:
			battleGain = e.Gain
		}
	}
	if riderGain < 0 || battleGain < 0 {
		t.Fatalf("两只宠物都应该有经验，rewards.exp = %+v", end.Rewards.Exp)
	}
	if want := b.rideExp(b.RiderUnit()); riderGain != want {
		t.Fatalf("骑宠经验 = %d，want %d（0.6 倍）", riderGain, want)
	}
}
