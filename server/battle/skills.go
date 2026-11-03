// 宠物技能的执行（STAGE-49）：按 petskill.txt 的函数名分派，对照
// gmsv/battle/pet_skill.c。一期实现：通常攻击、通常防御、连续攻击（多次）、
// 破除防御（skills_break.go）、异常攻击（毒、石化、混乱、泥醉、催眠攻击，STAGE-75）；
// 强化攻击类（一击必杀、突击、背水之战、忠犬、地球一周、不防守战法）在 skills_strong.go（STAGE-76）；
// 偷窃、捐献、旅程伙伴在 skills_steal.go；其余函数先按通常攻击处理（技能表里 PETSKILL_None 待机按跳过）。

package battle

import (
	"strconv"

	"github.com/athlan20/stoneage/server/gamedata"
)

// castSkill 执行宠物技能（skill 栏位上的技能模板）。
func (b *Battle) castSkill(pet *Unit, skill gamedata.PetSkill, cmd Cmd) Event {
	switch skill.Func {
	case "PETSKILL_None": // 待机：什么也不做
		return skipEvent(pet, "timeout")
	case "PETSKILL_NormalGuard": // 通常防御
		pet.Status.Guard = true
		return b.selfEffect(pet, "skill", "none", 0, []string{"guard"})
	case "PETSKILL_NormalAttack": // 通常攻击（等同普攻）
		return b.attackEvent(pet, cmd.Target, skill.Name)
	case "PETSKILL_ContinuationAttack": // 连续攻击：一回合打 N 次
		n, _ := strconv.Atoi(skill.Arg) // 原版 pet_skill.c：参数不在 1–10 时 N = 1
		if n < 1 || n > 10 {
			n = 1
		}
		return b.comboAttack(pet, cmd.Target, skill.Name, n)
	case "PETSKILL_GuardBreak": // 破除防御：防御中全额，没防御算没打中（skills_break.go）
		return b.guardBreak(pet, cmd.Target, skill.Name)
	case "PETSKILL_FallGround": // 落马术：攻击力 −30% 的一击，打中后按原版掷数落马（skills_break.go）
		return b.fallGround(pet, cmd.Target, skill)
	case "PETSKILL_Steal": // 偷窃：怪物不会被偷，只抽一次随机（skills_steal.go）
		return b.steal(pet, cmd.Target, skill.Name)
	case "PETSKILL_StealMoney": // 捐献：对怪物 5% 拿 10–100 石币，成功后宠物离场（skills_steal.go）
		return b.stealMoney(pet, cmd.Target, skill.Name)
	case "PETSKILL_Abduct": // 旅程伙伴：带走怪物，宠物自己离场（skills_steal.go）
		return b.abduct(pet, cmd.Target, skill.Name)
	case "PETSKILL_StatusChange": // 异常攻击：打折攻击，命中后按概率上异常状态
		if sc, ok := skill.ParseStatusChange(); ok {
			return b.statusChangeAttack(pet, cmd.Target, skill.Name, sc)
		}
		return b.attackEvent(pet, cmd.Target, skill.Name)
	case "PETSKILL_Mighty": // 一击必杀：倍率与对方回避（skills_strong.go）
		return b.mighty(pet, cmd.Target, skill)
	case "PETSKILL_ChargeAttack": // 突击：蓄力后加攻击力出手
		return b.charge(pet, cmd, skill)
	case "PETSKILL_PowerBalance": // 背水之战：攻击力上升，防御力下降（防御在 beginRound 登记）
		return b.powerBalance(pet, cmd.Target, skill)
	case "PETSKILL_Guardian": // 忠犬：攻击力下降，替主人挨打（替挡在 attackSeq 里判）
		return b.guardianAttack(pet, cmd.Target, skill)
	case "PETSKILL_EarthRound": // 地球一周：隐身一回合，下一回合加伤出手
		return b.earthRound(pet, cmd, skill)
	case "PETSKILL_NoGuard": // 不防守战法：不出手，回避与反击率变化（效果在 beginRound 登记）
		return b.noGuard(pet, skill)
	default:
		// 其余技能（偷窃、旅程伙伴等）的效果随需求逐步接入；先按通常攻击结算。
		return b.attackEvent(pet, cmd.Target, skill.Name)
	}
}

