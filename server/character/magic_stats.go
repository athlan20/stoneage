// STAGE-226：人物魔法熟练度与抗性数组和数据库 integer[4] 之间的转换（下标依次是地、水、火、风）。

package character

// int4s 把数据库读出的 integer[4] 转成定长数组；长度不对（不该出现）的部分补 0。
func int4s(v []int32) [4]int {
	var out [4]int
	for i := range out {
		if i < len(v) {
			out[i] = int(v[i])
		}
	}
	return out
}

// int32s4 把定长数组转成写库用的切片。
func int32s4(v [4]int) []int32 {
	out := make([]int32, len(v))
	for i, n := range v {
		out[i] = int32(n)
	}
	return out
}

// magicEquip 是抗魔装备加在抗性上的部分（地水火风，QU 不进抗性），战斗里成长时要从抗性里扣掉它（STAGE-227）。
func magicEquip(eq equipMods) [4]int {
	var out [4]int
	copy(out[:], eq.MagicDef[:4])
	return out
}

// battleResist 是人物防守时的魔法抗性：持久化的抗性加上穿在身上的抗魔装备（原版 battle_magic.c 的
// CHAR_EARTH_RESIST + CHAR_EQUITDEFMAGIC_E 合算）。QU 不进抗性，它是闪避的加成。
func battleResist(base [4]int, eq equipMods) [4]int {
	for i := range base {
		base[i] += eq.MagicDef[i]
	}
	return base
}
