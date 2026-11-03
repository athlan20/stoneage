// 真人同场战斗里队员的物品、精灵、捕捉、换宠（STAGE-275 第二轮，缺口 1）：读的是队员自己的背包、精灵、宠物和捕获者。
package battle

import (
	"testing"
)

// partyWith 开一场队长加给定队员的战斗（怪物用 win 剧本，固定种子）。队长和队员的快照由调用方给。
func partyWith(t *testing.T, leader *SideInput, rnd func(n int) int, mates ...PartyMate) *Battle {
	t.Helper()
	sc, err := Lookup("win")
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := sc.makeMonsters(rnd)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewParty("p_mate", "野外", leader, mates, monsters, rnd)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// hasEvent 判断一回合的事件里有没有某个单位发出的某种事件。
func hasEvent(r Round, kind string, actor int) bool {
	for _, ev := range r.Events {
		if ev.Kind == kind && ev.Actor != nil && *ev.Actor == actor {
			return true
		}
	}
	return false
}

// TestMateUsesOwnItem 队员用自己背包里的道具：队长的背包里没有这件东西也能用，用掉的是队员的槽位。
func TestMateUsesOwnItem(t *testing.T) {
	leader := testSide() // 队长背包空
	mate := testSide()
	mate.Items[5] = BattleItem{ID: 1235, Name: "普通的肉"}
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	turn := b.Turn()
	target := monsterNo(t, b)
	if _, perr := b.SubmitFor(1, turn, "player", "item", Cmd{Slot: 5, Target: 1}); perr != nil {
		t.Fatalf("队员用自己的道具被拒：%v", perr)
	}
	if _, perr := b.SubmitFor(1, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(0, turn, "player", "guard", Cmd{}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	round, _ := b.Settle()
	if !hasEvent(round, "item", 1) {
		t.Fatalf("队员的道具没有结算：%+v", round.Events)
	}
	if mate.Items[5].Name != "" {
		t.Errorf("队员的槽位应该清空")
	}
}

// TestMateItemCheckedAgainstOwnBag 队员指定的槽位在自己背包里是空的就被拒（不看队长的背包）。
func TestMateItemCheckedAgainstOwnBag(t *testing.T) {
	leader := testSide()
	leader.Items[5] = BattleItem{ID: 1235, Name: "普通的肉"}
	mate := testSide()
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	if _, perr := b.SubmitFor(1, b.Turn(), "player", "item", Cmd{Slot: 5, Target: 1}); perr == nil {
		t.Fatal("队员的背包是空的，不该因为队长有这件东西就通过")
	}
}

// TestMateMagicUsesOwnMagicAndMP 队员的精灵读队员自己的栏位和气力：队长的栏位是空的、队长的气力够，队员气力不够时仍被拒。
func TestMateMagicUsesOwnMagicAndMP(t *testing.T) {
	leader := testSide()
	leader.Magics[0] = -1
	mate := testSide()
	mate.MP = 3 // 岩石撞击要 10
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	target := monsterNo(t, b)
	if _, perr := b.SubmitFor(1, b.Turn(), "player", "magic", Cmd{Magic: 0, Target: target}); perr == nil {
		t.Fatal("队员气力不够，应当被拒")
	}
	mate.MP = 100 // 快照在开战时已复制，这里另开一场
	b2 := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	if _, perr := b2.SubmitFor(1, b2.Turn(), "player", "magic", Cmd{Magic: 0, Target: monsterNo(t, b2)}); perr != nil {
		t.Fatalf("队员的精灵和气力都够，应当接受：%v", perr)
	}
}

// TestMateSwapPetFromOwnBench 队员换宠从自己的备用宠物里选；换上的宠物本回合不能行动，队长的宠物不受影响。
func TestMateSwapPetFromOwnBench(t *testing.T) {
	leader := testSide()
	mate := testSide()
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	turn := b.Turn()
	next, perr := b.SubmitFor(1, turn, "player", "swap_pet", Cmd{PetSlot: 1})
	if perr != nil || next != "wait" {
		t.Fatalf("队员换宠：next=%s err=%v", next, perr)
	}
	if b.humanOwes(1) {
		t.Error("换上的宠物本回合不能行动，队员不该再欠宠物指令")
	}
	if _, perr := b.SubmitFor(1, turn, "pet", "attack", Cmd{Target: monsterNo(t, b)}); perr == nil {
		t.Fatal("换上的宠物不能再交指令")
	}
	if _, perr := b.SubmitFor(0, turn, "player", "guard", Cmd{}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: monsterNo(t, b)}); perr != nil {
		t.Fatal(perr)
	}
	round, _ := b.Settle()
	if !hasEvent(round, "swap_pet", 1) {
		t.Fatalf("队员的换宠没有结算：%+v", round.Events)
	}
	if u := b.Unit(6); u == nil || u.Name != "布比" {
		t.Fatalf("队员的出战宠物应是备用的布比，实际 %+v", u)
	}
	if u := b.Unit(5); u == nil || u.Name != "乌力" {
		t.Fatalf("队长的宠物不该变：%+v", u)
	}
}

// TestMateSwapPetNeedsOwnBench 队员的备用栏位是空的就换不了（不借用队长的备用宠物）。
func TestMateSwapPetNeedsOwnBench(t *testing.T) {
	leader := testSide()
	mate := testSide()
	mate.Bench = [PetSlots]*PetInput{}
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	if _, perr := b.SubmitFor(1, b.Turn(), "player", "swap_pet", Cmd{PetSlot: 1}); perr == nil {
		t.Fatal("队员没有备用宠物，应当被拒")
	}
}

// TestMateCapturePlacedForCaptor 队员捕捉到的宠物归队员：安排栏位走 PlaceCapturedFor（传队员号），队长的回调不被调用。
func TestMateCapturePlacedForCaptor(t *testing.T) {
	leader := testSide()
	mate := testSide()
	b := partyWith(t, leader, func(n int) int { return 0 }, PartyMate{No: 1, Side: mate})
	var gotOwner = -1
	leaderCalled := false
	b.Cbs.PlaceCaptured = func(*Unit, *CaptureKeep) (int, *Discarded) { leaderCalled = true; return 0, nil }
	b.Cbs.PlaceCapturedFor = func(owner int, m *Unit, _ *CaptureKeep) (int, *Discarded) {
		gotOwner = owner
		return 2, nil
	}
	m := b.Unit(monsterNo(t, b))
	m.Status.Sleep = 99 // 睡着的怪好抓（同 BATTLE_CaptureCheck 的 +15%）
	ev := b.capture(b.Unit(1), m, nil)
	if ev.OK == nil || !*ev.OK {
		t.Fatalf("捕捉应当成功：%+v", ev)
	}
	if gotOwner != 1 {
		t.Errorf("PlaceCapturedFor 的主人应为队员 1，实际 %d", gotOwner)
	}
	if leaderCalled {
		t.Error("队员捕捉时不该调用队长的栏位回调")
	}
	if b.captors[m] != 1 {
		t.Errorf("捕获者应记为队员 1，实际 %d", b.captors[m])
	}
}
