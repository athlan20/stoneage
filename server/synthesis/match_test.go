package synthesis

import "testing"

// STAGE-206 第 4 节：配方匹配的数值规则与 k 的抽取表。期望值手算，边界用 IEEE 双精度核对过（见 docs/synthesis.md 第 4 节）。

// allCands 给每个 k 都放一个候选，产物编号 100+k，用来观察抽到了哪个 k。
func allCands() *[6][]int {
	var c [6][]int
	for k := 1; k <= 5; k++ {
		c[k] = []int{100 + k}
	}
	return &c
}

func TestFoodCapIs814(t *testing.T) {
	if foodCap != 814 {
		t.Errorf("料理材料数值上限应为 814（1059/1.3 取整），实际 %d", foodCap)
	}
}

// 抽取表：r 落在哪一段，决定 k。每个 ideal 都按文档 4 节的表检查。
func TestRetryChoosesKByThreshold(t *testing.T) {
	cases := []struct {
		ideal int
		r     int
		want  int
	}{
		{1, 0, 1}, {1, 999, 1},
		{2, 249, 1}, {2, 250, 2}, {2, 999, 2},
		{3, 149, 1}, {3, 150, 2}, {3, 399, 2}, {3, 400, 3}, {3, 999, 3},
		{4, 69, 1}, {4, 70, 2}, {4, 259, 2}, {4, 260, 3}, {4, 699, 3}, {4, 700, 4}, {4, 999, 4},
		{5, 39, 1}, {5, 40, 2}, {5, 199, 2}, {5, 200, 3}, {5, 499, 3}, {5, 500, 4}, {5, 739, 4}, {5, 740, 5}, {5, 999, 5},
	}
	for _, c := range cases {
		r := newSeqRand(t, c.r, 0)
		got, ok := retryMatch(allCands(), c.ideal, r.rand)
		if !ok || got != 100+c.want {
			t.Errorf("ideal=%d r=%d：应选 k=%d（产物 %d），实际 产物 %d ok=%t", c.ideal, c.r, c.want, 100+c.want, got, ok)
		}
		checkCalls(t, r, 1000, 1)
	}
}

// 同一档只试一次：ideal=2 时 r=300 指向 k=2，但 k=2 没有候选，接着 r=300 又是 k=2，已试过，重抽；r=100 指向 k=1，有候选。
func TestRetrySkipsTriedBucket(t *testing.T) {
	var cands [6][]int
	cands[1] = []int{101}
	r := newSeqRand(t, 300, 300, 100, 0)
	got, ok := retryMatch(&cands, 2, r.rand)
	if !ok || got != 101 {
		t.Errorf("应在重抽后选中 k=1 的产物 101，实际 %d ok=%t", got, ok)
	}
	checkCalls(t, r, 1000, 1000, 1000, 1)
}

// 两档都没有候选：ideal=2，r=300（k=2）、r=100（k=1）各试一次后失败，不再抽数。
func TestRetryFailsWhenNoCandidates(t *testing.T) {
	var cands [6][]int
	r := newSeqRand(t, 300, 100)
	if got, ok := retryMatch(&cands, 2, r.rand); ok {
		t.Errorf("没有候选时应失败，实际得到 %d", got)
	}
	checkCalls(t, r, 1000, 1000)
}

// 道具数值：v=100 时 c 的范围是 [60.75, 164.6]，即 61 到 164 合格。
func TestMatchValueItemBoundaries(t *testing.T) {
	cases := []struct {
		v, c int
		want bool
	}{
		{100, 61, true}, {100, 60, false},
		{100, 164, true}, {100, 165, false},
		// v=1000 在第 9 档（rate≈1.170），下界 1000/1.170 = 854.58，上界 min(1170, 1000) = 1000。
		{1000, 855, true}, {1000, 854, false},
		{1000, 1000, true}, {1000, 1001, false},
	}
	for _, c := range cases {
		if got := matchValue(c.v, c.c, false); got != c.want {
			t.Errorf("道具 v=%d c=%d：应为 %t，实际 %t", c.v, c.c, c.want, got)
		}
	}
}

// 料理数值：v=100 时 c 在 [70, 130]；v=900 先夹到 814，c 在 [569.8, 1058.2] 即 570 到 1058。
func TestMatchValueFoodBoundaries(t *testing.T) {
	cases := []struct {
		v, c int
		want bool
	}{
		{100, 70, true}, {100, 69, false},
		{100, 130, true}, {100, 131, false},
		{900, 570, true}, {900, 569, false},
		{900, 1058, true}, {900, 1059, false},
	}
	for _, c := range cases {
		if got := matchValue(c.v, c.c, true); got != c.want {
			t.Errorf("料理 v=%d c=%d：应为 %t，实际 %t", c.v, c.c, c.want, got)
		}
	}
}
