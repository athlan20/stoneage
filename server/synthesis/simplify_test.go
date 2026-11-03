package synthesis

import "testing"

// STAGE-206 第 3.2 节：同原子汇总的手算例子。期望值先用独立脚本按同一公式算过，再写进来。

func simplifyOne(vals ...float64) int {
	out := simplifyAtoms([]atomSource{{index: 7, values: vals}})
	return out[0].value
}

func TestSimplifySingleSourceKeepsValue(t *testing.T) {
	if got := simplifyOne(700); got != 700 {
		t.Errorf("单个来源 700 应原样保留，实际 %d", got)
	}
}

func TestSimplifySingleSourceCapped(t *testing.T) {
	if got := simplifyOne(1500); got != 1000 {
		t.Errorf("非守护兽的单个来源 1500 应封顶为 1000，实际 %d", got)
	}
}

// 两件材料同原子：100 与 200 排序后 s = [100, 200]。
// j=1：tablenum(100)=2（107 档），rate 比 = (107/65)/2.4 = 0.685897…，加 100×0.1×0.685897 = 6.859，得 206.859，取整 206。
func TestSimplifyTwoSources(t *testing.T) {
	if got := simplifyOne(200, 100); got != 206 {
		t.Errorf("[100,200] 应汇总为 206，实际 %d", got)
	}
}

// 三件材料同原子：50、60、300，排序后 s = [50, 60, 300]。
// j=1：tablenum(50)=1（54 档），比 = (54/30)/2.4 = 0.75，加 50×0.1×0.75 = 3.75，s[1] = 63.75。
// j=2：tablenum(int(63.75)=63)=2，比 = 0.685897，加 63.75×0.25×0.685897 = 10.931，s[2] = 310.931，取整 310。
func TestSimplifyThreeSources(t *testing.T) {
	if got := simplifyOne(300, 50, 60); got != 310 {
		t.Errorf("[50,60,300] 应汇总为 310，实际 %d", got)
	}
}

// 三个相同来源：300、300、300。
// j=1：tablenum(300)=5（389 档），比 = (389/305)/2.4 = 0.531…，加 300×0.1×0.531 = 15.94，s[1] = 315.94。
// j=2：tablenum(315)=5，加 315.94×0.25×0.531 = 41.97，s[2] = 341.97，取整 341。
func TestSimplifyEqualSources(t *testing.T) {
	if got := simplifyOne(300, 300, 300); got != 341 {
		t.Errorf("[300,300,300] 应汇总为 341，实际 %d", got)
	}
}

// 累加后超过 1000 也要封顶：900、1000 → 1043.88，封顶为 1000。
func TestSimplifyCapAfterAccumulation(t *testing.T) {
	if got := simplifyOne(1000, 900); got != 1000 {
		t.Errorf("[900,1000] 累加后应封顶为 1000，实际 %d", got)
	}
}

// 小值档：10、20、30 都在第 0 档（rate 比为 1），s = [10, 21, 35.25]，取整 35。
func TestSimplifySmallValues(t *testing.T) {
	if got := simplifyOne(30, 10, 20); got != 35 {
		t.Errorf("[10,20,30] 应汇总为 35，实际 %d", got)
	}
}

func TestSimplifyKeepsAtomOrder(t *testing.T) {
	out := simplifyAtoms([]atomSource{
		{index: 3, values: []float64{5}},
		{index: 1, values: []float64{9, 8}},
		{index: 2, values: nil},
	})
	if len(out) != 2 || out[0] != (atomValue{3, 5}) || out[1].index != 1 {
		t.Errorf("输出应按输入顺序、跳过空来源，实际 %v", out)
	}
}
