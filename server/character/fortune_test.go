package character

import (
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 运气按游戏日掷：同一天内不再掷，跨了本地日历日才掷（STAGE-61，原版 CHAR_setLuck）。
func TestRollDailyLuckOncePerGameDay(t *testing.T) {
	day1 := time.Date(2026, 10, 11, 9, 0, 0, 0, time.Local)
	r := &record{}
	calls := 0
	rnd := func(n int) int { calls++; return 5 } // 掷出 5，落在 4 档（3 <= r < 10）
	if !rollDailyLuck(r, day1, rnd) || r.Luck != gamedata.RollLuck(5) {
		t.Fatalf("第一次进入应当掷一次，运气 %d", r.Luck)
	}

	// 同一天的晚些时候重登：不掷，运气保持。
	r.Luck = 1
	if rollDailyLuck(r, day1.Add(14*time.Hour), rnd) || r.Luck != 1 || calls != 1 {
		t.Errorf("同一天重登不应重掷：运气 %d，掷骰次数 %d", r.Luck, calls)
	}

	// 跨过本地午夜：掷一次。
	nextDay := time.Date(2026, 10, 12, 0, 1, 0, 0, time.Local)
	if !rollDailyLuck(r, nextDay, func(int) int { return 0 }) || r.Luck != 5 {
		t.Errorf("跨日后应重掷：运气 %d，want 5（掷出 0）", r.Luck)
	}
	if r.LuckRolledAt == nil || !r.LuckRolledAt.Equal(nextDay) {
		t.Errorf("上次掷的时间没有更新：%v", r.LuckRolledAt)
	}
}

// 被打倒的魅力减少：等级 10 以下减半（battle.c 的 levelflg）。
func TestCharmOnDeathHalvesAtLowLevel(t *testing.T) {
	if got := charmOnDeath(10, false); got != -1 {
		t.Errorf("10 级被打倒的魅力变化 = %d，want -1", got)
	}
	if got := charmOnDeath(11, false); got != -2 {
		t.Errorf("11 级被打倒的魅力变化 = %d，want -2", got)
	}
}

// 被打飞的魅力减少比被打倒多（STAGE-63，battle.c 的 CH_FIX_PLAYEULTIMATE = -4）：等级 10 以下同样减半。
func TestCharmOnFlyIsLarger(t *testing.T) {
	cases := []struct {
		level int
		flown bool
		want  int
	}{
		{10, true, -2}, // 10 级被打飞：-4 减半
		{11, true, -4}, // 11 级被打飞
		{1, true, -2},  // 新角色被打飞
		{1, false, -1}, // 新角色普通被打倒，对照
	}
	for _, c := range cases {
		if got := charmOnDeath(c.level, c.flown); got != c.want {
			t.Errorf("%d 级（打飞 %v）魅力变化 = %d，want %d", c.level, c.flown, got, c.want)
		}
	}
}

// 运气存进数据库（STAGE-61，验收 3）：同一天离开再进入不重掷，跨过本地午夜后进入时重掷。
func TestLuckSurvivesRelogAndRerollsNextDay(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.Local)
	setClock := func(tm time.Time) { mu.Lock(); now = tm; mu.Unlock() }
	rolls := []int{5, 0} // 第一次掷 5（4 档），第二天掷 0（5 档）
	calls := 0
	e := newEnv(t, func(s *Service) {
		s.opts.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
		s.opts.LuckRnd = func(int) int { mu.Lock(); defer mu.Unlock(); calls++; return rolls[calls-1] }
	})
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	rolled := func() int { mu.Lock(); defer mu.Unlock(); return calls }
	luckOf := func(m map[string]any) int {
		return int(m["status"].(map[string]any)["derived"].(map[string]any)["luck"].(float64))
	}

	if got := luckOf(c.ok("enter_world", `{"slot":0}`)); got != gamedata.RollLuck(5) {
		t.Fatalf("第一次进入的运气 = %d，want %d", got, gamedata.RollLuck(5))
	}
	c.ok("leave_world", `{}`)

	setClock(time.Date(2026, 10, 11, 21, 30, 0, 0, time.Local))
	if got := luckOf(c.ok("enter_world", `{"slot":0}`)); got != gamedata.RollLuck(5) || rolled() != 1 {
		t.Errorf("同一天重登运气应不变且不重掷：运气 %d，掷骰 %d 次", got, rolled())
	}
	c.ok("leave_world", `{}`)

	setClock(time.Date(2026, 10, 12, 0, 5, 0, 0, time.Local))
	if got := luckOf(c.ok("enter_world", `{"slot":0}`)); got != gamedata.RollLuck(0) || rolled() != 2 {
		t.Errorf("跨日后应重掷：运气 %d（want %d），掷骰 %d 次", got, gamedata.RollLuck(0), rolled())
	}
}
