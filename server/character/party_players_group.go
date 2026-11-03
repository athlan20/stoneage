package character

// 真人组队的离队、踢人、解散、断线（STAGE-274，docs/social-players.md 2.3、2.7，docs/protocol.md 5.67）。
// 这些操作都在释放请求方角色锁之后进入 partyHub，推送按“先改队伍、再逐人推送、最后更新 players 索引”的顺序做。

import (
	"context"
	"sort"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// membersLocked 返回 p 所在真人队的成员副本。调用时持有 partyHub.mu。
func (h *partyHub) membersLocked(p *Presence) []*partyMember {
	pt := p.realParty.Load()
	if pt == nil || pt.indexOf(p) < 0 {
		return nil
	}
	return append([]*partyMember(nil), pt.members...)
}

// partyNote 是一条待推的 party_changed：推给 to，reason 与 who 按协议 5.67 的表。withParty 为真时带队伍视图。
type partyNote struct {
	to        *Presence
	reason    string
	who       string
	withParty bool
}

// partyDepart 让 p 离开真人队（离队、被请出、断线）。剩 1 人时队伍解散，剩下的人收到 disbanded。
// 队长离队时按入队先后顺位（队长永远是 index 0，离队后重新编号）。调用时不能持有任何角色锁。
func (s *Service) partyDepart(p *Presence, reason string) {
	h := s.parties
	h.mu.Lock()
	pt := p.realParty.Load()
	if pt == nil || pt.indexOf(p) < 0 {
		h.mu.Unlock()
		return
	}
	idx := pt.indexOf(p)
	gone := pt.members[idx]
	wasLeader := idx == 0
	rest := make([]*partyMember, 0, len(pt.members)-1)
	rest = append(rest, pt.members[:idx]...)
	rest = append(rest, pt.members[idx+1:]...)
	// 按入队先后重新编号（协议 5.67）：队长离队时新队长是最早入队的剩余成员；队长没走就留在第 0 位，只排其余队员。
	if wasLeader {
		sortMembersByJoin(rest)
	} else if len(rest) > 1 {
		sortMembersByJoin(rest[1:])
	}
	pt.members = rest
	p.realParty.Store(nil)

	notes := []partyNote{{to: p, reason: reason, who: gone.name}}
	if len(rest) == 1 { // 只剩 1 人：自动解散，剩下的人只收到 disbanded
		surv := rest[0]
		surv.p.realParty.Store(nil)
		pt.members = nil
		notes = append(notes, partyNote{to: surv.p, reason: partyDisbanded, who: gone.name})
	} else if len(rest) > 1 {
		memberReason := "member_left"
		if reason == "kicked" {
			memberReason = "member_kicked"
		}
		for _, m := range rest {
			notes = append(notes, partyNote{to: m.p, reason: memberReason, who: gone.name, withParty: true})
		}
		if wasLeader {
			for _, m := range rest {
				notes = append(notes, partyNote{to: m.p, reason: "leader_changed", who: rest[0].name, withParty: true})
			}
		}
	}
	h.mu.Unlock()

	s.pushPartyNotes(notes, rest, gone)
}

// sortMembersByJoin 按入队序号排序（转让队长会打乱 members 的顺序，离队后要按入队先后重新编号）。
func sortMembersByJoin(ms []*partyMember) {
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].joined < ms[j].joined })
}

// pushPartyNotes 按顺序推送 party_changed，再更新 players 索引。rest 是剩下的成员，gone 是离开的人（也要更新）。
func (s *Service) pushPartyNotes(notes []partyNote, rest []*partyMember, gone *partyMember) {
	var view *playersPartyView
	if len(rest) > 1 {
		view = partyViewOf(rest, false)
	}
	for _, n := range notes {
		n.to.mu.Lock()
		data := map[string]any{"reason": n.reason, "who": n.who, "party": nil}
		if n.withParty && view != nil {
			data["party"] = view
		}
		n.to.pushAll("party_changed", data)
		n.to.mu.Unlock()
	}
	touched := append([]*partyMember{gone}, rest...)
	for _, m := range touched {
		m.p.mu.Lock()
		m.p.players.touch(m.p)
		m.p.mu.Unlock()
	}
}

