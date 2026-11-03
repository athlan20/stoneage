// 分身队自动战斗方案的测试（STAGE-195）：解析的合法、ignored 与默认值，party_clone 的响应，
// 以及带方案组的队在战斗里真的按方案出招。

package character

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/battle"
)

func TestParseAutoPlan(t *testing.T) {
	legal := `{"version":1,"target":"random",
		"player":{"act":"magic","magic_slot":2,"magic_name":"治愈的精灵 Lv1"},
		"pet":{"act":"skill","skill_slot":3,"skill_name":"通常攻击"},
		"first":{"enabled":true,"player":{"act":"guard"},"pet":{"act":"attack","skill_slot":1}},
		"recover":{"player_heal":false,"player_heal_pct":50,"pet_heal":true,"pet_heal_pct":10,"player_mp":true,"player_mp_pct":40},
		"capture":{"enabled":true,"hp_pct":20,"level_min":3,"level_max":9,"names":["乌力"]},
		"flee":{"enabled":true,"hp_pct":15,"names":[]},"playback":{"speed":2,"delay_ms":300}}`
	plan, status := parseAutoPlan(json.RawMessage(legal))
	if status != autoProfileApplied {
		t.Fatalf("合法方案 status = %s，应为 applied", status)
	}
	want := &battle.AutoPlan{
		Target:  "random",
		Player:  battle.PlanAct{Act: "magic", Slot: 2, Name: "治愈的精灵 Lv1"},
		Pet:     battle.PlanAct{Act: "skill", Slot: 3, Name: "通常攻击"},
		First:   battle.PlanFirst{Enabled: true, Player: battle.PlanAct{Act: "guard"}, Pet: battle.PlanAct{Act: "attack", Slot: 1}},
		Recover: battle.PlanRecover{PlayerHeal: false, PlayerHealPct: 50, PetHeal: true, PetHealPct: 10, PlayerMP: true, PlayerMPPct: 40},
		Capture: battle.PlanCapture{Enabled: true, LevelMin: 3, LevelMax: 9, Names: []string{"乌力"}},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("解析结果 = %+v，应为 %+v", plan, want)
	}

	// 缺的字段取客户端默认值：目标合击，人物和宠物普攻，补血 30%，首回合关。
	plan, status = parseAutoPlan(json.RawMessage(`{"version":1}`))
	if status != autoProfileApplied {
		t.Fatalf("只有版本的方案 status = %s，应为 applied", status)
	}
	if plan.Target != "combo" || plan.Player.Act != "attack" || plan.Pet.Act != "attack" || plan.First.Enabled {
		t.Errorf("默认值不对: %+v", plan)
	}
	if !plan.Recover.PlayerHeal || plan.Recover.PlayerHealPct != 30 || plan.Capture.LevelMin != 1 {
		t.Errorf("默认的补血或捕捉不对: %+v", plan.Recover)
	}

	ignored := map[string]string{
		"版本 99":    `{"version":99,"target":"combo"}`,
		"没有版本":     `{"target":"combo"}`,
		"null":     `null`,
		"JSON 不对":  `{"version":1,`,
		"类型不对":     `{"version":1,"target":5}`,
		"补血门槛不是数字": `{"version":1,"recover":{"player_heal_pct":"三十"}}`,
		"目标不认识":    `{"version":1,"target":"all"}`,
		"人物动作不认识":  `{"version":1,"player":{"act":"dance"}}`,
		"战宠动作不认识":  `{"version":1,"pet":{"act":"magic"}}`,
		"首回合动作不认识": `{"version":1,"first":{"enabled":true,"player":{"act":"flee"}}}`,
	}
	for name, raw := range ignored {
		plan, status := parseAutoPlan(json.RawMessage(raw))
		if status != autoProfileIgnored || plan != nil {
			t.Errorf("%s：status = %s，plan = %v，应为 ignored 且没有方案", name, status, plan)
		}
	}
}

// party_clone 带方案：applied；版本不对：ignored 但请求照常成功；不带字段：响应里没有 auto_profile。
func TestPartyCloneAutoProfileReply(t *testing.T) {
	_, c := partyEnv(t)
	c.ok("enter_world", `{"slot":0}`)

	data, _ := c.okp("party_clone", `{"auto_profile":{"version":1,"target":"combo"}}`)
	if data["auto_profile"] != autoProfileApplied {
		t.Fatalf("带合法方案的响应 auto_profile = %v，应为 applied", data["auto_profile"])
	}
	if len(members(t, data["party"])) != 5 {
		t.Fatalf("带方案组队时队伍应照常有 5 个成员")
	}
	c.okp("party_disband", `{}`)

	data, _ = c.okp("party_clone", `{"auto_profile":{"version":99}}`)
	if data["auto_profile"] != autoProfileIgnored || data["party"] == nil {
		t.Fatalf("版本不对的响应 = %v，应为 ignored 且照常组队", data)
	}
	c.okp("party_disband", `{}`)

	data, _ = c.okp("party_clone", `{}`)
	if _, present := data["auto_profile"]; present {
		t.Fatalf("没带方案时响应里不应有 auto_profile: %v", data)
	}
}

// 带方案组的队在战斗里按方案出招：方案说人物和宠物都防御，分身的 guard 事件出现；不带方案时分身普攻（见 TestPartyBattle）。
func TestPartyBattleFollowsProfile(t *testing.T) {
	e, c := partyEnv(t)
	c.ok("enter_world", `{"slot":0}`)
	c.okp("party_clone", `{"auto_profile":{"version":1,"first":{"enabled":false},"player":{"act":"guard"},"pet":{"act":"guard"}}}`)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	live := e.chars.LiveOf(e.session("alice"))
	if live.p.party.plan == nil || live.p.party.plan.Player.Act != "guard" {
		t.Fatalf("队伍上的方案 = %+v，应为人物防御", live.p.party.plan)
	}
	c.drainTo(t, "battle_start")
	c.drainTo(t, "battle_turn")
	q, _ := c.roundtrip(t, "battle", `{}`, "")
	turn := strconv.Itoa(int(q["turn"].(float64)))
	target := strconv.Itoa(aliveMonsters(q)[0])
	c.roundtrip(t, "battle_command", `{"turn":`+turn+`,"actor":"player","action":"attack","target":`+target+`}`, "")
	_, round := c.roundtrip(t, "battle_command", `{"turn":`+turn+`,"actor":"pet","action":"attack","target":`+target+`}`, "battle_round")

	guards := map[int]bool{}
	for _, ev := range round["events"].([]any) {
		m := ev.(map[string]any)
		if m["kind"] == "guard" && m["actor"] != nil {
			guards[int(m["actor"].(float64))] = true
		}
	}
	for _, no := range []int{1, 2, 3, 4, 6, 7, 8, 9} {
		if !guards[no] {
			t.Errorf("分身单位 %d 应防御（方案），实际防御的单位: %v", no, guards)
		}
	}
}
