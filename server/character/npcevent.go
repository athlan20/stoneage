package character

// 任务 NPC 的对话（STAGE-90，docs/quest.md）：对照 npc_exchangeman.c 的 NPC_TypeCheck（选块、说话）、
// NPC_ExChangeManWindowTalked 和 NPC_MsgDisp（问答与交易的每一步）。每一步都按条件重新选块，块已经不是
// 开窗时的那一块就只关窗口。窗口的文字和选项见 docs/protocol.md 5.34、5.35。暗号（STAGE-92）见 npcevent_keyword.go。

import (
	"context"
	"math/rand"
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// eventAsk 是窗口上等玩家选“是/否”的那一问。
type eventAsk int

const (
	askRequest eventAsk = iota + 1 // 接任务：REQUEST，还没进行中（RequestMsg）
	askStop                        // 放弃：REQUEST，已经进行中（StopMsg）
	askAccept                      // 交付：ACCEPT（AcceptMsg）
	askClean                       // 重置：CLEAN（CleanMainMsg）
	askRead                        // 读完的台词（感谢话、MESSAGE 台词）：只翻页，没有问答；可能带“说暗号”（STAGE-92）
	askSpeak                       // 等玩家说暗号：窗口接受文字回复（npc_reply）
)

// eventSession 记在对话窗口上：等的是哪一问，开窗时命中的块序号（没有块是 -1），以及玩家说的暗号
// （普通对话为空）。重新选块时要用同一句话，块才会和开窗时一样。pages 是窗口的台词（每页一条），
// page 是当前页，opts 是当前页的选项（和窗口的 options 一致，见 npcevent_pages.go）。
type eventSession struct {
	ask     eventAsk
	block   int
	said    string
	pages   []string
	page    int
	keyword bool // askRead：读完的最后一页上是否带“说暗号”
	opts    []eventOpt
}

// talkEvent 和任务 NPC 说话：按脚本选块，说闲聊句，或者开一个问答窗口，或者执行 MESSAGE 块。
func (s *Service) talkEvent(ctx context.Context, l *Live, r *record, npc gamedata.NPC, ev *gamedata.EventScript) *protocol.Error {
	idx, refuse := s.pickEventBlock(r, ev, "")
	if refuse {
		return protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 还不能对话", npcDisplayName(npc))
	}
	if idx < 0 {
		openEventTalk(l.p, r, npc, ev, []string{s.eventMainLine(ev)})
		return nil
	}
	return s.runEventBlock(ctx, l, r, npc, ev, idx, "")
}

// runEventBlock 执行命中的块：MESSAGE 直接做，ACCEPT、REQUEST、CLEAN 开问答窗口。said 是开窗时玩家说的暗号
// （普通对话为空），问答的后续选择要按它重新选块。
func (s *Service) runEventBlock(ctx context.Context, l *Live, r *record, npc gamedata.NPC, ev *gamedata.EventScript, idx int, said string) *protocol.Error {
	blk := &ev.Blocks[idx]
	cost := stoneCost(blk.DelStone, r.Level)
	switch blk.Type {
	case "MESSAGE":
		return s.eventMessage(ctx, l, r, npc, ev, blk)
	case "ACCEPT":
		openEventAsk(l.p, npc, askAccept, idx, said, eventTexts(blk.Msg, "accept", cost))
	case "REQUEST":
		if r.eventDoing(blk.Event) {
			if blk.Msg.Stop == "" { // 原版没有 StopMsg 时窗口是空的，这里直接不说话
				openEventText(l.p, npc, "")
				return nil
			}
			openEventAsk(l.p, npc, askStop, idx, said, []string{eventText(blk.Msg.Stop, cost)})
		} else {
			openEventAsk(l.p, npc, askRequest, idx, said, eventTexts(blk.Msg, "request", cost))
		}
	case "CLEAN":
		openEventAsk(l.p, npc, askClean, idx, said, []string{eventText(blk.Msg.CleanMain, cost)})
	}
	return nil
}

// pickEventBlock 按原版 NPC_TypeCheck 的顺序选块：事件已完成的块跳过（NPC_EventCheckFlg），暗号块只在玩家
// 说的话和暗号逐字相同时才考虑（原版 strcmp），否则跳过；条件不满足的跳过，命中第一个就用。遇到不支持的块
// （而且没被跳过）时拒绝对话，不让后面的块替它说话。普通对话的 said 为空，暗号块一律跳过。
// 返回块序号（-1 表示没有命中）和是否拒绝。
func (s *Service) pickEventBlock(r *record, ev *gamedata.EventScript, said string) (int, bool) {
	now := s.opts.Now()
	for i := range ev.Blocks {
		b := &ev.Blocks[i]
		if b.Event != -1 && r.eventDone(b.Event) {
			continue
		}
		if b.Keyword != "" && b.Keyword != said {
			continue
		}
		if b.Unsupported != "" {
			return -1, true
		}
		if eventCondHolds(r, b.Cond, now) {
			return i, false
		}
	}
	return -1, false
}

// eventChoose 回应问答窗口的选项（原版 NPC_ExChangeManWindowTalked）。选之前先按条件重新选块，
// 块已经不是开窗时的那一块（条件变了）就只关窗口，什么都不做。
func (s *Service) eventChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, w *npcWindow, no int) *protocol.Error {
	if no < 1 || no > len(w.options) {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
	}
	act := w.event.opts[no-1].act
	if w.event.ask == askRead { // 读完的台词：不按块走（条件变了也照常翻完），翻页或转成文字输入
		l.p.pushWindowClosed(WindowReasonChosen)
		if act == actKeyword {
			openEventSpeak(l.p, npc)
		} else {
			openEventPage(l.p, npc, w.event.turn(act))
		}
		return nil
	}
	ev, ok := gamedata.NpcEventOf(npc.ID)
	idx, refuse := -1, true
	if ok {
		idx, refuse = s.pickEventBlock(r, ev, w.event.said)
	}
	l.p.pushWindowClosed(WindowReasonChosen)
	if !ok || refuse || idx != w.event.block {
		return nil
	}
	if act == actNext || act == actPrev { // 问答的台词翻页：块还是开窗时的那一块，才翻到下一页
		openEventPage(l.p, npc, w.event.turn(act))
		return nil
	}
	blk := &ev.Blocks[idx]
	yes := act == actYes
	switch w.event.ask {
	case askRequest:
		if yes {
			return s.eventRequestYes(ctx, l, r, npc, blk)
		}
	case askStop:
		if yes {
			return s.eventGiveUp(ctx, l, r, npc, blk)
		}
		openEventText(l.p, npc, blk.Msg.NoStop)
	case askAccept:
		if yes {
			return s.eventAccept(ctx, l, r, npc, blk)
		}
	case askClean:
		if yes {
			return s.eventClean(ctx, l, r, npc, blk)
		}
	}
	return nil
}

