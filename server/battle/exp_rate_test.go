// 全服经验倍数（STAGE-59）的单元测试：倍数的计算规则（舍去小数、至少 1）、
// 设置值的校验、表的加载与通知重读、金币不受倍数影响。

package battle

import "testing"

func TestApplyExpRate(t *testing.T) {
	cases := []struct {
		name string
		base int64
		rate float64
		want int64
	}{
		{"1 倍原样", 2, 1, 2},
		{"2 倍翻倍", 2, 2, 4},
		{"1.5 倍舍去小数", 3, 1.5, 4},
		{"小数舍去", 5, 1.9, 9},
		{"基础 1 乘 0.1 至少给 1", 1, 0.1, 1},
		{"基础 0 仍是 0", 0, 100, 0},
	}
	for _, c := range cases {
		if got := applyExpRate(c.base, c.rate); got != c.want {
			t.Errorf("%s：applyExpRate(%d, %g) = %d，应 %d", c.name, c.base, c.rate, got, c.want)
		}
	}
}

// rewards 乘上回调给的倍数；金币不吃经验倍数（STAGE-60 起按人物等级算）。
func TestRewardsExpRate(t *testing.T) {
	b := newFormulaBattle(func(int) int { return 0 })
	m := b.Unit(15)        // win 剧本里 15 号是怪
	m.Level, m.Rank = 3, 5 // 品质 5 的系数是 0：基础经验正好是表里的 3
	b.slain = []*Unit{m}
	b.Cbs.ExpRate = func() float64 { return 2 }
	r := b.rewards()
	if len(r.Exp) != 2 || r.Exp[0].Gain != 6 || r.Exp[1].Gain != 6 {
		t.Fatalf("2 倍经验 = %+v，人物和宠物都应 6", r.Exp)
	}
	// 金币按原版（STAGE-60）：人物等级 1 × BATTLEGOLD(10)，不吃经验倍数。
	if r.Gold != 10 {
		t.Fatalf("金币 = %d，应按人物等级 1 × BATTLEGOLD = 10", r.Gold)
	}
	// 回调给坏值（0）时按 1 倍。
	b.Cbs.ExpRate = func() float64 { return 0 }
	if r := b.rewards(); r.Exp[0].Gain != 3 {
		t.Fatalf("坏倍数的经验 = %d，应按 1 倍给 3", r.Exp[0].Gain)
	}
}
