// 怪物整回合登记、防御指令、不防守战法反击、地球一周隐身的测试（STAGE-291 批 B）。
package battle

import "testing"

// 怪物计划里的不防守战法：回合开始时登记（回避、反击率、可以反击），不必等它出手。
func TestMonsterNoGuardRegisteredAtRoundStart(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	b.guards = map[int]int{}
	mon := enemiesOf(b)[0]
	mon.Skills[0] = 150
	mon.plan = &monsterPlan{act: aiAction{kind: "skill", skill: 1, target: 0}, ok: true}
	b.registerMonsterStance()
	if !mon.stance.NoGuard || mon.stance.NoGuardCounter != 50 {
		t.Errorf("怪物不防守战法未登记：%+v", mon.stance)
	}
}

// 怪物计划里的忠犬：登记替挡关系，替的是“格号 −5”的同伴（同一方）。
func TestMonsterGuardianProtectsSlotMinusFive(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	b.guards = map[int]int{}
	mon := enemiesOf(b)[0]
	ally := &Unit{No: mon.No - 5, Kind: KindMonster, HP: 10, MaxHP: 10}
	b.units[ally.No] = ally
	mon.Skills[0] = 20
	mon.plan = &monsterPlan{act: aiAction{kind: "skill", skill: 1, target: 0}, ok: true}
	b.registerMonsterStance()
	if got := b.guards[ally.No]; got != mon.No {
		t.Fatalf("忠犬登记 guards[%d] = %d，want %d", ally.No, got, mon.No)
	}
}

// 怪物计划里的防御指令：整回合算防御（破防、落马术的判定）。
func TestMonsterGuardPlanCommitsGuard(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	b.guards = map[int]int{}
	mon := enemiesOf(b)[0]
	mon.plan = &monsterPlan{act: aiAction{kind: "guard"}, ok: true}
	b.registerMonsterStance()
	if !guarding(mon) {
		t.Error("怪物计划防御后，回合内应算防御")
	}
	if mon.Status.Guard {
		t.Error("计划防御不应提前置位 Status.Guard（行动时才置位）")
	}
}

// 破除防御按本回合指令判定：目标交了防御（即使还没行动），破防就打全额、不算没打中。
func TestGuardBreakUsesCommittedGuard(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	pet := b.PetUnit()
	mon := enemiesOf(b)[0]
	mon.guardRound = true
	ev := b.guardBreak(pet, mon.No, "破除防御")
	if damageTo(ev, mon.No) <= 0 {
		t.Errorf("防御指令中的怪应被破防打全额，效果 %+v", ev.Effects)
	}
	if b.lastAtk.cont {
		t.Error("防御中的目标应结束反击循环（原版 iRet = FALSE）")
	}
}

// 不防守战法的反击率参与反击判定：宠物用了不防守战法，对怪物的反击率多出 NoGuardCounter，且能反击。
func TestNoGuardCounterUsedAgainstMonster(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	pet := b.PetUnit()
	mon := enemiesOf(b)[0]
	base := b.counterPer(pet, mon)
	pet.stance.NoGuard, pet.stance.NoGuardCounter = true, 50
	if got := b.counterPer(pet, mon); got != base+50 {
		t.Errorf("反击率 %v，want %v（基础 %v 加不防守 50）", got, base+50, base)
	}
	if !b.canCounter(pet) {
		t.Error("不防守战法的宠物应能反击")
	}
}

// 地球一周隐身中的单位不被范围精灵取到（原版 BATTLE_TargetCheck 的 CHAR_ISATTACKED）。
func TestEarthHiddenNotInMagicMask(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	you := b.PlayerUnit()
	mon := enemiesOf(b)[0]
	mon.pend = &pending{kind: pendEarth}
	hit := func(row, col int) bool { return true }
	for _, x := range b.magicMaskTargets(you, hit) {
		if x == mon {
			t.Fatal("隐身中的怪物不应被范围精灵取到")
		}
	}
}
