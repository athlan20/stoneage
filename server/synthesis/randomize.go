package synthesis

import "math"

// STAGE-206 第 3.3 节：随机化（原版 ITEM_randRange，item_gen.c:339，以及 ITEM_mergeItem 的随机化分支）。

// Rand 是注入的随机源：返回 [0, n) 的均匀整数，和 battle.Battle.Rnd 同一个约定；测试传固定序列。
type Rand func(n int) int

// randRangeDomain 对应原版 ITEM_RANDRANGEDOM（1000）：min_rate、max_rate 以千分之一为单位。
const randRangeDomain = 1000

// foodRandMin、foodRandMax 对应原版 ItemRandTable[1]（料理的非固定原子范围，0.9 倍到 1.1 倍）。
const (
	foodRandMin = 900
	foodRandMax = 1100
)

// randRange 复现 ITEM_randRange。minRate、maxRate 是原版的 int 参数：调用方算出的 double 要先截断成 int。
// 下界、上界是 base/1000 × rate 四舍五入到偶数（rint）。范围为 0 返回 base，范围为负返回 0，
// 两个 rate 都是 0 返回 0。注意 base 为负时（BASEADD 为负）范围只能是 0 或负，所以基本返回 0。
func randRange(base, minRate, maxRate int, rnd Rand) int {
	if minRate > maxRate {
		minRate, maxRate = maxRate, minRate
	}
	lo := int(math.RoundToEven(float64(base) / randRangeDomain * float64(minRate)))
	hi := int(math.RoundToEven(float64(base) / randRangeDomain * float64(maxRate)))
	if minRate == 0 && maxRate == 0 {
		return 0
	}
	span := hi - lo
	if span == 0 {
		return base
	}
	if span < 0 {
		return 0
	}
	// 原版 RAND(0, range) 是闭区间，对应 [0, span] 共 span+1 个数。
	return lo + rnd(span+1)
}

// fixedAtom 是宠物的一个固定原子，已按 buildFixedAtoms（petfix.go）整理过：BASEADD 和 FIXMIN、FIXMAX。
type fixedAtom struct {
	index   int // 原子下标
	baseAdd int
	fixMin  int
	fixMax  int
}

// randomizeValue 对一个汇总后的原子值做随机化，返回新值。fix 为宠物的固定原子（没有则 nil），
// dish 为真时是料理（searchtable=1），否则是道具（searchtable=0）。
// 注意：这里不判断“这个原子是否要随机化”，那是调用方的事（见 engine.go 对原版循环的说明）。
func randomizeValue(value int, fix *fixedAtom, dish bool, rnd Rand) int {
	if dish {
		return randomizeFood(value, fix, rnd)
	}
	return randomizeItem(value, fix, rnd)
}

// randomizeItem 是道具的随机化。
//   - 非固定原子：下界 0.87/rate、上界 1.05×rate（都是 int 的 min_rate、max_rate 再乘 v/1000）。
//   - 固定原子：基数是 v + BASEADD，下界 0.87/rate × FIXMIN，上界 1.05×rate × FIXMAX（rate 按 BASEADD 之前的 v 算）。
func randomizeItem(value int, fix *fixedAtom, rnd Rand) int {
	rate := rateOf(value)
	if fix == nil {
		minRate := int(1 / rate * 0.87 * randRangeDomain)
		maxRate := int(rate * 1.05 * randRangeDomain)
		return randRange(value, minRate, maxRate, rnd)
	}
	minRate := int(1 / rate * 0.87 * (float64(fix.fixMin) / randRangeDomain) * randRangeDomain)
	maxRate := int(rate * 1.05 * (float64(fix.fixMax) / randRangeDomain) * randRangeDomain)
	return randRange(value+fix.baseAdd, minRate, maxRate, rnd)
}

// randomizeFood 是料理的随机化。
//   - 非固定原子：下界 0.9、上界 1.1。
//   - 固定原子：基数是 v + BASEADD，上下界都用 FIXMIN（原版 item_gen.c 的料理分支把上界也写成了 fixedmin，
//     疑似笔误；这里照原版保留，FIXMAX 不参与料理的计算）。
func randomizeFood(value int, fix *fixedAtom, rnd Rand) int {
	if fix == nil {
		return randRange(value, foodRandMin, foodRandMax, rnd)
	}
	f := float64(fix.fixMin) / randRangeDomain
	minRate := int(foodRandMin * f)
	maxRate := int(foodRandMax * f)
	return randRange(value+fix.baseAdd, minRate, maxRate, rnd)
}