// attackEvent 是带技能名的一次普通攻击。
func (b *Battle) attackEvent(pet *Unit, target int, name string) Event {
	ev := b.attack(pet, b.Unit(target))
	ev.Kind = "skill"
	ev.Name = name
	return ev
}

// comboAttack 是连续攻击（PETSKILL_ContinuationAttack，原版 battle.c 的 gDamageDiv 与出手循环，battle.c:6439）：
// 一回合打 N 下，每一下的伤害平分（battle_event.c:2266 damage /= gDamageDiv，最少 1）。
// 被闪开、打空、挡住都不停手，打满 N 下才停（原版循环不看单次结果）；只有自己被反射打倒才提前停。
// 原目标倒下时用 BATTLE_TargetAdjust 改打对方另一个活着的单位。N 下合在一个事件里（协议 10.7）。
// 反击循环接的是最后一下（lastAtk），见 counter_chain.go。
func (b *Battle) comboAttack(pet *Unit, target int, name string, n int) Event {
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: name}
	t := b.adjustTarget(pet, b.Unit(target)) // 出手前先按 TargetAdjust 选目标
	if t == nil {
		return skipEvent(pet, "timeout")
	}
	for i := 1; ; i++ {
		dmg, result, hit := b.attackSeq(pet, t, false)
		if dmg > 0 {
			dmg = comboShare(dmg, n)
		}
		switch result {
		case "dodge", "miss", "block":
			b.effect(&ev, hit, result, 0)
		default:
			if b.physHurt(pet, hit, dmg, result, &ev) && dmg > 0 {
				b.wakeUp(hit, &ev)
			}
		}
		if i >= n || !pet.Alive() { // 打满 N 下，或者自己被反射打倒（原版 ContFlg = FALSE）
			break
		}
		if t = b.adjustTarget(pet, b.Unit(target)); t == nil { // 原目标倒了，改打对方另一个活着的
			break
		}
	}
	return ev
}

// comboShare 是连续攻击每一下的伤害：原版 damage /= gDamageDiv，截断后最少 1。
func comboShare(dmg, n int) int {
	if d := dmg / n; d >= 1 {
		return d
	}
	return 1
}

// statusChangeAttack 是异常攻击（PETSKILL_StatusChange，毒/石化/混乱/泥醉/催眠攻击）：
// 攻击力按“攻%”增减（原版 CHAR_WORKATTACKPOWER = 攻 + 攻 × 攻% / 100），
// 伤害大于 0 才判定上状态（原版 BATTLE_Attack 里 damage > 0 且 gBattleStausChange >= 0）；
// 被闪避、挡住或打空时不上状态。
func (b *Battle) statusChangeAttack(pet *Unit, target int, name string, sc gamedata.StatusChangeArg) Event {
	t := b.Unit(target)
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: name}
	if t == nil || !t.Alive() {
		return skipEvent(pet, "timeout")
	}
	atk := *pet // 只改这一次攻击的攻击力，宠物本身的数值不变
	atk.Attack = pet.Attack + int(float64(pet.Attack)*sc.AtkPct/100)
	dmg, result, hit := b.attackSeq(&atk, t, false) // hit 是实际挨打的单位（可能是替挡的忠犬）
	switch result {
	case "dodge", "miss", "block":
		b.effect(&ev, hit, result, 0)
		return ev
	}
	if b.physHurt(pet, hit, dmg, result, &ev) && dmg > 0 {
		b.wakeUp(hit, &ev)
		if b.statusSkillHit(pet, hit, sc) {
			addStatus(&ev, hit, sc.Status)
		}
	}
	return ev
}

// statusSkillHit 是异常攻击的上状态判定（BATTLE_StatusAttackCheck，偏移 suitpoison = 30）：
// 目标已带着任何异常状态就不中；成功后状态持续 turn + 1 回合（原版 gBattleStausTurn + 1）。
func (b *Battle) statusSkillHit(attacker, target *Unit, sc gamedata.StatusChangeArg) bool {
	if target.Status.anyBad() {
		return false
	}
	if b.roll(100) >= statusPer(attacker, target, poisonRate) {
		return false
	}
	target.Status.set(sc.Status, sc.Turn+1)
	return true
}
