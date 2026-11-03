// 队长离场后（逃跑、超时、掉线）其余队员继续交指令（STAGE-275 第二轮，缺口 5）：物品、精灵、攻击都不能因为
// PlayerUnit 为 nil 而 panic，队员读的是自己的人物和快照。
package battle

import (
	"testing"
	"time"
)

// threeHumans 开一场队长加两名队员的战斗：1 号带一件道具，2 号带精灵；两人都有出战宠物。
func threeHumans(t *testing.T, rnd func(n int) int) *Battle {
	t.Helper()
	m1 := testSide()
	m1.Items[5] = BattleItem{ID: 1235, Name: "普通的肉"}
	return partyWith(t, testSide(), rnd, PartyMate{No: 1, Side: m1}, PartyMate{No: 2, Side: testSide()})
}

// matesAct 让两名队员交上道具、精灵指令和宠物攻击，然后结算，结算不能 panic，两人的行动都要出现。
func matesAct(t *testing.T, b *Battle) Round {
	t.Helper()
	turn := b.Turn()
	target := monsterNo(t, b)
	mustSubmit(t, b, 1, turn, "player", "item", Cmd{Slot: 5, Target: 1})
	mustSubmit(t, b, 1, turn, "pet", "attack", Cmd{Target: target})
	mustSubmit(t, b, 2, turn, "player", "magic", Cmd{Magic: 0, Target: target})
	mustSubmit(t, b, 2, turn, "pet", "attack", Cmd{Target: target})
	if !b.AllSubmitted() {
		t.Fatal("两名队员都交齐了，应当可以结算")
	}
	round, _ := b.Settle()
	if !hasEvent(round, "item", 1) {
		t.Errorf("队员 1 的道具没有结算：%+v", round.Events)
	}
	if !hasEvent(round, "magic", 2) {
		t.Errorf("队员 2 的精灵没有结算：%+v", round.Events)
	}
	return round
}

// TestPartyLeaderEscapeMatesKeepActing 队长逃跑后，队员的道具、精灵、攻击照常。
func TestPartyLeaderEscapeMatesKeepActing(t *testing.T) {
	b := threeHumans(t, func(int) int { return 0 }) // 骰子出 0：队长必定逃脱
	mustSubmit(t, b, 0, b.Turn(), "player", "escape", Cmd{})
	mustSubmit(t, b, 0, b.Turn(), "pet", "attack", Cmd{Target: monsterNo(t, b)})
	matesAct(t, b)
	if b.PlayerUnit() != nil {
		t.Error("队长逃跑后人物应已离场")
	}
	if b.Departed(0) != "escape" {
		t.Errorf("队长的离场原因应为 escape，实际 %q", b.Departed(0))
	}
}

// TestPartyLeaderOfflineMatesKeepActing 队长掉线后，队员照常交指令、结算。
func TestPartyLeaderOfflineMatesKeepActing(t *testing.T) {
	b := threeHumans(t, rollRnd(9999))
	if _, ok := b.Offline(0); !ok {
		t.Fatal("队长掉线应当成功")
	}
	matesAct(t, b)
	if b.Over() {
		t.Error("两名队员还在场，战斗不该结束")
	}
}

// TestPartyLeaderTimeoutMatesKeepActing 队长超时被踢后（离场原因为 timeout，队伍解散），队员已交的指令照常结算。
func TestPartyLeaderTimeoutMatesKeepActing(t *testing.T) {
	b := threeHumans(t, rollRnd(9999))
	now := time.Unix(1000, 0)
	clockAt(b, &now)
	turn := b.Turn()
	target := monsterNo(t, b)
	mustSubmit(t, b, 1, turn, "player", "item", Cmd{Slot: 5, Target: 1})
	mustSubmit(t, b, 1, turn, "pet", "attack", Cmd{Target: target})
	mustSubmit(t, b, 2, turn, "player", "magic", Cmd{Magic: 0, Target: target})
	mustSubmit(t, b, 2, turn, "pet", "attack", Cmd{Target: target})
	mustSubmit(t, b, 0, turn, "player", "guard", Cmd{}) // 队长只交了人物指令，宠物没交，从这一条起算截止
	now = now.Add(PartyCommandWait + time.Second)
	out := b.ExpireDue(now)
	if len(out) != 1 || out[0].No != 0 || !out[0].Disband {
		t.Fatalf("应只踢出队长并解散，实际 %+v", out)
	}
	round, _ := b.Settle()
	if !hasEvent(round, "item", 1) || !hasEvent(round, "magic", 2) {
		t.Errorf("队员的指令应照常结算：%+v", round.Events)
	}
}

// TestPartyMateTimeoutOthersKeepActing 一名队员超时离场后，队长和另一名队员照常交指令、结算。
func TestPartyMateTimeoutOthersKeepActing(t *testing.T) {
	b := threeHumans(t, rollRnd(9999))
	now := time.Unix(1000, 0)
	clockAt(b, &now)
	turn := b.Turn()
	target := monsterNo(t, b)
	mustSubmit(t, b, 0, turn, "player", "guard", Cmd{})
	mustSubmit(t, b, 0, turn, "pet", "attack", Cmd{Target: target})
	mustSubmit(t, b, 2, turn, "player", "magic", Cmd{Magic: 0, Target: target})
	mustSubmit(t, b, 2, turn, "pet", "attack", Cmd{Target: target})
	now = now.Add(PartyCommandWait + time.Second)
	out := b.ExpireDue(now)
	if len(out) != 1 || out[0].No != 1 {
		t.Fatalf("只应踢出没交指令的队员 1，实际 %+v", out)
	}
	round, _ := b.Settle()
	if !hasEvent(round, "magic", 2) {
		t.Errorf("队员 2 的精灵没有结算：%+v", round.Events)
	}
	if b.Over() {
		t.Error("队长和队员 2 还在场，战斗不该结束")
	}
}
