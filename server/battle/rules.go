// 战斗公式：命中、闪避、暴击、伤害、防御与属性相克照搬原版
// gmsv/battle/battle_event.c：BATTLE_DuckCheck、BATTLE_CriticalCheck、BATTLE_DamageCalc、
// BATTLE_CriDamageCalc、BATTLE_GuardAdjust、BATTLE_AttrCalc
// （2.5 的 version.h 启用 _BATTLE_NEWPOWER，防御取 0.70 倍）。
// 武器的暴击加成与反击系数表（CounterTbl）自 STAGE-60 接入：人物按手里的武器，
// 怪物按 enemy.txt 样式配的底武，宠物空手。行动的结果（反击、逃跑、捕捉、换宠）在 actions.go。

package battle

import (
	"math"
)

// ptrTrue 供事件里的 *bool 字段引用。
var ptrTrue = true

// Callbacks 是引擎需要的外部决策，由 character 服务提供。
type Callbacks struct {
	// PlaceCaptured 在捕捉判定成功时被调用：给这只怪物安排一个宠物栏位，返回栏位号。
	// keep 是这次捕捉的资质筛选（nil 不筛）；被筛掉时返回 discarded 而不入栏位（STAGE-264）。
	// 没有空的随身栏位（宠物栏满）时返回 −1 且 discarded 为 nil，这次捕捉按失败处理、怪物不离场（STAGE-267）。
	PlaceCaptured func(m *Unit, keep *CaptureKeep) (slot int, discarded *Discarded)
	// PlaceCapturedFor 是真人同场时队员捕捉的安排（owner 为捕获者的人物单位号，1–4）；队长捕捉仍走 PlaceCaptured。
	PlaceCapturedFor func(owner int, m *Unit, keep *CaptureKeep) (slot int, discarded *Discarded)
	// ExpRate 返回结算那一刻的全服经验倍数（STAGE-59，game_settings 的 exp_rate），
	// 在 rewards 里调用；nil 或非正数按 1 倍。引擎本身不碰数据库。
	ExpRate func() float64
}

// 原版公式里的常量（battle_event.c 顶部的 #define 与全局变量）。
const (
	damageRate    = 2.0                       // DAMAGE_RATE：攻防差的伤害系数
	attrMax       = 100                       // ATTR_MAX
	dAttr         = 1.0 / (attrMax * attrMax) // D_ATTR
	kawashiMax    = 75                        // KAWASHI_MAX_RATE：闪避率上限（%）
	gKawashiPara  = 0.02                      // 闪避的敏捷差除数
	gCriticalPara = 0.09                      // 暴击的敏捷差除数
	gCounterPara  = 0.08                      // 反击的敏捷差除数
)

// roll 是原版的 RAND(1, n)：均匀返回 [1, n]。
func (b *Battle) roll(n int) int { return b.Rnd(n) + 1 }

// attack 执行一次普通攻击并生成事件（BATTLE_AttackSeq + BATTLE_Attack 的主线）。
func (b *Battle) attack(attacker, target *Unit) Event {
	no := attacker.No
	ev := Event{Kind: "attack", Actor: &no, Weapon: attacker.motionOf()}
	if target == nil || !target.Alive() {
		return skipEvent(attacker, "timeout")
	}
	dmg, result, hit := b.attackSeq(attacker, target, false)
	b.strike(&ev, attacker, hit, target, dmg, result)
	return ev
}

