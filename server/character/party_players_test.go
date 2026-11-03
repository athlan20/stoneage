package character

import (
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-274：真人队的容量、顺位接任、自动解散、过期、断线清理、与分身队互斥（不需要数据库、不需要连接）。

// fakeClock 是可以手动推进的时钟（请求的到期和频率用 Service.opts.Now）。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newPartyTestService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	s := New(nil, nil, Options{Now: clk.Now})
	return s, clk
}

// fakePartyPresence 是一个只有数据、没有连接的角色（推送是空操作）。
func fakePartyPresence(s *Service, id int64, name string) *Presence {
	return &Presence{
		charID:  id,
		rec:     record{ID: id, Name: name, Floor: 1006, Level: 1, HP: 10},
		members: map[protocol.Role]*Live{},
		done:    make(chan struct{}),
		parties: s.parties,
	}
}

// partyInvite 让 from 邀请 to 并由 to 接受（不经过协议，直接走服务端的函数）。
func partyInvite(t *testing.T, s *Service, from, to *Presence) *protocol.Error {
	t.Helper()
	r, perr := s.newPartyRequest(from, from.rec.Name, to, to.rec.Name, "invite")
	if perr != nil {
		t.Fatalf("发出邀请失败: %v", perr)
	}
	taken := s.takePartyRequest(r.id, func(*partyRequest) bool { return true })
	if taken == nil {
		t.Fatalf("请求 %s 应该还在", r.id)
	}
	_, perr = s.acceptPartyRequest(to, taken)
	return perr
}

func partyNames(s *Service, p *Presence) []string {
	var out []string
	for _, m := range s.parties.snapshot(p) {
		out = append(out, m.name)
	}
	return out
}

func wantErrCode(t *testing.T, err *protocol.Error, code string) {
	t.Helper()
	if err == nil || err.Code != code {
		t.Fatalf("错误码应为 %s，实际 %v", code, err)
	}
}

func TestPartyPlayersCapacity(t *testing.T) {
	s, _ := newPartyTestService(t)
	leader := fakePartyPresence(s, 1, "队长")
	if perr := partyInvite(t, s, leader, fakePartyPresence(s, 2, "甲")); perr != nil {
		t.Fatalf("第一次邀请应建队: %v", perr)
	}
	for i, name := range []string{"乙", "丙", "丁"} {
		if perr := partyInvite(t, s, leader, fakePartyPresence(s, int64(3+i), name)); perr != nil {
			t.Fatalf("第 %d 个成员应能入队: %v", i+2, perr)
		}
	}
	if got := len(s.parties.snapshot(leader)); got != partyMaxPlayers {
		t.Fatalf("成员数 = %d，应为 %d", got, partyMaxPlayers)
	}
	wantErrCode(t, partyInvite(t, s, leader, fakePartyPresence(s, 7, "戊")), protocol.CodePartyFull)
}

func TestPartyPlayersLeaderSuccession(t *testing.T) {
	s, _ := newPartyTestService(t)
	leader := fakePartyPresence(s, 1, "队长")
	a := fakePartyPresence(s, 2, "甲")
	b := fakePartyPresence(s, 3, "乙")
	partyInvite(t, s, leader, a)
	partyInvite(t, s, leader, b)
	s.partyDepart(leader, "left")
	if got := partyNames(s, a); len(got) != 2 || got[0] != "甲" || got[1] != "乙" {
		t.Fatalf("队长离队后应按入队先后顺位，得到 %v", got)
	}
	if leader.realParty.Load() != nil {
		t.Fatal("离队的人身上不应再有队伍")
	}
	s.partyDepart(a, "left")
	if got := partyNames(s, b); len(got) != 0 {
		t.Fatalf("只剩一人时队伍应解散，得到 %v", got)
	}
}

func TestPartyPlayersAutoDisband(t *testing.T) {
	s, _ := newPartyTestService(t)
	leader := fakePartyPresence(s, 1, "队长")
	a := fakePartyPresence(s, 2, "甲")
	partyInvite(t, s, leader, a)
	s.partyDepart(a, "kicked")
	if leader.realParty.Load() != nil || len(s.parties.snapshot(leader)) != 0 {
		t.Fatal("请出后只剩队长一人，队伍应解散")
	}
}

