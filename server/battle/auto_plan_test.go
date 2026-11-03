// 分身按队长的自动战斗方案出招的单元测试（STAGE-195）：目标顺序对齐客户端、合击同目标、补血、
// 首回合、常规动作的退回、宠物技能、自动捕捉回避、玩家指定优先。

package battle

import "testing"

// basePlan 是一份最普通的方案：合击顺序，人物和宠物都普通攻击，血量低于 30% 补血。
func basePlan() *AutoPlan {
	return &AutoPlan{
		Target:  "combo",
		Player:  PlanAct{Act: "attack"},
		Pet:     PlanAct{Act: "attack"},
		Recover: PlanRecover{PlayerHeal: true, PlayerHealPct: 30},
	}
}

// planBattle 开一场 4 个分身的战斗（两只怪血厚，不会在这场里倒下），挂上方案。
func planBattle(t *testing.T, plan *AutoPlan) *Battle {
	t.Helper()
	b := newCloneBattle(t, 4, rollRnd(9999))
	for _, no := range []int{15, 16} {
		b.Unit(no).HP, b.Unit(no).MaxHP = 100000, 100000
	}
	b.SetAutoPlan(plan)
	return b
}

// setEnemy 在敌方格子上放一只血厚的怪（测试目标顺序用）。
func setEnemy(b *Battle, no int) {
	b.units[no] = &Unit{No: no, Kind: KindMonster, Name: "测试怪", Level: 1, HP: 100000, MaxHP: 100000, Agility: 1}
}

// cloneNos 是 4 个分身和它们的宠物的单位号。
var cloneNos = []int{1, 2, 3, 4, 6, 7, 8, 9}

// planNow 按当前状态定好这回合的分身指令（不结算）。
func planNow(b *Battle) {
	b.planAuto()
}

func expectCmd(t *testing.T, b *Battle, no int, want Cmd) {
	t.Helper()
	got := b.autoCmds[no]
	if got == nil {
		t.Errorf("单位 %d 没有指令，应为 %+v", no, want)
		return
	}
	if got.Action != want.Action || got.Target != want.Target || got.Magic != want.Magic || got.Skill != want.Skill {
		t.Errorf("单位 %d 的指令 = %+v，应为 %+v", no, *got, want)
	}
}

// 合击顺序：前排先、再后排，前排按 ROW_ORDER（3,1,0,2,4）。这里 15 号没了，前排的第一个是 16。
// 人物分身和宠物分身全部打 16。
func TestPlanComboSameTarget(t *testing.T) {
	b := planBattle(t, basePlan())
	planNow(b)
	for _, no := range cloneNos {
		expectCmd(t, b, no, Cmd{Action: "attack", Target: 16})
	}
}

// 目标顺序与客户端 enemy_order 一致：18、16、13、10 依次倒下，依次改打下一个。
func TestPlanComboFollowsClientOrder(t *testing.T) {
	b := planBattle(t, basePlan())
	b.leave(b.Unit(15))
	b.leave(b.Unit(16))
	for _, no := range []int{10, 13, 16, 18} {
		setEnemy(b, no)
	}
	for _, want := range []int{18, 16, 13, 10} {
		planNow(b)
		expectCmd(t, b, 1, Cmd{Action: "attack", Target: want})
		b.Unit(want).Down = true
	}
}

// random 模式用 b.Rnd 在活着的敌人里取一个：这里固定取下标 1，列表是 [16, 15]，所以打 15。
func TestPlanRandomUsesRnd(t *testing.T) {
	plan := basePlan()
	plan.Target = "random"
	b := planBattle(t, plan)
	b.Rnd = fixedRnd(1)
	planNow(b)
	for _, no := range cloneNos {
		expectCmd(t, b, no, Cmd{Action: "attack", Target: 15})
	}
}