// strike 把一次攻击判定的结果记进事件，并处理命中后的附带效果（睡着的被打醒、普攻附毒）。
// 普通攻击和合击（combo.go）共用。
// intent 是出手时想打的目标（STAGE-167）：忠犬代受后 hit 是宠物、intent 还是原目标，
// 效果里带上 intent，客户端才知道动作该往谁身上走、伤害落在谁身上。
func (b *Battle) strike(ev *Event, attacker, hit, intent *Unit, dmg int, result string) {
	switch result {
	case "dodge":
		b.effect(ev, hit, "dodge", 0)
		noteIntent(ev, hit, intent)
	case "block": // 防御完全挡住（BATTLE_RET_ALLGUARD）
		b.effect(ev, hit, "block", 0)
		noteIntent(ev, hit, intent)
	case "miss":
		b.effect(ev, hit, "miss", 0)
		noteIntent(ev, hit, intent)
	default:
		landed := b.physHurt(attacker, hit, dmg, result, ev) // 无效、吸收、反射（STAGE-199）时没有落在 hit 上
		noteIntent(ev, hit, intent)
		if landed && dmg > 0 {
			b.wakeUp(hit, ev)                  // 睡着的目标被打醒
			if b.poisonAttack(attacker, hit) { // 普攻附毒（suitpoison）
				addStatus(ev, hit, "poison")
			}
		}
	}
}

// addStatus 给事件里目标为 no 的最后一条效果补上状态标记。
func addStatus(ev *Event, target *Unit, name string) {
	if len(ev.Effects) == 0 {
		return
	}
	eff := &ev.Effects[len(ev.Effects)-1]
	if eff.Target == target.No {
		eff.StatusAdd = append(eff.StatusAdd, name)
	}
}

// attackSeq 是一次攻击的判定（BATTLE_AttackSeq）：闪避 → 忠犬替挡 → 暴击 → 伤害 → 防御减伤 → 出手的倍率。
// 返回的 hit 是实际挨打的单位：闪避时是原目标，被忠犬替挡时是忠犬（原版 GuardianIndex）。
// counter 为 true 时是反击（伤害打 75 折，BATTLE_Counter）；原版反击（opt=-1）同样判闪避。
func (b *Battle) attackSeq(attacker, target *Unit, counter bool) (dmg int, result string, hit *Unit) {
	dmg, result, hit = b.attackJudge(attacker, target, counter)
	if !counter { // STAGE-292：普通攻击与攻击类技能的结果记下，攻击结算后交给反击循环（counter_chain.go）
		b.lastAtk = atkRec{ok: true, result: result, target: target, cont: mainCont(result)}
	}
	return dmg, result, hit
}

// attackJudge 是 attackSeq 的判定主体：闪避 → 忠犬替挡 → 暴击 → 伤害。
func (b *Battle) attackJudge(attacker, target *Unit, counter bool) (dmg int, result string, hit *Unit) {
	if attacker.Kind == KindPlayer && !attacker.Auto { // 主人攻击自己的宠物：宠物忠诚下降（AI_FIX_SEKKAN，loyalty.go）
		b.addLoyalty(target, loyalOwnerHit)
	}
	if b.duckCheck(attacker, target) {
		return 0, "dodge", target
	}
	return b.landSeq(attacker, target, counter)
}

// landSeq 是闪避之后的部分：忠犬替挡 → 暴击 → 伤害 → 防御减伤 → 出手的倍率。
// 合击（BATTLE_COM_COMBO）不判闪避，直接从这里开始。
func (b *Battle) landSeq(attacker, target *Unit, counter bool) (dmg int, result string, hit *Unit) {
	hit = target
	// 反击不触发忠犬替挡（原版 BATTLE_Counter 传 Guardian = -2）；反击也不带出手者的技能倍率（原版反击前清零 gBattleDamageModyfy）
	if g := b.guardianOf(attacker, target); g != nil && !counter {
		hit = g
	}
	dmg, result = b.hitSeq(attacker, hit, counter)
	if hit != target && (result == "miss" || result == "block") {
		// 替挡的忠犬至少挨 1 点（原版：GuardianIndex != -1 时 MISS 改成 NORMAL、伤害记 1）
		dmg, result = 1, "hit"
	}
	if (result == "hit" || result == "crit") && !counter {
		dmg = scaleDamage(dmg, attacker.act.Damage)
	}
	return dmg, result, hit
}

