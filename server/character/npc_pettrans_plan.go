package character

// STAGE-236：宠物转生窗口的纯逻辑（入口分流、资格检查、选中宠物的检查、转生的数值与栏位变化），不碰数据库和连接。
// 出处：original-data/gmsv/npc/npc_transmigration.c 的 NPC_TransmigrationTalked（入口，49–128）、
// NPC_PetTransManCheck（资格，npc_transmigration.c:1377）、NPC_PetTransManStatus（转生，1399）和窗口 case 4（1319–1351）。
// 窗口流程和台词在 npc_pettrans.go，写库在 npc_pettrans_store.go。

import "github.com/athlan20/stoneage/server/gamedata"

// 玛蕾菲雅的宠物编号（petbase.tsv）：718 是一转玛蕾，401 是二转玛蕾（原版 _PET_2TRANS）。
const (
	malaPetID  = 718
	malaPetID2 = 401
	malaLevel  = 79 // 能转生的玛蕾只认 79 级
	// petTransMinLevel 是玩家和被转生宠物的等级下限（原版 NPC_PetTransManCheck、case 4）。
	petTransMinLevel = 80
	// petTransMinStat 是玛蕾四项资质的下限，低于它说“你太不爱惜我了”（原版 NPC_TransmigrationTalked）。
	petTransMinStat = 15
	// petTransMaxTrans 是宠物转生次数的上限：转生次数大于它就不能再转（原版 TRANS > 1 拒绝）。
	petTransMaxTrans = 1
)

// petTransEntry 是 talk 精灵王时的分流结果。
type petTransEntry int

const (
	entryNotTalkable petTransEntry = iota // 身上没有玛蕾：人物转生（TENSEI）本期不做，回“还不能对话”
	entryTalk                             // 只说一句话（文字窗口，没有选项）
	entryStart                            // 进入宠物转生（case 0）
)

// planPetTransEntry 照 NPC_TransmigrationTalked 的顺序检查身上的玛蕾（只看随身栏位）。
// 玩家等级不够 80 时，任何一只玛蕾都先给提示；79 级玛蕾不满足资质下限时给“没门”；
// 79 级玛蕾恰好一只才进入转生，多于一只给“那麽多玛雷菲雅”。
// 原版还有“玛蕾超过 79 级要被收走”的分支，这里不做：遇到它回“还不能对话”（不改数据）。
func planPetTransEntry(playerLevel int, pets []petRec) (petTransEntry, string) {
	count := 0
	for i := range pets {
		p := &pets[i]
		if !p.carried() || !isMalaPet(p) {
			continue
		}
		if playerLevel < petTransMinLevel {
			return entryTalk, "由于你的等级小于80,没能力进行转宠！"
		}
		switch {
		case p.Level > malaLevel:
			return entryNotTalkable, ""
		case p.Level == malaLevel:
			if p.Base.Vital < petTransMinStat || p.Base.Strength < petTransMinStat ||
				p.Base.Toughness < petTransMinStat || p.Base.Dexterity < petTransMinStat {
				return entryTalk, "操，你太不爱惜我了，还要我帮你转宠？没门！"
			}
			count++
		}
	}
	switch {
	case count > 1:
		return entryTalk, "你怎会有那麽多玛雷菲雅呢？"
	case count == 1:
		return entryStart, ""
	}
	return entryNotTalkable, ""
}

// isMalaPet 报告宠物是不是玛蕾菲雅（718 或 401）。
func isMalaPet(p *petRec) bool {
	return p.TemplateID == malaPetID || p.TemplateID == malaPetID2
}

// petTransQualified 是 case 1 的资格检查（NPC_PetTransManCheck）：玩家 80 级以上，且事件 4、69、70 都已完成。
func petTransQualified(r *record) bool {
	if r.Level < petTransMinLevel {
		return false
	}
	for _, ev := range []int{4, 69, 70} {
		if !r.eventDone(ev) {
			return false
		}
	}
	return true
}

// petTransPetCheck 是 case 4 对所选宠物的检查（npc_transmigration.c:1319–1349），返回要说的话；空串表示通过。
// 骑乘中、等级不够 80、本身是玛蕾、转生次数超过上限，都不能转生。
func petTransPetCheck(p *petRec) string {
	switch {
	case p.State == petStateRide:
		return "骑乘中的宠物不能转生喔！"
	case p.Level < petTransMinLevel:
		return "接受转生祝福的宠物必需要等级80以上喔！"
	case isMalaPet(p):
		return "这只宠物不能转生喔！"
	case p.Transmigration > petTransMaxTrans:
		return "这只宠物已经接受过祝福了喔！"
	}
	return ""
}

// petTransResult 是一次转生的结果：要删的玛蕾、被转生宠物转生后的样子、转完之后的整张宠物表。
type petTransResult struct {
	Removed petRec
	Target  petRec
	Next    []petRec
}

// planPetTransmute 是 NPC_PetTransManStatus 的数值部分：找出玛蕾、按 STAGE-235 的重算得到新宠物、删掉玛蕾。
// 找不到玛蕾、宠物栏位没有宠物、或者宠物的模板不认识时返回 ok=false，原版在这种情况下直接返回 FALSE，
// 窗口不动、数据不动。玛蕾的查找规则同原版：转生次数为 0 的宠物找 718 或 401，转过一次的只找 401，且都要 79 级。
// 这里不管道具：转生成功后删灵石和项链是 planPetTransItems 的事（STAGE-237，不检查数量）。
func planPetTransmute(pets []petRec, slot int, rnd gamedata.Rnd) (petTransResult, bool) {
	ti := indexOfSlot(pets, slot)
	if ti < 0 {
		return petTransResult{}, false
	}
	target := pets[ti]
	tmpl, ok := gamedata.PetByID(target.TemplateID)
	if !ok {
		return petTransResult{}, false
	}
	mi := -1
	for i := range pets {
		p := &pets[i]
		if i == ti || !p.carried() || p.Level != malaLevel {
			continue
		}
		if p.TemplateID == malaPetID2 || (target.Transmigration == 0 && p.TemplateID == malaPetID) {
			mi = i
			break
		}
	}
	if mi < 0 {
		return petTransResult{}, false
	}
	mala := pets[mi]
	res := gamedata.Transmigrate(mala.Base, target.Base, tmpl, target.Level, target.Rank, target.Transmigration, rnd)
	applyPetTransmig(&target, res)
	next := make([]petRec, 0, len(pets)-1)
	for i := range pets {
		switch i {
		case mi:
			continue
		case ti:
			next = append(next, target)
		default:
			next = append(next, pets[i])
		}
	}
	return petTransResult{Removed: mala, Target: target, Next: next}, true
}

// indexOfSlot 返回栏位上宠物的下标，没有时返回 -1。
func indexOfSlot(pets []petRec, slot int) int {
	for i := range pets {
		if pets[i].Slot == slot {
			return i
		}
	}
	return -1
}
