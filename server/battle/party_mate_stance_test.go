// 真人同场战斗的回合登记与合击（STAGE-275 第二轮，缺口 3、4）：每个在场真人的宠物技能效果都登记；真人同场不触发合击。
package battle

import "testing"

// TestPartyMateGuardianRegistered 队员的宠物交了忠犬，结算开始时登记为替挡：主人是队员 1，忠犬是 6 号。
func TestPartyMateGuardianRegistered(t *testing.T) {
	mate := testSide()
	mate.BattlePet.Skills[0] = 20 // 忠犬（同 skillTestBattle 的 1 号栏位）
	b := partyWith(t, testSide(), rollRnd(9999), PartyMate{No: 1, Side: mate})
	target := monsterNo(t, b)
	turn := b.Turn()
	mustSubmit(t, b, 0, turn, "player", "guard", Cmd{})
	mustSubmit(t, b, 0, turn, "pet", "attack", Cmd{Target: target})
	mustSubmit(t, b, 1, turn, "player", "guard", Cmd{})
	mustSubmit(t, b, 1, turn, "pet", "skill", Cmd{Skill: 1, Target: target})
	b.beginRound()
	if got, ok := b.guards[1]; !ok || got != 6 {
		t.Fatalf("队员的忠犬应登记为替 1 号挡：guards=%v", b.guards)
	}
	if g := b.guardianOf(b.Unit(target), b.Unit(1)); g == nil || g.No != 6 {
		t.Errorf("怪物打队员时应由忠犬替挡，实际 %+v", g)
	}
}

// TestPartyLeaderGuardianStillWorks 队长的忠犬照常登记（队员的登记不影响队长的）。
func TestPartyLeaderGuardianStillWorks(t *testing.T) {
	leader := testSide()
	leader.BattlePet.Skills[0] = 20
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: testSide()})
	target := monsterNo(t, b)
	turn := b.Turn()
	mustSubmit(t, b, 0, turn, "player", "guard", Cmd{})
	mustSubmit(t, b, 0, turn, "pet", "skill", Cmd{Skill: 1, Target: target})
	mustSubmit(t, b, 1, turn, "player", "guard", Cmd{})
	mustSubmit(t, b, 1, turn, "pet", "attack", Cmd{Target: target})
	b.beginRound()
	if got, ok := b.guards[0]; !ok || got != 5 {
		t.Fatalf("队长的忠犬应登记为替 0 号挡：guards=%v", b.guards)
	}
	if _, ok := b.guards[1]; ok {
		t.Error("队员没交忠犬，不该登记替挡")
	}
}

// TestPartyNoComboEvenWhenEnabled 真人同场时不触发合击（原版成员合击较复杂，本期不做）：即使 Combo 开着、
// 骰子每次都让合击成功，全员普攻也不出现 combo 事件。
func TestPartyNoComboEvenWhenEnabled(t *testing.T) {
	b := partyWith(t, testSide(), func(n int) int { return 0 }, PartyMate{No: 1, Side: testSide()}, PartyMate{No: 2, Side: testSide()})
	b.Combo = true
	target := monsterNo(t, b)
	turn := b.Turn()
	for _, no := range []int{0, 1, 2} {
		mustSubmit(t, b, no, turn, "player", "attack", Cmd{Target: target})
		mustSubmit(t, b, no, turn, "pet", "attack", Cmd{Target: target})
	}
	order := b.actingOrder()
	for i := range order {
		if group, _ := b.comboGroup(order, i); len(group) > 0 {
			t.Fatalf("真人同场不该有合击组：%d 号起头的组 %d 人", order[i].No, len(group))
		}
	}
	round, _ := b.Settle()
	for _, ev := range round.Events {
		if ev.Kind == "combo" {
			t.Fatalf("真人同场出现了合击事件：%+v", ev)
		}
	}
}

// mustSubmit 交一条指令，被拒就失败。
func mustSubmit(t *testing.T, b *Battle, no, turn int, actor, action string, cmd Cmd) {
	t.Helper()
	if _, perr := b.SubmitFor(no, turn, actor, action, cmd); perr != nil {
		t.Fatalf("真人 %d 的 %s %s 被拒：%v", no, actor, action, perr)
	}
}
