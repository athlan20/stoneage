package character

// 真人同场战斗测试的公共辅助（STAGE-275 第三轮）：可指定遇敌随机数与间隔的环境、按 needs 交指令、打到结束。

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"
)

// realPartyEnvWith 同 realPartyEnv，但遇敌随机数与遇敌间隔由调用方给（间隔很长时，队长打完不会自动开下一场）。
func realPartyEnvWith(t *testing.T, rnd func(int) int, interval time.Duration, pre func(e *env)) (*env, *client, *client, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Now()}
	e := newEnv(t, func(s *Service) {
		s.opts.EncounterInterval = interval
		s.opts.WildRnd = rnd
		s.opts.Now = clk.Now
	})
	e.chars.partyPlayersOn.Store(true)
	a := e.loggedIn("alice")
	a.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	a.ok("enter_world", `{"slot":0}`)
	b := e.loggedIn("bob")
	b.ok("create_character", createJSON(0, "小红", 0))
	clearStartPets(t, e, "小红")
	if pre != nil { // 进入世界之前的准备（如给队员一只宠物）
		pre(e)
	}
	b.ok("enter_world", `{"slot":0}`)
	a.roundtrip(t, "party_invite", `{"name":"小红"}`, "")
	req := b.drainTo(t, "party_request")["request"].(map[string]any)
	b.roundtrip(t, "party_reply", fmt.Sprintf(`{"id":%q,"accept":true}`, req["id"]), "")
	a.warpTo(t, 5)
	return e, a, b, clk
}

// battleNeeds 查询这个真人本回合需要交的指令（battle 查询的 needs）。
func battleNeeds(t *testing.T, c *client) (map[string]any, []string) {
	t.Helper()
	q, _ := c.roundtrip(t, "battle", `{}`, "")
	list, _ := q["needs"].([]any)
	var needs []string
	for _, n := range list {
		needs = append(needs, n.(string))
	}
	return q, needs
}

// cmdPick 给出一个身份（player / pet）这回合的动作与目标（目标为 -1 表示没有）；mons 是场上活着的怪物。
type cmdPick func(actor string, mons []int) (action string, target int)

// pickAttack 人物与宠物都攻击第一只活着的怪物（防御之类的动作不用）。
func pickAttack(actor string, mons []int) (string, int) { return "attack", mons[0] }

// submitCmds 按 needs 逐条交指令，动作由 pick 决定。
func submitCmds(t *testing.T, c *client, turn int, pick cmdPick) {
	t.Helper()
	q, needs := battleNeeds(t, c)
	submitWith(t, c, turn, pick, q, needs)
}

// submitWith 按已经查到的 needs 交指令（q 是同一时刻的 battle 查询结果）。
func submitWith(t *testing.T, c *client, turn int, pick cmdPick, q map[string]any, needs []string) {
	t.Helper()
	monsters := aliveMonsters(q)
	for _, actor := range needs {
		action, target := pick(actor, monsters)
		body := map[string]any{"turn": turn, "actor": actor, "action": action}
		if target >= 0 {
			body["target"] = target
		}
		raw, _ := json.Marshal(body)
		c.roundtrip(t, "battle_command", string(raw), "")
	}
}

// submitNeeds 同 submitCmds：人物防御、宠物防御（pick 为空时的默认动作）。
func submitNeeds(t *testing.T, c *client, turn int, pick func(actor string) string) {
	t.Helper()
	submitCmds(t, c, turn, func(actor string, _ []int) (string, int) { return pick(actor), -1 })
}

// settled 在一回合的 battle_round 之后调用：battle 查询要等服务端的战斗锁（结算、结束写回都在锁里做完），
// 响应到达时 battle_end 已经排在前面，所以这里不靠真实时间等，直接取。没有结束返回 nil。
func (c *client) settled(t *testing.T) map[string]any {
	t.Helper()
	c.roundtrip(t, "battle", `{}`, "")
	return c.endIfAny(50 * time.Millisecond)
}

