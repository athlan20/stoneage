package duel

import "testing"

func TestShare(t *testing.T) {
	cases := map[int]int{-5: 0, 0: 0, 9: 0, 10: 1, 100: 10, 105: 10, 100000000: 10000000}
	for dp, want := range cases {
		if got := Share(dp); got != want {
			t.Errorf("Share(%d) = %d，want %d", dp, got, want)
		}
	}
}

func TestSplitGain(t *testing.T) {
	cases := []struct{ pool, n, want int }{
		{10, 1, 10}, {10, 2, 5}, {10, 3, 3}, {3, 4, 1}, {0, 2, 1}, {5, 0, 0},
	}
	for _, c := range cases {
		if got := SplitGain(c.pool, c.n); got != c.want {
			t.Errorf("SplitGain(%d, %d) = %d，want %d", c.pool, c.n, got, c.want)
		}
	}
}

func TestSettle(t *testing.T) {
	cases := []struct{ dp, cum, want int }{
		{100, 0, 100}, {100, -10, 90}, {5, -10, 0}, {MaxDP, 10, MaxDP}, {MinDP, -1, MinDP},
	}
	for _, c := range cases {
		if got := Settle(c.dp, c.cum); got != c.want {
			t.Errorf("Settle(%d, %d) = %d，want %d", c.dp, c.cum, got, c.want)
		}
	}
}

func TestEscapeImmune(t *testing.T) {
	for _, first := range []bool{true, false} {
		for _, attack := range []bool{true, false} {
			for _, knocked := range []bool{true, false} {
				want := first && !attack && !knocked
				if got := EscapeImmune(first, attack, knocked); got != want {
					t.Errorf("EscapeImmune(%v,%v,%v) = %v，want %v", first, attack, knocked, got, want)
				}
			}
		}
	}
}

// 对手（敌方人物 10）击倒人物 0：0 扣开战 DP 的一成，10 得满额。
func TestKnockoutByEnemy(t *testing.T) {
	l := NewLedger()
	l.Join(0, 100)
	l.Join(10, 100)
	l.Knockout(0, []int{10})
	if d := l.Delta(0); d != -10 {
		t.Errorf("被击倒者累计 = %d，want -10", d)
	}
	if d := l.Delta(10); d != 10 {
		t.Errorf("击倒者累计 = %d，want 10", d)
	}
	if got := l.Final(0); got != 90 {
		t.Errorf("Final(0) = %d，want 90", got)
	}
	if got := l.Final(10); got != 110 {
		t.Errorf("Final(10) = %d，want 110", got)
	}
}

// 两名出手者平分；同一主人重复出现只算一次。
func TestKnockoutSplitAndDedup(t *testing.T) {
	l := NewLedger()
	l.Join(0, 100)
	l.Join(10, 100)
	l.Join(11, 100)
	l.Knockout(0, []int{10, 10, 11})
	if d := l.Delta(10); d != 5 {
		t.Errorf("10 的累计 = %d，want 5", d)
	}
	if d := l.Delta(11); d != 5 {
		t.Errorf("11 的累计 = %d，want 5", d)
	}
}

// 误伤：己方人物 1（开战 200，份额 20）被己方打倒，20 平分给对方活着的 10、11。
func TestFriendlyFireGoesToEnemies(t *testing.T) {
	l := NewLedger()
	l.Join(1, 200)
	l.Join(10, 100)
	l.Join(11, 100)
	l.Knockout(1, []int{10, 11})
	if d := l.Delta(1); d != -20 {
		t.Errorf("误伤者被击倒的累计 = %d，want -20", d)
	}
	if d := l.Delta(10); d != 10 {
		t.Errorf("10 的累计 = %d，want 10", d)
	}
}

// 开战 DP 低于 10 时份额为 0，但击倒者至少拿 1 点。
func TestKnockoutMinimumOne(t *testing.T) {
	l := NewLedger()
	l.Join(0, 5)
	l.Join(10, 100)
	l.Knockout(0, []int{10})
	if d := l.Delta(0); d != 0 {
		t.Errorf("份额为 0 时被击倒者累计 = %d，want 0", d)
	}
	if d := l.Delta(10); d != 1 {
		t.Errorf("击倒者累计 = %d，want 1", d)
	}
}

// 没有出手者（空列表）时只扣不加。
func TestKnockoutNoGainers(t *testing.T) {
	l := NewLedger()
	l.Join(0, 100)
	l.Knockout(0, nil)
	if d := l.Delta(0); d != -10 {
		t.Errorf("累计 = %d，want -10", d)
	}
}

// 同一人可以倒下多次，每次都按开战 DP 结算一次。
func TestKnockoutTwice(t *testing.T) {
	l := NewLedger()
	l.Join(0, 100)
	l.Join(10, 100)
	l.Knockout(0, []int{10})
	l.Knockout(0, []int{10})
	if d := l.Delta(0); d != -20 {
		t.Errorf("两次倒下后累计 = %d，want -20", d)
	}
}

// 逃跑：覆盖累计为 -2 × 份额，份额平分给对方活着的人物。
func TestEscapeOverridesCumulative(t *testing.T) {
	l := NewLedger()
	l.Join(0, 100)
	l.Join(10, 100)
	l.Join(11, 100)
	l.Knockout(0, []int{10}) // 0 先被击倒 -10，10 +10
	l.Knockout(11, []int{10})
	// 10 此时累计 +20；10 逃跑后被覆盖为 -20，对方 0、11 各得 max(1, 10/2)=5
	l.Escape(10, []int{0, 11})
	if d := l.Delta(10); d != -20 {
		t.Errorf("逃跑者累计 = %d，want -20（覆盖）", d)
	}
	if d := l.Delta(0); d != -10+5 {
		t.Errorf("对方 0 累计 = %d，want -5", d)
	}
}

func TestEscapeLowDP(t *testing.T) {
	l := NewLedger()
	l.Join(0, 0)
	l.Join(10, 100)
	l.Escape(0, []int{10})
	if d := l.Delta(0); d != -2 {
		t.Errorf("低 DP 逃跑累计 = %d，want -2", d)
	}
	if d := l.Delta(10); d != 1 {
		t.Errorf("对方累计 = %d，want 1", d)
	}
}

func TestEscapeNoOpponents(t *testing.T) {
	l := NewLedger()
	l.Join(0, 300)
	l.Escape(0, nil)
	if d := l.Delta(0); d != -60 {
		t.Errorf("累计 = %d，want -60", d)
	}
}

func TestFinalClamps(t *testing.T) {
	l := NewLedger()
	l.Join(0, 0)
	l.Join(1, MaxDP)
	l.Escape(0, nil)
	if got := l.Final(0); got != 0 {
		t.Errorf("Final(0) = %d，want 0", got)
	}
	l.Knockout(1, nil)
	if got := l.Final(1); got != MaxDP-10000000 {
		t.Errorf("Final(1) = %d，want %d", got, MaxDP-10000000)
	}
}
