// 人物被打飞（STAGE-63，对照 gmsv/battle/battle_event.c 的 BATTLE_DamageSub 与 battle.c 的 BATTLE_UltimateExtra）。
// 打飞只记在人物单位上：倒下时由 character 服务按 charmOnDeath 多扣魅力；怪物的打飞标记不在这里做。

package battle

// flyCheck 判定人物这一下有没有被打飞（IsUltimate）：
// 单次伤害 ≥ 最大血量×1.2+20 直接打飞；否则把这一下超出血量的部分（over，只有打倒时 > 0）
// 加进累计值，累计达到同一阈值也打飞。打飞后累计值清零（原版两种情况都清零）。
func (b *Battle) flyCheck(u *Unit, dmg, over int) {
	limit := float64(u.MaxHP)*1.2 + 20
	if float64(dmg) >= limit {
		u.Flown, u.Overkill = true, 0
		return
	}
	if over <= 0 {
		return
	}
	u.Overkill += over
	if float64(u.Overkill) >= limit {
		u.Flown, u.Overkill = true, 0
	}
}
