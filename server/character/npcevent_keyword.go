package character

// 任务 NPC 的暗号（STAGE-92，docs/quest.md「暗号」）：脚本里带 KeyWord 的块，玩家对 NPC 说出逐字相同的话
// 才会考虑（原版 npc_exchangeman.c 的 strcmp）。闲聊窗口和 MESSAGE 台词的最后一页上有“说暗号”选项（见
// npcevent_pages.go），选了以后窗口转成文字输入（accepts_text），玩家用 npc_reply 说出话来；没有块匹配时说闲话（NomalMainMsg）。

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// eventSpeakPrompt 是文字输入窗口的正文（docs/protocol.md 5.34 的 accepts_text）。
const eventSpeakPrompt = "你想说什么？"

// hasPendingKeyword 报告脚本里有没有还没完成的暗号块（完成了的块不会再被考虑，不给选项）。
func hasPendingKeyword(r *record, ev *gamedata.EventScript) bool {
	for i := range ev.Blocks {
		b := &ev.Blocks[i]
		if b.Keyword != "" && !(b.Event != -1 && r.eventDone(b.Event)) {
			return true
		}
	}
	return false
}

// openEventSpeak 开文字输入窗口（玩家选了“说暗号”之后）：窗口带 accepts_text，等 npc_reply。
func openEventSpeak(p *Presence, npc gamedata.NPC) {
	p.npcWindow = &npcWindow{npc: npc.ID, event: &eventSession{ask: askSpeak, block: -1}}
	view := npcWindowView(npc, eventSpeakPrompt)
	view["accepts_text"] = true
	p.pushAll("npc_window", view)
}

// eventSpeak 处理玩家说出的话（npc_reply 的暗号窗口）：先关掉输入窗口，再按这句话选块。
// 命中暗号块就按那一块执行（和普通块一样，交易、问答）；没有块命中就说闲聊句，窗口上仍带“说暗号”。
// 不支持的块命中时拒绝（和普通对话一样，关窗、不改数据）。
func (s *Service) eventSpeak(ctx context.Context, l *Live, r *record, npc gamedata.NPC, text string) *protocol.Error {
	ev, ok := gamedata.NpcEventOf(npc.ID)
	idx, refuse := -1, true
	if ok {
		idx, refuse = s.pickEventBlock(r, ev, text)
	}
	l.p.pushWindowClosed(WindowReasonChosen)
	if !ok || refuse {
		return nil
	}
	if idx < 0 {
		openEventTalk(l.p, r, npc, ev, []string{s.eventMainLine(ev)})
		return nil
	}
	return s.runEventBlock(ctx, l, r, npc, ev, idx, text)
}
