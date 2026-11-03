// 攻击精灵的伤害（STAGE-227，原版 gmsv/battle/battle_magic.c 的 BATTLE_MultiAttMagic 的 _FIX_MAGICDAMAGE 分支，
// version.h 已开启）：威力按施法者的攻击熟练度与守方的抗性算 A 值，再把施法者的属性换成“精灵属性 × 等级”做四属相克；
// 施法失败伤害 ×0.7。这一套没有非 FIX 公式里的 ±10 随机。随机数全部走 b.Rnd，测试可注入。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// magicElemIndex 是精灵属性的下标（原版 BATTLE_GetAttr 与 CHAR_EARTH_EXP 的次序：地 0、水 1、火 2、风 3）。
// 攻击精灵的属性只会是这四个；其他写法按地处理（不会出现，数据里攻击系都是地水火风）。
func magicElemIndex(name string) int {
	switch name {
	case "水":
		return 1
	case "火":
		return 2
	case "风":
		return 3
	}
	return 0
}

// magicCastOK 是这次施放成不成功（原版 BATTLE_MultiAttMagic 的 Check = rand()%100，Check 大于攻击熟练度就失败）。
// 每次施放只掷一次，对所有目标都一样，所以在目标循环之前掷。
func (b *Battle) magicCastOK(caster *Unit, f int) bool {
	return b.Rnd(100) <= caster.MagicExp[f]
}

// magicPower 是 FIX 分支的威力 APower：Kmagic = 攻熟×1.4 − 守抗（不低于 0），Mmagic = 攻熟（不低于 1），
// Amagic = Kmagic² / Mmagic² + rand%20/100，APower = 威力 ×（1 + 等级/10）× Amagic，截断成整数。
// 原版用 float 算，这里用 float32，按同样的顺序算，截断才一致。
func (b *Battle) magicPower(caster, target *Unit, t gamedata.MagicTemplate, f int) int {
	att := caster.MagicExp[f]
	k := float32(float64(att)*1.4 - float64(resistOf(target, f)))
	if k < 0 {
		k = 0
	}
	m := float32(att)
	if m < 1 {
		m = 1
	}
	a := k*k/(m*m) + float32(b.Rnd(20))/100
	return int(float32(t.Power) * (1 + float32(t.Lv)/10) * a)
}

// magicAdjust 是 BATTLE_getMagicAdjustInt：施法者的属性换成“精灵属性 × 等级”，只留精灵属性那一格的值
// （精灵属性 = 等级×10 + 等级×10×(施法者该属性/50)，整数除法）；无属性分量保留施法者自己的值（原版只清了三个元素，
// 无属性那一格没有清，照抄）。然后乘上威力做四属相克，场地加成按换过之后的属性算（原版 BATTLE_FieldAttAdjust 的参数是换过的值）。
func (b *Battle) magicAdjust(caster, target *Unit, f, lv, apower int) int {
	self := elemVector(caster.Elem)
	mlv := lv * 10
	var my [5]float64
	my[f] = float64(mlv + mlv*(int(self[f])/50))
	my[4] = self[4]
	return b.attrAdjustVec(my, b.fieldPowerOf(my), target, apower)
}

// magicHit 是攻击精灵对一个没闪过的目标的结算：算威力、相克得到伤害；施法失败 ×0.7；
// 守方是人物时按伤害记抗性成长（原版在扣血之前调用 Magic_ComputeDefExp，伤害是 ×0.7 之后的）；然后扣血。
// 宠物和怪物的抗性与熟练度不成长（STAGE-227 裁定：宠物按 0，怪物的抗性按等级现算）。
func (b *Battle) magicHit(caster, target *Unit, t gamedata.MagicTemplate, ok bool, ev *Event) {
	f := magicElemIndex(t.Elem)
	apower := b.magicPower(caster, target, t, f)
	dmg := b.magicAdjust(caster, target, f, t.Lv, apower)
	if !ok {
		dmg = int(float64(dmg) * 0.7)
	}
	if target.Kind == KindPlayer {
		magicComputeDefExp(target, f, t.Lv, dmg)
	}
	b.hurt(target, dmg, "hit", ev)
}
