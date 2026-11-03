package character

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// partyEnv 起一个有角色的世界：新建 1 级人物，给一只宠物（出战），进入世界。
func partyEnv(t *testing.T) (*env, *client) {
	t.Helper()
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	if _, err := GivePet(context.Background(), e.chars.db, e.chars.opts.PetNotifyChannel, "勇者小明", 1, 1,
		func(n int) int { return n / 2 }); err != nil {
		t.Fatal(err)
	}
	return e, c
}

func members(t *testing.T, party any) []map[string]any {
	t.Helper()
	p, ok := party.(map[string]any)
	if !ok {
		t.Fatalf("party 应是对象: %v", party)
	}
	var out []map[string]any
	for _, m := range p["members"].([]any) {
		out = append(out, m.(map[string]any))
	}
	return out
}

// 组队、重复组队、查看、解散的完整流程（STAGE-162 验收 1）。
func TestPartyCloneAndDisband(t *testing.T) {
	_, c := partyEnv(t)
	enter := c.ok("enter_world", `{"slot":0}`)
	if v, present := enter["party"]; !present || v != nil {
		t.Fatalf("没有队伍时 enter_world 的 party 应为 null: %v（存在 %v）", v, present)
	}
	if got := c.ok("party", `{}`); got["party"] != nil {
		t.Fatalf("没有队伍时 party 应为 null: %v", got)
	}

	data, pushes := c.okp("party_clone", `{}`)
	wantPushes(t, pushes, "party_changed")
	push := pushes[0]["data"].(map[string]any)
	if push["reason"] != "created" {
		t.Fatalf("推送 reason = %v，应为 created", push["reason"])
	}
	ms := members(t, data["party"])
	if len(ms) != 5 {
		t.Fatalf("成员数 = %d，应为 5", len(ms))
	}
	if len(members(t, push["party"])) != 5 {
		t.Fatalf("推送里的队伍应有 5 个成员")
	}
	if ms[0]["leader"] != true || ms[0]["clone"] != false || ms[0]["name"] != "勇者小明" {
		t.Fatalf("第一位应是队长: %v", ms[0])
	}
	for i, m := range ms[1:] {
		want := "勇者小明·分身" + string(rune('1'+i))
		if m["name"] != want || m["clone"] != true || m["leader"] != false || int(m["index"].(float64)) != i+1 {
			t.Fatalf("分身 %d = %v，名字应为 %s", i+1, m, want)
		}
		for _, k := range []string{"level", "hp", "max_hp", "mp", "max_mp", "stats", "derived", "equipment", "magic", "position"} {
			if m[k] == nil {
				t.Errorf("分身 %d 缺少字段 %s", i+1, k)
			}
		}
		if m["level"] != ms[0]["level"] || m["max_hp"] != ms[0]["max_hp"] {
			t.Errorf("分身的等级和血量上限应与队长一致: %v vs %v", m, ms[0])
		}
		pet, _ := m["pet"].(map[string]any)
		leaderPet, _ := ms[0]["pet"].(map[string]any)
		if pet == nil || pet["name"] != leaderPet["name"] || pet["level"] != leaderPet["level"] {
			t.Errorf("分身应带和队长一样的出战宠物: %v vs %v", pet, leaderPet)
		}
	}

	c.wantErr(t, "party_clone", `{}`, "ALREADY_IN_PARTY")
	if got := c.ok("party", `{}`); len(members(t, got["party"])) != 5 {
		t.Fatalf("查看队伍应有 5 个成员: %v", got)
	}
	if st := c.ok("status", `{}`); st["party"] == nil {
		t.Fatal("status 应带当前队伍")
	}

	_, pushes = c.okp("party_disband", `{}`)
	wantPushes(t, pushes, "party_changed")
	push = pushes[0]["data"].(map[string]any)
	if push["reason"] != "disbanded" || push["party"] != nil {
		t.Fatalf("解散推送 = %v", push)
	}
	if got := c.ok("party", `{}`); got["party"] != nil {
		t.Fatalf("解散后 party 应为 null: %v", got)
	}
	c.wantErr(t, "party_disband", `{}`, "NOT_IN_PARTY")
	c.okp("party_clone", `{}`) // 解散之后可以重新组
}

// 倒下的角色不能组队，不产生分身。
func TestPartyCloneDead(t *testing.T) {
	e, c := partyEnv(t)
	e.exec(t, `UPDATE characters SET hp = 0 WHERE name = '勇者小明'`)
	c.ok("enter_world", `{"slot":0}`)
	c.wantErr(t, "party_clone", `{}`, "CHARACTER_DEAD")
	if got := c.ok("party", `{}`); got["party"] != nil {
		t.Fatalf("失败后不应有队伍: %v", got)
	}
}

// 队长移动（传送）后所有成员同步到新位置：position_changed 之后紧跟 party_changed(moved)。
func TestPartyFollowsLeader(t *testing.T) {
	_, c := partyEnv(t)
	c.ok("enter_world", `{"slot":0}`)
	c.okp("party_clone", `{}`)
	c.send("warp", `{"place":1}`)
	var pos, moved map[string]any
	for pos == nil || moved == nil {
		m := c.recv()
		switch m["type"] {
		case "position_changed":
			pos = m["data"].(map[string]any)
		case "party_changed":
			if pos == nil {
				t.Fatal("party_changed 应在 position_changed 之后")
			}
			moved = m["data"].(map[string]any)
		}
	}
	if moved["reason"] != "moved" {
		t.Fatalf("reason = %v，应为 moved", moved["reason"])
	}
	for _, m := range members(t, moved["party"]) {
		p := m["position"].(map[string]any)
		if p["floor"] != pos["floor"] || p["x"] != pos["x"] || p["y"] != pos["y"] {
			t.Fatalf("成员 %v 的位置 %v 应与队长 %v 一致", m["name"], p, pos)
		}
	}
}

