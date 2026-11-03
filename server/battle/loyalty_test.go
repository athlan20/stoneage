// 宠物忠诚（STAGE-127）的测试：听话判定的分档、不听话时的改写、忠诚的增减，对照原版 BATTLE_PetLoyalCheck 与 AI_FIX_*。

package battle

import "testing"

// diceRnd 让 d100（听话判定，逃跑、捕捉）出 dice（1–100），判定类的 d10000 出大值，不闪避、不暴击。
func diceRnd(dice int) func(n int) int {
	return func(n int) int {
		if n == 100 {
			return dice - 1
		}
		return 9999 % n
	}
}

// sideWithLoyalty 是测试用的一方，出战宠物的基础忠诚由参数给定（可变部分为 0）。
func sideWithLoyalty(base int) *SideInput {
	side := testSide()
	side.BattlePet.LoyaltyBase = base
	return side
}

func TestPickLoyalModeBands(t *testing.T) {
	cases := []struct {
		ai, dice int
		want     loyalMode
	}{
		{80, 1, loyalNormal},
		{79, 9, loyalRetarget}, {79, 10, loyalNormal},
		{70, 9, loyalRetarget}, {70, 10, loyalNormal},
		{69, 19, loyalRetarget}, {69, 20, loyalNormal},
		{60, 19, loyalRetarget}, {60, 20, loyalNormal},
		{59, 34, loyalRetarget}, {59, 35, loyalNormal},
		{50, 34, loyalRetarget}, {50, 35, loyalNormal},
		{49, 49, loyalRetarget}, {49, 50, loyalNormal},
		{40, 49, loyalRetarget}, {40, 50, loyalNormal},
		{39, 69, loyalRandom}, {39, 70, loyalNormal},
		{20, 69, loyalRandom}, {20, 70, loyalNormal},
		{19, 79, loyalOwner}, {19, 80, loyalEnemy},
		{10, 79, loyalOwner}, {10, 80, loyalEnemy},
		{9, 59, loyalOwner}, {9, 60, loyalEscape},
		{0, 100, loyalEscape},
	}
	for _, c := range cases {
		if got := pickLoyalMode(c.ai, c.dice); got != c.want {
			t.Errorf("pickLoyalMode(ai=%d, dice=%d) = %v，want %v", c.ai, c.dice, got, c.want)
		}
	}
}

// 忠诚满（≥80）的宠物一定听话，而且不摇骰子：loyaltyCheck 返回的指令就是提交的那一条。
func TestLoyalPetObeysWithoutRoll(t *testing.T) {
	calls := 0
	rnd := func(n int) int {
		if n == 100 {
			calls++
		}
		return 0
	}
	b := newBattleWithSide(t, sideWithLoyalty(100), "win", rnd)
	cmd := &Cmd{Actor: "pet", Action: "attack", Target: 15}
	plan := b.loyaltyCheck(b.PetUnit(), cmd)
	if plan.mode != loyalNormal || plan.cmd != cmd {
		t.Fatalf("忠诚满的宠物 plan = %+v，want 听话且指令不变", plan)
	}
	if calls != 0 {
		t.Errorf("忠诚满的宠物摇了 %d 次 d100，want 0", calls)
	}
}

// 没有任何技能的宠物不论抽中哪一种，都是不出手（PETAI_MODE_NOACT）：disobey 的 reason 是 no_skill，之后是 skip。
func TestDisobeyWithoutSkillDoesNothing(t *testing.T) {
	side := sideWithLoyalty(0)
	side.BattlePet.Skills = [7]int{-1, -1, -1, -1, -1, -1, -1}
	b := newBattleWithSide(t, side, "win", diceRnd(1))
	submitAttacks(t, b, 15, 15)
	round, _ := b.Settle()
	var dis, act *Event
	for i := range round.Events {
		switch ev := &round.Events[i]; {
		case ev.Kind == "disobey" && ev.Actor != nil && *ev.Actor == 5:
			dis = ev
		case ev.Actor != nil && *ev.Actor == 5 && act == nil:
			act = ev
		}
	}
	if dis == nil || dis.Reason != "no_skill" {
		t.Fatalf("宠物没有技能却没有 no_skill 的 disobey：%+v", round.Events)
	}
	if act == nil || act.Kind != "skip" || act.Reason != "disobey" {
		t.Fatalf("宠物这回合应该不出手（skip disobey），实际 %+v", act)
	}
}