// hitSeq 是闪避之后的暴击、伤害与防御减伤（BATTLE_AttackSeq 的后半段）。
func (b *Battle) hitSeq(attacker, target *Unit, counter bool) (int, string) {
	var dmg int
	if b.roll(10000) < criticalCheck(attacker, target) {
		dmg = criDamageCalc(attacker, target, b.damageCalc(attacker, target))
		if counter {
			dmg = int(float64(dmg) * 0.75)
		}
		if dmg < 1 {
			dmg = b.Rnd(2)
		}
		return dmg, "crit"
	}
	dmg = b.damageCalc(attacker, target)
	if target.Status.Guard {
		dmg = guardAdjust(dmg, b.Rnd(100)+1)
		if counter {
			dmg = int(float64(dmg) * 0.75)
		}
		if dmg < 1 {
			dmg = b.Rnd(2)
		}
		if dmg == 0 {
			return 0, "block"
		}
		return dmg, "hit"
	}
	if counter {
		dmg = int(float64(dmg) * 0.75)
	}
	if dmg < 1 {
		dmg = b.Rnd(2)
	}
	if dmg == 0 {
		return 0, "miss"
	}
	return dmg, "hit"
}

// scaleDamage 把伤害乘上出手的倍率（原版 gBattleDamageModyfy，在防御减伤之后乘，结果截断）；倍率为 0 表示不变。
func scaleDamage(dmg int, mul float64) int {
	if mul == 0 || dmg <= 0 {
		return dmg
	}
	return int(float64(dmg) * mul)
}

// damageCalc 是基础伤害（BATTLE_DamageCalc，_BATTLE_NEWPOWER 分支）。
func (b *Battle) damageCalc(attacker, target *Unit) int {
	attack := attacker.attackNow()
	defense := target.defenseNow() * 0.70
	var dmg float64
	switch {
	case defense <= attack && attack < defense*8.0/7.0:
		dmg = float64(b.Rnd(10000)) / 10000 * attack / 16 // RAND(0, attack×D_16)
	case defense > attack:
		dmg = float64(b.Rnd(2)) // RAND(0, 1)
	default: // attack >= defense×8/7
		k0 := float64(b.Rnd(10000))/10000*attack/8 - attack/16
		dmg = (attack-defense)*damageRate + k0
	}
	dmg = math.Max(dmg, 0)
	return b.attrAdjust(attacker, target, int(dmg))
}

// criDamageCalc 是暴击伤害（BATTLE_CriDamageCalc）：基础伤害加防御 × 等级比的 0.5。
// 这里的防御是原版的 CHAR_WORKDEFENCEPOWER，骑乘时也不并入宠物。
func criDamageCalc(attacker, target *Unit, base int) int {
	extra := target.baseDefense() * float64(attacker.Level) / float64(target.Level) * 0.5
	return base + int(extra)
}

// guardAdjust 是防御的减伤（BATTLE_GuardAdjust）：按随机档位折减，1/4 概率完全挡住。
func guardAdjust(dmg, roll int) int {
	switch {
	case roll <= 25:
		return 0
	case roll <= 50:
		return int(float64(dmg) * 0.10)
	case roll <= 70:
		return int(float64(dmg) * 0.20)
	case roll <= 85:
		return int(float64(dmg) * 0.30)
	case roll <= 95:
		return int(float64(dmg) * 0.40)
	default:
		return int(float64(dmg) * 0.50)
	}
}

// duckCheck 是闪避判定（BATTLE_DuckCheck）。防御中的目标不闪避（原版直接返回 FALSE），带防御次数的也不闪避。
func (b *Battle) duckCheck(attacker, target *Unit) bool {
	if target.Status.Guard {
		return false
	}
	if target.Def.any() { // 带着防御次数（光、镜子、守护）的单位不闪避（STAGE-199，BATTLE_DuckCheck 的 GetDamageReact 分支）
		return false
	}
	at, df := dexPair(attacker, target, duckMods)
	big, small, wari := spread(at, df)
	per := math.Sqrt((big-small)/gKawashiPara) * wari
	if target.Kind == KindPlayer {
		per += float64(target.Luck)
	}
	per += float64(attacker.act.Duck)         // 一击必杀：对方的闪避率加上回避值（gBattleDuckModyfy）
	per += float64(target.stance.NoGuardDuck) // 不防守战法：自己的回避率（BATTLE_DuckCheck 的 NOGUARD 分支）
	if attacker.Status.Drunk > 0 {            // 攻击方醉了，攻击更容易被躲开（原版 RAND(20,30)，同样在乘 100 之前加）
		per += float64(20 + b.Rnd(11))
	}
	per *= 100
	per = math.Min(per, kawashiMax*100)
	per = math.Max(per, 1)
	return float64(b.roll(10000)) <= per
}

