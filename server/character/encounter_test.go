// 遇敌开关的验收（STAGE-40）：村子里打不开、野外打开马上进战斗、打完隔一会儿自动开下一场、
// 关掉后当前这场照常打完、传送后自动关闭、status 带开关状态。
// 野外的怪物组成按遇敌表抽，随机源固定种子，整场进程可复现。

package character

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"
)

// encounterEnv 起一个遇敌间隔可调、随机源固定的环境，并建好角色站在村里。
func encounterEnv(t *testing.T, interval time.Duration, seed uint64) (*env, *client) {
	t.Helper()
	e := newEnv(t, func(s *Service) {
		s.opts.EncounterInterval = interval
		s.opts.WildRnd = rand.New(rand.NewPCG(seed, seed*2+1)).IntN
	})
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// turnOnEncounter 在野外打开遇敌开关：等到第一场的 battle_start、battle_turn 和开关推送，
// 并消费掉 on 请求的响应（它排在所有推送之后）。
func (c *client) turnOnEncounter(t *testing.T) map[string]any {
	t.Helper()
	c.send("encounter", `{"set":"on"}`)
	start := c.drainTo(t, "battle_start")
	c.drainTo(t, "battle_turn")
	c.drainTo(t, "encounter_changed")
	if on := drainOK(c); on["encounter"] != true {
		t.Fatalf("encounter on 应成功: %v", on)
	}
	return start
}

// drainOK 读到下一条成功响应并返回 data；路上先到的推送留在待读队列里。
func drainOK(c *client) map[string]any {
	for {
		m := c.recv()
		if m["ok"] == true {
			return m["data"].(map[string]any)
		}
	}
}

// warpTo 传送到地点并等到位置推送（warp 的响应之前会先推 position_changed）。
func (c *client) warpTo(t *testing.T, place int) {
	t.Helper()
	c.roundtrip(t, "warp", `{"place":`+strconv.Itoa(place)+`}`, "position_changed")
}

// expectError 发一条请求，跳过路上先到的推送，等到错误响应并核对错误码。
func (c *client) expectError(t *testing.T, typ, data, wantCode string) {
	t.Helper()
	c.send(typ, data)
	for {
		m := c.recv()
		if e, ok := m["error"].(map[string]any); ok {
			if code, _ := e["code"].(string); code != wantCode {
				t.Fatalf("%s 应回复 %s: %v", typ, wantCode, m)
			}
			return
		}
		if m["ok"] == true {
			t.Fatalf("%s 不应成功: %v", typ, m)
		}
	}
}

// playWildToEnd 让角色一直打第一个还站着的怪物（没有可打目标时防御），把当前这场打完
// （结果不定：可能赢、可能输）；返回 battle_end 的 data。战斗已在进行中即可调用（每轮先查当前回合号）。
func (c *client) playWildToEnd(t *testing.T) map[string]any {
	t.Helper()
	for {
		q, _ := c.roundtrip(t, "battle", `{}`, "")
		turn := int(q["turn"].(float64))
		body := `"actor":"player","action":"guard"`
		if monsters := aliveMonsters(q); len(monsters) > 0 {
			body = `"actor":"player","action":"attack","target":` + strconv.Itoa(monsters[0])
		}
		if needs, ok := q["needs"].([]any); ok && len(needs) > 0 {
			c.roundtrip(t, "battle_command",
				`{"turn":`+strconv.Itoa(turn)+`,`+body+`}`, "battle_round")
		} else {
			c.drainTo(t, "battle_round")
		}
		if end := c.drainToEnd(); end != nil {
			return end
		}
		c.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(turn)+`}`, "")
	}
}

// drainEncounterOff 读到 encounter_changed 的关闭推送（自动关闭时带 reason），并顺手消费随附的 notice。
func (c *client) drainEncounterOff(t *testing.T) map[string]any {
	t.Helper()
	for {
		m := c.recv()
		if m["type"] == "encounter_changed" {
			d := m["data"].(map[string]any)
			if d["encounter"] == false {
				return d
			}
		}
	}
}

// TestEncounterInVillageRejected 是验收 3：村里（村长家、村里的门点）没有敌人，
// encounter on 回复 NO_ENEMY_HERE，开关保持关闭。
func TestEncounterInVillageRejected(t *testing.T) {
	_, c := encounterEnv(t, 50*time.Millisecond, 1)
	// 新角色出生在村长家。
	c.expectError(t, "encounter", `{"set":"on"}`, "NO_ENEMY_HERE")
	// 传送到萨姆吉尔村里（村内地图），一样没有敌人。
	c.warpTo(t, 1)
	c.expectError(t, "encounter", `{"set":"on"}`, "NO_ENEMY_HERE")
	// 开关没有打开，status 里是 false，也没有战斗推送。
	st, _ := c.roundtrip(t, "status", `{}`, "")
	if st["encounter"] != false {
		t.Fatalf("status.encounter 应为 false: %v", st)
	}
	c.expectQuiet(t, 300*time.Millisecond)
}