// agent 操控期间玩家不能组队/解散；agent 组队后两条连接都收到推送；
// 最后一条连接离开后分身消失，重新进入没有队伍。
func TestPartyControlAndLeave(t *testing.T) {
	e, c := partyEnv(t)
	c.ok("enter_world", `{"slot":0}`)
	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed") // agent 进入即接管控制权
	c.drainTo(t, "control_changed")

	c.wantErr(t, "party_clone", `{}`, "CONTROLLED_BY_AGENT")
	if got := c.ok("party", `{}`); got["party"] != nil { // 查看类不受控制权限制
		t.Fatalf("party 应为 null: %v", got)
	}
	_, pushes := agent.okp("party_clone", `{}`)
	wantPushes(t, pushes, "party_changed")
	c.drainTo(t, "party_changed") // 玩家连接也收到
	c.wantErr(t, "party_disband", `{}`, "CONTROLLED_BY_AGENT")

	// 玩家离开时 agent 还在，队伍保留。
	c.ok("leave_world", `{}`)
	if got := agent.ok("party", `{}`); got["party"] == nil {
		t.Fatal("agent 还在线，队伍应保留")
	}
	// agent 也离开：角色离开世界，队伍消失。
	agent.ok("leave_world", `{}`)
	c.ok("enter_world", `{"slot":0}`)
	if got := c.ok("party", `{}`); got["party"] != nil {
		t.Fatalf("所有连接离开后队伍应消失: %v", got)
	}
}

// playPartyBattle 打到战斗结束：每回合人物攻击第一个站着的怪，宠物也攻击它；返回 battle_end 的 data。
func playPartyBattle(t *testing.T, c *client) map[string]any {
	t.Helper()
	for {
		q, _ := c.roundtrip(t, "battle", `{}`, "")
		turn := strconv.Itoa(int(q["turn"].(float64)))
		target := strconv.Itoa(aliveMonsters(q)[0])
		c.roundtrip(t, "battle_command", `{"turn":`+turn+`,"actor":"player","action":"attack","target":`+target+`}`, "")
		c.roundtrip(t, "battle_command", `{"turn":`+turn+`,"actor":"pet","action":"attack","target":`+target+`}`, "battle_round")
		if end := c.drainToEnd(); end != nil {
			return end
		}
		c.roundtrip(t, "battle_ack", `{"turn":`+turn+`}`, "")
	}
}

// 战斗：分身一起上场（带 auto），不进 needs；奖励只有队长；战斗中可以解散再重组（STAGE-189）；
// 战后队伍仍在、分身满血、等级跟着队长，并推 party_changed(battle_end)。
func TestPartyBattle(t *testing.T) {
	e, c := partyEnv(t)
	c.ok("enter_world", `{"slot":0}`)
	c.okp("party_clone", `{}`)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	start := c.drainTo(t, "battle_start")
	units := start["units"].([]any)
	autos, total := 0, 0
	for _, u := range units {
		m := u.(map[string]any)
		total++
		if m["auto"] == true {
			autos++
			if no := int(m["no"].(float64)); !(no >= 1 && no <= 4 || no >= 6 && no <= 9) {
				t.Errorf("auto 单位的号码 %d 不在 1–4 / 6–9", no)
			}
			if !strings.Contains(m["name"].(string), "分身") && m["kind"] == "player" {
				t.Errorf("分身人物的名字应带“分身”: %v", m["name"])
			}
		}
	}
	if autos != 8 || total != 12 { // 己方 5 人 + 5 宠，敌方 2
		t.Fatalf("auto 单位 %d 个、总数 %d，应为 8 和 12", autos, total)
	}
	turn := c.drainTo(t, "battle_turn")
	if needs := turn["needs"].([]any); len(needs) != 2 {
		t.Fatalf("分身不进 needs: %v", needs)
	}
	// 战斗中解散再重组（STAGE-189）：这一场的分身照常打完，新队伍战后按结算后的队长重建。
	_, pushes := c.okp("party_disband", `{}`)
	wantPushes(t, pushes, "party_changed")
	if reason := pushes[0]["data"].(map[string]any)["reason"]; reason != "disbanded" {
		t.Fatalf("战斗中解散的 reason = %v，应为 disbanded", reason)
	}
	_, pushes = c.okp("party_clone", `{}`)
	wantPushes(t, pushes, "party_changed")

	end := playPartyBattle(t, c)
	if end["result"] != "win" {
		t.Fatalf("战斗结果 = %v，应为 win", end["result"])
	}
	rewards := end["rewards"].(map[string]any)
	for _, x := range rewards["exp"].([]any) {
		if name := x.(map[string]any)["name"].(string); strings.Contains(name, "分身") {
			t.Fatalf("分身不领奖励: %v", rewards)
		}
	}
	if n := len(rewards["exp"].([]any)); n != 2 {
		t.Fatalf("经验条目 = %d，应只有队长和队长的宠物", n)
	}

	pc := c.drainTo(t, "party_changed")
	if pc["reason"] != "battle_end" {
		t.Fatalf("reason = %v，应为 battle_end", pc["reason"])
	}
	ms := members(t, pc["party"])
	for _, m := range ms[1:] {
		if m["hp"] != m["max_hp"] {
			t.Fatalf("分身战后应满血: %v", m)
		}
		if m["level"] != ms[0]["level"] {
			t.Fatalf("分身等级 %v 应跟着队长 %v（STAGE-189）", m["level"], ms[0]["level"])
		}
	}
	got := c.ok("party", `{}`)
	if len(members(t, got["party"])) != 5 {
		t.Fatalf("战后队伍应还在: %v", got)
	}
	c.okp("party_disband", `{}`)
}