// eventMainLine 是没有块命中时说的闲聊句：逗号分句，随机一句（原版 NomalMainMsg）。
func (s *Service) eventMainLine(ev *gamedata.EventScript) string {
	if len(ev.Main) == 0 {
		return "……"
	}
	return ev.Main[s.eventRand(len(ev.Main))]
}

// npcEventOpen 报告任务脚本是否放开：测试用 Options.NpcEventEnabled 换白名单，零值用 gamedata 的白名单。
func (s *Service) npcEventOpen(file string) bool {
	if s.opts.NpcEventEnabled != nil {
		return s.opts.NpcEventEnabled(file)
	}
	return gamedata.NpcEventEnabled(file)
}

// eventRand 是脚本的随机源（选闲聊句、随机道具）：测试里跟 NpcTalkRnd 一样固定，生产环境用全局随机。
func (s *Service) eventRand(n int) int {
	if f := s.talkRnd(); f != nil {
		return f(n)
	}
	return rand.Intn(n)
}

// eventText 替换台词里的 %8d（原版 sprintf 的金币占位，取 DelStone 的费用）。
func eventText(text string, cost int) string {
	return strings.ReplaceAll(text, "%8d", strconv.Itoa(cost))
}

// openEventText 开一个只能关掉的窗口说话；台词为空时原版不弹窗，这里只关掉原来的窗口。
func openEventText(p *Presence, npc gamedata.NPC, text string) {
	if text == "" {
		p.pushWindowClosed(WindowReasonClosed)
		return
	}
	p.npcWindow = &npcWindow{npc: npc.ID}
	p.pushAll("npc_window", npcWindowView(npc, text))
}
