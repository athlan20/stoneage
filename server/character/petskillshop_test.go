package character

// 宠物技能店（STAGE-143，docs/protocol.md 5.51）：talk 弹技能列表，选技能→选宠物→选技能栏位→确认，
// 成交后金币扣掉学习价格、栏位里的技能被覆盖、宠物页推送并回到列表；金币不够、栏位超出宠物的栏位数、
// 没有宠物、离店太远、战斗中都被拒，金币和技能不变。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// 1763 号是 1003 地图（超级饲育员），坐标 18,13，教 0/1/2/3/10/50/100/200/201 号技能
// （0 是待机，原版照常上架，所以列表第一项是它）。
const petskillShopNPC = 1763

// 凯比（1062）有 6 个技能栏位，攻击和防御在前两栏。
const petskillShopPetTemplate = 1062

// setupPetskillShop 进入世界、站到技能店旁边，身上有一只凯比（1 号栏位是攻击、2 号是防御，其余空）。
func setupPetskillShop(t *testing.T, e *env, c *client) {
	t.Helper()
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 1003, 18, 13
		r.Gold = 10000
		r.Pets = append(r.Pets, petRec{Slot: 0, TemplateID: petskillShopPetTemplate, Name: "凯比", Level: 5, HP: 10,
			Stats:  gamedata.Ability{Vital: 1000, Strength: 1000, Toughness: 1000, Dexterity: 1000},
			Skills: [gamedata.PetSkillSlots]int{1, 2, -1, -1, -1, -1, -1}})
	})
}

// petskillState 读内存里的金币和凯比的技能栏，检查数据是否不变或已按预期改变。
func petskillState(t *testing.T, e *env) (int64, [gamedata.PetSkillSlots]int) {
	t.Helper()
	var gold int64
	var skills [gamedata.PetSkillSlots]int
	withRec(t, e, "alice", func(r *record) {
		gold = r.Gold
		if len(r.Pets) != 1 {
			t.Fatalf("应只有一只宠物: %v", r.Pets)
		}
		skills = r.Pets[0].Skills
	})
	return gold, skills
}

func TestPetskillShopLearn(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	setupPetskillShop(t, e, c)

	// talk → 列表：9 个技能都上架，第一个是待机（500 金币，倍率 1.0），第二个是攻击（1000 金币）。
	m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC))
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 应成功并推送列表: %v %v", m, pushes)
	}
	opts := pushes[0]["data"].(map[string]any)["options"].([]any)
	if len(opts) != 9 {
		t.Fatalf("应上架 9 个技能: %v", opts)
	}
	if label := opts[0].(map[string]any)["label"].(string); label != "待机（500 金币）：什么也不做" {
		t.Errorf("第一项应是待机及价格和说明: %q", label)
	}
	if label := opts[1].(map[string]any)["label"].(string); label != "攻击（1000 金币）：通常攻击" {
		t.Errorf("第二项应是攻击及价格和说明: %q", label)
	}

	// 攻击（列表第 2 项）→ 选凯比（1 号）→ 选 3 号技能栏位（空）→ 确认页 → 学习。
	if _, pushes := c.callp("npc_choose", `{"no":2}`); len(pushes) != 1 {
		t.Fatalf("选技能后应推宠物列表: %v", pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "教") {
		t.Errorf("宠物列表应有引导语: %v", pushes[0])
	}
	if _, pushes := c.callp("npc_choose", `{"no":1}`); len(pushes) != 1 {
		t.Fatalf("选宠物后应推技能栏位: %v", pushes)
	} else if slots := pushes[0]["data"].(map[string]any)["options"].([]any); len(slots) != 6 {
		t.Errorf("凯比有 6 个技能栏位: %v", slots)
	}
	if _, pushes := c.callp("npc_choose", `{"no":3}`); len(pushes) != 1 {
		t.Fatalf("选栏位后应推确认页: %v", pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "学习[攻击]") || !strings.Contains(text, "学习价格  1000 金币") {
		t.Errorf("确认页应写清技能、栏位和价格: %v", pushes[0])
	}
	m, pushes = c.callp("npc_choose", `{"no":1}`)
	if m["ok"] != true {
		t.Fatalf("确认学习应成功: %v", m)
	}
	// 推送：宠物页、状态（金币），然后回到技能列表（同原版，可以接着学）。
	if len(pushes) != 3 || pushes[0]["type"] != "pets_changed" || pushes[1]["type"] != "status_changed" {
		t.Fatalf("学习应推 pets_changed、status_changed 再回到列表: %v", pushes)
	} else if last := pushes[2]["data"].(map[string]any); len(last["options"].([]any)) != 9 {
		t.Errorf("成交后应回到技能列表: %v", pushes[2])
	}

	gold, skills := petskillState(t, e)
	if gold != 9000 {
		t.Errorf("金币应扣掉 1000，实际 %d", gold)
	}
	if skills[2] != 1 || skills[0] != 1 || skills[1] != 2 {
		t.Errorf("3 号栏位应是攻击，其余不变: %v", skills)
	}
}

func TestPetskillShopOverwriteSlot(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	setupPetskillShop(t, e, c)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC))

	// 连续攻击（10，2000 金币）覆盖 2 号栏位的防御。列表里第 5 项是 10（第 1 项是待机）。
	c.callp("npc_choose", `{"no":5}`)
	c.callp("npc_choose", `{"no":1}`)
	c.callp("npc_choose", `{"no":2}`)
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true || len(pushes) != 3 {
		t.Fatalf("覆盖栏位应成功: %v %v", m, pushes)
	}
	gold, skills := petskillState(t, e)
	if gold != 8000 {
		t.Errorf("金币应扣掉连续攻击的 2000，实际 %d", gold)
	}
	if skills[1] != 10 {
		t.Errorf("2 号栏位应换成连续攻击（10），实际 %d", skills[1])
	}
}

