package character

import (
	"testing"
	"time"
)

// STAGE-207：冷却秒数 = 5 + (材料数 − 2)，2 件 5 秒、3 件 6 秒，按整数秒比较（原版 time_t）。
func TestMergeCooldownSeconds(t *testing.T) {
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	cases := []struct {
		name  string
		last  time.Time
		now   time.Time
		n     int
		cools bool
	}{
		{"从来没合成过", time.Time{}, base, 2, false},
		{"2 件，过了 4 秒还在冷却", base, base.Add(4 * time.Second), 2, true},
		{"2 件，正好 5 秒可以合成", base, base.Add(5 * time.Second), 2, false},
		{"3 件，5 秒还在冷却", base, base.Add(5 * time.Second), 3, true},
		{"3 件，6 秒可以合成", base, base.Add(6 * time.Second), 3, false},
		{"15 件，冷却 18 秒", base, base.Add(17 * time.Second), 15, true},
		{"15 件，18 秒可以合成", base, base.Add(18 * time.Second), 15, false},
		{"亚秒不计：小数部分不算", base.Add(900 * time.Millisecond), base.Add(5 * time.Second), 2, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mergeCooldown(c.last, c.now, c.n); got != c.cools {
				t.Errorf("mergeCooldown = %v，应为 %v", got, c.cools)
			}
		})
	}
}
