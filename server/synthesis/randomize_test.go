package synthesis

import "testing"

// STAGE-206 第 3.3 节：randRange 各分支与四种随机化情况的手算期望。
// 手算值先用独立脚本核对过（rint 为四舍五入到偶数，rate 与 int 截断按原版写法）。

// seqRand 按顺序返回给定的数，并记录每次调用的 n，检查返回值在 [0, n) 内。
type seqRand struct {
	t     *testing.T
	vals  []int
	calls []int
}

func newSeqRand(t *testing.T, vals ...int) *seqRand {
	return &seqRand{t: t, vals: vals}
}

func (s *seqRand) rand(n int) int {
	s.calls = append(s.calls, n)
	if len(s.vals) == 0 {
		s.t.Fatalf("随机数用完了（n=%d）", n)
	}
	v := s.vals[0]
	s.vals = s.vals[1:]
	if v < 0 || v >= n {
		s.t.Fatalf("返回值 %d 超出 [0,%d)", v, n)
	}
	return v
}

func checkCalls(t *testing.T, s *seqRand, want ...int) {
	t.Helper()
	if len(s.calls) != len(want) {
		t.Fatalf("随机数调用 %v，应为 %v", s.calls, want)
	}
	for i := range want {
		if s.calls[i] != want[i] {
			t.Fatalf("随机数调用 %v，应为 %v", s.calls, want)
		}
	}
}

func TestRandRangeSwapsRates(t *testing.T) {
	// 1000 的基数，rate 900、800 交换成 800、900：范围 [800, 900] 共 101 个数。
	r := newSeqRand(t, 5)
	if got := randRange(1000, 900, 800, r.rand); got != 805 {
		t.Errorf("randRange(1000,900,800) = %d，应为 805", got)
	}
	checkCalls(t, r, 101)
}

func TestRandRangeBothZero(t *testing.T) {
	r := newSeqRand(t)
	if got := randRange(1000, 0, 0, r.rand); got != 0 {
		t.Errorf("两个 rate 都是 0 应返回 0，实际 %d", got)
	}
	checkCalls(t, r)
}

func TestRandRangeEqualBoundsReturnBase(t *testing.T) {
	// 500 × 1000/1000 = 500，上下界都是 500，范围为 0，返回基数。
	r := newSeqRand(t)
	if got := randRange(500, 1000, 1000, r.rand); got != 500 {
		t.Errorf("上下界相等应返回基数 500，实际 %d", got)
	}
	// 基数 5：上下界 rint(2.5)=2 与 rint(2.5)=2 相等，返回基数 5。
	if got := randRange(5, 500, 500, r.rand); got != 5 {
		t.Errorf("四舍五入后相等应返回基数 5，实际 %d", got)
	}
	checkCalls(t, r)
}

func TestRandRangeNegativeSpanReturnsZero(t *testing.T) {
	// 基数为负：lo = rint(-50) = -50，hi = rint(-100) = -100，范围 -50 < 0，返回 0。
	r := newSeqRand(t)
	if got := randRange(-100, 500, 1000, r.rand); got != 0 {
		t.Errorf("负基数的范围为负时应返回 0，实际 %d", got)
	}
	checkCalls(t, r)
}

func TestRandRangeRoundsHalfToEven(t *testing.T) {
	// 基数 10，rate 250、350：lo = rint(2.5) = 2，hi = rint(3.5) = 4，范围 2 → 随机 [2,4]。
	r := newSeqRand(t, 0, 2)
	if got := randRange(10, 250, 350, r.rand); got != 2 {
		t.Errorf("随机 0 时应得 2，实际 %d", got)
	}
	if got := randRange(10, 250, 350, r.rand); got != 4 {
		t.Errorf("随机 2 时应得 4，实际 %d", got)
	}
	checkCalls(t, r, 3, 3)
}

// 情况一：非固定道具。v=100，档 2（107/65，rate≈1.646）。
// 下界 int(0.87/rate×1000)=528 → rint(52.8)=53；上界 int(rate×1.05×1000)=1728 → rint(172.8)=173。
func TestRandomizeItemNonFixed(t *testing.T) {
	r := newSeqRand(t, 0)
	if got := randomizeValue(100, nil, false, r.rand); got != 53 {
		t.Errorf("道具非固定 v=100 最小应为 53，实际 %d", got)
	}
	checkCalls(t, r, 121)
	r = newSeqRand(t, 120)
	if got := randomizeValue(100, nil, false, r.rand); got != 173 {
		t.Errorf("道具非固定 v=100 最大应为 173，实际 %d", got)
	}
}

// 情况二：固定道具。v=100，BASEADD=20 → 基数 120；FIXMIN=700、FIXMAX=900，rate 仍按 v=100 算。
// 下界 int(0.87/rate×0.7×1000)=369 → rint(120/1000×369)=rint(44.28)=44；
// 上界 int(1.05×rate×0.9×1000)=1555 → rint(186.6)=187。
func TestRandomizeItemFixed(t *testing.T) {
	fix := &fixedAtom{index: 0, baseAdd: 20, fixMin: 700, fixMax: 900}
	r := newSeqRand(t, 0)
	if got := randomizeValue(100, fix, false, r.rand); got != 44 {
		t.Errorf("道具固定 v=100 最小应为 44，实际 %d", got)
	}
	checkCalls(t, r, 144)
	r = newSeqRand(t, 143)
	if got := randomizeValue(100, fix, false, r.rand); got != 187 {
		t.Errorf("道具固定 v=100 最大应为 187，实际 %d", got)
	}
}

// 情况三：非固定料理。v=100，范围 [90, 110]。
func TestRandomizeFoodNonFixed(t *testing.T) {
	r := newSeqRand(t, 0)
	if got := randomizeValue(100, nil, true, r.rand); got != 90 {
		t.Errorf("料理非固定 v=100 最小应为 90，实际 %d", got)
	}
	checkCalls(t, r, 21)
	r = newSeqRand(t, 20)
	if got := randomizeValue(100, nil, true, r.rand); got != 110 {
		t.Errorf("料理非固定 v=100 最大应为 110，实际 %d", got)
	}
}

// 情况四：固定料理。v=100，BASEADD=-20 → 基数 80；上下界都用 FIXMIN=700（FIXMAX=900 不参与）：
// 下界 int(900×0.7)=630 → rint(50.4)=50；上界 int(1100×0.7)=770 → rint(61.6)=62。
func TestRandomizeFoodFixedUsesFixMinForBothEnds(t *testing.T) {
	fix := &fixedAtom{index: 0, baseAdd: -20, fixMin: 700, fixMax: 900}
	r := newSeqRand(t, 0)
	if got := randomizeValue(100, fix, true, r.rand); got != 50 {
		t.Errorf("料理固定 v=100 最小应为 50，实际 %d", got)
	}
	checkCalls(t, r, 13)
	r = newSeqRand(t, 12)
	if got := randomizeValue(100, fix, true, r.rand); got != 62 {
		t.Errorf("料理固定 v=100 最大应为 62，实际 %d", got)
	}
}

// BASEADD 让基数为负（v=10，BASEADD=-100，基数 -90）：道具固定的范围为负，返回 0。
func TestRandomizeItemFixedNegativeBaseReturnsZero(t *testing.T) {
	fix := &fixedAtom{index: 0, baseAdd: -100, fixMin: 700, fixMax: 900}
	r := newSeqRand(t)
	if got := randomizeValue(10, fix, false, r.rand); got != 0 {
		t.Errorf("基数为负时应返回 0，实际 %d", got)
	}
	checkCalls(t, r)
}
