package gamedata

// 任务脚本的宠物写法（STAGE-197，docs/quest.md）：宠物条件、收宠物、送宠物的数据。转换见 tools/npcevent_pet.py，
// 对照 gmsv/npc/npc_exchangeman.c 的 NPC_PetLvCheck、NPC_EventDelPet、NPC_EventAddPet。执行在 server/character。

import "fmt"

// PetAsk 是一个宠物项：等级用 Cmp 和 Level 比较（= < >，比的是宠物等级，同 NPC_EventBigSmallLastCheck），
// 编号是宠物模板编号，Count 是需要（或收走）的只数。条件里 Count 至少为 1；DelPet 里 Count 为 0 表示全部收走
// （但仍要求至少 1 只，同 NPC_PetLvCheck 的缺省）。
type PetAsk struct {
	Cmp      string `json:"cmp"`
	Level    int    `json:"level"`
	Template int    `json:"template"`
	Count    int    `json:"count"`
}

// Match 报告一只宠物（模板编号和等级）是否符合这一项的编号和等级条件（只数等级比较，不管只数）。
func (a PetAsk) Match(template, level int) bool {
	return template == a.Template && cmpInt(level, a.Cmp, a.Level)
}

// cmpInt 按比较符比较两个整数（= < >），和 npcevent 条件的 LV 比较是同一套语义。
func cmpInt(v int, cmp string, n int) bool {
	switch cmp {
	case "=":
		return v == n
	case "<":
		return v < n
	case ">":
		return v > n
	}
	return false
}

// markPetGifts 检查 GetPet 的怪物编号：怪物要在敌人表里，它的宠物模板要在宠物模板表里（原版 ENEMY_getInt 找不到就失败，
// 这里提前标不支持，不让块在运行时才失败）。查不到的块标 Unsupported，原因写清楚。
func markPetGifts(list []EventScript) {
	for i := range list {
		for j := range list[i].Blocks {
			blk := &list[i].Blocks[j]
			if len(blk.GetPet) == 0 || blk.Unsupported != "" {
				continue
			}
			if why := checkPetGifts(blk.GetPet); why != "" {
				blk.Unsupported = why
			}
		}
	}
}

// checkPetGifts 返回 GetPet 编号里第一个查不到的原因，全部能查到返回空串。
func checkPetGifts(enemyIDs []int) string {
	for _, id := range enemyIDs {
		enemy, ok := EnemyByID(id)
		if !ok {
			return fmt.Sprintf("GetPet 的怪物 %d 不在敌人表里", id)
		}
		if _, ok := PetByID(enemy.Template); !ok {
			return fmt.Sprintf("GetPet 的怪物 %d 的宠物模板 %d 不在宠物表里", id, enemy.Template)
		}
	}
	return ""
}