// 低血补血：分身 HP ≤ 门槛、栏位上有自愈或治疗他人的精灵、MP 够，就放精灵；自愈不带目标，治疗他人救自己。
func TestPlanHealsWithHealingMagic(t *testing.T) {
	b := planBattle(t, basePlan())
	b.Unit(1).Magics[1] = 0  // 治愈的精灵 Lv1（自愈）
	b.Unit(2).Magics[2] = 10 // 滋润的精灵 Lv1（治疗他人）
	for _, no := range []int{1, 2} {
		b.Unit(no).HP = 60
	}
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "magic", Magic: 1, Target: -1})
	expectCmd(t, b, 2, Cmd{Action: "magic", Magic: 2, Target: 2})
}

// 没有治愈精灵、或气力不够：不补血，照常普通攻击，回合正常结算。
func TestPlanHealFallsBackToAttack(t *testing.T) {
	b := planBattle(t, basePlan())
	b.Unit(1).HP = 60                         // 没有精灵
	b.Unit(2).HP, b.Unit(2).Magics[1] = 60, 0 // 有精灵，气力为 0
	b.Unit(2).MP = 0
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 16})
	expectCmd(t, b, 2, Cmd{Action: "attack", Target: 16})

	submitAll(b, 15)
	round, _ := b.Settle()
	for _, e := range round.Events {
		if e.Kind == "magic" && e.Actor != nil && (*e.Actor == 1 || *e.Actor == 2) {
			t.Fatalf("没有可用的精灵却放了精灵: %+v", e)
		}
	}
}

// 补血的实际结算：低血的分身放了治愈精灵，事件是 magic，血量回升。
func TestPlanHealSettles(t *testing.T) {
	b := planBattle(t, basePlan())
	b.Unit(1).Magics[1] = 0
	b.Unit(1).HP = 60
	submitAll(b, 15)
	round, _ := b.Settle()
	found := false
	for _, e := range round.Events {
		if e.Kind == "magic" && e.Actor != nil && *e.Actor == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("分身 1 应放出治愈精灵: %+v", round.Events)
	}
	if hp := b.Unit(1).HP; hp <= 60 {
		t.Fatalf("治愈后分身 1 的血量 = %d，应高于 60", hp)
	}
}

// 首回合：First.Enabled 时第 1 回合用首回合动作，第 2 回合回到常规动作。
func TestPlanFirstTurn(t *testing.T) {
	plan := basePlan()
	plan.First = PlanFirst{Enabled: true, Player: PlanAct{Act: "guard"}, Pet: PlanAct{Act: "guard"}}
	b := planBattle(t, plan)
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "guard", Target: -1})
	expectCmd(t, b, 6, Cmd{Action: "guard", Target: -1})

	b.nextTurn()
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 16})
	expectCmd(t, b, 6, Cmd{Action: "attack", Target: 16})
}

// 常规动作的 guard：人物和宠物都防御，不出手。
func TestPlanGuardAction(t *testing.T) {
	plan := basePlan()
	plan.Player = PlanAct{Act: "guard"}
	plan.Pet = PlanAct{Act: "guard"}
	b := planBattle(t, plan)
	planNow(b)
	for _, no := range cloneNos {
		expectCmd(t, b, no, Cmd{Action: "guard", Target: -1})
	}
}

// 人物的精灵动作：栏位和名字对得上、MP 够时放精灵；名字对不上、复活类、MP 不够都退回普通攻击。
func TestPlanMagicAction(t *testing.T) {
	plan := basePlan()
	plan.Player = PlanAct{Act: "magic", Slot: 0, Name: "岩石撞击"}
	b := planBattle(t, plan)
	planNow(b)
	// 岩石撞击是单体攻击：目标是敌方第一个（合击顺序）。
	expectCmd(t, b, 1, Cmd{Action: "magic", Magic: 0, Target: 16})

	plan.Player = PlanAct{Act: "magic", Slot: 0, Name: "别的精灵"}
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 16})

	b.Unit(1).Magics[2] = 50 // 复活精灵：己方倒下的，自动不用
	plan.Player = PlanAct{Act: "magic", Slot: 2}
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 16})

	b.Unit(1).Magics[3] = 240 // 彩虹的精灵：任选一方（STAGE-200 的 any 目标），分身不用，退回攻击
	plan.Player = PlanAct{Act: "magic", Slot: 3}
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 16})

	plan.Player = PlanAct{Act: "magic", Slot: 0}
	b.Unit(1).MP = 0 // 气力不够（岩石撞击耗 10）
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 16})
}

