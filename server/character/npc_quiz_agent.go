package character

// STAGE-332：agent 连接对猜谜大师 talk 时直接认定（方案 A，docs/protocol.md 5.33）。题库是现成的静态数据，agent 答 100 题
// 没有游戏价值，所以不出题：条件通过就直接发奖，走和人类答满 100 题同一套发奖（quizGrant）。
// 校验顺序：任务 2 已完成（talk 时已查，见 talkQuiz）→ 有入门证 2753（没有时说 NoEntryMsg，不改数据）→
// 已经持有谜之箱 2759 时只说台词、不再扣证、不再发（幂等）→ 扣 2753、发 2759，推 inventory_changed，消息栏说认定台词。
// 之后仍由大师的弟子交箱子，任务 3 由弟子完成。原版大师本身不查宠物等级（77 级的检查在伊那多那里，给入门证时做），这里不查宠物。

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// quizBoxItem 是谜之箱子（原版 GetItem:2759），答满 100 题或 agent 直接认定时发给玩家。
const quizBoxItem = 2759

// quizAgentKind 是 agent 直接认定的结果种类。
type quizAgentKind int

const (
	qaGrant   quizAgentKind = iota // 通过：扣证、发箱子，next 是交易结果
	qaHolding                      // 已经持有箱子：只说台词，不改数据
	qaNoEntry                      // 没有入门证：原版 NoEntryMsg，不改数据
	qaFull                         // 背包放不下箱子：不改数据（扣证也放不下时才会出现）
)

// quizAgentOutcome 是 quizAgentPlan 的结果；msg 是要说的台词（qaGrant、qaHolding 时是认定台词）。
type quizAgentOutcome struct {
	kind quizAgentKind
	next eventResult
	msg  string
}

// quizAgentPlan 算 agent 直接认定的结果（纯计算，不碰数据库）。
func quizAgentPlan(r *record, cfg *gamedata.NpcQuiz) quizAgentOutcome {
	if quizHolds(r, quizBoxItem) {
		return quizAgentOutcome{kind: qaHolding, msg: quizBorderText(cfg, cfg.Quiznum)}
	}
	if !quizEntryOK(r, cfg.EntryItem) {
		return quizAgentOutcome{kind: qaNoEntry, msg: quizText(cfg.NoEntryMsg)}
	}
	next, ok := quizGrant(r, quizEntryResult(r, cfg.EntryItem).items, quizBoxItem)
	if !ok {
		return quizAgentOutcome{kind: qaFull, msg: "道具栏已满"}
	}
	return quizAgentOutcome{kind: qaGrant, next: next, msg: quizBorderText(cfg, cfg.Quiznum)}
}

// quizAgentPass 是 agent 连接 talk 猜谜大师的处理。调用时要持有 p.mu。
func (s *Service) quizAgentPass(ctx context.Context, l *Live, r *record, npc gamedata.NPC, cfg *gamedata.NpcQuiz) *protocol.Error {
	p := l.p
	out := quizAgentPlan(r, cfg)
	switch out.kind {
	case qaNoEntry:
		quizSay(p, npc, out.msg) // 原版 NoEntryMsg 窗口（没有选项）
	case qaFull:
		p.pushAll("notice", map[string]string{"level": "info", "text": out.msg})
	case qaGrant:
		if perr := s.commitEvent(ctx, l, r, out.next); perr != nil {
			return perr
		}
		p.pushAll("notice", map[string]string{"level": "info", "text": npcDisplayName(npc) + "：" + out.msg})
	case qaHolding:
		p.pushAll("notice", map[string]string{"level": "info", "text": npcDisplayName(npc) + "：" + out.msg})
	}
	return nil
}
