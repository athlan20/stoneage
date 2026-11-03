package character

// 宠物技能店（STAGE-143，docs/protocol.md 5.51）：talk 宠物技能店 NPC 弹技能列表（名字、价格、说明），
// 选一个技能、一只随身宠物、一个技能栏位，确认页成交：金币扣掉学习价格（技能的 PETSKILL_COST ×
// 店的 skill_rate），技能写进选中的栏位、覆盖原来的技能，成交后回到列表。规则出处
// gmsv/npc/npc_petskillshop.c。骑乘中的宠物也能学（学技能与骑乘无关）。

import (
	"context"
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// petskillShopReach 是能和宠物技能店对话的距离（格，切比雪夫距离），同原版
// NPC_PetSkillShopWindowTalked 的 `NPC_Util_CharDistance > 2` 规则。
const petskillShopReach = 2

// 宠物技能店窗口的阶段。
const (
	skillStageList    = "list"    // 技能列表
	skillStagePet     = "pet"     // 选宠物（随身列表）
	skillStageSlot    = "slot"    // 选技能栏位
	skillStageConfirm = "confirm" // 确认页
)

// petskillShopSession 记录宠物技能店对话的阶段和已选的技能、宠物、栏位。
type petskillShopSession struct {
	stage     string
	skill     int // 已选的技能编号
	slot      int // 已选的宠物栏位（随身 0–19，内部用下标，窗口里的编号是它加 1）
	skillSlot int // 已选的技能栏位（1 起，同宠物页）
}

// skillListOptions 是技能列表：编号从 1 起，每项「名字（价格 金币）：说明」。
func skillListOptions(shop *gamedata.NpcPetskillShop) []npcOption {
	options := []npcOption{}
	for i, sk := range shop.ListedSkills() {
		options = append(options, npcOption{No: i + 1,
			Label: fmt.Sprintf("%s（%d 金币）：%s", sk.Name, shop.LearnCost(sk), sk.Comment)})
	}
	return options
}

// petskillPetOptions 是随身宠物列表：编号 1 起（同宠物页和 pet 命令，窗口里客户端只能选正数），
// 每项「名字 Lv」。
func petskillPetOptions(r *record) []npcOption {
	options := []npcOption{}
	for i := range r.Pets {
		p := &r.Pets[i]
		if !p.carried() {
			continue
		}
		options = append(options, npcOption{No: p.Slot + 1, Label: fmt.Sprintf("%s Lv%d", p.Name, p.Level)})
	}
	return options
}

// petSkillSlotOptions 是一只宠物的技能栏位：编号 1 起（同宠物页），按这只宠物的栏位数列出，
// 每项写清栏位里现在是什么技能，空栏位写「（空）」。
func petSkillSlotOptions(p *petRec) []npcOption {
	options := []npcOption{}
	for i := 0; i < petSkillSlots(p); i++ {
		options = append(options, npcOption{No: i + 1,
			Label: fmt.Sprintf("%d 号栏位：%s", i+1, petSkillName(p.Skills[i]))})
	}
	return options
}

// petSkillSlots 是这只宠物的技能栏位数（模板的 E_T_SLOT，不超过宠物页的 7 栏）。
func petSkillSlots(p *petRec) int {
	tmpl, ok := gamedata.PetByID(p.TemplateID)
	if !ok {
		return 0
	}
	return min(tmpl.SkillSlots, gamedata.PetSkillSlots)
}

// petSkillName 是栏位里技能的名字，空栏位（-1）或查不到的编号写「（空）」。
func petSkillName(id int) string {
	if name, ok := gamedata.PetSkillName(id); ok {
		return name
	}
	return "（空）"
}

// checkPetskillShopReach 检查角色离宠物技能店够不够近（同一张地图、切比雪夫距离不超过 petskillShopReach）。
func checkPetskillShopReach(r *record, npc gamedata.NPC) *protocol.Error {
	dx, dy := abs(r.X-npc.X), abs(r.Y-npc.Y)
	if r.Floor != npc.Floor || max(dx, dy) > petskillShopReach {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "离%s太远了，走近一点再说", npcDisplayName(npc))
	}
	return nil
}

// petskillView 推送宠物技能店的窗口。
func (s *Service) petskillView(l *Live, npc gamedata.NPC, text string, options []npcOption) {
	l.p.pushAll("npc_window", npcWindowView(npc, text, options...))
}

