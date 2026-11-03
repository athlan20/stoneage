package character

// 真人组队的跟随（STAGE-274，docs/social-players.md 2.5，docs/protocol.md 5.67）：队长的位置变化后队员同步过去；
// 队员自己的换图、遇敌请求回复 PARTY_FOLLOWER。

import (
	"sort"

	"github.com/athlan20/stoneage/server/protocol"
)

// followPositionView 是跟随时的 position_changed：在普通位置上多一个 reason（协议 5.67）。
type followPositionView struct {
	positionView
	Reason string `json:"reason"`
}

// followPositionOf 取跟随用的位置推送内容。
func followPositionOf(r *record) followPositionView {
	return followPositionView{positionView: r.position(), Reason: "follow"}
}

// isFollower 报告 p 是不是真人队的队员（不是队长）。只读队伍，调用时可以持有也可以不持有 p.mu。
func (p *Presence) isFollower() bool {
	if p.parties == nil || p.realParty.Load() == nil {
		return false
	}
	ms := p.parties.snapshot(p)
	return len(ms) > 0 && ms[0].p != p
}

// followerError 是队员的换图、遇敌请求的检查：队员回复 PARTY_FOLLOWER（协议 5.67）。
func followerError(p *Presence) *protocol.Error {
	if p.isFollower() {
		return protocol.Errorf(protocol.CodePartyFollower, "跟随队长，离队后才能自己行动")
	}
	return nil
}

// followRealParty 在队长的位置变化之后同步队员（挂在 pushPosition 上，调用时持有队长的 mu）。
// 队员按 charID 升序加锁（队长之后唯一的嵌套，见 party_players.go 的锁顺序）。战斗中的队员不动，等战斗结束后再同步。
// 同步的队员收到 npc_window_closed（若开着窗口）、position_changed（reason follow），全体收到 party_changed（moved）。
func (p *Presence) followRealParty(r *record) {
	if p.parties == nil || p.realParty.Load() == nil {
		return
	}
	ms := p.parties.snapshot(p)
	if len(ms) < 2 || ms[0].p != p {
		return
	}
	followers := make([]*Presence, 0, len(ms)-1)
	for _, m := range ms[1:] {
		followers = append(followers, m.p)
	}
	sort.Slice(followers, func(i, j int) bool { return followers[i].charID < followers[j].charID })
	for _, f := range followers {
		f.mu.Lock()
	}
	defer func() {
		for _, f := range followers {
			f.mu.Unlock()
		}
	}()
	for _, f := range followers {
		if f.battle != nil {
			continue
		}
		f.rec.Floor, f.rec.X, f.rec.Y = r.Floor, r.X, r.Y
		f.players.moved(f)
		f.pushWindowClosed(WindowReasonMoved)
		f.pushAll("position_changed", followPositionOf(&f.rec))
	}
	// 所有成员的锁都已持有（队长在调用方、队员在上面），直接读，不再加锁。
	view := partyViewOf(ms, true)
	for _, m := range ms {
		m.p.pushAll("party_changed", map[string]any{"reason": partyMoved, "who": ms[0].name, "party": view})
	}
}
