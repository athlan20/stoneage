// 分身按方案出招的具体动作（STAGE-195，auto_plan.go 的决策分派到这里）：人物补血、常规动作，宠物的常规动作，
// 以及敌方目标的挑选。目标顺序必须和客户端 client/src/app/battle/auto.rs 的 enemy_order 一致：先前排、再后排，
// 每排按列走 ROW_ORDER（客户端 slots::ROW_ORDER），人物、宠物都取这串的第一个，才能打出合击。
// STAGE-312 起没有方案、或方案选 combo 时也一律取这个第一个，不再随机挑。

package battle

import (
	"github.com/athlan20/stoneage/server/gamedata"
)

// rowOrder 是一排里从第 0 列到第 4 列对应的单位位置（客户端 slots::ROW_ORDER）。
var rowOrder = [5]int{3, 1, 0, 2, 4}

// enemyOrder 给出 u 的敌方活着的单位，顺序：前排第 0–4 列，再后排第 0–4 列（单位号 = 敌方号×10 + 排偏移 + rowOrder[列]）。
func (b *Battle) enemyOrder(u *Unit) []*Unit {
	side := 1 - u.Side()
	var out []*Unit
	for _, row := range []int{firstPetNo, 0} { // 前排（宠物位 5–9 对应的 15–19）先，后排（10–14）后
		for _, pos := range rowOrder {
			if t := b.Unit(side*unitsPerSide + row + pos); t != nil && t.Alive() {
				out = append(out, t)
			}
		}
	}
	return out
}

// defaultEnemy 是默认的普通攻击目标：合击顺序里第一个还能打的敌人（STAGE-312）。
// 也是分身改选目标时的顺序（autoRetargetTo）；没有能打的敌人返回 nil。
func (b *Battle) defaultEnemy(u *Unit) *Unit {
	for _, t := range b.enemyOrder(u) {
		if t.targetable() {
			return t
		}
	}
	return nil
}

// pickEnemy 按方案的目标模式挑一个敌人：combo 取顺序里的第一个，random 用 b.Rnd 在活着的敌人里随机。
// 没有活着的敌人返回 nil。
func (b *Battle) pickEnemy(u *Unit) *Unit {
	if b.plan.Target == "random" {
		list := b.enemyOrder(u)
		if len(list) == 0 {
			return nil
		}
		return list[b.Rnd(len(list))]
	}
	return b.defaultEnemy(u)
}

// autoRetargetTo 是分身单位改选目标的规则（STAGE-312）：默认按合击顺序顺延到第一个还能打的敌人
// （当前目标倒下后就是“顺序里的下一个”）；方案显式选了 random 的仍按原版随机改选。
func (b *Battle) autoRetargetTo(u *Unit) *Unit {
	if b.plan != nil && b.plan.Target == "random" {
		return b.randomTarget(1 - u.Side())
	}
	return b.defaultEnemy(u)
}

// attackCmd 是普通攻击一个敌人；没有敌人可打就防御。
func (b *Battle) attackCmd(u *Unit) *Cmd {
	t := b.pickEnemy(u)
	if t == nil {
		return guardCmd()
	}
	return &Cmd{Action: "attack", Target: t.No}
}

// playerCmd 是人物分身的常规动作（或首回合动作）。精灵动作用不了就退回普通攻击。
func (b *Battle) playerCmd(u *Unit, act PlanAct) *Cmd {
	switch act.Act {
	case "guard":
		return guardCmd()
	case "magic":
		if c := b.magicCmd(u, act); c != nil {
			return c
		}
	}
	return b.attackCmd(u)
}

// magicCmd 按栏位找分身的精灵：栏位上要有精灵、名字要和保存的一致（Name 为空则不比）、MP 够；
// 目标按精灵的目标类型：敌方一个选合击顺序的第一个，己方一个选自己，全体或自己的不带目标，
// 己方倒下的（复活）不用。用不了返回 nil。
func (b *Battle) magicCmd(u *Unit, act PlanAct) *Cmd {
	if act.Slot < 0 || act.Slot > 4 {
		return nil
	}
	t, ok := b.unitMagic(u, act.Slot)
	if !ok || (act.Name != "" && t.Name != act.Name) || t.MP > u.MP {
		return nil
	}
	target := -1
	switch t.BattleTarget() {
	case gamedata.TargetEnemy:
		e := b.pickEnemy(u)
		if e == nil {
			return nil
		}
		target = e.No
	case gamedata.TargetAlly:
		target = u.No
	case gamedata.TargetAllyDown, gamedata.TargetAny: // 复活类、任选一方的（彩虹、极光，STAGE-200）：分身不用，退回攻击
		return nil
	}
	return &Cmd{Action: "magic", Magic: act.Slot, Target: target}
}

// healCmd 是人物分身的补血：按栏位从小到大找能用的治愈精灵（自愈 heal，或治疗他人 heal_other），MP 够就用。
// heal_other 救自己，全体的不带目标。找不到返回 nil（不补血，照常打）。
func (b *Battle) healCmd(u *Unit) *Cmd {
	for slot := 0; slot < 5; slot++ {
		t, ok := b.unitMagic(u, slot)
		if !ok || (t.Func != gamedata.MagicFuncHeal && t.Func != gamedata.MagicFuncHealOther) || t.MP > u.MP {
			continue
		}
		target := -1
		if t.Func == gamedata.MagicFuncHealOther && !t.All {
			target = u.No
		}
		return &Cmd{Action: "magic", Magic: slot, Target: target}
	}
	return nil
}

// petCmd 是分身宠物的常规动作。战宠没有补血（分身没有背包），技能用不了就退回普通攻击。
func (b *Battle) petCmd(u *Unit, act PlanAct) *Cmd {
	switch act.Act {
	case "guard":
		return guardCmd()
	case "skill":
		if c := b.skillCmd(u, act); c != nil {
			return c
		}
	}
	return b.attackCmd(u)
}

// skillCmd 按栏位找宠物技能：栏位要有战斗中能用的技能、名字要和保存的一致（Name 为空则不比）；
// 目标按技能的目标类型：敌方选合击顺序的第一个，己方一个选自己，己方倒下的不用，其余不带目标。用不了返回 nil。
func (b *Battle) skillCmd(u *Unit, act PlanAct) *Cmd {
	if act.Slot < 1 || act.Slot > 7 {
		return nil
	}
	id := u.Skills[act.Slot-1]
	sk, ok := gamedata.PetSkillByID(id)
	if !ok || sk.Field != gamedata.PetSkillFieldBattle || (act.Name != "" && sk.Name != act.Name) {
		return nil
	}
	target := -1
	switch sk.BattleTarget() {
	case gamedata.TargetEnemy:
		e := b.pickEnemy(u)
		if e == nil {
			return nil
		}
		target = e.No
	case gamedata.TargetAlly:
		target = u.No
	case gamedata.TargetAllyDown:
		return nil
	}
	return &Cmd{Action: "skill", Skill: act.Slot, Target: target}
}
