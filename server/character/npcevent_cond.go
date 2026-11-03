package character

// 任务脚本的条件（STAGE-90，对照 npc_exchangeman.c 的 NPC_ExChangeManEventCheck 和 NPC_EventFreeIfCheck）。

import (
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

// eventCondHolds 是 EVENT 的整体判断：候选组任一全部满足即可（逗号是或，& 是与）。
func eventCondHolds(r *record, groups [][]gamedata.EventTerm, now time.Time) bool {
	return eventMatchedGroup(r, groups, now) >= 0
}

// eventMatchedGroup 返回命中的候选组序号（从 0 起），没有命中返回 -1。EVENT 逗号分候选，组内 & 与。
func eventMatchedGroup(r *record, groups [][]gamedata.EventTerm, now time.Time) int {
	for i, group := range groups {
		ok := true
		for _, t := range group {
			if !eventTermHolds(r, t, now) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// eventTermHolds 判断一个条件。ITEM 只数背包（不含穿戴，同 NPC_EventItemCheck）；
// 数量条件 ITEM=编号*数量 按背包里的堆叠数累计（同 NPC_EventReduce）。
func eventTermHolds(r *record, t gamedata.EventTerm, now time.Time) bool {
	switch t.Op {
	case "ITEM":
		have := bagCount(r, t.ID)
		if t.Cmp == "!=" {
			return have == 0
		}
		return have >= t.N
	case "LV":
		return cmpInt(r.Level, t.Cmp, t.N)
	case "ENDEV":
		return r.eventDone(t.N) == (t.Cmp == "=")
	case "NOWEV":
		// STAGE-321：原版 NPC_NOWEventNoCheck 的 flg=3（!=）无论事件是否进行中都返回 TRUE，
		// 所以 NOWEV!= 恒为真（extra/event/marju01 的写法，原版如此）。
		if t.Cmp == "!=" {
			return true
		}
		return r.eventDoing(t.N)
	case "TIME":
		return gamedata.EventTimeSection(now) != t.N
	case "NEVER":
		// STAGE-323：原版不认的条件（NOEEV）恒不成立。
		return false
	case "PET":
		return petTermHolds(r, t.Pet) // STAGE-197，npcevent_pet.go
	}
	return false
}

// bagCount 数背包里编号为 id 的道具数（每格的数量相加）。
func bagCount(r *record, id int) int {
	n := 0
	for _, it := range r.Items {
		if it.ItemID == id && it.Slot >= firstBagSlot {
			n += it.Count
		}
	}
	return n
}

func cmpInt(v int, cmp string, n int) bool {
	switch cmp {
	case "=":
		return v == n
	case "<":
		return v < n
	case ">":
		return v > n
	case "!=":
		return v != n
	}
	return false
}

// stoneCost 是 DelStone 的金币数：固定值，或等级乘系数（NPC_EventGetCost 的 LV*N）。
func stoneCost(c *gamedata.StoneCost, level int) int {
	if c == nil {
		return 0
	}
	if c.PerLv > 0 {
		return level * c.PerLv
	}
	return c.Fixed
}
