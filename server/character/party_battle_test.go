package character

// 真人同场战斗（STAGE-275，docs/protocol.md 10.14）的接线用例：开战每人一条 battle_start、交指令的进度、
// 120 秒超时踢出（注入时钟，不真等）、掉线离队、队长掉线解散、开关关闭时退回只有队长出战。
// 需要数据库（STONEAGE_TEST_DATABASE_URL），与其它战斗用例一样走真实连接。

import (
	"math/rand/v2"
	"testing"
	"time"
)

// realPartyEnv 起一个服务端，alice（队长，勇者小明）和 bob（队员，小红）组队并进入野外。
// 时钟注入为 clk，遇敌随机种子固定；真人同场的开关打开。
func realPartyEnv(t *testing.T, seed uint64) (*env, *client, *client, *fakeClock) {
	t.Helper()
	return realPartyEnvWith(t, rand.New(rand.NewPCG(seed, seed*2+1)).IntN, 60*time.Millisecond, nil)
}

// partyOfName 返回角色当前的真人同场战斗（测试里直接读服务端状态）。
func partyOfName(t *testing.T, e *env, name string) *partyBattle {
	t.Helper()
	e.chars.mu.Lock()
	var p *Presence
	for _, cand := range e.chars.presences {
		if cand.rec.Name == name {
			p = cand
		}
	}
	e.chars.mu.Unlock()
	if p == nil {
		t.Fatalf("%s 不在世界中", name)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.battle == nil || p.battle.pb == nil {
		t.Fatalf("%s 不在真人同场战斗中", name)
	}
	return p.battle.pb
}

func TestPartyBattleStartsForEveryMember(t *testing.T) {
	_, a, b, _ := realPartyEnv(t, 11)
	start := a.turnOnEncounter(t)
	if start["kind"] != "party" || start["you"] != float64(0) {
		t.Fatalf("队长应收到 party 的 battle_start（you=0）: %v", start)
	}
	mate := b.drainTo(t, "battle_start")
	if mate["kind"] != "party" || mate["you"] != float64(1) {
		t.Fatalf("队员应收到 party 的 battle_start（you=1）: %v", mate)
	}
	turn := b.drainTo(t, "battle_turn")
	if turn["deadline_ms"] != nil {
		t.Fatalf("开战后还没人交指令，deadline_ms 应为 null: %v", turn)
	}
}

func TestPartyBattleProgressUntilEveryoneSubmits(t *testing.T) {
	_, a, b, _ := realPartyEnv(t, 12)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")
	a.roundtrip(t, "battle_command", `{"turn":1,"actor":"player","action":"guard"}`, "")
	prog := b.drainTo(t, "battle_progress")
	if prog["done"] != float64(1) || prog["total"] != float64(2) {
		t.Fatalf("队长交完后，队员看到的进度应为 1/2: %v", prog)
	}
	if prog["deadline_ms"] == nil {
		t.Fatalf("有人交过指令后应带截止时间: %v", prog)
	}
}

// 一人没交指令，120 秒到了被踢出战斗并离队；队伍只剩一人，自动解散。
func TestPartyBattleTimeoutKicksSilentMate(t *testing.T) {
	e, a, b, clk := realPartyEnv(t, 13)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")
	a.roundtrip(t, "battle_command", `{"turn":1,"actor":"player","action":"guard"}`, "")
	b.drainTo(t, "battle_progress")
	pb := partyOfName(t, e, "勇者小明")
	clk.Add(121 * time.Second) // 超过 120 秒的交指令截止时间
	pb.s.partyExpire(pb, pb.b.Turn())

	end := b.drainTo(t, "battle_end")
	if end["result"] != "forced_out" {
		t.Fatalf("超时的队员应收到 forced_out: %v", end)
	}
	kicked := b.drainTo(t, "party_changed")
	if kicked["reason"] != "kicked" || kicked["cause"] != "battle_timeout" {
		t.Fatalf("超时的队员应收到 kicked（battle_timeout）: %v", kicked)
	}
	leave := a.drainTo(t, "battle_leave")
	if leave["cause"] != "timeout" || leave["no"] != float64(1) {
		t.Fatalf("队长应收到 battle_leave（timeout，no=1）: %v", leave)
	}
	if d := a.drainTo(t, "party_changed"); d["reason"] != "disbanded" {
		t.Fatalf("两人队踢掉一人后应解散: %v", d)
	}
}

// 队员掉线：立即离场，队长收到 battle_leave（offline），队伍解散，战斗继续。
func TestPartyBattleMateOfflineLeavesAtOnce(t *testing.T) {
	_, a, b, _ := realPartyEnv(t, 14)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")
	_ = b.conn.Close()

	leave := a.drainTo(t, "battle_leave")
	if leave["cause"] != "offline" || leave["no"] != float64(1) {
		t.Fatalf("队长应收到 battle_leave（offline，no=1）: %v", leave)
	}
	if d := a.drainTo(t, "party_changed"); d["reason"] != "disbanded" || d["cause"] != "battle_offline" {
		t.Fatalf("两人队掉一人后解散（battle_offline）: %v", d)
	}
}

// 关掉开关：真人队遇敌退回只有队长出战的老行为，队员不进战斗。
func TestPartyBattleSwitchOffKeepsLeaderAlone(t *testing.T) {
	e, a, b, _ := realPartyEnv(t, 15)
	e.chars.partyPlayersOn.Store(false)
	drainSocket(b) // 跟随 warp 的推送还在连接上，先读完
	start := a.turnOnEncounter(t)
	if start["kind"] != "wild" {
		t.Fatalf("开关关闭时应是单人野外战斗: %v", start)
	}
	b.expectQuiet(t, 300*time.Millisecond)
}

// drainSocket 把连接上已经到达的推送全部读完（pending 也清掉），之后的断言才只看这次操作的结果。
func drainSocket(c *client) {
	c.pending = nil
	_ = c.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	for {
		if _, err := c.r.ReadBytes('\n'); err != nil {
			break
		}
	}
	_ = c.conn.SetReadDeadline(time.Time{})
}

// 一人超时被踢（两人队自动解散），剩下的人继续打；整场时限到了，他以 draw 结束，拿到自己的结算（不带奖励）。
func TestPartyBattleTimeLimitDrawsForRemaining(t *testing.T) {
	e, a, b, clk := realPartyEnv(t, 16)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")
	a.roundtrip(t, "battle_command", `{"turn":1,"actor":"player","action":"guard"}`, "")
	b.drainTo(t, "battle_progress")
	pb := partyOfName(t, e, "勇者小明")
	clk.Add(121 * time.Second)
	pb.s.partyExpire(pb, pb.b.Turn())
	b.drainTo(t, "battle_end")

	clk.Add(3601 * time.Second)
	pb.s.partyTimeUp(pb)
	end := a.drainTo(t, "battle_end")
	if end["result"] != "draw" {
		t.Fatalf("整场时限到了应以 draw 结束: %v", end)
	}
	if e := partyOfNameOrNil(e, "勇者小明"); e != nil {
		t.Fatalf("战斗结束后队长不应还在战斗中")
	}
}

// partyOfNameOrNil 同 partyOfName，但不在战斗中时返回 nil。
func partyOfNameOrNil(e *env, name string) *partyBattle {
	e.chars.mu.Lock()
	var p *Presence
	for _, cand := range e.chars.presences {
		if cand.rec.Name == name {
			p = cand
		}
	}
	e.chars.mu.Unlock()
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.battle == nil {
		return nil
	}
	return p.battle.pb
}