// 忠诚低、骰子落在“打主人”档：宠物先给 disobey（reason owner），然后打人物。
func TestDisobeyOwnerAttack(t *testing.T) {
	b := newBattleWithSide(t, sideWithLoyalty(0), "win", diceRnd(1))
	submitAttacks(t, b, 15, 15)
	round, _ := b.Settle()
	dis := round.Events[0]
	if dis.Kind != "disobey" || dis.Reason != "owner" || dis.Actor == nil || *dis.Actor != 5 {
		t.Fatalf("第一个事件 = %+v，want 宠物 5 的 disobey（owner）", dis)
	}
	if len(round.Events) < 2 || round.Events[1].Kind != "attack" || *round.Events[1].Actor != 5 ||
		round.Events[1].Effects[0].Target != 0 {
		t.Fatalf("disobey 之后应是宠物攻击人物：%+v", round.Events[1])
	}
}

// 骰子落在“跑掉”档：宠物离场（pet_escape），战斗不结束，人物还在；结束状态里宠物不在场。
func TestDisobeyPetRunsAway(t *testing.T) {
	b := newBattleWithSide(t, sideWithLoyalty(0), "win", diceRnd(100))
	submitAttacks(t, b, 15, 15)
	round, end := b.Settle()
	if end != nil {
		t.Fatalf("宠物跑掉不应结束战斗：%+v", end)
	}
	var left *Event
	for i := range round.Events {
		if round.Events[i].Kind == "pet_escape" {
			left = &round.Events[i]
		}
	}
	if left == nil || len(left.Left) != 1 || left.Left[0] != 5 {
		t.Fatalf("没有宠物 5 跑掉的 pet_escape：%+v", round.Events)
	}
	if b.PetUnit() != nil {
		t.Errorf("宠物跑掉后仍在场")
	}
	for _, e := range b.PetEnds() {
		if e.Slot == 0 && e.Active {
			t.Errorf("宠物跑掉后 PetEnds 仍标记为在场")
		}
	}
}

// 合击不收忠诚不足的宠物：它这回合不听话，人物和宠物打同一个怪也不合成 combo。
func TestDisobeyedPetNotInCombo(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	b.PetUnit().loyBase = 0 // 忠诚 0，这回合一定不听话
	submitAttacks(t, b, 15, 15)
	round, _ := b.Settle()
	for _, ev := range round.Events {
		for _, a := range ev.Actors {
			if ev.Kind == "combo" && a == 5 {
				t.Fatalf("忠诚不足的宠物不应参与合击：%+v", round.Events)
			}
		}
	}
}

// 抽中“乱来”时原版会清掉忠犬标记（battle.c 的 PETAI_MODE_RANDOMACT，STAGE-127）：
// 宠物出手时撤销登记，这回合不再替主人挡刀；它出手之前主人挨打仍由它替挡。
func TestDisobeyRandomClearsGuardian(t *testing.T) {
	b := skillTestBattle(t, 20) // 1 号栏位是忠犬
	b.PetUnit().loyBase = 25    // 20–39 档
	b.Rnd = diceRnd(1)          // d100 出 1：乱来
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物提交失败: %v", perr)
	}
	if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: 15}); perr != nil {
		t.Fatalf("忠犬提交失败: %v", perr)
	}
	b.beginRound()
	pet, boar, owner := b.PetUnit(), b.Unit(15), b.PlayerUnit()
	if g := b.guardianOf(boar, owner); g != pet {
		t.Fatalf("提交忠犬后应先登记忠犬，得到 %v", g)
	}
	events := b.act(pet) // 宠物出手：判定乱来并撤销忠犬
	if len(events) == 0 || events[0].Kind != "disobey" || events[0].Reason != "random" {
		t.Fatalf("第一个事件应是 disobey（random）：%+v", events)
	}
	if g := b.guardianOf(boar, owner); g != nil {
		t.Errorf("乱来的宠物不应再替主人挡刀，得到 %v", g)
	}
}

