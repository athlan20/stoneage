package character

// 任务 NPC 的每一步结算（STAGE-90，docs/quest.md「选块与对话」）：接任务、交付、MESSAGE 先做交易（planEvent），
// 交易成功后在同一个事务里写背包和进度（commitEvent），最后说台词；放弃和重置只写进度。
// 选块和窗口分派在 npcevent.go。

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// eventRequestYes 是接任务（原版 MsgDisp 2）：先做 REQUEST 的物品交易（空位或金币不够就只说 ItemFullMsg 之类，
// 什么都不记），成功后设进行中，再执行 EndSetFlg、CleanFlg，最后说 ThanksMsg。
func (s *Service) eventRequestYes(ctx context.Context, l *Live, r *record, npc gamedata.NPC, blk *gamedata.EventBlock) *protocol.Error {
	if len(blk.Msg.Pages("thanks")) == 0 {
		return nil
	}
	plan, fail, ok := planEvent(r, blk, s.eventSelOf(l, r, blk), s.eventRand)
	if !ok {
		openEventText(l.p, npc, fail)
		return nil
	}
	prog := progressOf(r)
	applyEventEdit(prog, editSetDoing, blk.Event)
	endSetFlg(prog, blk.Event, blk.EndSet)
	cleanFlg(prog, blk.Clean)
	res := eventResult{items: plan.items, gold: plan.gold, charm: r.Charm, prog: prog, pets: plan.pets, notes: plan.notes}
	if blk.Event > 0 { // 收走的东西记下来，放弃时退回（STAGE-94，npcevent_refund.go）；没有事件号的块不能放弃，不用记
		res.refund = &refundWrite{Event: blk.Event, eventRefund: eventRefund{Items: plan.taken, Gold: plan.takenGold}}
	}
	if perr := s.commitEvent(ctx, l, r, res); perr != nil {
		return perr
	}
	l.queueNPCWarp(npc, blk)
	openEventRead(l.p, npc, blk.Msg.Pages("thanks"), false)
	return nil
}

// eventAccept 是交付（原版 MsgDisp 8）：物品和金币先做交易，成功后执行 EndSetFlg（完成）、
// Charm（魅力）和 CleanFlg，最后说 ThanksMsg。交易不成立（空位、金币）时只说对应的台词，什么都不改。
func (s *Service) eventAccept(ctx context.Context, l *Live, r *record, npc gamedata.NPC, blk *gamedata.EventBlock) *protocol.Error {
	if len(blk.Msg.Pages("thanks")) == 0 {
		return nil
	}
	plan, fail, ok := planEvent(r, blk, s.eventSelOf(l, r, blk), s.eventRand)
	if !ok {
		openEventText(l.p, npc, fail)
		return nil
	}
	prog := progressOf(r)
	endSetFlg(prog, blk.Event, blk.EndSet)
	keepDoing(prog, blk)
	charm := r.Charm
	if blk.Charm > 0 && blk.Event > 0 && charm < 100 { // 原版：事件号大于 0、魅力不满 100 才加，封顶 100
		charm = min(100, charm+blk.Charm)
	}
	cleanFlg(prog, blk.Clean)
	if perr := s.commitEvent(ctx, l, r, eventResult{items: plan.items, gold: plan.gold, charm: charm, prog: prog, pets: plan.pets, notes: plan.notes}); perr != nil {
		return perr
	}
	l.queueNPCWarp(npc, blk)
	openEventRead(l.p, npc, blk.Msg.Pages("thanks"), false)
	return nil
}

// eventMessage 是 MESSAGE 块（原版 NPC_TypeCheck 的 MESSAGE 分支）：先做交易，再设进行中（事件号不是 -1 时）、
// 执行 EndSetFlg、CleanFlg，最后说 NomalMsg（没有时说 NomalWindowMsg，可能分页）。没有台词的块什么都不做。
func (s *Service) eventMessage(ctx context.Context, l *Live, r *record, npc gamedata.NPC, ev *gamedata.EventScript, blk *gamedata.EventBlock) *protocol.Error {
	pages := blk.Msg.Pages("nomal")
	if len(pages) == 0 {
		pages = blk.Msg.Pages("nomal_window")
	}
	if len(pages) == 0 {
		return nil
	}
	plan, fail, ok := planEvent(r, blk, s.eventSelOf(l, r, blk), s.eventRand)
	if !ok {
		openEventText(l.p, npc, fail)
		return nil
	}
	prog := progressOf(r)
	applyEventEdit(prog, editSetDoing, blk.Event)
	endSetFlg(prog, blk.Event, blk.EndSet)
	cleanFlg(prog, blk.Clean)
	if perr := s.commitEvent(ctx, l, r, eventResult{items: plan.items, gold: plan.gold, charm: r.Charm, prog: prog, pets: plan.pets, notes: plan.notes}); perr != nil {
		return perr
	}
	l.queueNPCWarp(npc, blk)
	openEventTalk(l.p, r, npc, ev, pages)
	return nil
}

// eventClean 是重置（原版 MsgDisp 12）：CleanFlg 列出的事件清完成和进行中（可以重做），说 CleanFlgMsg。
func (s *Service) eventClean(ctx context.Context, l *Live, r *record, npc gamedata.NPC, blk *gamedata.EventBlock) *protocol.Error {
	if blk.Msg.CleanFlg == "" {
		return nil
	}
	prog := progressOf(r)
	cleanFlg(prog, blk.Clean)
	if perr := s.commitEvent(ctx, l, r, eventResult{items: r.Items, gold: r.Gold, charm: r.Charm, prog: prog, pets: r.Pets}); perr != nil {
		return perr
	}
	openEventText(l.p, npc, blk.Msg.CleanFlg)
	return nil
}
