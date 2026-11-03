// 真人同场战斗的回合推进（STAGE-275，protocol 10.14）：等全员交齐、120 秒截止、超时踢出、掉线、逃跑、胜负。
package battle

import (
	"testing"
	"time"
)

// monsterNo 返回场上第一只活着的怪物的单位号。
func monsterNo(t *testing.T, b *Battle) int {
	t.Helper()
	for no := 10; no < 20; no++ {
		if u := b.Unit(no); u != nil && u.Kind == KindMonster && u.Alive() {
			return no
		}
	}
	t.Fatal("没有活着的怪物")
	return -1
}

// partySubmitAll 让队长和队员都交上攻击指令（队员的宠物也攻击），每人的指令都要被接受。
func partySubmitAll(t *testing.T, b *Battle, mates ...int) {
	t.Helper()
	target := monsterNo(t, b)
	turn := b.Turn()
	if _, perr := b.SubmitFor(0, turn, "player", "attack", Cmd{Target: target}); perr != nil {
		t.Fatalf("队长人物指令被拒：%v", perr)
	}
	if _, perr := b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatalf("队长宠物指令被拒：%v", perr)
	}
	for _, no := range mates {
		if _, perr := b.SubmitFor(no, turn, "player", "attack", Cmd{Target: target}); perr != nil {
			t.Fatalf("队员 %d 人物指令被拒：%v", no, perr)
		}
		if _, perr := b.SubmitFor(no, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
			t.Fatalf("队员 %d 宠物指令被拒：%v", no, perr)
		}
	}
}

// clockAt 把引擎的时钟固定到 t（测试里不真等 120 秒）。
func clockAt(b *Battle, t *time.Time) {
	b.Clock = func() time.Time { return *t }
}