// criticalCheck 是会心一击的概率（BATTLE_CriticalCheckPlayer，宠物和怪物同一条公式），
// 返回 0–10000。武器带暴击加成（At_Soubi × 0.5，STAGE-60）。
func criticalCheck(attacker, target *Unit) int {
	at, df, divpara, root := dexPairCritical(attacker, target)
	big, small, wari := spread(at, df)
	work := (big - small) / divpara
	if work < 0 {
		work = 0
	}
	per := work
	if root {
		per = math.Sqrt(work)
	}
	per += float64(attacker.WeaponCritical) * 0.5 // 武器的暴击修正（At_Soubi × 0.5）
	per *= wari
	if attacker.Kind == KindPlayer {
		per += float64(attacker.Luck)
	}
	per *= 100
	per = math.Min(per, 10000)
	per = math.Max(per, 1)
	return int(per)
}

// spread 取两者的大、小与比值（各判定公式的公共部分）。
func spread(at, df float64) (big, small, wari float64) {
	if at >= df {
		return at, df, 1.0
	}
	if df <= 0 {
		return df, at, 0.0
	}
	return df, at, at / df
}

// duckMods 是闪避判定的敏捷修正（BATTLE_DuckCheck 的 else-if 链）。
func duckMods(attacker, target *Unit) (atMod, dfMod float64) {
	switch {
	case attacker.Kind == KindMonster && target.Kind == KindPet:
		return 0.8, 1.0
	case attacker.Kind != KindMonster && target.Kind == KindPet:
		return 1.0, 0.8
	case attacker.Kind != KindPlayer && target.Kind == KindPlayer:
		return 0.6, 1.0
	case attacker.Kind == KindPlayer && target.Kind != KindPlayer:
		return 1.0, 0.6
	}
	return 1.0, 1.0
}

// dexPair 按修正系数取攻守双方的敏捷：攻击方用出手的敏捷，防守方用被打的敏捷（骑乘时两者不同，ride.go）。
func dexPair(attacker, target *Unit, mods func(a, t *Unit) (float64, float64)) (at, df float64) {
	atMod, dfMod := mods(attacker, target)
	return attacker.agilityAct() * atMod, target.agilityHit() * dfMod
}

// dexPairCritical 是暴击判定的敏捷与参数（BATTLE_CriticalCheckPlayer 的 else-if 链，
// 两类对战把除数换成 10、不开根号）。
func dexPairCritical(attacker, target *Unit) (at, df, divpara float64, root bool) {
	divpara, root = gCriticalPara, true
	atMod, dfMod := 1.0, 1.0
	switch {
	case attacker.Kind == KindPet && target.Kind == KindMonster:
		dfMod = 0.8
	case attacker.Kind == KindMonster && target.Kind == KindPet:
		divpara, root = 10.0, false
	case attacker.Kind != KindPlayer && target.Kind == KindPlayer:
		divpara, root = 10.0, false
	case attacker.Kind == KindPlayer && target.Kind != KindPlayer:
		dfMod = 0.6
	}
	return attacker.agilityAct() * atMod, target.agilityHit() * dfMod, divpara, root
}

