package character

// 任务脚本的宠物条件（STAGE-197，对照 npc_exchangeman.c 的 NPC_PetLvCheck）：PET<比较符><等级>-<编号>[*数量]。
// 数的是背包栏（随身 20 个栏位，STAGE-346）里符合的宠物，骑乘和出战的照样算；任务送的宠物（event_given）不数。
// 收宠物和送宠物的执行在 npcevent_pet_act.go。

import (
	"github.com/athlan20/stoneage/server/gamedata"
)

// petCount 数背包栏里符合宠物项的只数。寄放在宠物店的（不在随身栏位）和任务送的（EventGiven）不数，
// 原版只数 CHAR_ENDEVENT == 0 的随身宠物。
func petCount(r *record, a gamedata.PetAsk) int {
	n := 0
	for i := range r.Pets {
		p := &r.Pets[i]
		if !p.carried() || p.EventGiven {
			continue
		}
		if a.Match(p.TemplateID, p.Level) {
			n++
		}
	}
	return n
}

// petTermHolds 判断 PET 条件：符合的只数够 Count（缺省按 1 只算，原版 loop 缺省 1）。
func petTermHolds(r *record, a *gamedata.PetAsk) bool {
	if a == nil {
		return false
	}
	return petCount(r, *a) >= max(a.Count, 1)
}
