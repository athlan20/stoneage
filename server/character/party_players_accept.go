package character

// 真人组队：答复者接受请求时的重新校验、建队或入队、加入时的位置（STAGE-274，docs/social-players.md 2.2、2.4）。

import (
	"github.com/athlan20/stoneage/server/protocol"
)

// acceptPartyRequest 在答复者 me 接受请求 r 时执行。请求已经从表里取出（不论成败都作废）。
// 先读双方的战斗、离线状态（各自加锁），再在 partyHub.mu 下校验并改队伍（队伍成员只在那把锁下读写）。
// 校验失败时发起人和答复者都收到 cancelled（带 code），并返回对应错误码。
func (s *Service) acceptPartyRequest(me *Presence, r *partyRequest) (*playersPartyView, *protocol.Error) {
	h := s.parties
	fromSt, meSt := peerOf(r.from), peerOf(me)
	perr := partyAcceptPrecheck(fromSt, meSt, h.snapshot(r.from), h.snapshot(me))
	var ms []*partyMember
	var joiner *Presence
	var joinerName, who, reason string
	if perr == nil {
		h.mu.Lock()
		fromMs, meMs := h.membersLocked(r.from), h.membersLocked(me)
		joiner = me
		switch r.kind {
		case "invite":
			joinerName = meSt.name
			switch {
			case len(fromMs) > 0 && fromMs[0].p != r.from:
				perr = protocol.Errorf(protocol.CodeNotPartyLeader, "发起人已经不是队长")
			case fromSt.clone:
				perr = protocol.Errorf(protocol.CodeAlreadyInParty, "发起人已经有分身队")
			case len(meMs) > 0 || meSt.clone:
				perr = protocol.Errorf(protocol.CodeAlreadyInParty, "你已经有队伍了")
			case len(fromMs) >= partyMaxPlayers:
				perr = protocol.Errorf(protocol.CodePartyFull, "队伍已满 %d 人", partyMaxPlayers)
			}
		default: // apply
			joiner, joinerName = r.from, fromSt.name
			switch {
			case len(meMs) == 0 || meMs[0].p != me:
				perr = protocol.Errorf(protocol.CodeNotPartyLeader, "你已经不是队长")
			case fromSt.clone || len(fromMs) > 0:
				perr = protocol.Errorf(protocol.CodeAlreadyInParty, "对方已经有队伍了")
			case len(meMs) >= partyMaxPlayers:
				perr = protocol.Errorf(protocol.CodePartyFull, "队伍已满 %d 人", partyMaxPlayers)
			}
		}
		if perr == nil {
			var pt *realParty
			if r.kind == "invite" && len(fromMs) == 0 {
				// 邀请的发起人还没有队伍：此刻建队，发起人是队长，答复者是第二个成员。
				pt = &realParty{members: []*partyMember{{p: r.from, name: r.fromName, joined: h.nextJoin()}}}
				r.from.realParty.Store(pt)
				reason, who = "created", meSt.name
			} else {
				pt = me.realParty.Load()
				if r.kind == "invite" {
					pt = r.from.realParty.Load()
				}
				reason, who = "member_joined", joinerName
			}
			pt.members = append(pt.members, &partyMember{p: joiner, name: joinerName, joined: h.nextJoin()})
			joiner.realParty.Store(pt)
			ms = append([]*partyMember(nil), pt.members...)
		}
		h.mu.Unlock()
	}
	if perr != nil {
		s.pushPartyResult(r, "cancelled", perr.Code, r.from, me)
		return nil, perr
	}

	// 答复者与队长不在同一张地图：传到队长的位置（协议 5.67「接受时的位置」）。
	if leader := ms[0].p; peerOf(leader).floor != peerOf(joiner).floor {
		s.moveToLeader(joiner, leader)
	}
	// 发起人先收到 accepted，再收到 party_changed；答复者先收到 party_changed，再收到响应（由调用方返回）。
	s.pushPartyResult(r, "accepted", "", r.from)
	view := partyViewOf(ms, false)
	for _, m := range ms {
		m.p.mu.Lock()
		m.p.pushAll("party_changed", map[string]any{"reason": reason, "who": who, "party": view})
		m.p.players.touch(m.p)
		m.p.mu.Unlock()
	}
	return view, nil
}

// partyAcceptPrecheck 是接受前读出的战斗、离开状态的检查（不涉及队伍结构，见 acceptPartyRequest 的加锁说明）。
func partyAcceptPrecheck(fromSt, meSt peerState, fromMs, meMs []*partyMember) *protocol.Error {
	switch {
	case fromSt.leaving:
		return protocol.Errorf(protocol.CodePlayerNotFound, "发起人已经离开世界")
	case meSt.battle:
		return protocol.Errorf(protocol.CodeInBattle, "战斗中不能加入队伍")
	case fromSt.battle:
		return protocol.Errorf(protocol.CodeTargetInBattle, "对方在战斗中")
	case partyBattling(fromMs) || partyBattling(meMs):
		return protocol.Errorf(protocol.CodePartyInBattle, "队伍战斗中不能变更")
	}
	return nil
}

// moveToLeader 把 p 传到队长 leader 的位置（加入时的同步）：先读队长的位置（自己加锁），再锁 p 改位置。两把锁不嵌套。
func (s *Service) moveToLeader(p, leader *Presence) {
	leader.mu.Lock()
	floor, x, y := leader.rec.Floor, leader.rec.X, leader.rec.Y
	leader.mu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	p.rec.Floor, p.rec.X, p.rec.Y = floor, x, y
	p.players.moved(p) // STAGE-273：换图，同图的人看到它离开和到来
	p.pushWindowClosed(WindowReasonMoved)
	if p.encounterOn {
		p.stopEncounter(encounterOffWarp)
	}
	p.pushAll("position_changed", followPositionOf(&p.rec))
}
