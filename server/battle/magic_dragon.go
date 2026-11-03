// 召唤龙阵（STAGE-202，原版 battle/battle_magic.c 的 MAGIC_ToCallDragon_Battle 与 BATTLE_MultiToCallDragonMagic）：
// 奔龙阵（选敌方一个）、强恩阵、奔雷阵、GM 烟火（敌方全体）。对每个活着的敌方单位各打一下：先过魔法闪避
// （magicDodge，BATTLE_MagicDodge），没闪过的伤害用物理伤害公式（BATTLE_DamageCalc），攻击力是精灵的威力，
// 攻击方是一个临时造的虚拟单位（无属性、运气 1、没有武器和骑乘、没有暴击）。不反击，不附带异常状态，
// 不走防御结算（physHurt），所以光、镜子、守护（STAGE-199）对它无效。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// castDragon 执行召唤龙阵，效果写进 ev。全体（强恩阵、奔雷阵、GM 烟火）打敌方场上所有活着的单位；
// 单体（奔龙阵）打提交时选的那一个。强恩阵的目标类型是 enemy_all、不用选：原版的 23–26 循环限定在敌方的两排，
// 选哪一排都打到这一方的两排，实际上等同敌方全体，照原版保留。
func (b *Battle) castDragon(u *Unit, t gamedata.MagicTemplate, cmd Cmd, ev *Event) {
	for _, target := range b.dragonTargets(u, cmd, t) {
		if b.magicDodge(target) {
			b.effect(ev, target, "dodge", 0) // 闪过了：伤害为 0
			continue
		}
		b.hurt(target, b.dragonDamage(t.Power, target), "hit", ev)
	}
}

// dragonTargets 是召唤龙阵的目标：单体是提交时选的敌方一个（再确认一次还活着，提交时已用目标类型校验过），
// 全体是敌方场上所有活着的单位（单位号从小到大）。
func (b *Battle) dragonTargets(u *Unit, cmd Cmd, t gamedata.MagicTemplate) []*Unit {
	if !t.All {
		if x := b.Unit(cmd.Target); x != nil && x.Alive() && x.Side() != u.Side() {
			return []*Unit{x}
		}
		return nil
	}
	var out []*Unit
	for _, x := range b.units {
		if x != nil && x.Alive() && x.Side() != u.Side() {
			out = append(out, x)
		}
	}
	return out
}

// dragonDamage 是召唤龙阵打到一个没闪过的目标的伤害（BATTLE_DamageCalc）：虚拟的攻击方只设需要的字段——
// 攻击力是威力，运气 1（原版 CHAR_LUCK），Elem 为零（无属性），没有武器、骑乘和暴击。
// 防御、属性相克都由 damageCalc 按目标现在的状态算，和普通攻击同一套。
func (b *Battle) dragonDamage(power int, target *Unit) int {
	virt := &Unit{Kind: KindMonster, Attack: power, Luck: 1}
	return b.damageCalc(virt, target)
}

// magicDodge 是魔法闪避（BATTLE_MagicDodge）：掷 1–100，点数小于等于闪避率就闪过了，大于才没闪过
// （原版 `if (rand()%100+1 > (int)fLuck) return 0;`，所以刚好等于闪避率也算闪过）。
func (b *Battle) magicDodge(target *Unit) bool {
	return b.roll(100) <= magicDodgeRate(target)
}

// magicDodgeRate 是魔法闪避率（百分点，取整）：人物是运气 × 3；宠物和怪物是等级 × 0.2，最多 30。
// 原版人物那一项还加了属性抵抗（CHAR_EARTH_RESIST + nFieldAttr 的 0.15 倍），召唤龙阵传进来的 nFieldAttr
// 是 -1（属性参数被注释掉了），取到的是地抗性前面那个字段（头文件不在本仓库，没核实是什么），这里不加，只用运气（任务书的规定）。
func magicDodgeRate(target *Unit) int {
	if target.Kind == KindPlayer {
		return target.Luck * 3
	}
	return min(target.Level*2/10, 30) // 等级 × 0.2 向下取整（整数算，和原版 (int)(LV*0.2) 一致）
}
