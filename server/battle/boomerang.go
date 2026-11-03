// 回旋镖普攻（STAGE-167，原版 gmsv/battle/battle.c 的 BATTLE_COM_BOOMERANG）：
// 目标落在哪一排，就沿这一排横扫，最多 5 个不同目标，每个目标独立判定命中/闪避/暴击/代受，
// 伤害乘 0.3（原版 gBattleDamageModyfy = 0.3，battle_event.c 里乘在每次命中上）。
//
// 这一条以前是缺失的：Go 侧的普攻统一单击，回旋镖和斧头走同一条路。表现层不能把单击画成五次命中，
// 所以规则先补在这里，客户端只按事件里的 row 与 effects 顺序重放。

package battle

// boomerangDamageScale 是回旋镖每次命中的伤害倍率（原版 gBattleDamageModyfy = 0.3）。
const boomerangDamageScale = 0.3

// boomerang 是回旋镖的普攻：横扫目标所在的横排。
func (b *Battle) boomerang(attacker, target *Unit) Event {
	no := attacker.No
	ev := Event{Kind: "attack", Actor: &no, Weapon: motionBoomerang}
	// 目标已经先倒下的也照样扫它那一排：原版只看指令里记下的编号（defNo /= 5），不看这一刻是否还活着。
	row := -1
	if target != nil {
		row = rowOf(target.No)
		// 打自己所在的那一排不算出手（原版：attackNo/5 == defNo 时 BATTLE_NoAction）。
		if rowOf(no) == row {
			return skipEvent(attacker, "timeout")
		}
	}
	// 目标离场，或者整排都没有能打的目标时，按原版改选一次默认目标；还不行就不出手。
	if row < 0 || !b.rowHasTarget(attacker, row) {
		alt, ok := b.defaultTargetRow(attacker)
		if !ok {
			return skipEvent(attacker, "timeout")
		}
		row = alt
	}
	ev.Row = &row
	for _, defNo := range boomerangRow(row, attacker.Side()) {
		def := b.Unit(defNo)
		// 空位、已离场、隐身、同阵营都跳过：保留扫过缺口的连续运动，不补受击。
		if def == nil || !def.targetable() || def.Side() == attacker.Side() {
			continue
		}
		dmg, result, hit := b.attackSeq(attacker, def, false)
		b.strike(&ev, attacker, hit, def, scaledBoomerangDamage(dmg), result)
	}
	return ev
}

// rowHasTarget 判断这一排还有没有能打的目标。
func (b *Battle) rowHasTarget(attacker *Unit, row int) bool {
	for _, no := range boomerangRow(row, attacker.Side()) {
		if def := b.Unit(no); def != nil && def.targetable() && def.Side() != attacker.Side() {
			return true
		}
	}
	return false
}

// defaultTargetRow 是整排失效后的改选（原版 BATTLE_DefaultAttacker 的排号）：
// 按编号找对方第一个还能打的单位，用它的排；一个都没有时 ok 为 false。
func (b *Battle) defaultTargetRow(attacker *Unit) (int, bool) {
	for no := 0; no < unitsPerSide*2; no++ {
		def := b.Unit(no)
		if def != nil && def.targetable() && def.Side() != attacker.Side() {
			return rowOf(def.No), true
		}
	}
	return 0, false
}

// scaledBoomerangDamage 按原版系数折算一次命中的伤害（向下取整，至少 1 点）。
func scaledBoomerangDamage(dmg int) int {
	if dmg <= 0 {
		return dmg
	}
	out := int(float64(dmg) * boomerangDamageScale)
	if out < 1 {
		return 1
	}
	return out
}
