// STAGE-235：把宠物转生重算的结果写回宠物结构（纯内存，不落库、不发协议；转生窗口和落库在后续需求做）。

package character

import "github.com/athlan20/stoneage/server/gamedata"

// applyPetTransmig 照 NPC_PetTransManStatus 与 GetNewPet 的写入顺序，把重算结果写进宠物：
// 资质、品质、能力按结果；LV=1；经验清零（CHAR_setMaxExp 0）；血量补满到新的最大 HP；
// TRANS+1；LIMITLEVEL=-1。
// 没有动：栏位 Slot（原版没有对应字段，CHAR_SLOT=7 是技能栏数，Go 里固定为 PetSkillSlots=7）、技能、
// 击杀数、任务宠标记、起始值 Origin、AI。
func applyPetTransmig(p *petRec, r gamedata.TransResult) {
	p.Base = r.Base
	p.Rank = r.Rank
	p.Stats = r.Stats
	p.Level = 1
	p.Exp = 0
	p.HP = gamedata.PetMaxHP(r.Stats)
	p.Transmigration++
	limit := -1 // 个体等级上限（STAGE-234 起是 *int）：-1 为没有个体上限，取模板值时才用 nil
	p.LimitLevel = &limit
}
