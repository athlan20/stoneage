// 战斗接线的并发与收场用例：回放确认按连接去重（STAGE-54）、捕捉成功不死锁且战斗后
// 还能继续操作（STAGE-52）、战斗中最后一条连接断开按原版 BATTLE_Exit 收场并保存（STAGE-53）。

package character

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// roundtrip 发一条请求并等它的响应；expectPush 非空时还要等到这个推送
// （结算在请求的处理里同步做，推送和响应谁先到不一定，两边都等到才返回）。
// 中间读到的其他消息放回待读队列的队首，由后续的 drainTo 消费。
func (c *client) roundtrip(t *testing.T, typ, data, expectPush string) (resp, push map[string]any) {
	t.Helper()
	c.send(typ, data)
	var leftover []map[string]any
	defer func() { c.pending = append(leftover, c.pending...) }()
	for resp == nil || (expectPush != "" && push == nil) {
		m := c.recv()
		switch {
		case m["ok"] != nil:
			if m["ok"] != true {
				t.Fatalf("%s 应成功: %v", typ, m)
			}
			resp, _ = m["data"].(map[string]any)
		case expectPush != "" && m["type"] == expectPush:
			push, _ = m["data"].(map[string]any)
		default:
			leftover = append(leftover, m)
		}
	}
	return resp, push
}

// drainTo 读取连接上的消息直到出现指定类型的推送，超时则失败（卡死的表现就是等不到）。
func (c *client) drainTo(t *testing.T, typ string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("5 秒内没有等到 %s（战斗推进可能卡住了）", typ)
		}
		m := c.recv()
		if m["type"] == typ {
			return m["data"].(map[string]any)
		}
	}
}

// expectQuiet 断言一段时间内这条连接上没有任何消息（用来验证重复确认没有推进战斗）。
func (c *client) expectQuiet(t *testing.T, d time.Duration) {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	if _, err := c.r.ReadBytes('\n'); err == nil {
		t.Fatalf("%v 内不应再有消息（战斗被提前推进了）", d)
	}
	_ = c.conn.SetReadDeadline(time.Time{})
}

// drainToEnd 等 battle_end（跳过路上别的消息）；1 秒内没等到说明战斗还要打下一回合，返回 nil。
func (c *client) drainToEnd() map[string]any {
	for {
		var m map[string]any
		if len(c.pending) > 0 {
			m = c.pending[0]
			c.pending = c.pending[1:]
		} else {
			_ = c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			line, err := c.r.ReadBytes('\n')
			if err != nil {
				return nil
			}
			_ = json.Unmarshal(line, &m)
		}
		if m["type"] == "battle_end" {
			return m["data"].(map[string]any)
		}
	}
}

// TestBattleAckCountedPerConnection 是 STAGE-54 的验收：同一条连接对同一回合重复确认
// 只计一次；要等另一条连接确认后战斗才进入下一回合。
func TestBattleAckCountedPerConnection(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed") // agent 进入即接管控制权

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	agent.drainTo(t, "battle_turn")

	// 操控方（agent）交指令触发结算，两条连接各收到 battle_round。
	agent.roundtrip(t, "battle_command", `{"turn":1,"actor":"player","action":"guard"}`, "battle_round")
	c.drainTo(t, "battle_round")

	// 玩家连接连发两次同回合确认：只计一次，不能推进。
	c.roundtrip(t, "battle_ack", `{"turn":1}`, "")
	c.roundtrip(t, "battle_ack", `{"turn":1}`, "")
	c.expectQuiet(t, 300*time.Millisecond)
	agent.expectQuiet(t, 300*time.Millisecond)

	// 第二条连接（agent）确认后才进入下一回合（确认的响应在 battle_turn 推送之后）。
	agent.roundtrip(t, "battle_ack", `{"turn":1}`, "battle_turn")
	c.drainTo(t, "battle_turn")
}

// aliveMonsters 从 battle 查询里取还站着的怪物（hp_pct 为 0 表示已倒下）。
func aliveMonsters(q map[string]any) []int {
	var out []int
	for _, u := range q["units"].([]any) {
		m := u.(map[string]any)
		if m["kind"] == "monster" && m["hp_pct"].(float64) > 0 {
			out = append(out, int(m["no"].(float64)))
		}
	}
	return out
}

