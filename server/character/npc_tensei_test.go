package character

// STAGE-266：主城精灵王（人物转生）：NPC 坐标、资格三分支、台词表、执行后的数值（纯函数），
// 以及窗口流程（取消不改数据、确定后的推送和结果）。窗口测试需要数据库（同美容师的测试）。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 萨姆吉尔村（地图 1000）的主城精灵王：npcs.tsv 的 1793 号，坐标 (96,99)。
const tenseiNPC = 1793

func TestTenseiNPCsInTowns(t *testing.T) {
	for _, c := range []struct{ floor, id, x, y int }{
		{1000, 1793, 96, 99}, {2000, 1794, 69, 58}, {3000, 1795, 94, 97}, {4000, 1796, 69, 57},
	} {
		found := false
		for _, n := range gamedata.NPCsByFloor(c.floor) {
			if n.ID == c.id {
				found = true
				if n.Kind != tenseiKind || n.Type != "转生" || n.Name != "精灵王" || n.X != c.x || n.Y != c.y {
					t.Errorf("%d 号应是 %d 的精灵王 (%d,%d)，实际 %+v", c.id, c.floor, c.x, c.y, n)
				}
			}
		}
		if !found {
			t.Errorf("地图 %d 应有 %d 号主城精灵王", c.floor, c.id)
		}
	}
}

func TestTenseiQualify(t *testing.T) {
	for _, c := range []struct {
		level, trans int
		text         string
		ok           bool
	}{
		{80, 0, "", true},
		{120, 4, "", true},
		{79, 0, tenseiLowMsg, false},
		{79, 4, tenseiLowMsg, false},
		{90, 5, tenseiMaxedMsg, false},
		{79, 5, tenseiMaxedMsg, false}, // 两个都不够时先说“最强的力量”（原版顺序）
	} {
		text, ok := tenseiQualify(&record{Level: c.level, Transmigration: c.trans})
		if ok != c.ok || text != c.text {
			t.Errorf("lv=%d trans=%d: 得到 (%q, %v)，应为 (%q, %v)", c.level, c.trans, text, ok, c.text, c.ok)
		}
	}
	if !strings.Contains(tenseiMaxedMsg, "最强的力量") || !strings.Contains(tenseiLowMsg, "还没有达成") {
		t.Error("两句资格台词不对")
	}
}

func TestTenseiScriptTable(t *testing.T) {
	for n := 0; n < TenseiMaxTrans; n++ {
		tx := tenseiScript[n]
		if tx.mainmsg == "" || tx.yesmsg1 == "" || tx.yesmsg2 == "" {
			t.Errorf("转生 %d 次的台词缺段: %+v", n, tx)
		}
		if !strings.Contains(tx.mainmsg, "你下定决心要进行转生了吗") {
			t.Errorf("转生 %d 次的确认台词不对: %q", n, tx.mainmsg)
		}
	}
	// 下标按转生次数取：转生 0 次用 TENSEI:1，转生 4 次用 TENSEI:5。
	if got := tenseiScriptOf(&record{Transmigration: 4}); got != tenseiScript[4] {
		t.Errorf("转生 4 次应取第 5 段台词")
	}
	if got := tenseiScriptOf(&record{Transmigration: 0}); got != tenseiScript[0] {
		t.Errorf("转生 0 次应取第 1 段台词")
	}
}

