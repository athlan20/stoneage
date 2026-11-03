package character

// 真人同场战斗中的离场（STAGE-275，docs/social-players.md 2.3、2.6，docs/protocol.md 10.14）：
// 逃跑（个人，队长逃跑解散）、超时踢出（队长超时解散）、掉线（立即退场，队长掉线解散）。
// 离场的真人先结束自己的战斗状态（写回、battle_end），再推其余人 battle_leave，最后改队伍、推 party_changed。
// 所有函数调用时持有 lockAll（掉线由 partyOffline 自己拿锁）。

import (
	"github.com/athlan20/stoneage/server/battle"
)

// partyChangedCause 是战斗中离队的 party_changed.cause（10.14）。
var partyCauses = map[string]string{
	"escape":  "battle_escape",
	"timeout": "battle_timeout",
	"offline": "battle_offline",
}

// applyDepartures 处理引擎给出的离场（TakeDepartures / ExpireDue / Offline 的结果）。调用时持有 lockAll。
func (pb *partyBattle) applyDepartures(deps []battle.Departure) {
	for _, d := range deps {
		if d.No < 0 || d.No >= len(pb.seats) || pb.gone[d.No] {
			continue
		}
		p := pb.seats[d.No]
		var end *battle.EndView
		if d.Cause != "offline" {
			end = pb.b.EndFor(d.No) // escaped / forced_out，当场给出
		}
		pb.endMember(d.No, end, d.Cause != "offline")
		if d.Cause != "escape" { // 逃跑不推 battle_leave（10.14），以 party_changed 为准
			pb.pushEach("battle_leave", func(int) any { return pb.b.LeaveFor(d) })
		}
		pb.s.partyLeaveInBattle(p, d)
	}
}

// partyLeaveInBattle 把离场的真人从真人队里摘掉，并推 party_changed（离场的人与留下的人各一套）。
// 队长离场、或队伍只剩 1 人时整队解散，其余人留在战斗里。调用时持有 lockAll（全部成员都是这场战斗的人）。
func (s *Service) partyLeaveInBattle(p *Presence, d battle.Departure) {
	cause := partyCauses[d.Cause]
	h := s.parties
	h.mu.Lock()
	pt := p.realParty.Load()
	if pt == nil || pt.indexOf(p) < 0 {
		h.mu.Unlock()
		return
	}
	gone := pt.members[pt.indexOf(p)]
	var rest []*partyMember
	for _, m := range pt.members {
		if m.p != p {
			rest = append(rest, m)
		}
	}
	disband := d.Disband || len(rest) < 2 // 队长离场或只剩 1 人：整队解散（其余人留在战斗里）
	if disband {
		for _, m := range pt.members {
			m.p.realParty.Store(nil)
		}
		pt.members = nil
	} else {
		pt.members = rest
		p.realParty.Store(nil)
	}
	h.mu.Unlock()

	// 离场的人：逃跑、被踢收到 party_changed（reason 为 left / kicked），掉线的不推送。
	if d.Cause != "offline" {
		reason := "left"
		if d.Cause == "timeout" {
			reason = "kicked"
		}
		p.pushAll("party_changed", map[string]any{"reason": reason, "who": gone.name, "party": nil, "cause": cause})
	}
	// 留下的人：队长离场（或只剩 1 人）解散，其余为成员离队 / 被踢。
	if disband {
		for _, m := range rest {
			m.p.pushAll("party_changed", map[string]any{"reason": partyDisbanded, "who": gone.name, "party": nil, "cause": cause})
		}
		return
	}
	memberReason := "member_left"
	if d.Cause == "timeout" {
		memberReason = "member_kicked"
	}
	view := partyViewOf(rest, true)
	for _, m := range rest {
		m.p.pushAll("party_changed", map[string]any{"reason": memberReason, "who": gone.name, "party": view, "cause": cause})
	}
}

// partyOffline 是真人掉线时的离场（10.14）：立即退场，不推送，留下的人收到 battle_leave 和 party_changed。
// 调用方不持有 p.mu（锁顺序：战斗锁在前）。p 不在这场战斗中时什么都不做。
func (s *Service) partyOffline(pb *partyBattle, p *Presence) {
	unlock := pb.lockAll()
	defer unlock()
	if pb.done || p.battle == nil || p.battle.pb != pb {
		return
	}
	no := pb.noOf(p)
	if no < 0 {
		return
	}
	d, ok := pb.b.Offline(no)
	if !ok {
		return
	}
	pb.applyDepartures([]battle.Departure{d})
	if pb.done {
		return
	}
	if pb.b.Over() {
		pb.finish()
		return
	}
	if pb.b.AllSubmitted() {
		pb.settle()
	}
}
