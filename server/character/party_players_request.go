package character

// 真人组队的请求（STAGE-274，docs/protocol.md 5.67）：邀请、申请、答复、撤回，以及 30 秒到期。
// 请求表在 partyHub 上（party_players.go），只在内存里。

import (
	"context"
	"fmt"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// partyPlayersGate 是六个组队请求的开关检查：功能关闭时回 WRONG_STATE（协议 5.67）。
func (s *Service) partyPlayersGate() *protocol.Error {
	if !s.partyPlayersOn.Load() {
		return protocol.Errorf(protocol.CodeWrongState, "真人组队尚未开放")
	}
	return nil
}

// reqView 是请求对象（协议 5.67 的 request）：不带 party，party 只在推给对象的那条里。
func reqView(r *partyRequest) map[string]any {
	return map[string]any{
		"id": r.id, "kind": r.kind, "from": r.fromName, "to": r.toName,
		"expires_in": int(partyRequestTTL / time.Second),
	}
}

func (s *Service) handlePartyInvite(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	return s.partyStartRequest(sess, req, "invite")
}

func (s *Service) handlePartyJoin(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	return s.partyStartRequest(sess, req, "apply")
}

// partyStartRequest 处理 party_invite 与 party_join（两者只在校验细节上不同，协议 5.67 的校验顺序）。
func (s *Service) partyStartRequest(sess *gateway.Session, req *protocol.Request, kind string) (any, error) {
	if perr := s.partyPlayersGate(); perr != nil {
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
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	me := peerOf(a.p)
	mine := s.parties.snapshot(a.p)
	if perr := partyCheckSelf(me, mine); perr != nil {
		return nil, perr
	}
	tgt := s.findOnline(in.Name)
	if tgt == nil || tgt.hidden.Load() { // 隐身者视同不存在，不泄露他在线（协议 5.69）
		return nil, protocol.Errorf(protocol.CodePlayerNotFound, "没有这个在线角色")
	}
	tp := peerOf(tgt)
	theirs := s.parties.snapshot(tgt)
	if perr := partyTargetCheck(me, tp, tgt == a.p, kind); perr != nil {
		return nil, perr
	}
	if kind == "invite" {
		switch {
		case tp.clone || len(theirs) > 0:
			return nil, protocol.Errorf(protocol.CodeTargetInParty, "对方已经有队伍了")
		case len(mine) > 0 && mine[0].p != a.p:
			return nil, protocol.Errorf(protocol.CodeNotPartyLeader, "只有队长能邀请")
		case len(mine) >= partyMaxPlayers:
			return nil, protocol.Errorf(protocol.CodePartyFull, "队伍已满 %d 人", partyMaxPlayers)
		case me.clone:
			return nil, protocol.Errorf(protocol.CodeAlreadyInParty, "有分身队，先解散再邀请真人")
		}
	} else {
		switch {
		case me.clone || len(mine) > 0:
			return nil, protocol.Errorf(protocol.CodeAlreadyInParty, "已经有队伍了，先离队再申请")
		case len(theirs) == 0 || theirs[0].p != tgt || tp.clone:
			return nil, protocol.Errorf(protocol.CodeTargetNotLeader, "对方不是队长，或者还没有队伍")
		case len(theirs) >= partyMaxPlayers:
			return nil, protocol.Errorf(protocol.CodePartyFull, "对方队伍已满 %d 人", partyMaxPlayers)
		}
	}
	r, perr := s.newPartyRequest(a.p, a.name, tgt, tp.name, kind)
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"request": reqView(r)}, nil
}

// partyTargetCheck 是对方的前置检查（协议 5.67 的顺序）：同图、不是自己、对方拒绝邀请（只对邀请，STAGE-276，5.69）、对方战斗中。
func partyTargetCheck(me, tp peerState, self bool, kind string) *protocol.Error {
	switch {
	case tp.floor != me.floor:
		return protocol.Errorf(protocol.CodePlayerNotNearby, "对方不在同一张地图")
	case self:
		return protocol.Errorf(protocol.CodeInvalidTarget, "不能对自己发起组队")
	case kind == "invite" && tp.noInvite:
		return protocol.Errorf(protocol.CodeTargetRefuses, "对方不接收组队邀请")
	case tp.battle:
		return protocol.Errorf(protocol.CodeTargetInBattle, "对方在战斗中")
	case tp.trade: // STAGE-294：对方在交易中，不能被组队（docs/trade.md 2.5）
		return protocol.Errorf(protocol.CodeTargetInTrade, "对方在交易中")
	}
	return nil
}

// partyCheckSelf 是发起方自己的前置检查：倒下、战斗中、队伍战斗中（协议 5.67 的前三条）。
func partyCheckSelf(me peerState, mine []*partyMember) *protocol.Error {
	switch {
	case me.dead:
		return protocol.Errorf(protocol.CodeCharacterDead, "角色已经倒下，不能组队")
	case me.battle:
		return protocol.Errorf(protocol.CodeInBattle, "战斗中不能组队")
	case partyBattling(mine):
		return protocol.Errorf(protocol.CodePartyInBattle, "队伍战斗中不能变更")
	}
	return nil
}

// newPartyRequest 登记一条请求（检查待处理、收件上限、频率），推给对象。
func (s *Service) newPartyRequest(from *Presence, fromName string, to *Presence, toName, kind string) (*partyRequest, *protocol.Error) {
	h := s.parties
	now := s.opts.Now()
	key := pairKey{from: fromName, to: toName}
	h.mu.Lock()
	incoming := 0
	for _, q := range h.requests {
		if q.fromName == fromName && q.toName == toName {
			h.mu.Unlock()
			return nil, protocol.Errorf(protocol.CodeRequestPending, "已经有一条待处理的组队请求")
		}
		if q.toName == toName {
			incoming++
		}
	}
	if incoming >= partyMaxIncoming {
		h.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeTargetBusy, "对方待处理的组队请求已满 %d 条", partyMaxIncoming)
	}
	if t, ok := h.sent[key]; ok && now.Sub(t) < partyRateWindow {
		left := partyRateWindow - now.Sub(t)
		secs := int((left + time.Second - 1) / time.Second)
		h.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeRateLimited, "发得太快了，过一会儿再试").
			WithData(map[string]any{"retry_after_seconds": secs})
	}
	h.seq++
	r := &partyRequest{
		id: fmt.Sprintf("r-%d", h.seq), kind: kind, from: from, to: to,
		fromName: fromName, toName: toName, expires: now.Add(partyRequestTTL),
	}
	h.requests[r.id] = r
	h.sent[key] = now
	h.armLocked()
	// 推给对象的 party 摘要：邀请时是发起人的队伍，申请时是对方的队伍（没有时为 null）。
	owner := from
	if kind == "apply" {
		owner = to
	}
	summary := partySummary(h.membersLocked(owner))
	h.mu.Unlock()

	pushed := reqView(r)
	pushed["party"] = summary
	to.mu.Lock()
	to.pushAll("party_request", map[string]any{"request": pushed})
	to.mu.Unlock()
	return r, nil
}

