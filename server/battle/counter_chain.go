// 反击循环（STAGE-292）：普通攻击和攻击类宠物技能结算后，被打的一方与攻击方轮流反击，最多 5 次
// （原版 battle.c 的 `for k < 5 && ContFlg`，battle.c:6489 起）。单次反击的判定与伤害见 actions.go 的 counter。

package battle

// atkRec 记下这一次出手的攻击判定结果（attackSeq 写入），攻击结算后交给反击循环。
type atkRec struct {
	ok     bool   // 确实做过一次攻击判定（没有目标、目标已倒时为假）
	result string // attackSeq 的结果：hit、crit、dodge、miss、block
	target *Unit  // 攻击的原目标（忠犬替挡不改它）
	cont   bool   // 这一下结算后反击循环能否开始（默认 mainCont(result)；破防、落马术按原版 iRet 另行改写）
}

// canCounter 判断单位此刻能不能反击（BATTLE_Counter 的 COM1 检查）：不防守战法随时可以；
// 本回合已出过手的，看出手的是不是攻击；还没出手的，看交的指令是不是攻击（怪物看它提前定好的行动）。
func (b *Battle) canCounter(u *Unit) bool {
	if u.stance.NoGuard || u.comAtk {
		return true
	}
	if u.actedRound {
		return false
	}
	if u.Kind == KindMonster {
		p := b.peekPlan(u)
		return p.ok && p.act.kind == "attack"
	}
	cmd := b.cmdOf(u)
	return cmd != nil && cmd.Action == "attack"
}

// strikeCounter 是攻击出手结算后的反击循环入口。rec 是这一下攻击的结果（attackSeq 记下的）；
// own 为真表示出手的单位指令变成攻击，之后能被反过来反击（突击、地球一周的释放不能，原版 COM1=NONE）。
func (b *Battle) strikeCounter(u *Unit, own bool) []Event {
	rec := b.lastAtk
	b.lastAtk = atkRec{}
	if !rec.ok || rec.target == nil {
		return nil
	}
	if own {
		u.comAtk = true
	}
	return b.counterChain(u, rec.target, rec.cont)
}

// skillCounter 是宠物技能出手后的反击循环：普通攻击、连续攻击、异常攻击、一击必杀、背水之战、忠犬是攻击类出手；
// 突击、地球一周的释放也是攻击，但出手者之后不能反击。破除防御、落马术按原版 iRet 进循环（cont 由 skills_break.go 改写）；精灵、合击、回旋镖、偷窃、捐献、旅程伙伴不进循环。
func (b *Battle) skillCounter(u *Unit, fn string) []Event {
	switch fn {
	case "PETSKILL_NormalAttack", "PETSKILL_ContinuationAttack", "PETSKILL_StatusChange",
		"PETSKILL_Mighty", "PETSKILL_PowerBalance", "PETSKILL_Guardian", "PETSKILL_GuardBreak", "PETSKILL_FallGround":
		return b.strikeCounter(u, true)
	case "PETSKILL_ChargeAttack", "PETSKILL_EarthRound":
		return b.strikeCounter(u, false)
	}
	b.lastAtk = atkRec{}
	return nil
}

// counterChain 跑反击循环：k 为偶数时被打方反击攻击方，奇数时攻击方反击回去，最多 5 次。
// 某一下反击判定没过，或者结果不让连环继续（会心、挡下、打空、被打倒），循环到此为止。
// cont 是攻击本身的结果是否让循环开始。
func (b *Battle) counterChain(attacker, target *Unit, cont bool) []Event {
	var evs []Event
	for k := 0; k < 5 && cont; k++ {
		c, t := target, attacker
		if k%2 == 1 {
			c, t = attacker, target
		}
		ev, fired, next := b.counter(c, t)
		if !fired {
			break
		}
		evs = append(evs, ev)
		cont = next
	}
	return evs
}

// mainCont 决定普通攻击（或攻击类技能）结算后反击循环能否开始，依据原版 BATTLE_Attack 的 iRet：
// 普通命中、被闪开、打空都继续（原版 MISS 分支不改 iRet）；会心（iRet=FALSE）和被防御挡下（COM1==GUARD，
// iRet=FALSE）都结束。被打倒、被反射吸收在 counter 的判定里结束。
func mainCont(result string) bool {
	return result == "hit" || result == "dodge" || result == "miss"
}