// endIfAny 在 d 之内等到 battle_end 就返回它；路上的其它消息留在待读队列里。等不到返回 nil。
func (c *client) endIfAny(d time.Duration) map[string]any {
	for i, m := range c.pending {
		if m["type"] == "battle_end" {
			c.pending = append(c.pending[:i:i], c.pending[i+1:]...)
			return m["data"].(map[string]any)
		}
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()
	for {
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			return nil
		}
		var m map[string]any
		_ = json.Unmarshal(line, &m)
		if m["type"] == "battle_end" {
			return m["data"].(map[string]any)
		}
		c.pending = append(c.pending, m)
	}
}

// collectUntil 读到 stop 类型的推送为止（含它），返回途中的全部消息，供断言某类消息没有出现。
func collectUntil(t *testing.T, c *client, stop string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for {
		m := c.recv()
		out = append(out, m)
		if m["type"] == stop {
			return out
		}
	}
}

// hasType 判断消息列表里有没有某类推送。
func hasType(msgs []map[string]any, typ string) bool {
	for _, m := range msgs {
		if m["type"] == typ {
			return true
		}
	}
	return false
}

// fightAlone 让这个真人一直打到战斗结束（每回合 pick 决定动作，然后确认）。返回 battle_end。
func fightAlone(t *testing.T, c *client, maxTurns int, pick cmdPick) map[string]any {
	t.Helper()
	for i := 0; i < maxTurns; i++ {
		n := int(c.drainTo(t, "battle_turn")["turn"].(float64))
		submitCmds(t, c, n, pick)
		c.drainTo(t, "battle_round")
		if end := c.settled(t); end != nil {
			return end
		}
		c.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(n)+`}`, "")
	}
	t.Fatalf("%d 回合内战斗没有结束", maxTurns)
	return nil
}

// fightTogether 让两个真人同场从 first 回合起打到结束：每回合两人各交指令（pa、pb），两人都收到结束时返回两份 battle_end。
// pb 为 nil 时队员用 pa（不用它的话传 pickAttack）。pick 的回调里可以读服务端状态（这时没有持有服务端的锁）。
func fightTogether(t *testing.T, a, b *client, maxTurns, first int, pa, pb cmdPick) (map[string]any, map[string]any) {
	t.Helper()
	na, nb := first, first // first 回合的 battle_turn 已由调用方读掉
	for i := 0; i < maxTurns; i++ {
		if i > 0 {
			na = int(a.drainTo(t, "battle_turn")["turn"].(float64))
			nb = int(b.drainTo(t, "battle_turn")["turn"].(float64))
		}
		// 先查两人的 needs 再交：一方已经没有需要交的指令（如队员倒下）时，另一方的指令可能直接结算、结束战斗，
		// 之后再查这一方会得到“不在战斗中”。所以只按开始时查到的 needs 交，结算由服务端决定。
		qa, needsA := battleNeeds(t, a)
		qb, needsB := battleNeeds(t, b)
		submitWith(t, a, na, pa, qa, needsA)
		submitWith(t, b, nb, pb, qb, needsB)
		a.drainTo(t, "battle_round")
		b.drainTo(t, "battle_round")
		endA := a.settled(t)
		endB := b.settled(t)
		if (endA == nil) != (endB == nil) {
			t.Fatalf("两人的战斗结束不同步：队长 %v 队员 %v", endA, endB)
		}
		if endA != nil {
			return endA, endB
		}
		a.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(na)+`}`, "")
		b.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(nb)+`}`, "")
	}
	t.Fatalf("%d 回合内战斗没有结束", maxTurns)
	return nil, nil
}

// recOf 取角色当前内存里的记录副本（测试读的是服务端状态，战斗结束后调用）。
func recOf(t *testing.T, e *env, name string) record {
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
	return p.rec
}

// eventOf 取 battle_round 里第一条指定种类的事件。
func eventOf(round map[string]any, kind string) map[string]any {
	for _, ev := range round["events"].([]any) {
		if e := ev.(map[string]any); e["kind"] == kind {
			return e
		}
	}
	return nil
}

// rand2IntN 返回一个固定种子的随机数函数（测试用）。
func rand2IntN(seed1, seed2 uint64) func(int) int {
	return rand.New(rand.NewPCG(seed1, seed2)).IntN
}
