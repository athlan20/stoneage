package character

// 真人组队的转让队长（STAGE-277，docs/protocol.md 5.67 party_leader）：只换新队长和原队长的位置，
// 其余队员相对顺序不变，位置（坐标）不动。跟随关系不变：队员仍跟随队长，新队长的位置变化由 followRealParty 同步。

import (
	"context"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// handlePartyLeader 转让队长（协议 5.67）：只有队长能转，对象必须是队员，不能转给自己，战斗中不能转。
func (s *Service) handlePartyLeader(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
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
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字符串字段 name（新队长的角色名）")
	}
	a, err := s.partyActorOf(sess)
	if err != nil {
		return nil, err
	}
	ms, perr := s.partyActorInParty(a)
	if perr != nil {
		return nil, perr
	}
	if perr := partyLeaderCheck(a.p, ms, in.Name); perr != nil {
		return nil, perr
	}
	ms, perr = s.partyTransferLeader(a.p, in.Name)
	if perr != nil {
		return nil, perr
	}
	s.pushLeaderChanged(ms, in.Name)
	return map[string]any{}, nil
}

// partyLeaderCheck 是转让队长的校验，顺序同协议 5.67：不是队长 → 队伍战斗中 → 转给自己 → 对象不是队员。
// ms 是发起人所在真人队的成员（非空），ms[0] 是队长。
func partyLeaderCheck(me *Presence, ms []*partyMember, name string) *protocol.Error {
	switch {
	case ms[0].p != me:
		return protocol.Errorf(protocol.CodeNotPartyLeader, "只有队长能转让队长")
	case partyBattling(ms):
		return protocol.Errorf(protocol.CodePartyInBattle, "队伍战斗中不能转让队长")
	case name == ms[0].name:
		return protocol.Errorf(protocol.CodeInvalidTarget, "不能把队长转给自己")
	}
	for _, m := range ms[1:] {
		if m.name == name {
			return nil
		}
	}
	return protocol.Errorf(protocol.CodeNotPartyMember, "%s 不是队员", name)
}

// partyTransferLeader 在 partyHub 锁里重新校验（发起人仍是队长、对象仍是队员），把对象排到第 0 位、
// 原队长排到第 1 位，其余队员保持原来的相对顺序。返回新的成员快照。
func (s *Service) partyTransferLeader(leader *Presence, name string) ([]*partyMember, *protocol.Error) {
	h := s.parties
	h.mu.Lock()
	defer h.mu.Unlock()
	pt := leader.realParty.Load()
	if pt == nil || pt.indexOf(leader) != 0 {
		return nil, protocol.Errorf(protocol.CodeNotPartyLeader, "你已经不是队长")
	}
	var target *partyMember
	rest := make([]*partyMember, 0, len(pt.members))
	for _, m := range pt.members[1:] {
		if m.name == name && target == nil {
			target = m
			continue
		}
		rest = append(rest, m)
	}
	if target == nil {
		return nil, protocol.Errorf(protocol.CodeNotPartyMember, "%s 不是队员", name)
	}
	pt.members = append([]*partyMember{target, pt.members[0]}, rest...)
	return append([]*partyMember(nil), pt.members...), nil
}

// pushLeaderChanged 在转让之后推送：全体队员（含原队长）收到 party_changed（leader_changed，who 为新队长），
// 同图玩家的 players 摘要随之更新。调用时不持有任何锁；发起人的 party_changed 在响应之前推出。
func (s *Service) pushLeaderChanged(ms []*partyMember, who string) {
	view := partyViewOf(ms, false)
	for _, m := range ms {
		m.p.mu.Lock()
		m.p.pushAll("party_changed", map[string]any{"reason": "leader_changed", "who": who, "party": view})
		m.p.mu.Unlock()
	}
	for _, m := range ms {
		m.p.mu.Lock()
		m.p.players.touch(m.p)
		m.p.mu.Unlock()
	}
}
