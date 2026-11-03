// 魔法熟练度与抗性的成长（STAGE-227，原版 gmsv/battle/battle_magic.c 的 Magic_ComputeAttExp、Magic_ComputeDefExp）。
// 原版的抗性是角色上的持久化值，抗魔装备另算；这里 MagicResist 已含装备，成长只动去掉装备后的基础值（magicEq），
// 上下限的判断也按基础值，与原版一致。原版的升级提示（CHAR_talkToCli）协议里没有对应消息，这里不发。

package battle

// magicComputeAttExp 是攻击熟练度的成长（原版只在施法失败、施法者是人物时调用）：攻击元素的经验加 等级×3×命中数，
// 满 100 升一级（上限 100）；相克的下一个元素的经验减 等级×3×命中数×0.5，经验为负时降一级（只在它的熟练度大于 1 时）。
func magicComputeAttExp(u *Unit, f, lv, hits int) {
	add := lv * 3 * hits
	ex := u.MagicAttExp[f] + add
	if ex > 100 {
		ex = 0
		if u.MagicExp[f] < 100 {
			u.MagicExp[f]++
		}
	}
	if ex < 0 {
		ex = 0
	}
	u.MagicExp[f] = clampMagic(u.MagicExp[f], 100)
	u.MagicAttExp[f] = ex

	n := (f + 1) % 4 // 相克的魔法
	if u.MagicExp[n] > 1 {
		ex = int(float64(u.MagicAttExp[n]) - float64(add)*0.5) // 原版 magicex1 -= addEx*0.5，截断成整数
		if ex < 0 {
			ex = 0
			u.MagicExp[n] = max(u.MagicExp[n]-1, 0)
		}
		u.MagicAttExp[n] = ex
	}
}

// magicComputeDefExp 是防御（抗性）的成长（原版 Magic_ComputeDefExp）：伤害不到 200 不涨；否则抗性经验加
// (伤害/20)×(等级×2)，满 100 升一级（基础值上限 100）；相克的下一个元素的经验减 2，经验为负时降一级并把经验置 90。
// dmg 是 ×0.7 之后真正的伤害。
func magicComputeDefExp(u *Unit, f, lv, dmg int) {
	if dmg < 200 {
		return
	}
	add := (dmg / 20) * (lv * 2)
	base := clampMagic(u.MagicResist[f]-u.magicEq[f], 100)
	ex := u.MagicDefExp[f] + add
	if ex > 100 {
		ex = 0
		if base < 100 {
			base++
			u.MagicResist[f] = base + u.magicEq[f]
		}
	}
	if ex < 0 {
		ex = 0
	}
	u.MagicDefExp[f] = ex

	n := (f + 1) % 4 // 相克的魔法
	base = u.MagicResist[n] - u.magicEq[n]
	if base > 1 {
		ex = u.MagicDefExp[n] - 2
		if ex < 0 {
			ex = 90
			base = max(base-1, 0)
			u.MagicResist[n] = base + u.magicEq[n]
		}
		u.MagicDefExp[n] = ex
	}
}

// clampMagic 把熟练度或抗性限制在 0 到 hi 之间（原版 magiclv1 的上下限）。
func clampMagic(v, hi int) int {
	return min(max(v, 0), hi)
}

// PlayerMagic 是人物战后要写回角色的魔法数据（STAGE-227）：熟练度、两种经验，以及抗性的基础值（不含抗魔装备，
// 装备每次进战场都重新合算）。没有人物单位时返回零值。
func (b *Battle) PlayerMagic() (exp, resist, attExp, defExp [4]int) { return b.PlayerMagicFor(0) }

// PlayerMagicFor 是真人 no 战后要写回的魔法数据（真人同场时每个队员各自写回）。
func (b *Battle) PlayerMagicFor(no int) (exp, resist, attExp, defExp [4]int) {
	u := b.humanOf(no)
	if u == nil && b.duel != nil { // 决斗：逃跑、掉线的人精灵熟练度与抗性照常保留（pk.md 2.1）
		u = b.FinalHuman(no)
	}
	if u == nil {
		return
	}
	for i := range resist {
		resist[i] = u.MagicResist[i] - u.magicEq[i]
	}
	return u.MagicExp, resist, u.MagicAttExp, u.MagicDefExp
}
