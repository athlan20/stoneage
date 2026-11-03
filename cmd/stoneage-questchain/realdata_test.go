package main

import (
	"reflect"
	"testing"
)

// 真实数据的回归：不依赖数据库，直接用 gamedata 里的任务脚本和任务表。

// endSet 返回闭包里成立的完成事件号（排序后，便于比较）。
func endSet(c *Closure) map[int]bool {
	out := map[int]bool{}
	for f := range c.Has {
		if f.Kind == KindEnd {
			out[f.N] = true
		}
	}
	return out
}

// questDone 报告任务编号是否可能走完（完成事件号在闭包里）。
func questDone(m *Model, c *Closure, no int) bool {
	for _, q := range m.Quests {
		if q.No == no {
			return c.Has[Fact{Kind: KindEnd, N: q.EndEvent}]
		}
	}
	return false
}

func TestRealScenarioCountsAreMonotone(t *testing.T) {
	m := realModel()
	r := realRules()
	a := len(endSet(m.CloseA(r)))
	b := len(endSet(m.CloseB(r)))
	c := len(endSet(m.CloseC(r)))
	t.Logf("完成事件数：A=%d B=%d C=%d", a, b, c)
	if !(a <= b && b <= c) {
		t.Errorf("要求 A ≤ B ≤ C，得到 A=%d B=%d C=%d", a, b, c)
	}
}

func TestRealKnownChainsAreReachableInA(t *testing.T) {
	m := realModel()
	a := m.CloseA(realRules())
	// 成人仪式（任务 1）能走完；22 伐木、26 龙洞是 docs/quest.md 里写明“能”的。
	for _, no := range []int{1, 22, 26} {
		if !questDone(m, a, no) {
			t.Errorf("任务 %d 在场景 A 应能走完", no)
		}
	}
}

// 闭包的结果和等级需求的不动点应该一致（同一套可达规则）。
func TestRealLevelsAgreeWithClosure(t *testing.T) {
	m := realModel()
	r := realRules()
	for name, pair := range map[string]struct {
		c     *Closure
		allow Allow
	}{
		"A": {m.CloseA(r), r.AllowA()},
		"B": {m.CloseB(r), r.AllowB()},
		"C": {m.CloseC(r), r.AllowC()},
	} {
		lv := m.Levels(pair.allow)
		for f := range pair.c.Has {
			if _, ok := lv[f]; !ok {
				t.Errorf("场景 %s：闭包里成立的 %c%d，等级需求里没有", name, f.Kind, f.N)
			}
		}
		for f := range lv {
			if !pair.c.Has[f] && !m.external(f) {
				t.Errorf("场景 %s：等级需求里的 %c%d，闭包里不成立", name, f.Kind, f.N)
			}
		}
	}
}

// 最后一批之后的白名单加全部批次，应该和场景 B 的结果一样。
func TestRealBatchesEndAtScenarioB(t *testing.T) {
	m := realModel()
	r := realRules()
	batches := r.PlanBatches(m.Files)
	if len(batches) == 0 {
		t.Fatalf("应该有推荐批次")
	}
	last := m.CloseCumulative(r, batches, len(batches))
	if !reflect.DeepEqual(endSet(last), endSet(m.CloseB(r))) {
		t.Errorf("全部批次之后的完成事件，应该和场景 B 一样")
	}
}

// 输出稳定：同一份数据两次生成的报告逐字相同。
func TestRealReportIsStable(t *testing.T) {
	first := renderReport(realModel(), realRules())
	second := renderReport(realModel(), realRules())
	if first != second {
		t.Errorf("两次生成的报告不一样")
	}
}
