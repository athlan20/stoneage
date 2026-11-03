package character

// STAGE-277：转让队长（party_leader）与队伍频道（chat channel=party）的单元测试。
// 收件范围的真实推送由 casetest 的 party_chat 用例覆盖；这里只测成员快照、校验和重排。

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// partyFour 建一支四人真人队：甲（队长，入队 1）、乙（2）、丙（3）、丁（4）。
func partyFour(t *testing.T) (*Service, map[string]*Presence) {
	t.Helper()
	s, _ := newPartyTestService(t)
	ps := map[string]*Presence{}
	for i, name := range []string{"甲", "乙", "丙", "丁"} {
		ps[name] = fakePartyPresence(s, int64(i+1), name)
	}
	for _, name := range []string{"乙", "丙", "丁"} {
		if perr := partyInvite(t, s, ps["甲"], ps[name]); perr != nil {
			t.Fatalf("%s 入队失败: %v", name, perr)
		}
	}
	return s, ps
}

func wantNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("成员 = %v，应为 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("成员 = %v，应为 %v", got, want)
		}
	}
}

func containsPresence(ps []*Presence, p *Presence) bool {
	for _, x := range ps {
		if x == p {
			return true
		}
	}
	return false
}

func TestPartyLeaderTransferReordersMembers(t *testing.T) {
	s, ps := partyFour(t)
	if _, perr := s.partyTransferLeader(ps["甲"], "丁"); perr != nil {
		t.Fatalf("转让失败: %v", perr)
	}
	// 新队长排到 0，原队长排到 1，其余（乙、丙）相对顺序不变。
	wantNames(t, partyNames(s, ps["乙"]), "丁", "甲", "乙", "丙")
	if ps["丁"].realParty.Load() == nil || ps["丁"].realParty.Load() != ps["甲"].realParty.Load() {
		t.Fatal("转让后仍应是同一支队")
	}
	// 新队长再转给丙：丙 丁 甲 乙。
	if _, perr := s.partyTransferLeader(ps["丁"], "丙"); perr != nil {
		t.Fatalf("二次转让失败: %v", perr)
	}
	wantNames(t, partyNames(s, ps["甲"]), "丙", "丁", "甲", "乙")
}

func TestPartyLeaderDepartureRenumbersByJoinOrder(t *testing.T) {
	s, ps := partyFour(t)
	// 甲 → 丁 → 丙：丙 甲 丁 乙（乙入队最早之后是甲，但队里的顺序已被转让打乱）。
	s.partyTransferLeader(ps["甲"], "丁")
	s.partyTransferLeader(ps["丁"], "丙")
	wantNames(t, partyNames(s, ps["甲"]), "丙", "丁", "甲", "乙")
	// 队员乙离队：队长丙留在第 0 位，其余按入队先后重新编号（甲 1、丁 4）。
	s.partyDepart(ps["乙"], "left")
	wantNames(t, partyNames(s, ps["甲"]), "丙", "甲", "丁")
	// 队长丙离队：新队长是最早入队的剩余成员（甲），其余按入队先后。
	s.partyDepart(ps["丙"], "left")
	wantNames(t, partyNames(s, ps["甲"]), "甲", "丁")
}

func TestPartyLeaderCheckOrder(t *testing.T) {
	s, ps := partyFour(t)
	ms := s.parties.snapshot(ps["甲"])
	wantErrCode(t, partyLeaderCheck(ps["乙"], ms, "甲"), protocol.CodeNotPartyLeader)
	wantErrCode(t, partyLeaderCheck(ps["甲"], ms, "甲"), protocol.CodeInvalidTarget)
	wantErrCode(t, partyLeaderCheck(ps["甲"], ms, "路人"), protocol.CodeNotPartyMember)
	if perr := partyLeaderCheck(ps["甲"], ms, "乙"); perr != nil {
		t.Fatalf("合法的转让不应被拒绝: %v", perr)
	}
	// 队长不是 ps["丙"] 的时候，丙连自己的名字都是队员视角：不是队长优先。
	wantErrCode(t, partyLeaderCheck(ps["丙"], ms, "丙"), protocol.CodeNotPartyLeader)
	// 战斗中（队员在战斗中）拒绝，排在对象检查之前。
	ps["乙"].mu.Lock()
	ps["乙"].battle = &battleState{}
	ps["乙"].mu.Unlock()
	wantErrCode(t, partyLeaderCheck(ps["甲"], ms, "路人"), protocol.CodePartyInBattle)
	ps["乙"].mu.Lock()
	ps["乙"].battle = nil
	ps["乙"].mu.Unlock()
}

func TestPartyLeaderCheckNoParty(t *testing.T) {
	s, _ := newPartyTestService(t)
	outsider := fakePartyPresence(s, 9, "路人")
	if ms := s.parties.snapshot(outsider); len(ms) != 0 {
		t.Fatalf("没有队伍的角色不应有成员: %v", ms)
	}
}

func TestPartyChatRecipients(t *testing.T) {
	s, ps := partyFour(t)
	// 队里的每个真人（队长、队员）都是收件人，含发言人自己。
	for _, name := range []string{"甲", "乙"} {
		members, perr := s.chatPartyMembers(ps[name])
		if perr != nil {
			t.Fatalf("%s 发队聊失败: %v", name, perr)
		}
		if len(members) != 4 || !containsPresence(members, ps[name]) {
			t.Fatalf("%s 的收件人应是整队（含自己），得到 %d 人", name, len(members))
		}
	}
}

func TestPartyChatRejectsOutsiders(t *testing.T) {
	s, ps := partyFour(t)
	// 没有队伍：NOT_IN_PARTY。
	outsider := fakePartyPresence(s, 9, "路人")
	_, perr := s.chatPartyMembers(outsider)
	wantErrCode(t, perr, protocol.CodeNotInParty)
	// 只有分身队（没有真人队）：同样 NOT_IN_PARTY。
	clone := fakePartyPresence(s, 10, "分身")
	clone.party = &partyState{}
	_, perr = s.chatPartyMembers(clone)
	wantErrCode(t, perr, protocol.CodeNotInParty)
	// 同图但不在队里的人，队聊的收件人里没有他。
	members, _ := s.chatPartyMembers(ps["甲"])
	for _, m := range members {
		if m == outsider || m == clone {
			t.Fatal("队外的角色不应是队聊收件人")
		}
	}
}

func TestPartyChatAfterLeaveGetsNothing(t *testing.T) {
	s, ps := partyFour(t)
	s.partyDepart(ps["乙"], "left")
	_, perr := s.chatPartyMembers(ps["乙"])
	wantErrCode(t, perr, protocol.CodeNotInParty)
	members, perr := s.chatPartyMembers(ps["甲"])
	if perr != nil {
		t.Fatalf("队长发队聊失败: %v", perr)
	}
	for _, m := range members {
		if m == ps["乙"] {
			t.Fatal("离队的人不应再是收件人")
		}
	}
	if len(members) != 3 {
		t.Fatalf("收件人应为剩下的 3 人，得到 %d", len(members))
	}
}

func TestPartyChatKickedMemberGetsNothing(t *testing.T) {
	s, ps := partyFour(t)
	s.partyDepart(ps["丙"], "kicked")
	_, perr := s.chatPartyMembers(ps["丙"])
	wantErrCode(t, perr, protocol.CodeNotInParty)
}
