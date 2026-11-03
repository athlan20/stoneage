// 真人同场战斗的推送内容（STAGE-275，10.14）：battle_start 每人一条、battle_turn 的 deadline_ms 为 null、
// battle_progress 的截止时间、battle_leave 的内容，以及 mp 对全队可见。
package battle

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPartyStartPerMember(t *testing.T) {
	b := partyFixture(t, 3)
	v := b.StartViewFor(2)
	if v.You != 2 || v.Kind != "party" || v.Party == nil || len(v.Party.Members) != 3 {
		t.Fatalf("battle_start = you=%d kind=%s members=%+v", v.You, v.Kind, v.Party)
	}
	if !v.Party.Members[0].Leader || v.Party.Members[1].Leader {
		t.Fatal("队长应排在前面并标记 leader")
	}
	if other := b.StartViewFor(0); other.You != 0 {
		t.Fatal("每人的 you 应该不同")
	}
	// 队友都带 mp（10.14）。
	for _, u := range v.Units {
		if u.Kind == "player" && u.MP == nil {
			t.Fatalf("真人单位 %d 缺 mp", u.No)
		}
	}
}

func TestPartyTurnDeadlineNullAndOwnNeeds(t *testing.T) {
	b := partyFixture(t, 2)
	tv := b.TurnFor(1, 1000)
	raw, _ := json.Marshal(tv)
	if !strings.Contains(string(raw), `"deadline_ms":null`) {
		t.Fatalf("battle_turn 的 deadline_ms 应为 null：%s", raw)
	}
	if tv.Progress.Total != 2 || len(tv.Needs) != 2 {
		t.Fatalf("队员的 battle_turn = %+v", tv)
	}
	b.mateCmds[1] = &Cmd{Actor: "player", Action: "guard"}
	if got := b.TurnFor(1, 1000).Needs; len(got) != 2 {
		t.Fatalf("交过的人 needs 仍按需要交的算，实际 %v", got)
	}
}

func TestPartyProgressDeadline(t *testing.T) {
	b := partyFixture(t, 2)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	clockAt(b, &now)
	before := b.ProgressPush(now.UnixMilli())
	if before.DeadlineMS != nil {
		t.Fatal("还没有人交指令，不带截止时间")
	}
	partySubmitAll(t, b, 1)
	p := b.ProgressPush(now.UnixMilli())
	if p.DeadlineMS == nil || *p.DeadlineMS != now.Add(PartyCommandWait).UnixMilli() || p.Done != 2 || p.Total != 2 {
		t.Fatalf("battle_progress = %+v", p)
	}
}

func TestPartyLeaveView(t *testing.T) {
	b := partyFixture(t, 2)
	d := b.depart(1, "timeout")
	lv := b.LeaveFor(d)
	if lv.No != 1 || lv.Cause != "timeout" || len(lv.Left) != 2 || lv.Progress.Total != 1 {
		t.Fatalf("battle_leave = %+v", lv)
	}
}

func TestPartyQueryAndMates(t *testing.T) {
	b := partyFixture(t, 2)
	q := b.QueryFor(1, 0, 1000)
	if q.You != 1 || q.Phase != "command" || len(q.Needs) != 2 || q.Submitted["player"] {
		t.Fatalf("队员的查询 = %+v", q.QueryCore)
	}
}
