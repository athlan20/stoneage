package character

// STAGE-294：交易的邀请、答复、撤回（docs/trade.md 2.2，docs/protocol.md 5.72）。
// 请求表在 tradeHub 上（trade.go），只在内存里。

import (
	"context"
	"fmt"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// tradeReqView 是请求对象（协议 5.72 的 request）。
func tradeReqView(r *tradeRequest) map[string]any {
	return map[string]any{
		"id": r.id, "from": r.fromName, "to": r.toName,
		"expires_in": int(tradeRequestTTL / time.Second),
	}
}

// handleTradeInvite 是 trade_invite（操作类，世界中）：校验顺序同 docs/trade.md 2.2。
func (s *Service) handleTradeInvite(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.tradeGate(); perr != nil {
		return nil, perr
	}
	var in struct {
		Name string `json:"name"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Name == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字符串字段 name（对方角色名）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := tradeInfoOf(l.p)
	switch {
	case me.dead:
		return nil, protocol.Errorf(protocol.CodeCharacterDead, "角色已经倒下，不能交易")
	case me.battle:
		return nil, protocol.Errorf(protocol.CodeInBattle, "战斗中不能交易")
	case me.trade != nil:
		return nil, protocol.Errorf(protocol.CodeInTrade, "交易中，先完成或取消交易")
	case me.real:
		return nil, protocol.Errorf(protocol.CodeAlreadyInParty, "真人队里不能交易，先离队")
	}
	tgt := s.findOnline(in.Name)
	if tgt == nil || tgt.hidden.Load() { // 隐身者视同不存在（协议 5.69）
		return nil, protocol.Errorf(protocol.CodePlayerNotFound, "没有这个在线角色")
	}
	to := tradeInfoOf(tgt)
	switch {
	case to.floor != me.floor:
		return nil, protocol.Errorf(protocol.CodePlayerNotNearby, "对方不在同一张地图")
	case tgt == l.p:
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "不能和自己交易")
	case to.noTrade:
		return nil, protocol.Errorf(protocol.CodeTargetRefuses, "对方不接收交易邀请")
	case to.battle:
		return nil, protocol.Errorf(protocol.CodeTargetInBattle, "对方在战斗中")
	case to.real:
		return nil, protocol.Errorf(protocol.CodeTargetInParty, "对方在真人队里，不能交易")
	case to.trade != nil:
		return nil, protocol.Errorf(protocol.CodeTargetInTrade, "对方已经在交易中")
	}
	r, perr := s.trades.newRequest(me, to)
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"request": tradeReqView(r)}, nil
}

// newRequest 登记一条邀请（检查待处理、收件上限、频率），推给对方。
func (h *tradeHub) newRequest(from, to tradeInfo) (*tradeRequest, *protocol.Error) {
	now := h.s.opts.Now()
	key := pairKey{from: from.name, to: to.name}
	h.mu.Lock()
	defer h.mu.Unlock()
	incoming := 0
	for _, q := range h.requests {
		if q.fromID == from.id && q.toID == to.id {
			return nil, protocol.Errorf(protocol.CodeRequestPending, "已经有一条待处理的交易邀请")
		}
		if q.toID == to.id {
			incoming++
		}
	}
	if incoming >= tradeMaxIncoming {
		return nil, protocol.Errorf(protocol.CodeTargetBusy, "对方待处理的交易邀请已满 %d 条", tradeMaxIncoming)
	}
	if t, ok := h.sent[key]; ok && now.Sub(t) < tradeRateWindow {
		secs := int((tradeRateWindow - now.Sub(t) + time.Second - 1) / time.Second)
		return nil, protocol.Errorf(protocol.CodeRateLimited, "发得太快了，过一会儿再试").
			WithData(map[string]any{"retry_after_seconds": secs})
	}
	h.seq++
	r := &tradeRequest{
		id: fmt.Sprintf("t-%d", h.seq), from: from.p, to: to.p,
		fromID: from.id, toID: to.id, fromName: from.name, toName: to.name,
		fromLevel: from.level, fromTrans: from.trans, expires: now.Add(tradeRequestTTL),
	}
	h.requests[r.id] = r
	h.sent[key] = now
	h.armLocked()
	pushed := tradeReqView(r)
	pushed["from_level"] = r.fromLevel
	pushed["from_transmigration"] = r.fromTrans
	pushTo(to.p, "trade_request", map[string]any{"request": pushed})
	return r, nil
}

// handleTradeReply 是 trade_reply：只有请求的对象能答复；接受时重新校验，通过后请求变成交易（id 沿用）。
func (s *Service) handleTradeReply(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.tradeGate(); perr != nil {
		return nil, perr
	}
	var in struct {
		ID     string `json:"id"`
		Accept *bool  `json:"accept"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.ID == "" || in.Accept == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字段 id（请求编号）和布尔字段 accept")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := tradeInfoOf(l.p)
	h := s.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.requests[in.ID]
	if r == nil || r.toID != me.id {
		return nil, protocol.Errorf(protocol.CodeRequestNotFound, "没有这条交易邀请，可能已经过期")
	}
	if !*in.Accept {
		delete(h.requests, r.id)
		pushTo(r.from, "trade_request_result", map[string]any{"id": r.id, "status": "declined", "from": r.fromName, "to": r.toName})
		return map[string]any{}, nil
	}
	tr, perr := h.acceptLocked(r, me)
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"trade": tradeViewFor(tr, me.id, s.opts.Now())}, nil
}

// acceptLocked 答复接受：双方重新校验，通过后建立交易，并清掉双方之间和双方参与的其他邀请。调用时持有 h.mu。
// 校验失败时请求作废，发起人收到 cancelled（带 code），返回同一个错误。
func (h *tradeHub) acceptLocked(r *tradeRequest, me tradeInfo) (*trade, *protocol.Error) {
	now := h.s.opts.Now()
	from := tradeInfoOf(r.from)
	fail := func(perr *protocol.Error) (*trade, *protocol.Error) {
		delete(h.requests, r.id)
		pushTo(r.from, "trade_request_result", map[string]any{
			"id": r.id, "status": "cancelled", "from": r.fromName, "to": r.toName,
			"code": perr.Code,
		})
		return nil, perr
	}
	switch {
	case me.dead || from.dead:
		return fail(protocol.Errorf(protocol.CodeCharacterDead, "有一方已经倒下，不能交易"))
	case me.battle || from.battle:
		return fail(protocol.Errorf(protocol.CodeInBattle, "有一方在战斗中，不能交易"))
	case me.real || from.real:
		return fail(protocol.Errorf(protocol.CodeAlreadyInParty, "有一方在真人队里，不能交易"))
	case me.trade != nil || from.trade != nil:
		return fail(protocol.Errorf(protocol.CodeTargetInTrade, "有一方已经在交易中"))
	}
	a := &tradeSide{CharID: from.id, Name: from.name, P: from.p}
	b := &tradeSide{CharID: me.id, Name: me.name, P: me.p}
	tr := newTrade(r.id, a, b, now)
	delete(h.requests, r.id)
	for id, q := range h.requests { // 双方其他待处理的邀请全部作废
		if q.fromID == from.id || q.fromID == me.id || q.toID == from.id || q.toID == me.id {
			delete(h.requests, id)
			other := q.to
			if q.toID == from.id || q.toID == me.id {
				other = q.from
			}
			pushTo(other, "trade_request_result", map[string]any{"id": id, "status": "cancelled", "from": q.fromName, "to": q.toName})
		}
	}
	h.attachLocked(tr)
	pushTo(r.from, "trade_request_result", map[string]any{"id": r.id, "status": "accepted", "from": r.fromName, "to": r.toName})
	pushTo(r.from, "trade_updated", map[string]any{"trade": tradeViewFor(tr, from.id, now)})
	h.armLocked()
	return tr, nil
}

// handleTradeCancel 是 trade_cancel：请求阶段只有发起人能撤回；交易阶段双方都能取消（整笔作废）。
func (s *Service) handleTradeCancel(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.tradeGate(); perr != nil {
		return nil, perr
	}
	var in struct {
		ID string `json:"id"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.ID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字段 id（交易编号）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := tradeInfoOf(l.p)
	h := s.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.requests[in.ID]; r != nil {
		if r.fromID != me.id {
			return nil, protocol.Errorf(protocol.CodeRequestNotFound, "只有发起人能撤回这条邀请")
		}
		delete(h.requests, r.id)
		pushTo(r.to, "trade_request_result", map[string]any{"id": r.id, "status": "cancelled", "from": r.fromName, "to": r.toName})
		h.armLocked()
		return map[string]any{}, nil
	}
	tr := h.byChar[me.id]
	if tr == nil || tr.ID != in.ID {
		return nil, protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	h.cancelLocked(tr, me.id, me.name, "CANCELLED")
	return map[string]any{}, nil
}