// 执行后的数值：四项按 CalcTensei，等级 1、经验 0、能力点 n×10、累计 +20 和 +min(等级,130)、HP 补满、
// 骑着的宠物下骑（状态回到休息），MP 和骑乘等级不动。
func TestTenseiApplyRecord(t *testing.T) {
	r := &record{
		Stats:           Stats{Vital: 100, Strength: 80, Toughness: 60, Dexterity: 40},
		Level:           90,
		Transmigration:  2,
		SkillPoints:     5,
		Exp:             999,
		HP:              1,
		MP:              33,
		RideLevel:       120,
		TransQuestTotal: 40,
		TransLevelTotal: 300,
		Pets: []petRec{
			{Slot: 0, Name: "乌力", Level: 30, State: petStateRest},
			{Slot: 1, Name: "雷龙", Level: 90, State: petStateRide},
		},
	}
	want, err := CalcTensei(TenseiInput{
		Vital: 100, Strength: 80, Toughness: 60, Dexterity: 40,
		Level: 90, Transmigration: 2, TransQuestTotal: 40, TransLevelTotal: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tenseiApplyRecord(r); err != nil {
		t.Fatal(err)
	}
	if r.Transmigration != 3 || r.Level != 1 || r.Exp != 0 || r.SkillPoints != 30 {
		t.Errorf("转生次数/等级/经验/能力点不对: trans=%d lv=%d exp=%d sp=%d", r.Transmigration, r.Level, r.Exp, r.SkillPoints)
	}
	if r.Stats != (Stats{Vital: want.Vital, Strength: want.Strength, Toughness: want.Toughness, Dexterity: want.Dexterity}) {
		t.Errorf("四项能力 = %+v，应为 CalcTensei 的结果 %+v", r.Stats, want)
	}
	if r.TransQuestTotal != 60 || r.TransLevelTotal != 300+90 {
		t.Errorf("累计任务数/等级 = %d/%d，应为 60/390", r.TransQuestTotal, r.TransLevelTotal)
	}
	if r.HP != r.maxHP() || r.MP != 33 || r.RideLevel != 120 {
		t.Errorf("HP 应补满到 %d、MP 应不动（33）、骑乘等级不动（120）: hp=%d mp=%d ride=%d", r.maxHP(), r.HP, r.MP, r.RideLevel)
	}
	if r.Pets[1].State != petStateRest || r.Pets[0].State != petStateRest {
		t.Errorf("骑着的宠物应下骑: %+v", r.Pets)
	}
}

func TestTenseiApplyRecordCapsLevelTotal(t *testing.T) {
	r := &record{Stats: Stats{Vital: 50, Strength: 50, Toughness: 50, Dexterity: 50}, Level: 150, TransLevelTotal: 10}
	if err := tenseiApplyRecord(r); err != nil {
		t.Fatal(err)
	}
	if r.TransLevelTotal != 10+130 {
		t.Errorf("累计等级应按 130 封顶计入，实际 %d", r.TransLevelTotal)
	}
}

func TestTenseiApplyRecordMaxedFails(t *testing.T) {
	r := &record{Level: 90, Transmigration: TenseiMaxTrans}
	if err := tenseiApplyRecord(r); err == nil {
		t.Error("转生次数已满时应报错，且不改数据")
	}
	if r.Transmigration != TenseiMaxTrans || r.Level != 90 {
		t.Errorf("报错后数据不应变: %+v", r)
	}
}

// tenseiSetup 建一个在精灵王旁边、等级和转生次数指定的角色（地图 1000，同 NPC 格）。
func tenseiSetup(t *testing.T, e *env, level, trans int) *client {
	t.Helper()
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 1000, 96, 99
		r.Level, r.Transmigration = level, trans
	})
	return c
}

func tenseiWindowText(t *testing.T, push map[string]any) (string, []any) {
	t.Helper()
	d, ok := push["data"].(map[string]any)
	if !ok {
		t.Fatalf("npc_window 缺 data: %v", push)
	}
	opts, _ := d["options"].([]any)
	return d["text"].(string), opts
}

func TestTenseiLowLevelIsOneTextWindow(t *testing.T) {
	e := newEnv(t)
	c := tenseiSetup(t, e, 79, 0)
	m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, tenseiNPC))
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("79 级 talk 应只推一个窗口: %v %v", m, pushes)
	}
	text, opts := tenseiWindowText(t, pushes[0])
	if !strings.Contains(text, "你好像还没有达成哦") || len(opts) != 0 {
		t.Errorf("条件窗口不对（应无选项）: %q %v", text, opts)
	}
}

func TestTenseiMaxedIsOneTextWindow(t *testing.T) {
	e := newEnv(t)
	c := tenseiSetup(t, e, 100, 5)
	_, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, tenseiNPC))
	if len(pushes) != 1 {
		t.Fatalf("5 转 talk 应只推一个窗口: %v", pushes)
	}
	text, opts := tenseiWindowText(t, pushes[0])
	if !strings.Contains(text, "最强的力量") || len(opts) != 0 {
		t.Errorf("上限窗口不对（应无选项）: %q %v", text, opts)
	}
}

