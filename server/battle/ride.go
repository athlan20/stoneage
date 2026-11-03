// 骑乘（STAGE-124）：骑着宠物打架时，人和宠合成一个人。规则照搬原版 gmsv/battle/battle_event.c
// 的 BATTLE_getRidePet、BATTLE_adjustRidePet3A（_BATTLE_NEWPOWER 分支）、BATTLE_DamageSub 的
// 分摊段，经验见 battle.c 胜利结算的骑宠段。
//
// 骑着的宠物不放进格子（原版也不在战斗的单位表里）：它不出手、不被选为目标，只把数值并进人物，
// 挨打时由 rideSplit 分走一部分血量。宠物血量归零后仍骑着、伤害全由人承担（docs/ride.md §5）；
// 原版这时会落马（DamageSub 把 RIDEPET 设回 -1），那是 §7 本期不做的落马。

package battle

// rideExpRate 是骑宠拿到的经验倍数（battle.c：nowexp *= 0.6）。
const rideExpRate = 0.6

// ridden 表示人物这会儿在骑宠。宠物血量归零后仍算骑着（docs/ride.md §5：原版代码没有自动下骑，
// 伤害全由人承担，见 rideSplit），所以只看有没有骑。
func (u *Unit) ridden() bool { return u.mount != nil }

// throwing 表示手里是投掷武器（BATTLE_IsThrowWepon），投掷武器的骑乘数值与近战不同。
func (u *Unit) throwing() bool { return u.Weapon == weaponThrou }

// baseDefense 是单位自己的防御力（含背水之战的防%），不并入骑宠。
func (u *Unit) baseDefense() float64 {
	return float64(u.Defense + pctOf(u.Defense, float64(u.stance.DefensePct)))
}

// attackNow 是这一刻的攻击力：骑乘时人与宠合成（近战 人×0.8 + 宠×0.8，投掷 人 + 宠×0.4），
// 再加上本次攻击的临时增减（BATTLE_adjustRidePet3A 的 CHAR_WORKATTACKPOWER 分支）。
func (u *Unit) attackNow() float64 {
	act := float64(u.act.Attack)
	if !u.ridden() {
		return float64(u.Attack) + act
	}
	pet := float64(u.mount.Attack)
	if u.throwing() {
		return float64(u.Attack) + pet*0.4 + act
	}
	return float64(u.Attack)*0.8 + pet*0.8 + act
}

// defenseNow 是这一刻的防御力（进伤害公式前还要乘 0.70）。骑乘时是 (人 + 宠) × 0.7，
// 即原版 BATTLE_adjustRidePet3A 的防御分支。
func (u *Unit) defenseNow() float64 {
	if !u.ridden() {
		return u.baseDefense()
	}
	return (u.baseDefense() + u.mount.baseDefense()) * 0.7
}

// agilityAct 是出手时用的敏捷（行动顺序、攻击方的闪避、暴击、反击）：骑乘时人 × 0.2 + 宠 × 0.8，
// 投掷武器是人 × 0.8 + 宠 × 0.2（BATTLE_adjustRidePet3A 的 ATTACKSIDE 分支）。
func (u *Unit) agilityAct() float64 {
	if !u.ridden() {
		return float64(u.Agility)
	}
	human, pet := float64(u.Agility), float64(u.mount.Agility)
	if u.throwing() {
		return human*0.8 + pet*0.2
	}
	return human*0.2 + pet*0.8
}

// agilityHit 是被打时用的敏捷（作为防守方的闪避、暴击、反击）：骑乘时人 × 0.1 + 宠 × 0.9
// （BATTLE_adjustRidePet3A 的 DEFFENCESIDE 分支）。
func (u *Unit) agilityHit() float64 {
	if !u.ridden() {
		return float64(u.Agility)
	}
	return float64(u.Agility)*0.1 + float64(u.mount.Agility)*0.9
}

// rideSplit 把骑乘时受到的一下伤害分一部分给宠物（BATTLE_DamageSub 的正常挨打分支）：
// 人物分到 dmg × 宠防 / (人防 + 宠防) + 1，宠物分到 dmg − 人物那份 + 1。返回人物那一份，
// 宠物的血量在这里直接扣掉，归零时宠物倒下。宠物已经倒下（或不骑乘）时全部由人承担。
func (u *Unit) rideSplit(dmg int) int {
	if u.Kind != KindPlayer || !u.ridden() || !u.mount.Alive() {
		return dmg
	}
	pet := u.mount
	human := max(int(u.baseDefense()), 1)
	petDef := max(int(pet.baseDefense()), 1)
	part := dmg*petDef/(human+petDef) + 1
	pet.HP -= dmg - part + 1
	if pet.HP <= 0 {
		pet.HP = 0
		pet.Down = true
	}
	return part
}

// rideHeal 是 rideSplit 的回复对称版（STAGE-257，battle_magic.c 的 _MAGIC_REHPAI 骑乘分支）：总回复量按
// 缺口水位拆给人物和骑宠。人物那份 = 总量 × (人物缺口×100/缺口和) / 100，两步都是整数运算，与原版一致；
// 骑宠得到其余部分。人、宠各自封顶自己的最大血量，封顶后多出来的不回灌另一方。骑宠血量回到 1 以上时
// 清掉倒下标记，之后的伤害又分摊给它（救回后继续骑着，docs/ride.md §5）。返回人物实际加上的血量。
func (u *Unit) rideHeal(power int) int {
	pet := u.mount
	humanGap := max(u.MaxHP-u.HP, 0)
	petGap := max(pet.MaxHP-pet.HP, 0)
	allGap := max(humanGap+petGap, 1)
	humanPart := power * (humanGap * 100 / allGap) / 100
	petPart := power - humanPart

	before := u.HP
	u.HP = min(u.HP+humanPart, u.MaxHP)
	pet.HP = min(pet.HP+petPart, pet.MaxHP)
	if pet.HP > 0 {
		pet.Down = false
	}
	return u.HP - before
}

// rideExp 是骑宠这场战斗拿到的经验：按宠物自己的等级算（unitExp，和出战宠物同一条路），
// 再乘 0.6（battle.c 的骑宠段）。
func (b *Battle) rideExp(pet *Unit) int64 {
	return int64(float64(b.unitExp(pet)) * rideExpRate)
}
