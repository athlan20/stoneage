// STAGE-201：魔障、沉默、虚弱三个状态精灵的挂法（gmsv/magic/magic.c 的 MAGIC_Barrier、MAGIC_Nocast、
// MAGIC_Weaken，battle_magic.c 的 BATTLE_MultiParamChangeTurn）。挂上的成败仍走 statusCheck（同状态精灵），
// 这里只管两个特例：回合数的 +1 与沉默不中宠物。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// statusTurnsOf 是状态精灵挂上的回合数：魔障、虚弱是精灵的 turn + 1（MAGIC_Barrier 写 turn+1，
// BATTLE_MultiParamChangeTurn 写 CHAR_WORKWEAKEN 为 turn+1），其余（含沉默）照精灵表的 turn。
func statusTurnsOf(t gamedata.MagicTemplate) int {
	switch t.Status {
	case "barrier", "weaken":
		return t.Turns + 1
	}
	return t.Turns
}

// statusCanLand 表示目标能不能挂上这个状态（成败判定之后再看）：沉默不中宠物（MAGIC_Nocast 的 CHAR_TYPEPET 排除）。
func statusCanLand(status string, target *Unit) bool {
	return status != "nocast" || target.Kind != KindPet
}