// elemVector 是 BATTLE_GetAttr 的四属性加无属性（下标 0–3 地水火风，4 为无）：
// 各属性先截到 0，无属性 = 100 − 四属性之和（不低于 0），所以没有属性的单位无属性为 100。
func elemVector(e Elements) [5]float64 {
	v := [4]int{e.Earth, e.Water, e.Fire, e.Wind}
	sum := 0
	for i := range v {
		if v[i] < 0 {
			v[i] = 0
		}
		sum += v[i]
	}
	none := attrMax - sum
	if none < 0 {
		none = 0
	}
	return [5]float64{float64(v[0]), float64(v[1]), float64(v[2]), float64(v[3]), float64(none)}
}

// attrAdjust 是四属性相克（BATTLE_AttrAdjust + BATTLE_AttrCalc）：地水火风相克
// （水克火、火克风、风克地、地克水）系数 1.5，被克 0.6，其余 1.0；无属性与各属性之间
// 同样按表相克（STAGE-60 起按 BATTLE_GetAttr 的无属性 = 100 − 四属性和计算）。
func (b *Battle) attrAdjust(attacker, target *Unit, dmg int) int {
	return b.attrAdjustWith(attacker.Elem, attacker, target, dmg)
}

// attrAdjustWith 是 attrAdjust 的本体。攻击用的属性 atk 可以不是单位自己的（精灵用它的属性算相克），
// 场地加成仍按施放者自己的四属性（原版 BATTLE_AttrAdjust 也是这样），见 magic_field.go。
func (b *Battle) attrAdjustWith(atk Elements, attacker, target *Unit, dmg int) int {
	return b.attrAdjustVec(elemVector(atk), b.fieldPower(attacker), target, dmg)
}

// attrAdjustVec 是相克加场地的本体：my 是攻方地水火风无五个元素（未乘威力），atkField 是攻方的场地加成。
// 守方的五个元素取自 target。原版 BATTLE_AttrAdjust 与攻击精灵（BATTLE_getMagicAdjustInt）共用这一段。
func (b *Battle) attrAdjustVec(my [5]float64, atkField float64, target *Unit, dmg int) int {
	base := attrCalcVec(my, elemVector(target.Elem), dmg)
	// 场地（STAGE-200）：damage *= At_FieldPow / Df_FieldPow，截断在乘之后；无场地时比值为 1。
	return int(float64(base) * (atkField / b.fieldPower(target)))
}

// attrMod[i][j] 是我方第 i 属性对对方第 j 属性的系数（顺序地水火风无），即 BATTLE_AttrCalc 的 AJ_SAME/AJ_UP/AJ_DOWN。
var attrMod = [5][5]float64{
	{1.0, 1.5, 1.0, 0.6, 1.5}, // 地：克水、被风克
	{0.6, 1.0, 1.5, 1.0, 1.5}, // 水：克火、被地克
	{1.0, 0.6, 1.0, 1.5, 1.5}, // 火：克风、被水克
	{1.5, 1.0, 0.6, 1.0, 1.5}, // 风：克地、被火克
	{0.6, 0.6, 0.6, 0.6, 1.0}, // 无
}

// attrCalcVec 是 BATTLE_AttrCalc：攻方每个元素先乘威力，再按相克表对守方五个元素求和。
// 原版每一行（攻方的一个元素）的结果先截成整数再相加，最后乘 D_ATTR 截断，这里照做。
func attrCalcVec(my, vs [5]float64, dmg int) int {
	sum := 0
	for i := range my {
		row := 0.0
		for j := range vs {
			row += my[i] * float64(dmg) * vs[j] * attrMod[i][j]
		}
		sum += int(row)
	}
	return int(float64(sum) * dAttr)
}

// noteIntent 把「本来要打谁」记到最后一条效果上（STAGE-167）：忠犬代受、守护让实际承伤者变了时，
// 客户端要靠它把出手方向和动作落到原目标上，而不是跟着伤害跑。
func noteIntent(ev *Event, hit, intent *Unit) {
	if intent == nil || hit == nil || intent.No == hit.No || len(ev.Effects) == 0 {
		return
	}
	eff := &ev.Effects[len(ev.Effects)-1]
	if eff.Target == hit.No {
		no := intent.No
		eff.Intent = &no
	}
}
