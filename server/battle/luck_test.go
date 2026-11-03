package battle

import "testing"

// 运气决定逃跑的基础概率（BATTLE_EscapeCheck）：运气 5 档是 95%，1 档是 30%；敌我同等级时不再加减（STAGE-61）。
func TestEscapeChanceFollowsLuck(t *testing.T) {
	for _, tc := range []struct {
		luck int
		want bool
	}{{5, true}, {1, false}} {
		b := &Battle{escapeTries: map[int]int{}, Rnd: func(int) int { return 49 }} // roll 固定为 50
		u := &Unit{No: 0, Kind: KindPlayer, Level: 10, Luck: tc.luck}
		b.units[0] = u
		b.units[unitsPerSide] = &Unit{No: unitsPerSide, Kind: KindMonster, Level: 10}
		if got := b.escapeCheck(u); got != tc.want {
			t.Errorf("运气 %d 的逃跑成功 = %v，want %v", tc.luck, got, tc.want)
		}
	}
}