func TestPetskillShopRejects(t *testing.T) {
	// 每个分支都用新环境，互不影响；数据在失败前后都要不变。
	cases := []struct {
		name  string
		setup func(r *record)
		steps []string // 按顺序发的 npc_choose（talk 之后）
		last  string   // 最后一步的错误码
	}{
		{"金币不够", func(r *record) { r.Gold = 999 }, []string{"2", "1", "3", "1"}, protocol.CodeNotEnoughGold},
		{"栏位超过宠物的栏位数", nil, []string{"2", "1", "7"}, protocol.CodeNPCBadChoice},
		{"没有这只宠物", nil, []string{"2", "4"}, protocol.CodeNoPet},
		{"没有这个选项", nil, []string{"10"}, protocol.CodeNPCBadChoice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			c := e.loggedIn("alice")
			setupPetskillShop(t, e, c)
			if tc.setup != nil {
				withRec(t, e, "alice", tc.setup)
			}
			before, skillsBefore := petskillState(t, e)
			c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC))
			var m map[string]any
			for _, no := range tc.steps {
				m, _ = c.callp("npc_choose", fmt.Sprintf(`{"no":%s}`, no))
			}
			errInfo, _ := m["error"].(map[string]any)
			if m["ok"] == true || errInfo == nil || errInfo["code"] != tc.last {
				t.Fatalf("应回复 %s: %v", tc.last, m)
			}
			after, skillsAfter := petskillState(t, e)
			if after != before || skillsAfter != skillsBefore {
				t.Errorf("失败时金币和技能都不能变：金币 %d→%d，技能 %v→%v", before, after, skillsBefore, skillsAfter)
			}
		})
	}
}

func TestPetskillShopTooFar(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	setupPetskillShop(t, e, c)
	withRec(t, e, "alice", func(r *record) { r.X = 25 }) // 离店 7 格，同图超过 2 格
	// STAGE-263：同图超出距离时 talk 先把角色走到店旁边（相邻的空格），再照常弹窗。
	if m, _ := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC)); m["ok"] != true {
		t.Fatalf("同图离店远时 talk 应先走过去再开窗口: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if d := max(abs(r.X-18), abs(r.Y-13)); d != 1 || r.Floor != 1003 {
			t.Errorf("应走到店旁边（切比雪夫距离 1），实际 (%d,%d) 距离 %d 地图 %d", r.X, r.Y, d, r.Floor)
		}
	})
	// 走近到 2 格以内就能说话。
	withRec(t, e, "alice", func(r *record) { r.X = 20 })
	if m, _ := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC)); m["ok"] != true {
		t.Errorf("2 格以内应能对话: %v", m)
	}

	// 窗口开着走远：npc_choose 也按离店太远拒绝，金币和技能不变；走回来能接着选。
	before, skillsBefore := petskillState(t, e)
	c.callp("npc_choose", `{"no":2}`) // 选攻击，进入选宠物
	withRec(t, e, "alice", func(r *record) { r.X = 30 })
	m, _ := c.callp("npc_choose", `{"no":1}`)
	errInfo, _ := m["error"].(map[string]any)
	if m["ok"] == true || errInfo == nil || errInfo["code"] != protocol.CodeActionNotAllowed {
		t.Fatalf("走远后选择也应被拒: %v", m)
	}
	after, skillsAfter := petskillState(t, e)
	if after != before || skillsAfter != skillsBefore {
		t.Errorf("走远被拒时金币和技能都不能变：金币 %d→%d，技能 %v→%v", before, after, skillsBefore, skillsAfter)
	}
	withRec(t, e, "alice", func(r *record) { r.X = 19 })
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true || len(pushes) != 1 {
		t.Errorf("走回来应能接着选: %v %v", m, pushes)
	}
}

func TestPetskillShopLearnRechecksGold(t *testing.T) {
	// 选择之后金币被花掉：确认时重新检查，不够就拒绝，窗口留在确认页。
	e := newEnv(t)
	c := e.loggedIn("alice")
	setupPetskillShop(t, e, c)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC))
	c.callp("npc_choose", `{"no":2}`)
	c.callp("npc_choose", `{"no":1}`)
	c.callp("npc_choose", `{"no":3}`)
	withRec(t, e, "alice", func(r *record) { r.Gold = 0 })
	m, _ := c.callp("npc_choose", `{"no":1}`)
	errInfo, _ := m["error"].(map[string]any)
	if errInfo == nil || errInfo["code"] != protocol.CodeNotEnoughGold {
		t.Fatalf("确认时金币不够应拒绝: %v", m)
	}
	if _, skills := petskillState(t, e); skills[2] != -1 {
		t.Errorf("拒绝后 3 号栏位不应变: %v", skills)
	}
}

// TestPetskillShopBlockedInBattle：战斗中和宠物技能店说话被拒（talk 回复 IN_BATTLE），
// 金币和技能都不变（验收口径 2 的“战斗中”分支）。
func TestPetskillShopBlockedInBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	setupPetskillShop(t, e, c)
	goldBefore, skillsBefore := petskillState(t, e)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC), "IN_BATTLE")
	c.drainToEnd()

	gold, skills := petskillState(t, e)
	if gold != goldBefore || skills != skillsBefore {
		t.Errorf("战斗中拒绝时金币和技能都不能变：金币 %d→%d，技能 %v→%v", goldBefore, gold, skillsBefore, skills)
	}
}