// armLocked 在有待处理请求时挂上单个到期定时器（不为每条请求建 time.Timer）。调用时持有 h.mu。
func (h *partyHub) armLocked() {
	if h.timer != nil || len(h.requests) == 0 {
		return
	}
	h.timer = time.AfterFunc(time.Second, func() { h.s.expirePartyRequests() })
}

// expirePartyRequests 作废到期的请求，并推给双方（在线的）。测试可以直接调用，不用真等 30 秒。
func (s *Service) expirePartyRequests() {
	h := s.parties
	now := s.opts.Now()
	h.mu.Lock()
	h.timer = nil
	var expired []*partyRequest
	for id, r := range h.requests {
		if !now.Before(r.expires) {
			expired = append(expired, r)
			delete(h.requests, id)
		}
	}
	h.armLocked()
	h.mu.Unlock()
	for _, r := range expired {
		s.pushPartyResult(r, "expired", "", r.from, r.to)
	}
}

// pushPartyResult 推 party_request_result 给指定的角色（每人各一条，调用时不持有任何角色锁）。
func (s *Service) pushPartyResult(r *partyRequest, status, code string, to ...*Presence) {
	data := map[string]any{"id": r.id, "kind": r.kind, "status": status, "from": r.fromName, "to": r.toName}
	if code != "" {
		data["code"] = code
	}
	for _, p := range to {
		p.mu.Lock()
		p.pushAll("party_request_result", data)
		p.mu.Unlock()
	}
}

// takePartyRequest 取出并删除一条请求（只有 match 为真时才取）。找不到或不符合返回 nil。
func (s *Service) takePartyRequest(id string, match func(*partyRequest) bool) *partyRequest {
	h := s.parties
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.requests[id]
	if !ok || !match(r) {
		return nil
	}
	delete(h.requests, id)
	return r
}

// handlePartyReply 答复收到的请求（协议 5.67）：拒绝到此为止，接受时重新校验一次。
func (s *Service) handlePartyReply(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.partyPlayersGate(); perr != nil {
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
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字符串字段 id 和布尔字段 accept")
	}
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	r := s.takePartyRequest(in.ID, func(r *partyRequest) bool { return r.to == a.p })
	if r == nil {
		return nil, protocol.Errorf(protocol.CodeRequestNotFound, "请求不存在或已结束")
	}
	if !*in.Accept {
		s.pushPartyResult(r, "declined", "", r.from)
		return map[string]any{}, nil
	}
	view, perr := s.acceptPartyRequest(a.p, r)
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"party": view}, nil
}

// handlePartyCancel 撤回自己发出的请求（协议 5.67）。
func (s *Service) handlePartyCancel(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.partyPlayersGate(); perr != nil {
		return nil, perr
	}
	var in struct {
		ID string `json:"id"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.ID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字符串字段 id")
	}
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	r := s.takePartyRequest(in.ID, func(r *partyRequest) bool { return r.from == a.p })
	if r == nil {
		return nil, protocol.Errorf(protocol.CodeRequestNotFound, "请求不存在或已结束")
	}
	s.pushPartyResult(r, "cancelled", "", r.to)
	return map[string]any{}, nil
}
