package synthesis

import (
	"math"
	"testing"
)

// STAGE-206 第 3.1 节：分档表与文档逐项对照。min 按原版 ITEM_initRandTable 的写法（上一档 maxnum + 1），
// 文档 3.1 的 min 列（0、1、2……）与原版不一致，文档已更正；min 只在守护兽分支使用，合成里不用。
var wantRateMax = [rateCount]int{
	24, 54, 107, 181, 275, 389, 523, 677, 851, 1059,
	1285, 1522, 1755, 2021, 2315, 2629, 2963, 3317, 3691, 4000,
}

var wantRateMin = [rateCount]int{
	0, 25, 55, 108, 182, 276, 390, 524, 678, 852,
	1060, 1286, 1523, 1756, 2022, 2316, 2630, 2964, 3318, 3692,
}

// 文档 3.1 的 rate 列（保留三位小数）。
var wantRate = [rateCount]float64{
	2.400, 1.800, 1.646, 1.448, 1.341, 1.275, 1.231, 1.198, 1.174, 1.170,
	1.142, 1.124, 1.101, 1.107, 1.100, 1.093, 1.087, 1.082, 1.078, 1.051,
}

func TestRateTableMatchesDoc(t *testing.T) {
	for i := range rateTable {
		e := rateTable[i]
		if e.num != rateNums[i] {
			t.Errorf("档 %d: num 应为 %d，实际 %d", i, rateNums[i], e.num)
		}
		if e.maxnum != wantRateMax[i] {
			t.Errorf("档 %d: max 应为 %d，实际 %d", i, wantRateMax[i], e.maxnum)
		}
		if e.minnum != wantRateMin[i] {
			t.Errorf("档 %d: min 应为 %d，实际 %d", i, wantRateMin[i], e.minnum)
		}
		if got := math.Round(e.rate*1000) / 1000; got != wantRate[i] {
			t.Errorf("档 %d: rate 应为 %.3f，实际 %.3f", i, wantRate[i], got)
		}
	}
}

func TestTableNumBoundaries(t *testing.T) {
	cases := []struct{ v, want int }{
		{-5, 0}, {0, 0}, {24, 0}, {25, 1}, {54, 1}, {55, 2},
		{1059, 9}, {1060, 10}, {4000, 19}, {4001, 19}, {99999, 19},
	}
	for _, c := range cases {
		if got := tableNum(c.v); got != c.want {
			t.Errorf("tableNum(%d) = %d，应为 %d", c.v, got, c.want)
		}
	}
}

func TestRateOf(t *testing.T) {
	if got := rateOf(10); got != float64(24)/10 {
		t.Errorf("rateOf(10) = %v，应为 2.4", got)
	}
	if got := rateOf(100000); got != float64(4000)/3805 {
		t.Errorf("rateOf(超大值) 应取最后一档 %v，实际 %v", float64(4000)/3805, got)
	}
}