// TestEncounterChain 是验收 2：野外打开开关马上进战斗；关掉开关后当前这场照常打完，不再开新的一场。
func TestEncounterChain(t *testing.T) {
	_, c := encounterEnv(t, 60*time.Millisecond, 7)

	c.warpTo(t, 5) // 萨姆吉尔近郊（遇敌区域 21）
	start := c.turnOnEncounter(t)
	if start["kind"] != "wild" || len(start["units"].([]any)) < 2 { // 人物 + 至少一只怪
		t.Fatalf("打开开关应马上进战斗: %v", start)
	}

	// 开关还开着就先关掉：当前这场照常打完，之后不再有下一场。
	resp, _ := c.roundtrip(t, "encounter", `{"set":"off"}`, "encounter_changed")
	if resp["encounter"] != false {
		t.Fatalf("encounter off 应成功: %v", resp)
	}
	c.playWildToEnd(t)
	c.drainTo(t, "status_changed")         // battle_end 之后的数值变化
	c.expectQuiet(t, 400*time.Millisecond) // 间隔 60ms 的下一场不会再来了
}

// TestEncounterNextBattle 是验收 1 的连场：第一场结束后隔一会儿自动开第二场；打输则自动关掉。
func TestEncounterNextBattle(t *testing.T) {
	_, c := encounterEnv(t, 60*time.Millisecond, 11)

	c.warpTo(t, 5)
	first := c.turnOnEncounter(t)
	end := c.playWildToEnd(t)
	c.drainTo(t, "status_changed")
	var next map[string]any
	if end["result"] == "lose" {
		// 输了：被打飞（传回记录点）时开关自动关掉并说明原因（验收 4 的一部分）；只是被打死、原地复活时
		// 开关保持开着，隔一会儿照常开下一场（协议 5.29，STAGE-194）。两种走向都要认，战斗随机决定走哪一种。
		off, start := c.waitAfterLose(t)
		if off != nil {
			if off["reason"] != "lose" {
				t.Fatalf("打输自动关闭的 reason 应为 lose: %v", off)
			}
			c.expectQuiet(t, 300*time.Millisecond)
			return
		}
		next = start
	} else {
		next = c.drainTo(t, "battle_start") // 间隔 60ms 后的下一场
	}
	if next["battle_id"] == first["battle_id"] {
		t.Fatalf("第二场应是新的战斗: %v", next)
	}

	// 关掉开关，第二场照常打完，不再开第三场。
	if resp, _ := c.roundtrip(t, "encounter", `{"set":"off"}`, "encounter_changed"); resp["encounter"] != false {
		t.Fatalf("encounter off 应成功: %v", resp)
	}
	c.playWildToEnd(t)
	c.drainTo(t, "status_changed")
	c.expectQuiet(t, 400*time.Millisecond)
}

// waitAfterLose 等打输之后的第一条结果：encounter_changed 关闭推送（被打飞）返回 off；
// battle_start（只是被打死，开关还开着，下一场直接开）返回 start。
func (c *client) waitAfterLose(t *testing.T) (off, start map[string]any) {
	t.Helper()
	for {
		m := c.recv()
		switch m["type"] {
		case "encounter_changed":
			if d := m["data"].(map[string]any); d["encounter"] == false {
				return d, nil
			}
		case "battle_start":
			return nil, m["data"].(map[string]any)
		}
	}
}

// TestEncounterWarpTurnsOff 是验收 4 的一部分：两场之间的空档里传送走，
// 开关自动关闭（带原因与提示），之后不再开新的一场。
func TestEncounterWarpTurnsOff(t *testing.T) {
	_, c := encounterEnv(t, 400*time.Millisecond, 9)

	c.warpTo(t, 6) // 玛丽娜丝近郊
	c.turnOnEncounter(t)
	end := c.playWildToEnd(t)
	c.drainTo(t, "status_changed")

	if end["result"] == "lose" && !loseInPlace(end) {
		c.drainEncounterOff(t) // 被打飞送走也会自动关，路径不同但要关掉
		return
	}
	if end["result"] == "lose" { // 被打死原地复活：开关保持开着，下一场照常开（STAGE-194）
		c.drainUntilNextBattle(t)
		return
	}
	// 战斗结束到下一场之间（间隔 400ms）传送走：开关关掉，下一场不再开。
	c.warpTo(t, 1)
	off := c.drainEncounterOff(t)
	if off["reason"] != "warp" {
		t.Fatalf("传送自动关闭的 reason 应为 warp: %v", off)
	}
	c.expectQuiet(t, 900*time.Millisecond)
}

// TestEncounterQueryAndStatus 验证不带参数的查询和 status 里的开关字段，
// 以及观察方（agent 在操控时）发操作类请求被拦（协议 9.3）。
func TestEncounterQueryAndStatus(t *testing.T) {
	e, c := encounterEnv(t, 50*time.Millisecond, 3)
	q, _ := c.roundtrip(t, "encounter", `{}`, "")
	if q["encounter"] != false {
		t.Fatalf("初始应关闭: %v", q)
	}
	st, _ := c.roundtrip(t, "status", `{}`, "")
	if _, has := st["encounter"]; !has {
		t.Fatal("status 里应有 encounter 字段")
	}
	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed")   // agent 进入即接管控制权
	c.expectError(t, "encounter", `{"set":"on"}`, "CONTROLLED_BY_AGENT") // 玩家成了观察方
}
