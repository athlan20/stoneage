// 决斗的双真人战斗引擎（STAGE-306，docs/pk.md 2.1–2.3、4.4）：单位号、推送裁剪、决斗点数、逃跑必成、复活与偷窃对人物无效。
package battle

import (
	"testing"
	"time"
)

// duelSide 是一方的快照：数值同 testSide，决斗点数 dp，名字 name。
func duelSide(name string, dp int) *SideInput {
	s := testSide()
	s.Name, s.DuelPoint = name, dp
	return s
}

// duelFixture 开一场 1 对 1 的决斗：发起方「甲」0 号，被挑战方「乙」10 号，双方 DP 100，固定随机源。
func duelFixture(t *testing.T, rnd func(n int) int) *Battle {
	t.Helper()
	b, err := NewDuel("d_test", "新手村", []PartyMate{{No: 0, Side: duelSide("甲", 100)}}, []PartyMate{{No: 10, Side: duelSide("乙", 100)}}, rnd)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func submitBoth(t *testing.T, b *Battle, a0, a10 string, target0, target10 int) {
	t.Helper()
	turn := b.Turn()
	for _, c := range []struct {
		no      int
		action  string
		target  int
		pet     string
		petAct  string
		petTarg int
	}{{0, a0, target0, "pet", "guard", -1}, {10, a10, target10, "pet", "guard", -1}} {
		if _, perr := b.SubmitFor(c.no, turn, "player", c.action, Cmd{Target: c.target}); perr != nil {
			t.Fatalf("%d 号人物指令 %s 被拒：%v", c.no, c.action, perr)
		}
		if _, perr := b.SubmitFor(c.no, turn, c.pet, c.petAct, Cmd{Target: c.petTarg}); perr != nil {
			t.Fatalf("%d 号宠物指令被拒：%v", c.no, perr)
		}
	}
}

func TestNewDuelSeats(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	if !b.IsDuel() || !b.IsParty() || b.Kind != "duel" || b.AllowCapture || !b.AllowEscape {
		t.Fatalf("kind=%s duel=%v allow=%v/%v", b.Kind, b.IsDuel(), b.AllowEscape, b.AllowCapture)
	}
	for _, no := range []int{0, 5, 10, 15} {
		if b.Unit(no) == nil {
			t.Fatalf("单位 %d 应该在场", no)
		}
	}
	if !b.isMateUnit(b.Unit(10)) || !b.isMateUnit(b.Unit(15)) || b.isMateUnit(b.Unit(0)) || b.isMateUnit(b.Unit(5)) {
		t.Fatal("对方的人物和宠物按队员处理，发起方队长不是")
	}
	if got := b.needsOf(10); len(got) != 2 {
		t.Fatalf("被挑战方 needs = %v", got)
	}
	if got := ownerOf(15); got != 10 {
		t.Fatalf("ownerOf(15) = %d", got)
	}
	if _, err := NewDuel("d", "x", []PartyMate{{No: 1, Side: testSide()}}, []PartyMate{{No: 10, Side: testSide()}}, nil); err == nil {
		t.Fatal("发起方缺少 0 号队长应被拒绝")
	}
	if _, err := NewDuel("d", "x", []PartyMate{{No: 0, Side: testSide()}}, []PartyMate{{No: 11, Side: testSide()}}, nil); err == nil {
		t.Fatal("被挑战方缺少 10 号队长应被拒绝")
	}
}

func TestDuelStartViewHidesOpponent(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	for _, c := range []struct{ you, own, other int }{{0, 0, 10}, {10, 10, 0}} {
		v := b.DuelStartFor(c.you)
		if v.You != c.you || v.Kind != "duel" || v.Duel == nil || len(v.Duel.Sides) != 2 || v.Duel.Sides[1].Leader != "乙" {
			t.Fatalf("battle_start(%d) = %+v", c.you, v)
		}
		for _, u := range v.Units {
			own := u.No/10 == c.own/10
			if own != (u.HP != nil) {
				t.Fatalf("%d 号视角看 %d 号：hp=%v 己方=%v", c.you, u.No, u.HP, own)
			}
			if u.Kind == "player" && own != (u.MP != nil) {
				t.Fatalf("%d 号视角看 %d 号：mp=%v", c.you, u.No, u.MP)
			}
		}
	}
}

// 一方的人物被打倒：对方胜，倒下者扣开战 DP 的一成，出手者拿走。
func TestDuelKnockoutMovesPoints(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	b.Unit(10).HP = 1
	submitBoth(t, b, "attack", "guard", 10, -1)
	round, end := b.Settle()
	if end != nil || !b.Over() || b.Result() != "win" {
		t.Fatalf("发起方应胜，result=%s over=%v", b.Result(), b.Over())
	}
	if b.DuelDelta(0) != 10 || b.DuelDelta(10) != -10 {
		t.Fatalf("DP 变化 甲=%d 乙=%d，want +10/-10", b.DuelDelta(0), b.DuelDelta(10))
	}
	if e := b.EndFor(0); e.Result != "win" || e.Rewards != nil || e.Penalty != nil {
		t.Fatalf("甲的结局 %+v", e)
	}
	if e := b.EndFor(10); e.Result != "lose" || e.Rewards != nil || e.Penalty != nil {
		t.Fatalf("乙的结局 %+v", e)
	}
	// 回合推送按方裁剪：乙看不到甲的精确血量，看得到自己的。
	rb := round.ForSide(1)
	for _, st := range rb.State {
		if (st.No >= 10) != (st.HP != nil) {
			t.Fatalf("乙视角 state[%d] hp=%v", st.No, st.HP)
		}
	}
	for _, ev := range rb.Events {
		for _, ef := range ev.Effects {
			if ef.Target < 10 && ef.HP != nil {
				t.Fatalf("乙视角不该看到甲的血量：%+v", ef)
			}
		}
	}
	if round.State[0].HP == nil {
		t.Fatal("原回合内容不应被 ForSide 改写")
	}
}

// 对方打倒后 DP 下限为 0；自己人打倒（这里用混乱以外的直接模拟）平分给对方活着的人物。
func TestDuelFriendlyKnockoutGoesToOpponents(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	b.acting = []int{0}
	b.Unit(0).HP, b.Unit(0).Down = 0, true
	b.onDown(b.Unit(0)) // 出手者就是倒下者本人（反弹、混乱）：算自己人打倒，乙拿
	if b.DuelDelta(0) != -10 || b.DuelDelta(10) != 10 {
		t.Fatalf("DP 变化 甲=%d 乙=%d", b.DuelDelta(0), b.DuelDelta(10))
	}
}

// 逃跑必成：即使随机源出最差的值，逃跑也成功；第 1 回合没人攻击、没人倒下免罚。
func TestDuelEscapeAlwaysSucceedsAndFreeInFirstRound(t *testing.T) {
	b := duelFixture(t, rollRnd(99))
	submitBoth(t, b, "escape", "guard", -1, -1)
	round, _ := b.Settle()
	var esc *Event
	for i := range round.Events {
		if round.Events[i].Kind == "escape" {
			esc = &round.Events[i]
		}
	}
	if esc == nil || esc.OK == nil || !*esc.OK {
		t.Fatalf("逃跑应成功：%+v", round.Events)
	}
	ds := b.TakeDepartures()
	if len(ds) != 1 || ds[0].No != 0 || ds[0].Cause != "escape" || !ds[0].Disband {
		t.Fatalf("离场 = %+v", ds)
	}
	if !b.Over() || b.Result() != "lose" {
		t.Fatalf("发起方跑了，被挑战方胜：result=%s over=%v", b.Result(), b.Over())
	}
	if b.DuelDelta(0) != 0 || b.DuelDelta(10) != 0 {
		t.Fatalf("免罚时 DP 不变：甲=%d 乙=%d", b.DuelDelta(0), b.DuelDelta(10))
	}
	if e := b.EndFor(0); e.Result != "escaped" {
		t.Fatalf("甲 %+v", e)
	}
	if e := b.EndFor(10); e.Result != "win" {
		t.Fatalf("乙 %+v", e)
	}
}

// 有人交了攻击指令后逃跑：逃跑者扣两成（覆盖），一成给对方。
func TestDuelEscapeAfterAttackCostsTwoShares(t *testing.T) {
	b := duelFixture(t, rollRnd(99))
	submitBoth(t, b, "escape", "attack", -1, 0)
	b.Settle()
	if b.DuelDelta(0) != -20 || b.DuelDelta(10) != 10 {
		t.Fatalf("DP 变化 甲=%d 乙=%d，want -20/+10", b.DuelDelta(0), b.DuelDelta(10))
	}
}

func TestDuelOfflineAndTimeoutCostPoints(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	now := time.Now()
	clockAt(b, &now)
	// 第 1 回合甲交了攻击，乙没交：到点乙被踢出，按逃跑扣两成。
	turn := b.Turn()
	b.SubmitFor(0, turn, "player", "attack", Cmd{Target: 10})
	b.SubmitFor(0, turn, "pet", "guard", Cmd{})
	now = now.Add(PartyCommandWait + time.Second)
	ds := b.ExpireDue(now)
	if len(ds) != 1 || ds[0].No != 10 || ds[0].Cause != "timeout" {
		t.Fatalf("超时离场 = %+v", ds)
	}
	if b.DuelDelta(10) != -20 || b.DuelDelta(0) != 10 || !b.Over() || b.Result() != "win" {
		t.Fatalf("乙=%d 甲=%d result=%s", b.DuelDelta(10), b.DuelDelta(0), b.Result())
	}
	if e := b.EndFor(10); e.Result != "forced_out" {
		t.Fatalf("乙 %+v", e)
	}
	// 掉线同理。
	b2 := duelFixture(t, rollRnd(9999))
	b2.SubmitFor(0, 1, "player", "attack", Cmd{Target: 10})
	if _, ok := b2.Offline(10); !ok {
		t.Fatal("乙应能掉线离场")
	}
	if b2.DuelDelta(10) != -20 || b2.EndFor(10) != nil {
		t.Fatalf("掉线 乙=%d end=%v", b2.DuelDelta(10), b2.EndFor(10))
	}
}

// 复活对人物无效（精灵、道具都一样），宠物可以复活。
func TestDuelReviveSkipsHumans(t *testing.T) {
	a := magicSide(50)
	b, err := NewDuel("d_rev", "新手村", []PartyMate{{No: 0, Side: a}, {No: 1, Side: duelSide("丙", 100)}},
		[]PartyMate{{No: 10, Side: duelSide("乙", 100)}}, rollRnd(10))
	if err != nil {
		t.Fatal(err)
	}
	mate, pet := b.Unit(1), b.Unit(5)
	mate.HP, mate.Down = 0, true
	pet.HP, pet.Down = 0, true
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 1}); perr != nil {
		t.Fatalf("选倒下的队友人物提交应被接受（道具/气力照扣）：%v", perr)
	}
	b.act(b.Unit(0))
	if !mate.Down || mate.HP != 0 {
		t.Fatalf("人物不应被复活：%+v", mate)
	}
	b.cmds = map[string]*Cmd{}
	b.phase = PhaseCommand
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5}); perr != nil {
		t.Fatal(perr)
	}
	b.act(b.Unit(0))
	if pet.Down || pet.HP <= 0 {
		t.Fatalf("宠物应被复活：%+v", pet)
	}
}