// openPetskillShop 是 talk 成功后的技能列表。调用时要持有 p.mu（在 talk 的 Operate 里）。
// 离店太远时回复错误，窗口不变。
func (s *Service) openPetskillShop(l *Live, r *record, npc gamedata.NPC, shop *gamedata.NpcPetskillShop) *protocol.Error {
	if perr := checkPetskillShopReach(r, npc); perr != nil {
		return perr
	}
	l.p.npcWindow = &npcWindow{npc: npc.ID, petskill: &petskillShopSession{stage: skillStageList}}
	s.petskillView(l, npc, shopText(shop.Main), skillListOptions(shop))
	return nil
}

// petskillShopChoose 推进宠物技能店窗口（npc_choose）。调用时要持有 p.mu。
// 战斗中、离店太远都拒绝，阶段和数据都不变。
func (s *Service) petskillShopChoose(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcPetskillShop, no int) *protocol.Error {
	if perr := inBattle(l); perr != nil {
		return perr
	}
	if perr := checkPetskillShopReach(r, npc); perr != nil {
		return perr
	}
	ss := w.petskill
	switch ss.stage {
	case skillStageList:
		listed := shop.ListedSkills()
		if no < 1 || no > len(listed) {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
		}
		ss.stage, ss.skill = skillStagePet, listed[no-1].ID
		s.petskillView(l, npc, "要教哪一只宠物？", petskillPetOptions(r))
		return nil

	case skillStagePet:
		p := findPetAtSlot(r, no-1)
		if p == nil {
			return protocol.Errorf(protocol.CodeNoPet, "%d 号没有宠物", no)
		}
		ss.stage, ss.slot = skillStageSlot, no-1
		s.petskillView(l, npc, "要换掉哪一个技能栏位？", petSkillSlotOptions(p))
		return nil

	case skillStageSlot:
		p := findPetAtSlot(r, ss.slot)
		if p == nil {
			return protocol.Errorf(protocol.CodeNoPet, "%d 号没有宠物", ss.slot+1)
		}
		if no < 1 || no > petSkillSlots(p) {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "%s 没有 %d 号技能栏位", p.Name, no)
		}
		sk, ok := gamedata.PetSkillByID(ss.skill)
		if !ok {
			return protocol.Errorf(protocol.CodeWrongState, "这个技能已经不教了，重新打开试试")
		}
		ss.stage, ss.skillSlot = skillStageConfirm, no
		s.petskillView(l, npc, fmt.Sprintf("学习[%s]（%s）\n给[%s]的 %d 号技能栏位（现在是%s）\n\n    学习价格  %d 金币",
			sk.Name, sk.Comment, p.Name, no, petSkillName(p.Skills[no-1]), shop.LearnCost(sk)),
			[]npcOption{{No: 1, Label: "学习"}})
		return nil

	case skillStageConfirm:
		if no != 1 {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
		}
		return s.petskillLearn(ctx, l, r, w, npc, shop)
	}
	return protocol.Errorf(protocol.CodeWrongState, "窗口状态不对")
}

// petskillLearn 确认学习：重新检查宠物、栏位和金币（选择之后可能已经变了），通过后写库、
// 扣钱、换技能，推 pets_changed 和 status_changed，然后回到技能列表（同原版，可以接着学）。
// 任何一步被拒都不扣钱、不换技能，窗口留在确认页。
func (s *Service) petskillLearn(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcPetskillShop) *protocol.Error {
	ss := w.petskill
	sk, ok := gamedata.PetSkillByID(ss.skill)
	if !ok || sk.Illegal {
		return protocol.Errorf(protocol.CodeWrongState, "这个技能已经不教了，重新打开试试")
	}
	p := findPetAtSlot(r, ss.slot)
	if p == nil {
		return protocol.Errorf(protocol.CodeNoPet, "%d 号没有宠物", ss.slot+1)
	}
	if ss.skillSlot < 1 || ss.skillSlot > petSkillSlots(p) {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "%s 没有 %d 号技能栏位", p.Name, ss.skillSlot)
	}
	cost := shop.LearnCost(sk)
	if r.Gold < int64(cost) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "金币不够：学习「%s」要 %d 金币，你只有 %d", sk.Name, cost, r.Gold)
	}
	skills := p.Skills
	skills[ss.skillSlot-1] = sk.ID
	if _, err := s.db.Exec(ctx,
		`UPDATE character_pets SET skills = $3 WHERE character_id = $1 AND slot = $2`,
		r.ID, p.Slot, skillsArray(skills)); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	p.Skills = skills
	r.Gold -= int64(cost)
	l.p.pushAll("pets_changed", r.petsView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	ss.stage = skillStageList
	s.petskillView(l, npc, shopText(shop.Main), skillListOptions(shop))
	return nil
}