// 宠物技能：栏位有战斗技能、名字对得上时用技能（敌方单体，目标是合击顺序的第一个）；栏位空、名字对不上退回攻击。
func TestPlanPetSkill(t *testing.T) {
	plan := basePlan()
	plan.Pet = PlanAct{Act: "skill", Slot: 1}
	b := planBattle(t, plan)
	planNow(b)
	expectCmd(t, b, 6, Cmd{Action: "skill", Skill: 1, Target: 16})

	plan.Pet = PlanAct{Act: "skill", Slot: 2} // 栏位空
	planNow(b)
	expectCmd(t, b, 6, Cmd{Action: "attack", Target: 16})

	plan.Pet = PlanAct{Act: "skill", Slot: 1, Name: "别的技能"}
	planNow(b)
	expectCmd(t, b, 6, Cmd{Action: "attack", Target: 16})
}

// 自动捕捉回避：有符合条件的候选时，所有分身和分身宠物防御；候选等级超过队长 +5 就不算候选。
func TestPlanCaptureGuards(t *testing.T) {
	plan := basePlan()
	plan.Capture = PlanCapture{Enabled: true, LevelMin: 1}
	b := planBattle(t, plan)
	planNow(b)
	for _, no := range cloneNos {
		expectCmd(t, b, no, Cmd{Action: "guard", Target: -1})
	}
	submitAll(b, 15)
	round, _ := b.Settle()
	for _, e := range round.Events {
		if e.Kind == "attack" && e.Actor != nil && (*e.Actor == 1 || *e.Actor == 2 || *e.Actor == 6 || *e.Actor == 7) {
			t.Fatalf("有候选时分身还出手了: %+v", e)
		}
	}

	b2 := planBattle(t, plan)
	b2.Unit(15).Level, b2.Unit(16).Level = 7, 7 // 队长 1 级，7 > 1+5，不是候选
	planNow(b2)
	expectCmd(t, b2, 1, Cmd{Action: "attack", Target: 16})

	plan.Capture.Names = []string{"不存在"} // 名字不符合
	b3 := planBattle(t, plan)
	planNow(b3)
	expectCmd(t, b3, 1, Cmd{Action: "attack", Target: 16})
}

// 玩家给分身指定的目标优先于方案（STAGE-186）：方案说防御，指定的目标照打，不消耗随机数。
func TestPlanCloneCmdsWin(t *testing.T) {
	plan := basePlan()
	plan.Player = PlanAct{Act: "guard"}
	b := planBattle(t, plan)
	if perr := submitErr(b, "clone:1", Cmd{Target: 15}); perr != nil {
		t.Fatal(perr)
	}
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "attack", Target: 15})
	expectCmd(t, b, 2, Cmd{Action: "guard", Target: -1})
}

// 没有活着的敌人时防御（方案下的兜底）。
func TestPlanNoEnemyGuards(t *testing.T) {
	b := planBattle(t, basePlan())
	b.leave(b.Unit(15))
	b.leave(b.Unit(16))
	planNow(b)
	expectCmd(t, b, 1, Cmd{Action: "guard", Target: -1})
}

// 合击打开时，队长、分身和分身宠物打同一个目标（合击顺序的第一个），结算成一次 combo 事件，一人一条效果。
func TestPlanComboEvent(t *testing.T) {
	b := planBattle(t, basePlan())
	b.Combo = true
	b.Rnd = comboRnd(1, 0) // 起头判定出 1，必定成功
	submitAll(b, 16)
	round, _ := b.Settle()
	for _, e := range round.Events {
		if e.Kind != "combo" {
			continue
		}
		for _, eff := range e.Effects {
			if eff.Target != 16 {
				t.Fatalf("合击打到了 %d 号，应全部打 16: %+v", eff.Target, e)
			}
		}
		if len(e.Effects) < 4 {
			t.Fatalf("合击只有 %d 条效果，应包含分身和分身宠物: %+v", len(e.Effects), e)
		}
		return
	}
	t.Fatalf("没有 combo 事件: %+v", round.Events)
}