func TestPartyWaitsForAllHumans(t *testing.T) {
	b := partyFixture(t, 2)
	target := monsterNo(t, b)
	turn := b.Turn()
	if next, perr := b.SubmitFor(0, turn, "player", "attack", Cmd{Target: target}); perr != nil || next != "pet" {
		t.Fatalf("队长人物指令：next=%s err=%v", next, perr)
	}
	if _, perr := b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if b.AllSubmitted() {
		t.Fatal("队员还没交，不能推进")
	}
	if _, perr := b.SubmitFor(1, turn, "player", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if b.AllSubmitted() {
		t.Fatal("队员的宠物还没交，不能推进")
	}
	if _, perr := b.SubmitFor(1, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if !b.AllSubmitted() {
		t.Fatal("全员交齐后应该可以结算")
	}
	if pv := b.progress(); pv.Done != 2 || pv.Total != 2 {
		t.Fatalf("进度 = %+v，应为 2/2", pv)
	}
	round, end := b.Settle()
	if end != nil || b.Over() || len(round.Events) == 0 {
		t.Fatalf("第一回合不应结束：end=%v over=%v events=%d", end, b.Over(), len(round.Events))
	}
	if b.Turn() != 2 {
		t.Fatalf("结算后应进入第 2 回合，实际 %d", b.Turn())
	}
}

func TestPartyDeadlineStartsAtFirstCommand(t *testing.T) {
	b := partyFixture(t, 2)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	clockAt(b, &now)
	if got := b.ExpireDue(now.Add(time.Hour)); got != nil {
		t.Fatal("还没有人交指令时不计时")
	}
	partySubmitAll(t, b, 1) // 第一条指令在 now 被接受
	if b.deadline != now.Add(PartyCommandWait) {
		t.Fatalf("截止时间 = %v，应为 now+120s", b.deadline)
	}
}

func TestPartyTimeoutKicksSilentMate(t *testing.T) {
	b := partyFixture(t, 2)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	clockAt(b, &now)
	target := monsterNo(t, b)
	turn := b.Turn()
	if _, perr := b.SubmitFor(0, turn, "player", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if got := b.ExpireDue(now.Add(119 * time.Second)); got != nil {
		t.Fatalf("119 秒时不应踢人：%+v", got)
	}
	gone := b.ExpireDue(now.Add(PartyCommandWait))
	if len(gone) != 1 || gone[0].No != 1 || gone[0].Cause != "timeout" || gone[0].Disband {
		t.Fatalf("超时应只踢队员 1：%+v", gone)
	}
	if want := []int{1, 6}; len(gone[0].Left) != 2 || gone[0].Left[0] != want[0] || gone[0].Left[1] != want[1] {
		t.Fatalf("离场的单位 = %v，应为人物 1 与宠物 6", gone[0].Left)
	}
	if b.Unit(1) != nil || b.Unit(6) != nil || b.Departed(1) != "timeout" {
		t.Fatal("被踢的队员和宠物应该已经离场")
	}
	if b.Over() {
		t.Fatal("队员被踢，战斗应继续")
	}
	if !b.AllSubmitted() {
		t.Fatal("剩下的队长已交齐，应该可以结算")
	}
	if _, end := b.Settle(); end != nil || b.Over() {
		t.Fatal("队长还在，第一回合不应结束")
	}
	if b.Turn() != 2 {
		t.Fatalf("应进入第 2 回合，实际 %d", b.Turn())
	}
}

func TestPartyTimeoutLeaderDisbands(t *testing.T) {
	b := partyFixture(t, 2)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	clockAt(b, &now)
	target := monsterNo(t, b)
	if _, perr := b.SubmitFor(1, b.Turn(), "player", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(1, b.Turn(), "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	// 队员交齐了，只有队长没交：只踢队长。
	gone := b.ExpireDue(now.Add(PartyCommandWait))
	if len(gone) != 1 || gone[0].No != 0 || !gone[0].Disband || gone[0].Cause != "timeout" {
		t.Fatalf("队长超时应解散队伍：%+v", gone)
	}
	if b.Unit(0) != nil || b.Unit(5) != nil || b.Over() {
		t.Fatal("队长离场、战斗继续，队员应该还在")
	}
	if b.Unit(1) == nil {
		t.Fatal("队员 1 应该还在战斗中")
	}
}

func TestPartyOfflineMateLeavesWithoutEnd(t *testing.T) {
	b := partyFixture(t, 2)
	d, ok := b.Offline(1)
	if !ok || d.Cause != "offline" || d.Disband || len(d.Left) != 2 {
		t.Fatalf("掉线离场 = %+v ok=%v", d, ok)
	}
	if _, again := b.Offline(1); again {
		t.Fatal("已经离场的真人不能再掉线一次")
	}
	if got := b.TakeDepartures(); len(got) != 1 || len(b.TakeDepartures()) != 0 {
		t.Fatal("离场应该只被取走一次")
	}
	if b.Over() || b.Unit(1) != nil {
		t.Fatal("掉线的队员离场，战斗继续")
	}
	if pv := b.progress(); pv.Total != 1 {
		t.Fatalf("离场后进度应只数队长：%+v", pv)
	}
}

func TestPartyMateEscapeOnlySelf(t *testing.T) {
	b := partyFixture(t, 2)
	b.Rnd = func(n int) int { return 0 } // 逃跑判定必成
	target := monsterNo(t, b)
	turn := b.Turn()
	if _, perr := b.SubmitFor(1, turn, "player", "escape", Cmd{}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(1, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(0, turn, "player", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: target}); perr != nil {
		t.Fatal(perr)
	}
	_, end := b.Settle()
	if end != nil || b.Over() {
		t.Fatal("队员逃跑只是个人行为，战斗不结束")
	}
	ds := b.TakeDepartures()
	if len(ds) != 1 || ds[0].Cause != "escape" || ds[0].No != 1 || ds[0].Disband {
		t.Fatalf("逃跑离场 = %+v", ds)
	}
	if b.Unit(0) == nil || b.Unit(5) == nil {
		t.Fatal("队长和他的宠物不应因为队员逃跑离场")
	}
}

func TestPartyLeaderEscapeDisbands(t *testing.T) {
	b := partyFixture(t, 2)
	b.Rnd = func(n int) int { return 0 }
	target := monsterNo(t, b)
	turn := b.Turn()
	b.SubmitFor(0, turn, "player", "escape", Cmd{})
	b.SubmitFor(0, turn, "pet", "attack", Cmd{Target: target})
	b.SubmitFor(1, turn, "player", "attack", Cmd{Target: target})
	b.SubmitFor(1, turn, "pet", "attack", Cmd{Target: target})
	b.Settle()
	ds := b.TakeDepartures()
	if len(ds) != 1 || ds[0].No != 0 || !ds[0].Disband || ds[0].Cause != "escape" {
		t.Fatalf("队长逃跑应报告解散：%+v", ds)
	}
	if b.Over() || b.Unit(1) == nil {
		t.Fatal("队长逃跑后队员留在战斗中")
	}
}

func TestPartyLoseWhenNoHumanStands(t *testing.T) {
	b := partyFixture(t, 2)
	b.units[0].Down, b.units[0].HP = true, 0
	b.units[1].Down, b.units[1].HP = true, 0
	// 宠物还活着：己方仍然落败（只数非宠物玩家）。
	if _, end := b.Settle(); end != nil || !b.Over() || b.Result() != "lose" {
		t.Fatalf("全员倒下应判负：over=%v result=%s", b.Over(), b.Result())
	}
}

func TestPartyTimeLimitDraws(t *testing.T) {
	b := partyFixture(t, 2)
	start := b.startedAt
	if b.ExpireBattle(start.Add(PartyTimeLimit - time.Second)) {
		t.Fatal("时限前不应结束")
	}
	if !b.ExpireBattle(start.Add(PartyTimeLimit)) || !b.Over() || b.Result() != "draw" {
		t.Fatalf("到时限应以 draw 结束：over=%v result=%s", b.Over(), b.Result())
	}
}

func TestPartySubmitRules(t *testing.T) {
	b := partyFixture(t, 2)
	turn := b.Turn()
	if _, perr := b.SubmitFor(1, turn, "player", "item", Cmd{Slot: 5}); perr == nil {
		t.Fatal("队员暂不能用道具")
	}
	if _, perr := b.SubmitFor(1, turn, "pet", "attack", Cmd{Target: monsterNo(t, b)}); perr == nil {
		t.Fatal("没交人物指令前不能先交宠物指令")
	}
	if _, perr := b.SubmitFor(1, turn+1, "player", "guard", Cmd{}); perr == nil {
		t.Fatal("回合号不对应被拒")
	}
}