// partyActorInParty 检查发起方有真人队，返回成员快照；没有真人队时回复 NOT_IN_PARTY（分身队用 party_disband）。
func (s *Service) partyActorInParty(a *partyActor) ([]*partyMember, *protocol.Error) {
	ms := s.parties.snapshot(a.p)
	if len(ms) == 0 {
		return nil, protocol.Errorf(protocol.CodeNotInParty, "没有真人队（分身队用 party_disband 解散）")
	}
	return ms, nil
}

// handlePartyLeave 离队（协议 5.67）：任何成员都能离队，队长离队则顺位接任。
func (s *Service) handlePartyLeave(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	if perr := s.partyPlayersGate(); perr != nil {
		return nil, perr
	}
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	ms, perr := s.partyActorInParty(a)
	if perr != nil {
		return nil, perr
	}
	if partyBattling(ms) {
		return nil, protocol.Errorf(protocol.CodePartyInBattle, "队伍战斗中不能离队")
	}
	s.partyDepart(a.p, "left")
	return map[string]any{}, nil
}

// handlePartyKick 请出队员（协议 5.67）：只有队长，只能请队员，不能请自己。
func (s *Service) handlePartyKick(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
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
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字符串字段 name（队员角色名）")
	}
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	ms, perr := s.partyActorInParty(a)
	if perr != nil {
		return nil, perr
	}
	switch {
	case ms[0].p != a.p:
		return nil, protocol.Errorf(protocol.CodeNotPartyLeader, "只有队长能请出队员")
	case in.Name == a.name:
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "不能请出自己（离队用 party_leave）")
	}
	var target *partyMember
	for _, m := range ms[1:] {
		if m.name == in.Name {
			target = m
		}
	}
	if target == nil {
		return nil, protocol.Errorf(protocol.CodeNotPartyMember, "%s 不是队员", in.Name)
	}
	if partyBattling(ms) {
		return nil, protocol.Errorf(protocol.CodePartyInBattle, "队伍战斗中不能请出队员")
	}
	s.partyDepart(target.p, "kicked")
	return map[string]any{}, nil
}

// partyDisbandPlayers 解散真人队（party_disband，协议 5.67）：只有队长能解散，全体收到 disbanded。
func (s *Service) partyDisbandPlayers(sess *gateway.Session) (any, error) {
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	ms, perr := s.partyActorInParty(a)
	if perr != nil {
		return nil, perr
	}
	if ms[0].p != a.p {
		return nil, protocol.Errorf(protocol.CodeNotPartyLeader, "只有队长能解散队伍")
	}
	if partyBattling(ms) {
		return nil, protocol.Errorf(protocol.CodePartyInBattle, "队伍战斗中不能解散")
	}
	h := s.parties
	h.mu.Lock()
	ms = h.membersLocked(a.p)
	if pt := a.p.realParty.Load(); pt != nil {
		pt.members = nil
	}
	for _, m := range ms {
		m.p.realParty.Store(nil)
	}
	h.mu.Unlock()
	notes := make([]partyNote, 0, len(ms))
	for _, m := range ms {
		notes = append(notes, partyNote{to: m.p, reason: partyDisbanded, who: a.name})
	}
	for _, n := range notes {
		n.to.mu.Lock()
		n.to.pushAll("party_changed", map[string]any{"reason": partyDisbanded, "who": n.who, "party": nil})
		n.to.mu.Unlock()
	}
	for _, m := range ms {
		m.p.mu.Lock()
		m.p.players.touch(m.p)
		m.p.mu.Unlock()
	}
	return map[string]any{}, nil
}

// partyOnGone 在角色最后一条连接离开世界之后调用（不持有角色锁）：断线 = 离队（协议 5.67），
// 它的待处理请求全部 cancelled 推给另一方。
func (s *Service) partyOnGone(p *Presence) {
	h := s.parties
	h.mu.Lock()
	var gone []*partyRequest
	for id, r := range h.requests {
		if r.from == p || r.to == p {
			delete(h.requests, id)
			gone = append(gone, r)
		}
	}
	h.mu.Unlock()
	for _, r := range gone {
		other := r.to
		if r.to == p {
			other = r.from
		}
		s.pushPartyResult(r, "cancelled", "", other)
	}
	s.partyDepart(p, "left")
}