// TestCaptureBattleKeepsWorking 是 STAGE-52 的验收：整场带捕捉的战斗在时限内打完
// （死锁会卡死在结算里，drainTo 的 5 秒时限会失败），捕捉成功后角色还能继续操作，宠物也入库。
// 出招按场上实况自适应：先压血量再捕捉（捕捉概率随血量下降上升），捉到一只后清场。
func TestCaptureBattleKeepsWorking(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	if _, err := GivePet(context.Background(), e.chars.db, e.chars.opts.PetNotifyChannel, "勇者小明", 1, 1,
		func(n int) int { return n / 2 }); err != nil { // 乌力一只，出战
		t.Fatal(err)
	}
	c.ok("enter_world", `{"slot":0}`)

	if err := e.chars.StartBattle("勇者小明", "capture"); err != nil {
		t.Fatal(err)
	}
	captured := false
	for turn := 1; turn <= 30; turn++ {
		c.drainTo(t, "battle_turn")
		q, _ := c.roundtrip(t, "battle", `{}`, "")
		monsters := aliveMonsters(q)
		if len(monsters) == 0 {
			t.Fatal("场上没有怪物，战斗却没有结束")
		}
		needs := q["needs"].([]any)
		var round map[string]any
		for i, actor := range needs { // 顺序 player、pet
			body := `"actor":"` + actor.(string) + `","action":"guard"`
			if actor == "player" {
				action := "attack"
				if !captured && turn > 1 && len(monsters) > 1 {
					action = "capture" // 还有两只时捉一只（留着另一只保证战斗不提前结束）
				}
				body = `"actor":"player","action":"` + action + `","target":` + strconv.Itoa(monsters[0])
			}
			if i == len(needs)-1 { // 最后一条出招的响应之前会有 battle_progress、battle_round 推送
				_, round = c.roundtrip(t, "battle_command", `{"turn":`+strconv.Itoa(turn)+`,`+body+`}`, "battle_round")
			} else {
				c.roundtrip(t, "battle_command", `{"turn":`+strconv.Itoa(turn)+`,`+body+`}`, "")
			}
		}
		if round == nil { // needs 为空（人物宠物都不能动）时不会有出招，直接等回合结算
			round = c.drainTo(t, "battle_round")
		}
		for _, ev := range round["events"].([]any) {
			if e := ev.(map[string]any); e["kind"] == "capture" && e["ok"] == true {
				captured = true
			}
		}
		if end := c.drainToEnd(); end != nil {
			if end["result"] != "win" {
				t.Fatalf("战斗结果应为 win: %v", end)
			}
			break
		}
		c.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(turn)+`}`, "") // 确认后的 battle_turn 由下一轮循环消费
	}
	if !captured {
		t.Fatal("这场种子的战斗里一次捕捉都没成功，捕捉成功的路径没有覆盖到")
	}

	// 战斗结束、写库之后角色还能继续操作（STAGE-52 的卡死症状就是这里不再有回复）。
	c.roundtrip(t, "turn", `{"direction":0}`, "") // 路上先到的 status_changed / pets_changed 推送会被存回队列
	pets, _ := c.roundtrip(t, "pets", `{}`, "")
	if list := pets["pets"].([]any); len(list) != 2 { // 原有 1 只 + 捕到的 1 只
		t.Fatalf("应有 2 只宠物（捕捉到的要进列表），实际 %d: %v", len(list), list)
	}
	var rows int
	if err := e.chars.db.QueryRow(context.Background(),
		`SELECT count(*) FROM character_pets p JOIN characters c ON c.id = p.character_id WHERE c.name = '勇者小明'`).
		Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("宠物表应有 2 行（捕捉到的要写库），实际 %d", rows)
	}
	// STAGE-215：捕到的宠物有起始值，战斗结束后它没有再升级，起始值等于它现在的等级和能力。
	for _, p := range dbPets(t, e, "勇者小明") {
		if p.Slot == 1 {
			requireFreshOrigin(t, p, "捕捉到的宠物")
		}
	}
}

// TestBattleDisconnectExitsAndSaves 是 STAGE-53 的验收：战斗中最后一条连接断开，
// 战斗按原版 BATTLE_Exit 立即作废（不结算、不判负、不回出生点），受过的伤害照常保存。
func TestBattleDisconnectExitsAndSaves(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	before := c.ok("enter_world", `{"slot":0}`)
	hp0 := int(before["status"].(map[string]any)["hp"].(float64))
	f0, x0, y0 := position(before)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	// 固定种子下第 1 回合人物防御会把怪物的攻击全挡掉（STAGE-291：怪物计划在回合开始定下，走向变了），
	// 所以改成人物攻击，最多打几回合，直到人物挨过打；只数打在人物（编号 0）身上的伤害。
	delta := 0
	for turn := 1; turn <= 6 && delta == 0; turn++ {
		if turn > 1 {
			c.drainTo(t, "battle_turn")
		}
		_, round := c.roundtrip(t, "battle_command", fmt.Sprintf(`{"turn":%d,"actor":"player","action":"attack","target":15}`, turn), "battle_round")
		for _, ev := range round["events"].([]any) {
			effects, _ := ev.(map[string]any)["effects"].([]any)
			for _, ef := range effects {
				m := ef.(map[string]any)
				if d, ok := m["delta"].(float64); ok && d < 0 && m["target"].(float64) == 0 {
					delta += int(d)
				}
			}
		}
	}
	if delta == 0 {
		t.Fatal("几回合内人物都没挨打，后面的断言区分不出战斗血量有没有保存")
	}

	_ = c.conn.Close() // 不发 leave_world，直接断开

	c2 := e.loggedIn("alice")
	after, _ := c2.roundtrip(t, "enter_world", `{"slot":0}`, "")
	if _, inBattle := after["battle"]; inBattle {
		t.Fatalf("断开后战斗应已作废，enter_world 不应再有 battle 字段: %v", after)
	}
	hp1 := int(after["status"].(map[string]any)["hp"].(float64))
	if hp1 != hp0+delta || hp1 < 1 || hp1 > hp0 {
		t.Fatalf("血量应是战斗中扣过的值 %d（初始 %d，本回合 %d），实际 %d", hp0+delta, hp0, delta, hp1)
	}
	if f, x, y := position(after); f != f0 || x != x0 || y != y0 {
		t.Fatalf("战斗作废不应回出生点，位置应在 %d(%d,%d)，实际 %d(%d,%d)", f0, x0, y0, f, x, y)
	}
	// 数据库里保存的也是战斗后的血量。
	if _, _, _, dbHP := e.queryPos("勇者小明"); dbHP != hp1 {
		t.Fatalf("数据库血量 %d 应与重连读到的一致（%d）", dbHP, hp1)
	}
}
