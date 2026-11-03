package battle

// 敌人的站位与等级（STAGE-40 的野外遇敌与 STAGE-96 的 NPC 战斗共用）：大型敌人全场最多 5 只、第 6 个起要和前排
// 的普通体型换位（gmsv/npc/npcutil.c 的 NPC_Util_getEnemy 与 gmsv/char/enemy.c 的 ENEMY_getEnemy 收尾循环），
// 以及敌人等级的抽取。

import "github.com/athlan20/stoneage/server/gamedata"

// seatResult 是 placeEnemy 的结果。
type seatResult int

const (
	seated     seatResult = iota // 放下了
	seatFull                     // 大型敌人已满 5 只：这一只不出
	seatNoSwap                   // 第 6 个起的大型敌人找不到可以换位的普通体型：这一只不出
)

// placeEnemy 把敌人 e 放进阵容 out。bigCnt 记大型敌人的只数，同一场里的调用方共用它。
func placeEnemy(out []*gamedata.EnemyVariant, e *gamedata.EnemyVariant, bigCnt *int) ([]*gamedata.EnemyVariant, seatResult) {
	if tmpl, ok := gamedata.PetByID(e.Template); !ok || tmpl.Size != 1 {
		return append(out, e), seated
	}
	if *bigCnt >= 5 {
		return out, seatFull
	}
	if len(out) <= 4 {
		*bigCnt++
		return append(out, e), seated
	}
	// 第 6 个起：找前 5 个里的普通体型换位（大型站前排）；找不到就不出这只。
	for j := 0; j < 5; j++ {
		if t, ok := gamedata.PetByID(out[j].Template); ok && t.Size != 1 {
			*bigCnt++
			out = append(out, out[j])
			out[j] = e
			return out, seated
		}
	}
	return out, seatNoSwap
}

// enemyLevel 抽敌人的等级：敌人表给了范围就在范围里随机（同 RAND），定值时不耗随机数。
func enemyLevel(e *gamedata.EnemyVariant, rnd func(n int) int) int {
	level := e.LVMin
	if e.LVMax > e.LVMin {
		level += rnd(e.LVMax - e.LVMin + 1)
	}
	return level
}