// 取消路径：下一步 → 取消 → 确定关窗，角色数据一点都不变。
func TestTenseiCancelKeepsData(t *testing.T) {
	e := newEnv(t)
	c := tenseiSetup(t, e, 80, 0)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, tenseiNPC))
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	if len(pushes) != 2 || pushes[0]["type"] != "npc_window_closed" {
		t.Fatalf("下一步应先关旧窗口再推确认: %v", pushes)
	}
	_, opts := tenseiWindowText(t, pushes[1])
	if len(opts) != 2 {
		t.Fatalf("确认窗应有确定 / 取消: %v", opts)
	}
	_, pushes = c.callp("npc_choose", `{"no":2}`)
	if len(pushes) != 2 || pushes[0]["type"] != "npc_window_closed" {
		t.Fatalf("取消应先关确认窗再推取消台词: %v", pushes)
	}
	text, _ := tenseiWindowText(t, pushes[1])
	if !strings.Contains(text, "这样子吗") {
		t.Errorf("取消台词不对: %q", text)
	}
	_, pushes = c.callp("npc_choose", `{"no":1}`)
	if len(pushes) != 1 || pushes[0]["type"] != "npc_window_closed" {
		t.Errorf("取消台词的确定应只关窗: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Level != 80 || r.Transmigration != 0 || r.TransQuestTotal != 0 || r.TransLevelTotal != 0 {
			t.Errorf("取消后数据不应变: lv=%d trans=%d quest=%d level=%d", r.Level, r.Transmigration, r.TransQuestTotal, r.TransLevelTotal)
		}
	})
}

// 确定路径：执行后等级 1、转生 +1、能力点 10，推 status_changed 和 yesmsg1；下一步看 yesmsg2，确定关窗。
func TestTenseiDoneWritesResult(t *testing.T) {
	e := newEnv(t)
	c := tenseiSetup(t, e, 80, 0)
	var before record
	withRec(t, e, "alice", func(r *record) { before = *r })
	want, err := CalcTensei(TenseiInput{
		Vital: before.Stats.Vital, Strength: before.Stats.Strength, Toughness: before.Stats.Toughness, Dexterity: before.Stats.Dexterity,
		Level: before.Level, Transmigration: before.Transmigration,
		TransQuestTotal: before.TransQuestTotal, TransLevelTotal: before.TransLevelTotal,
	})
	if err != nil {
		t.Fatal(err)
	}

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, tenseiNPC))
	c.callp("npc_choose", `{"no":1}`)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	types := map[string]bool{}
	for _, p := range pushes {
		types[fmt.Sprint(p["type"])] = true
	}
	if !types["status_changed"] || !types["npc_window_closed"] || len(pushes) < 3 {
		t.Fatalf("确定后应推 status_changed、关窗、yesmsg1: %v", pushes)
	}
	text, opts := tenseiWindowText(t, pushes[len(pushes)-1])
	if !strings.Contains(text, "一片光芒包围") || len(opts) != 1 {
		t.Errorf("结果第一段不对: %q %v", text, opts)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Transmigration != 1 || r.Level != 1 || r.Exp != 0 || r.SkillPoints != 10 {
			t.Errorf("转生后 trans=%d lv=%d exp=%d sp=%d", r.Transmigration, r.Level, r.Exp, r.SkillPoints)
		}
		if r.Stats != (Stats{Vital: want.Vital, Strength: want.Strength, Toughness: want.Toughness, Dexterity: want.Dexterity}) {
			t.Errorf("四项 = %+v，应为 %+v", r.Stats, want)
		}
		if r.TransQuestTotal != 20 || r.TransLevelTotal != 80 {
			t.Errorf("累计 = %d/%d，应为 20/80", r.TransQuestTotal, r.TransLevelTotal)
		}
		if r.HP != r.maxHP() {
			t.Errorf("HP 应补满到 %d，实际 %d", r.maxHP(), r.HP)
		}
	})
	_, pushes = c.callp("npc_choose", `{"no":1}`)
	if len(pushes) != 2 || pushes[0]["type"] != "npc_window_closed" {
		t.Fatalf("下一步应先关窗再推 yesmsg2: %v", pushes)
	}
	if text, _ := tenseiWindowText(t, pushes[1]); !strings.Contains(text, "意识越来越模糊") {
		t.Errorf("结果第二段不对: %q", text)
	}
	c.callp("npc_choose", `{"no":1}`)
	c.wantErr(t, "npc_choose", `{"no":1}`, "WRONG_STATE")
}
