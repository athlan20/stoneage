// 回合内的技能效果登记与目标判定（STAGE-76）。
// 原版的忠犬、背水之战、不防守战法在玩家/宠物输入指令时就写进了战斗工作变量（PETSKILL_* 的 CHAR_setWorkInt），
// 之后整回合的判定都看这些变量，与行动顺序无关；这里在结算开始时把它们从提交的指令里登记好。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// beginRound 在结算开始时登记本回合指令定下的效果（原版 BATTLE_PreCommandSeq 之后的状态）。
// 只看人物和宠物的指令（真人同场时每个在场真人的都登记）；怪物的技能在它行动时才决定，效果从它出手那一刻起生效。
func (b *Battle) beginRound() {
	b.guards = map[int]int{}
	for _, u := range b.units {
		if u != nil {
			u.stance = stanceMods{}
			u.plan = nil
			u.obey = nil
			u.actedRound, u.comAtk = false, false // STAGE-292：反击用的出手记号按回合清
			u.guardRound = false
		}
	}
	b.planAuto() // 分身的指令也在这里定下，合击检查要先看（STAGE-162）
	for _, no := range b.humanNos() {
		b.registerPlayerStance(no) // 人物的精灵版宠物技能（STAGE-202，magic_attskill.go）
		b.registerHumanGuard(no)
		b.registerPetStance(no)
	}
	b.registerMonsterStance() // STAGE-291 批 B：怪物本回合计划里的忠犬、背水、不防守、防御
}

// registerPetStance 登记真人 no 的出战宠物这回合交的宠物技能（忠犬、背水之战、不防守战法）。
func (b *Battle) registerPetStance(no int) {
	pet := b.petOf(no)
	cmd := b.cmdOfHuman(no, "pet")
	if pet == nil || cmd == nil || cmd.Action != "skill" || !pet.Alive() {
		return
	}
	skill, ok := gamedata.PetSkillByID(pet.Skills[cmd.Skill-1])
	if !ok {
		return
	}
	if skill.Func == "PETSKILL_NormalGuard" {
		pet.guardRound = true
	}
	if skill.Func == "PETSKILL_Guardian" { // 忠犬：主人（同一格号的人物，宠物号减 5）挨打时由宠物替挡
		b.guards[pet.No-firstPetNo] = pet.No
	}
	stanceFor(pet, skill)
}

// stanceFor 登记宠物技能定下的整回合效果：背水之战的防御力按防% 变化，不防守战法的回避、反击率变化并且可以反击。
// 人物的精灵版宠物技能（STAGE-202）用同一套登记（registerPlayerStance）。
func stanceFor(u *Unit, skill gamedata.PetSkill) {
	switch skill.Func {
	case "PETSKILL_PowerBalance": // 背水之战：本回合自己的防御力按防% 变化
		pct, _ := optNumber(skill.Arg, "防%")
		u.stance.DefensePct = int(pct)
	case "PETSKILL_NoGuard": // 不防守战法：本回合回避、反击率变化，可以反击
		duck, _ := optNumber(skill.Arg, "回避%")
		counter, _ := optNumber(skill.Arg, "反击%")
		u.stance.NoGuard = true
		u.stance.NoGuardDuck = int(duck)
		u.stance.NoGuardCounter = int(counter)
	}
}

// guardianOf 返回替这一下攻击挨打的忠犬（原版 BATTLE_GuardianCheck）：目标被登记了忠犬，
// 忠犬还站着、没有睡、乱、麻、石化、魔障（STAGE-201），并且不是攻击者自己时才替挡。
func (b *Battle) guardianOf(attacker, target *Unit) *Unit {
	no, ok := b.guards[target.No]
	if !ok {
		return nil
	}
	g := b.Unit(no)
	if g == nil || !g.Alive() || g == attacker || g == target {
		return nil
	}
	if s := g.Status; s.Sleep > 0 || s.Confusion > 0 || s.Paralysis > 0 || s.Stone > 0 || s.Barrier > 0 {
		return nil
	}
	return g
}

// adjustTarget 是原版 BATTLE_TargetAdjust：指定的目标已经倒下或隐身时改选一个能选的单位。
// 分身单位按合击顺序顺延（STAGE-312，autoRetargetTo），其余从对方随机改选。对方一个都没有时返回 nil。
func (b *Battle) adjustTarget(u *Unit, t *Unit) *Unit {
	if t != nil && t.targetable() && t.Side() != u.Side() {
		return t
	}
	if u.Auto {
		return b.autoRetargetTo(u)
	}
	return b.randomTarget(1 - u.Side())
}

// randomTarget 从某一方能选的单位里随机挑一个（BATTLE_DefaultAttacker）。
func (b *Battle) randomTarget(side int) *Unit {
	var cands []*Unit
	for _, t := range b.units {
		if t != nil && t.Side() == side && t.targetable() {
			cands = append(cands, t)
		}
	}
	if len(cands) == 0 {
		return nil
	}
	return cands[b.Rnd(len(cands))]
}

// hidden 表示单位在地球一周的隐身中（原版 CHAR_ISATTACKED 为 0）：不能被选为目标。
func (u *Unit) hidden() bool { return u.pend != nil && u.pend.kind == pendEarth }

// targetable 表示单位现在能被选为攻击目标：还站着，并且没有隐身。
func (u *Unit) targetable() bool { return u.Alive() && !u.hidden() }
