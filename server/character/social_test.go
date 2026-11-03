package character

import (
	"encoding/json"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-276：社交开关（隐身、拒绝邀请）的纯逻辑与附近玩家索引的隐身状态（不需要数据库、不需要连接）。

func newSocialPresence(s *Service, id int64, name string, hidden bool) *Presence {
	p := fakePartyPresence(s, id, name)
	p.players = newPlayerIndex()
	p.hidden.Store(hidden)
	return p
}

func TestParseSocialArgs(t *testing.T) {
	cases := []struct {
		data                       string
		wantVis, wantInv, wantTrad *bool
		wantErr                    bool
	}{
		{data: `{}`},
		{data: `{"visible":false}`, wantVis: boolPtr(false)},
		{data: `{"invite":true}`, wantInv: boolPtr(true)},
		{data: `{"visible":true,"invite":false}`, wantVis: boolPtr(true), wantInv: boolPtr(false)},
		{data: `{"trade":false}`, wantTrad: boolPtr(false)}, // STAGE-296
		{data: `{"visible":true,"invite":false,"trade":true}`, wantVis: boolPtr(true), wantInv: boolPtr(false), wantTrad: boolPtr(true)},
		{data: `{"trade":"no"}`, wantErr: true},
		{data: `{"trade":null}`, wantErr: true},
		{data: `{"visible":"no"}`, wantErr: true},
		{data: `{"visible":null}`, wantErr: true},
		{data: `{"hidden":true}`, wantErr: true},
		{data: `{"visible":true,"name":"x"}`, wantErr: true},
		{data: `[]`, wantErr: true},
	}
	for _, c := range cases {
		req := &protocol.Request{Data: json.RawMessage(c.data)}
		vis, inv, trad, _, perr := parseSocialArgs(req)
		if c.wantErr {
			if perr == nil || perr.Code != protocol.CodeBadRequest {
				t.Errorf("%s：应回 BAD_REQUEST，得到 %v", c.data, perr)
			}
			continue
		}
		if perr != nil {
			t.Fatalf("%s：不应报错: %v", c.data, perr)
		}
		if !sameBoolPtr(vis, c.wantVis) || !sameBoolPtr(inv, c.wantInv) || !sameBoolPtr(trad, c.wantTrad) {
			t.Errorf("%s：解析结果不对 visible=%v invite=%v trade=%v", c.data, vis, inv, trad)
		}
	}
}

func TestSocialViewDefaultsAndInverse(t *testing.T) {
	if v := socialView(&record{}); v["visible"] != true || v["invite"] != true || v["trade"] != true {
		t.Errorf("零值记录应该是都开，得到 %v", v)
	}
	if v := socialView(&record{SocialHidden: true, SocialNoInvite: true, SocialNoTrade: true}); v["visible"] != false || v["invite"] != false || v["trade"] != false {
		t.Errorf("隐身、拒绝组队邀请和拒绝交易邀请应该都关，得到 %v", v)
	}
}

func TestPartySummaryCountsVisibleOnly(t *testing.T) {
	s, _ := newPartyTestService(t)
	leader := newSocialPresence(s, 1, "队长", false)
	hiddenMember := newSocialPresence(s, 2, "隐身队员", true)
	member := newSocialPresence(s, 3, "队员", false)
	ms := []*partyMember{{p: leader, name: "队长"}, {p: hiddenMember, name: "隐身队员"}, {p: member, name: "队员"}}

	sum, ok := partySummary(ms).(map[string]any)
	if !ok {
		t.Fatalf("有队伍时应该返回摘要")
	}
	if sum["leader"] != "队长" || sum["size"] != 2 || sum["max"] != partyMaxPlayers {
		t.Errorf("隐身队员不计入 size，得到 %v", sum)
	}

	leader.hidden.Store(true)
	sum = partySummary(ms).(map[string]any)
	if sum["leader"] != nil || sum["size"] != 1 {
		t.Errorf("隐身队长 leader 应为 nil，size 仍只计可见队员，得到 %v", sum)
	}
	if partySummary(nil) != nil {
		t.Errorf("没有队伍应该是 nil")
	}
}

func TestPartyTargetCheckOrder(t *testing.T) {
	me := peerState{floor: 1006}
	refusing := peerState{floor: 1006, noInvite: true}
	busy := peerState{floor: 1006, battle: true}
	refusingBusy := peerState{floor: 1006, noInvite: true, battle: true}
	otherMap := peerState{floor: 1007, noInvite: true}

	cases := []struct {
		name string
		tp   peerState
		self bool
		kind string
		want string
	}{
		{"邀请：拒绝邀请回 TARGET_REFUSES", refusing, false, "invite", protocol.CodeTargetRefuses},
		{"邀请：拒绝且战斗中，先 TARGET_REFUSES", refusingBusy, false, "invite", protocol.CodeTargetRefuses},
		{"申请：拒绝邀请不影响，战斗中回 TARGET_IN_BATTLE", refusingBusy, false, "apply", protocol.CodeTargetInBattle},
		{"申请：拒绝邀请不影响，放行", refusing, false, "apply", ""},
		{"邀请：不同图先 PLAYER_NOT_NEARBY", otherMap, false, "invite", protocol.CodePlayerNotNearby},
		{"邀请：自己先 INVALID_TARGET", refusing, true, "invite", protocol.CodeInvalidTarget},
		{"邀请：对方战斗中、未拒绝回 TARGET_IN_BATTLE", busy, false, "invite", protocol.CodeTargetInBattle},
		{"邀请：都正常放行", peerState{floor: 1006}, false, "invite", ""},
	}
	for _, c := range cases {
		perr := partyTargetCheck(me, c.tp, c.self, c.kind)
		got := ""
		if perr != nil {
			got = perr.Code
		}
		if got != c.want {
			t.Errorf("%s：want %q got %q", c.name, c.want, got)
		}
	}
}

func TestSocialVisibilityIndex(t *testing.T) {
	s, _ := newPartyTestService(t)
	x := newPlayerIndex()
	a := newSocialPresence(s, 1, "甲", false)
	a.players = x
	x.add(a, 1006, "甲")
	x.flush() // 第一个窗口：别人（这里没人）认为他在图上
	e := x.entries[a.charID]
	if !e.sent {
		t.Fatalf("窗口结束后应记为已发送")
	}

	// 隐身：窗口里推 leave
	a.hidden.Store(true)
	x.visibilityChanged(a)
	if c := x.pendingOf(1006, a.charID); c == nil || !c.leave {
		t.Fatalf("隐身应排队 leave，得到 %+v", c)
	}
	// 隐身期间的数据变化不推 update，记下 dirty
	x.touch(a)
	if c := x.pendingOf(1006, a.charID); c == nil || !c.leave {
		t.Fatalf("隐身时 touch 不应覆盖 leave")
	}
	x.flush()
	if e.sent || !e.dirty {
		t.Fatalf("leave 发出后 sent 应为 false，隐身期间的变化应记为 dirty，得到 sent=%v dirty=%v", e.sent, e.dirty)
	}

	// 窗口内先隐身又恢复：窗口开始时他已经隐身，别人没有他，不推
	a.hidden.Store(false)
	x.visibilityChanged(a)
	if c := x.pendingOf(1006, a.charID); c == nil || !c.enter {
		t.Fatalf("恢复可见应排队 enter，得到 %+v", c)
	}
	a.hidden.Store(true)
	x.visibilityChanged(a)
	if c := x.pendingOf(1006, a.charID); c != nil {
		t.Fatalf("窗口内先恢复又隐身，结束时与窗口开始相同，不应有待发项，得到 %+v", c)
	}

	// 别人已经有他（上一窗口发出了 enter）之后，同一窗口内隐身、数据变化、恢复：结束时可见，只补一条 update
	a.hidden.Store(false)
	x.visibilityChanged(a)
	x.flush()
	if !e.sent {
		t.Fatalf("上一窗口发出 enter 后别人应认为他在图上")
	}
	a.hidden.Store(true)
	x.visibilityChanged(a)
	x.touch(a) // 隐身期间的数据变化，记 dirty
	a.hidden.Store(false)
	x.visibilityChanged(a)
	if c := x.pendingOf(1006, a.charID); c == nil || c.leave || c.enter || c.entry == nil {
		t.Fatalf("同窗口隐身又恢复且数据变过，应只补一条 update，得到 %+v", c)
	}
	x.flush()
	if !e.sent || e.dirty {
		t.Fatalf("补发 update 后应仍认为在图上且不欠数据，得到 sent=%v dirty=%v", e.sent, e.dirty)
	}
}

func TestHiddenEntersAndLeavesSilently(t *testing.T) {
	s, _ := newPartyTestService(t)
	x := newPlayerIndex()
	a := newSocialPresence(s, 1, "甲", true)
	a.players = x
	x.add(a, 1006, "甲")
	if c := x.pendingOf(1006, a.charID); c != nil {
		t.Fatalf("隐身者进图不推 enter，得到 %+v", c)
	}
	x.remove(a)
	if c := x.pendingOf(1006, a.charID); c != nil {
		t.Fatalf("隐身者离图不推 leave，得到 %+v", c)
	}
	if len(x.entries) != 0 {
		t.Fatalf("离图后索引应清空")
	}
}

func boolPtr(b bool) *bool { return &b }

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
