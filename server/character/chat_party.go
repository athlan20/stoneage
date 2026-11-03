package character

// STAGE-277：队伍频道 chat channel=party（docs/protocol.md 5.68「队伍频道」，docs/chat.md 1.5）。
// 收件人是发言时本队的真人成员（玩家连接），与地图无关；分身队没有连接，不能发。agent 连接收不到（pushPlayerConn）。

import (
	"github.com/athlan20/stoneage/server/protocol"
)

// chatPartyMembers 取发言人所在真人队的成员 Presence（含发言人自己），作为本条队聊的收件人。
// 没有真人队（包括只有分身队）回复 NOT_IN_PARTY。调用时持有发言人的 mu（允许：Presence.mu → partyHub.mu）。
func (s *Service) chatPartyMembers(p *Presence) ([]*Presence, *protocol.Error) {
	ms := s.parties.snapshot(p)
	if len(ms) == 0 {
		return nil, protocol.Errorf(protocol.CodeNotInParty, "没有真人队，队聊要在队伍里说")
	}
	out := make([]*Presence, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.p)
	}
	return out, nil
}

// pushPartyChat 把一条队聊推给收件人的玩家连接。调用时不持有任何锁。
func pushPartyChat(members []*Presence, data map[string]any) {
	for _, p := range members {
		p.pushPlayerConn("chat_message", data)
	}
}