// 偷窃对人物无效：捐献对人物的成功率为 0，宠物不离场。
func TestDuelStealMoneyFailsOnHuman(t *testing.T) {
	b := duelFixture(t, rollPct(1))
	ev := b.stealMoney(b.Unit(5), 10, "捐献")
	if len(ev.Left) != 0 || b.Unit(5) == nil || b.stolenGold != 0 {
		t.Fatalf("对人物应没偷到：%+v gold=%d", ev, b.stolenGold)
	}
}

// 异常状态的命中率在决斗里不计等级差。
func TestDuelStatusIgnoresLevelDiff(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	hi, lo := b.Unit(0), b.Unit(10)
	hi.Level, lo.Level = 50, 1
	lo.Vital, lo.Strength, lo.Toughness, lo.Dexterity = 10, 10, 10, 10 // 抗性 10，免得除零
	got := statusPerRange(hi, lo, 30, 40, 2)
	hi.pvp, lo.pvp = false, false
	withDiff := statusPerRange(hi, lo, 30, 40, 2)
	if got >= withDiff {
		t.Fatalf("决斗里不计等级差：%d 应小于野外的 %d", got, withDiff)
	}
}

// 双方的人物同时倒下：平局。
func TestDuelBothDownIsDraw(t *testing.T) {
	b := duelFixture(t, rollRnd(9999))
	b.Unit(0).Down, b.Unit(10).Down = true, true
	if !b.checkDuelEnd() || b.Result() != "draw" || b.EndFor(0).Result != "draw" || b.EndFor(10).Result != "draw" {
		t.Fatalf("result=%s", b.Result())
	}
}