func TestPartyPlayersRequestExpires(t *testing.T) {
	s, clk := newPartyTestService(t)
	from := fakePartyPresence(s, 1, "发起人")
	to := fakePartyPresence(s, 2, "对象")
	r, perr := s.newPartyRequest(from, from.rec.Name, to, to.rec.Name, "invite")
	if perr != nil {
		t.Fatal(perr)
	}
	clk.Add(29 * time.Second)
	s.expirePartyRequests()
	if s.takePartyRequest(r.id, func(*partyRequest) bool { return true }) == nil {
		t.Fatal("29 秒时请求还不该过期")
	}
	r, _ = s.newPartyRequest(from, from.rec.Name, to, to.rec.Name, "invite")
	_ = r
	clk.Add(31 * time.Second)
	s.expirePartyRequests()
	if s.takePartyRequest(r.id, func(*partyRequest) bool { return true }) != nil {
		t.Fatal("31 秒时请求应已过期并被移除")
	}
}

func TestPartyPlayersRateLimitAndPending(t *testing.T) {
	s, clk := newPartyTestService(t)
	from := fakePartyPresence(s, 1, "发起人")
	to := fakePartyPresence(s, 2, "对象")
	if _, perr := s.newPartyRequest(from, from.rec.Name, to, to.rec.Name, "invite"); perr != nil {
		t.Fatal(perr)
	}
	_, perr := s.newPartyRequest(from, from.rec.Name, to, to.rec.Name, "invite")
	wantErrCode(t, perr, protocol.CodeRequestPending)
	clk.Add(11 * time.Second)
	s.expirePartyRequests() // 30 秒内还没过期，请求仍在，所以这里只测待处理
	clk.Add(20 * time.Second)
	s.expirePartyRequests()
	if _, perr := s.newPartyRequest(from, from.rec.Name, to, to.rec.Name, "invite"); perr != nil {
		t.Fatalf("过了 10 秒且请求已过期后应能再发: %v", perr)
	}
}

func TestPartyPlayersDisconnectCancelsRequests(t *testing.T) {
	s, _ := newPartyTestService(t)
	leader := fakePartyPresence(s, 1, "队长")
	a := fakePartyPresence(s, 2, "甲")
	partyInvite(t, s, leader, a)
	b := fakePartyPresence(s, 3, "乙")
	r, perr := s.newPartyRequest(b, b.rec.Name, leader, leader.rec.Name, "apply")
	if perr != nil {
		t.Fatal(perr)
	}
	s.partyOnGone(b) // 乙断线：它发出的申请 cancelled，它本来也没有队伍
	if s.takePartyRequest(r.id, func(*partyRequest) bool { return true }) != nil {
		t.Fatal("断线的人发出的请求应被取消")
	}
	s.partyOnGone(a) // 甲断线 = 离队，剩下队长一人，队伍解散
	if leader.realParty.Load() != nil {
		t.Fatal("断线后队伍应只剩队长并解散")
	}
}

func TestPartyPlayersMutuallyExclusiveWithClone(t *testing.T) {
	s, _ := newPartyTestService(t)
	clone := fakePartyPresence(s, 1, "分身队长")
	clone.party = &partyState{}
	other := fakePartyPresence(s, 2, "路人")
	// 有分身队的人发出的邀请，接受时被拒绝（ALREADY_IN_PARTY），请求作废。
	r, perr := s.newPartyRequest(clone, clone.rec.Name, other, other.rec.Name, "invite")
	if perr != nil {
		t.Fatal(perr)
	}
	taken := s.takePartyRequest(r.id, func(*partyRequest) bool { return true })
	_, perr = s.acceptPartyRequest(other, taken)
	wantErrCode(t, perr, protocol.CodeAlreadyInParty)
	// 真人队里不能加分身：分身队的组队入口在 party.go，这里只确认真人队成员的 party 字段不会被当成分身队。
	if other.realParty.Load() != nil || other.party != nil {
		t.Fatal("接受失败后被邀请的人不应进入任何队伍")
	}
}
