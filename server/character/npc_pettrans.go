package character

// STAGE-236：宠物转生窗口（精灵王 / 玛蕾菲雅，原版 npc_transmigration.c 的 PETTRANS 段，ten.arg 第 7–15 行）。
// 步骤：开始（mainmsg，下一步）→ 资格检查（selectmsg 是/否，不合格给 failmsg）→ 选宠（setpetmsg，最后一项是取消）
// → 确认（transmsg 是/否）→ 转生（main1msg），取消走 cancelmsg。窗口用 npc_window 的选项表达，选项编号从 1 开始。
// 规则在 npc_pettrans_plan.go，写库在 npc_pettrans_store.go。

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// npc_transmigration 是精灵王 NPC 的类型（gen-npcs.py 的 kind）。
const petTransKind = "npc_transmigration"

// 窗口的步骤（petTransSession.stage）。
const (
	ptStart     = iota // mainmsg：只有“下一步”
	ptSelect           // selectmsg：是 / 否
	ptChoosePet        // setpetmsg：每只随身宠物一项，最后一项是取消
	ptConfirm          // transmsg：是 / 否，正文前面拼上宠物名
)

// 台词（ten.arg 的 PETTRANS 段，原文保留；失败时的提示取自原版 C 代码）。
const (
	petTransMainMsg   = "玛蕾菲雅：谢谢你让我和精灵王重聚，让我与精灵王赐予你宠物转生的力量吧。"
	petTransSelectMsg = "玛蕾菲雅：你的宠物已经准备好要转生了吗？"
	petTransFailMsg   = "玛蕾菲雅：你的宠物还无法转生喔！"
	petTransSetPetMsg = "玛蕾菲雅：请选出一只要接受转生的宠物吧。"
	petTransConfirm   = "玛蕾菲雅：你确定好了吗？对宠物来说这可是件不得了的事情，身为它的饲主要好好考虑清楚喔！"
	petTransMain1Msg  = "玛蕾菲雅：宠物转生成功了！请好好对待它吧。\n那么我们就在这儿分手了，说不定以后还有机会可以再见面喔，请多保重。"
	petTransCancelMsg = "玛蕾菲雅：等你考虑清楚了以后再来吧！"
)

// petTransSession 是转生窗口的状态：当前步骤、选宠窗口里每个选项对应的栏位、确认时选中的栏位。
type petTransSession struct {
	stage int
	slots []int // ptChoosePet：第 i 项（选项编号 i+1）对应的宠物栏位
	slot  int   // ptConfirm：选中的宠物栏位
}

// petTransYesNo 是“是 / 否”两个选项。
func petTransYesNo() []npcOption {
	return []npcOption{{No: 1, Label: "是"}, {No: 2, Label: "否"}}
}

// petTransOpen 打开（或替换）转生窗口：窗口的状态挂在 npcWindow.pettrans 上，sess 为 nil 时只是一句话。
func petTransOpen(p *Presence, npc gamedata.NPC, sess *petTransSession, text string, options []npcOption) {
	p.npcWindow = &npcWindow{npc: npc.ID, pettrans: sess}
	p.pushAll("npc_window", npcWindowView(npc, text, options...))
}

// talkPetTrans 是 talk 精灵王：按身上的玛蕾分流（planPetTransEntry）。调用时要持有 p.mu。
func (s *Service) talkPetTrans(l *Live, r *record, npc gamedata.NPC) *protocol.Error {
	kind, text := planPetTransEntry(r.Level, r.Pets)
	switch kind {
	case entryTalk:
		petTransOpen(l.p, npc, nil, text, nil)
	case entryStart:
		petTransOpen(l.p, npc, &petTransSession{stage: ptStart}, petTransMainMsg, []npcOption{{No: 1, Label: "下一步"}})
	default:
		return protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 还不能对话", npcDisplayName(npc))
	}
	return nil
}