// 胜利：在场的宠物每打倒一只敌人加一次；敌人比宠物高级时加 AI_FIX_PETGOLDWIN（20）、否则加 AI_FIX_PETWIN（1）。
func TestSettleLoyaltyWin(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	pet := b.PetUnit() // 宠物 5 级
	b.result = "win"
	b.slain = []*Unit{{Level: 8}, {Level: 3}}
	b.settleLoyalty()
	if want := loyalWinHigher + loyalWin; pet.loyVar != want {
		t.Errorf("胜利后可变部分 = %d，want %d（高级敌人 +20，低级 +1）", pet.loyVar, want)
	}
	if !b.loyalChanged || !b.PetsChanged() {
		t.Errorf("忠诚变了，结束时要推 pets_changed")
	}
}

// 人物被打飞而输：出战宠物减 10×100；人物 10 级以下减半（AI_FIX_PLAYERULTIMATE / 2）。
func TestSettleLoyaltyFlownLose(t *testing.T) {
	b := newTestBattle(t, "lose", rollRnd(0))
	pet := b.PetUnit()
	b.result = "lose"
	b.PlayerUnit().Flown = true
	b.settleLoyalty()
	if want := loyalFlown / 2; pet.loyVar != want { // 测试人物 1 级，减半
		t.Errorf("被打飞输了，可变部分 = %d，want %d", pet.loyVar, want)
	}
	// 没有被打飞、只是输了：不扣
	b2 := newTestBattle(t, "lose", rollRnd(0))
	b2.result = "lose"
	b2.settleLoyalty()
	if b2.PetUnit().loyVar != 0 {
		t.Errorf("没被打飞输了，忠诚不应变化")
	}
}

// 主人攻击自己的宠物：宠物忠诚 -2×100（AI_FIX_SEKKAN），在每一下攻击判定时记。
func TestOwnerHitsOwnPetLowersLoyalty(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	pet := b.PetUnit()
	b.attackSeq(b.PlayerUnit(), pet, false)
	if pet.loyVar != loyalOwnerHit {
		t.Errorf("主人打宠物后可变部分 = %d，want %d", pet.loyVar, loyalOwnerHit)
	}
}

// 被道具或精灵回复：每场战斗只加一次（AI_FIX_PETRECOVERY）。
func TestRecoveryAddsLoyaltyOncePerBattle(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	pet := b.PetUnit()
	pet.HP = 10
	var ev Event
	b.healUnit(pet, 20, &ev)
	b.healUnit(pet, 20, &ev)
	if pet.loyVar != loyalRecovered {
		t.Errorf("两次回复后可变部分 = %d，want %d（每场一次）", pet.loyVar, loyalRecovered)
	}
}

// 复活精灵救起宠物：忠诚 +3×100（AI_FIX_PETRESSURECT）。
func TestReviveAddsLoyalty(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	pet := b.PetUnit()
	pet.Down, pet.HP = true, 0
	var ev Event
	b.reviveUnit(pet, 50, &ev)
	if pet.loyVar != loyalRevived {
		t.Errorf("复活后可变部分 = %d，want %d", pet.loyVar, loyalRevived)
	}
}

// 换下去再换回来：血量和忠诚的可变部分沿用上一次的值，不回到开战时的快照。
func TestSwapInKeepsHPAndLoyalty(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	old := b.PetUnit()
	old.HP, old.loyVar = 50, 300
	u := b.swapIn(0, b.sides[0].BattlePet, old.No)
	if u.HP != 50 || u.loyVar != 300 {
		t.Errorf("换回的宠物 HP=%d 可变部分=%d，want 50 与 300", u.HP, u.loyVar)
	}
	for _, e := range b.PetEnds() {
		if e.Slot == 0 && (e.HP != 50 || e.VariableAI != 300) {
			t.Errorf("PetEnds 的宠物 0 = %+v，want HP 50 可变部分 300", e)
		}
	}
}
