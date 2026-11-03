package synthesis

import "sort"

// STAGE-206 第 3.2 节：同原子的多个来源汇总（原版 ITEM_simplify_atoms，item_gen.c:376）。

// atomCapNonFamily 是非守护兽的原子数值上限（原版 ITEM_ATOMIND_MAX）。合成一律带宠物，所以一律封顶。
// 守护兽的上限（ITEM_ATOMIND_FM_MAX、家族等级）不在本包范围内。
const atomCapNonFamily = 1000

// oddsTable 是原版 ITEM_simplify_atoms 的 oddstable：第 j 个来源（从 0 起）的累加系数。
var oddsTable = [...]float64{0.1, 0.25, 0.35, 0.4, 0.42, 0.44, 0.46, 0.47, 0.48, 0.49, 0.5, 0.51, 0.52, 0.53}

// atomSource 是一个原子在所有参与材料里的数值来源（原版 struct item_ingindtable）。
type atomSource struct {
	index  int       // 原子下标（AtomIndex）
	values []float64 // 各材料上该原子的数值，按材料顺序
}

// atomValue 是汇总后的一个原子：下标和数值（原版 retinds / retvals）。
type atomValue struct {
	index int
	value int
}

// simplifyAtoms 把每个原子的来源数值排序后逐个累加，最后取整（截断）并封顶。
// 累加公式：s[j] += s[j-1] × odds[j-1] × rate(tablenum(s[j-1])) / rate(tablenum 0)。
// 注意 tablenum 用的是截断成整数的 s[j-1]，而乘积用的是未截断的 s[j-1]，与原版一致。
// 输出顺序与输入一致，values 会被原地排序，调用方不要再用。
func simplifyAtoms(srcs []atomSource) []atomValue {
	out := make([]atomValue, 0, len(srcs))
	for _, s := range srcs {
		if len(s.values) == 0 {
			continue
		}
		vals := s.values
		sort.Float64s(vals)
		for j := 1; j < len(vals); j++ {
			ratio := rateOf(int(vals[j-1])) / rateTable[0].rate
			// 先把乘积转成 float64 再加，避免融合成 FMA，和 C 逐步计算一致。
			vals[j] += float64(vals[j-1] * oddsTable[j-1] * ratio)
		}
		v := int(vals[len(vals)-1])
		if v > atomCapNonFamily {
			v = atomCapNonFamily
		}
		out = append(out, atomValue{index: s.index, value: v})
	}
	return out
}