// petTransChoose 处理转生窗口的选项（npc_choose）。调用时要持有 p.mu。
func (s *Service) petTransChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, sess *petTransSession, no int) *protocol.Error {
	p := l.p
	cancel := func() {
		p.pushWindowClosed(WindowReasonChosen)
		petTransOpen(p, npc, nil, petTransCancelMsg, nil)
	}
	switch sess.stage {
	case ptStart: // case 0 → case 1：资格检查
		if no != 1 {
			return petTransBadChoice(no)
		}
		p.pushWindowClosed(WindowReasonChosen)
		if !petTransQualified(r) {
			petTransOpen(p, npc, nil, petTransFailMsg, nil)
			return nil
		}
		petTransOpen(p, npc, &petTransSession{stage: ptSelect}, petTransSelectMsg, petTransYesNo())
	case ptSelect: // case 1 → case 2（是）或 case 5（否）
		switch no {
		case 1:
			p.pushWindowClosed(WindowReasonChosen)
			opts, slots := petTransPetOptions(r)
			petTransOpen(p, npc, &petTransSession{stage: ptChoosePet, slots: slots}, petTransSetPetMsg, opts)
		case 2:
			cancel()
		default:
			return petTransBadChoice(no)
		}
	case ptChoosePet: // case 2：选中一只宠物 → case 3；取消 → case 5
		if no < 1 || no > len(sess.slots)+1 {
			return petTransBadChoice(no)
		}
		if no == len(sess.slots)+1 {
			cancel()
			return nil
		}
		slot := sess.slots[no-1]
		pet := findPet(r.Pets, slot)
		if pet == nil {
			return protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
		}
		p.pushWindowClosed(WindowReasonChosen)
		text := fmt.Sprintf("是[%s]没错吧？ \n\n%s", pet.Name, petTransConfirm)
		petTransOpen(p, npc, &petTransSession{stage: ptConfirm, slot: slot}, text, petTransYesNo())
	case ptConfirm: // case 3：是 → case 4（转生）；否 → case 5
		switch no {
		case 1:
			return s.petTransExecute(ctx, l, r, npc, sess.slot)
		case 2:
			cancel()
		default:
			return petTransBadChoice(no)
		}
	}
	return nil
}

// petTransBadChoice 是选项编号不在窗口里时的错误（同 npc_choose 的其他窗口）。
func petTransBadChoice(no int) *protocol.Error {
	return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
}

// petTransPetOptions 是选宠窗口的选项：每只随身宠物一项（按栏位从小到大），最后一项是取消。
func petTransPetOptions(r *record) ([]npcOption, []int) {
	var opts []npcOption
	var slots []int
	for i := range r.Pets {
		p := &r.Pets[i]
		if !p.carried() {
			continue
		}
		slots = append(slots, p.Slot)
		opts = append(opts, npcOption{No: len(opts) + 1, Label: fmt.Sprintf("%s Lv%d", p.Name, p.Level)})
	}
	opts = append(opts, npcOption{No: len(opts) + 1, Label: "取消"})
	return opts, slots
}

// petTransExecute 是 case 4：先按 petTransPetCheck 检查宠物，通过后在一个事务里转生（commitPetTransmute：宠物、道具、事件 71 一起写，STAGE-237）。
// 检查不过，或者状态检查失败（原版 NPC_PetTransManStatus 返回 FALSE），都不发新窗口：
// 前者说一句提示关窗，后者什么都不做，客户端停在确认窗（原版行为，裁定见 docs/pet-rebirth-quest.md 第 8 节第 3 条）。
func (s *Service) petTransExecute(ctx context.Context, l *Live, r *record, npc gamedata.NPC, slot int) *protocol.Error {
	p := l.p
	pet := findPet(r.Pets, slot)
	if pet == nil {
		return nil
	}
	if msg := petTransPetCheck(pet); msg != "" {
		p.pushWindowClosed(WindowReasonChosen)
		petTransOpen(p, npc, nil, msg, nil)
		return nil
	}
	done, err := s.commitPetTransmute(ctx, p, r, slot, s.petTransRnd())
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, "宠物转生失败：%v", err)
	}
	if !done {
		return nil
	}
	p.pushWindowClosed(WindowReasonChosen)
	petTransOpen(p, npc, nil, petTransMain1Msg, nil)
	return nil
}

// petTransRnd 是转生用的随机数：测试时用 NpcTalkRnd 注入，正式运行用 math/rand。
func (s *Service) petTransRnd() gamedata.Rnd {
	if r := s.talkRnd(); r != nil {
		return r
	}
	return rand.Intn
}
