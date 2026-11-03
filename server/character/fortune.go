// 运气与魅力的规则（STAGE-61，对照 gmsv/char/char.c 的 CHAR_setLuck、CHAR_AddCharm，battle.c 的战斗魅力增减）。
// 不碰数据库：掷运气只改内存里的记录，写库在 markEntered；魅力的增减在 applyResult，随角色保存。

package character

import (
	"math/rand/v2"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

// rollDailyLuck 按游戏日掷运气：上次掷的日期（本地时区）和 now 不是同一天就掷一次（原版 CHAR_setLuck 的日期比较）。
// 同一天内重登运气不变。返回是否掷了。
func rollDailyLuck(r *record, now time.Time, rnd func(n int) int) bool {
	if r.LuckRolledAt != nil && sameDay(r.LuckRolledAt.In(time.Local), now.In(time.Local)) {
		return false
	}
	t := now
	r.Luck = gamedata.RollLuck(rnd(100))
	r.LuckRolledAt = &t
	return true
}

// sameDay 判断两个时间是不是同一天（调用方已换算到同一时区）。
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// luckRnd 返回掷运气的随机源：测试里按用例名固定种子，生产环境用全局随机（同 wildRnd）。
func (s *Service) luckRnd() func(n int) int {
	if s.opts.LuckRnd != nil {
		return s.opts.LuckRnd
	}
	return rand.IntN
}

// charmOnDeath 是人物在野外战斗中倒下时的魅力变化：被打飞按 BATTLE_UltimateExtra（CH_FIX_PLAYEULTIMATE），
// 普通被打倒按 BATTLE_NormalDeadExtra（CH_FIX_PLAYERDEAD）。等级 10 以下减半（levelflg = 2）。
func charmOnDeath(level int, flown bool) int {
	v := gamedata.CharmOnDeath
	if flown {
		v = gamedata.CharmOnFly
	}
	if level <= 10 {
		return v / 2
	}
	return v
}
