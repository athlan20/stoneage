package character

// 护士治疗（STAGE-71，docs/protocol.md 5.39）：talk 护士弹确认窗口（受伤报价、免费线
// 以下免费、全满提示），选治疗回满人物 HP/MP 和宠物 HP 并扣费；钱不够、满血、战斗中、
// 观察方都被拒且数据不变。

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 地图 117 的「监狱医生」是 54 号：参数 level=1（没有免费线）、hp_rate=40、mp_rate=60，
// 费用好算（1 级角色受伤治一次 = 40 + 60 = 100）。
const healerNPC = 54

func healerData(t *testing.T) *gamedata.NpcHealer {
	t.Helper()
	h := gamedata.NpcHealerOf(healerNPC)
	if h == nil {
		t.Fatal("54 号 NPC 应是护士（监狱医生）")
	}
	return h
}

func TestHealerHealsAndCharges(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	healerData(t)
	// 挪到护士所在地图并弄伤角色（HP 10/44）。
	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 117, 10, 10
		r.HP = 10
	})
	if m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, healerNPC)); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 护士应成功: %v %v", m, pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "收您 40 的 STONE") {
		// HP 缺口 40 费 + MP 满不加 = 40（1 级 × hp_rate 40）。
		t.Errorf("确认窗口应报价 40: %v", pushes[0])
	}

	// 选治疗：HP 回满、扣 40 金币、窗口关。
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("治疗应成功: %v", m)
	} else if len(pushes) != 3 {
		t.Fatalf("治疗应推 status_changed、pets_changed、closed: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.HP != r.maxHP() {
			t.Errorf("HP 应回满，实际 %d/%d", r.HP, r.maxHP())
		}
		if r.Gold != 1_000_000-40 {
			t.Errorf("金币应扣 40，实际 %d", r.Gold)
		}
	})
	c.wantErr(t, "npc_choose", `{"no":1}`, "WRONG_STATE") // 窗口已关
}

func TestHealerHealsPetsToo(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 117, 10, 10
		r.HP = r.maxHP() // 人物满血：费用只来自宠物？宠物免费（原版「宠物回复(免费)」）
		// 给一只受伤的宠物（出生宠已有则用之，否则造一只简化记录）。
		if len(r.Pets) == 0 {
			r.Pets = append(r.Pets, petRec{Slot: 0, TemplateID: 1, Name: "乌力", HP: 1,
				Stats: gamedata.Ability{Vital: 1000, Strength: 1000, Toughness: 1000, Dexterity: 1000}})
		}
		r.Pets[0].HP = 1
	})
	if m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, healerNPC)); m["ok"] != true {
		t.Fatalf("talk 应成功: %v", m)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "宠物") {
		t.Errorf("只有宠物受伤时应提示含宠物（费用为 0 不报价）: %v", pushes[0])
	}
	// 治疗：宠物回满、金币不动（人物本来就满，费用为 0）。
	goldBefore := int64(0)
	withRec(t, e, "alice", func(r *record) { goldBefore = r.Gold })
	if m, _ := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("治疗应成功: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Pets[0].HP != gamedata.PetMaxHP(r.Pets[0].Stats) {
			t.Errorf("宠物 HP 应回满，实际 %d", r.Pets[0].HP)
		}
		if r.Gold != goldBefore {
			t.Errorf("宠物回复免费，金币不该动: %d → %d", goldBefore, r.Gold)
		}
	})
}

func TestHealerRejectsWhenFullOrBroke(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 全满：确认窗口没有「治疗」选项（原版 OK 窗口「似乎没有必要回复唷！」）。
	withRec(t, e, "alice", func(r *record) { r.Floor, r.X, r.Y = 117, 10, 10 })
	m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, healerNPC))
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 应成功: %v %v", m, pushes)
	}
	if opts := pushes[0]["data"].(map[string]any)["options"].([]any); len(opts) != 0 {
		t.Errorf("全满时不应有治疗选项: %v", pushes[0])
	}
	c.wantErr(t, "npc_choose", `{"no":1}`, "ACTION_NOT_ALLOWED") // 窗口在，但没有可治的

	// 弄伤后把钱扣到不够：报价在窗口正文，选治疗回 NOT_ENOUGH_GOLD 且数据不变。
	withRec(t, e, "alice", func(r *record) { r.HP = 10; r.Gold = 10 })
	if m, _ := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, healerNPC)); m["ok"] != true {
		t.Fatalf("talk 应成功: %v", m)
	}
	if m := c.call("npc_choose", `{"no":1}`); m["ok"] != false {
		t.Fatalf("钱不够应失败: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "NOT_ENOUGH_GOLD" {
		t.Errorf("应回 NOT_ENOUGH_GOLD: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.HP != 10 || r.Gold != 10 {
			t.Errorf("被拒的治疗不该动数据: hp=%d gold=%d", r.HP, r.Gold)
		}
	})
}

// 免费线：地图 1005 的「萨姆吉尔的护士」（224 号）参数 level=10，
// 1 级角色治疗免费（原版「现在的等级可以免费帮你回复唷！」）。
func TestHealerFreeForLowLevel(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	withRec(t, e, "alice", func(r *record) { r.Floor, r.X, r.Y = 1005, 10, 10; r.HP = 10 })
	if m, pushes := c.callp("talk", `{"npc":224}`); m["ok"] != true {
		t.Fatalf("talk 应成功: %v", m)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "免费") {
		t.Errorf("免费线以下应提示免费: %v", pushes[0])
	}
	if m, _ := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("治疗应成功: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.HP != r.maxHP() || r.Gold != 1_000_000 {
			t.Errorf("免费治疗应回满且不扣钱: hp=%d gold=%d", r.HP, r.Gold)
		}
	})
}

// TestHealerBlockedInBattleAndByObserver：战斗中 talk 被拒；观察方 choose 被拒，agent 自己可以治。
func TestHealerBlockedInBattleAndByObserver(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, healerNPC), "IN_BATTLE")
	c.drainToEnd()
}

// STAGE-168：宠物战斗中倒下（HP 0，已写库），护士治疗后内存和数据库都是满血，设出战应成功，
// 不能再回 PET_FAINTED（pet_state 的规则判定读的是数据库）。
func TestHealerHealedPetCanBattleAfterFainting(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 117, 10, 10
		r.HP = r.maxHP()
		r.Pets = []petRec{{Slot: 0, TemplateID: 1, Name: "乌力", Level: 1, HP: 0, State: petStateRest,
			Stats:  gamedata.Ability{Vital: 1000, Strength: 1000, Toughness: 1000, Dexterity: 1000},
			Skills: [gamedata.PetSkillSlots]int{-1, -1, -1, -1, -1, -1, -1}}}
		// 战斗倒下时的写库（finishBattle 的同款收尾）。
		e.chars.savePets(r.ID, r.Pets)
	})
	c.wantErr(t, "pet_state", `{"slot":0,"state":"battle"}`, "PET_FAINTED")

	if m, _ := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, healerNPC)); m["ok"] != true {
		t.Fatalf("talk 护士应成功: %v", m)
	}
	if m, _ := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("治疗应成功: %v", m)
	}
	if m, _ := c.callp("pet_state", `{"slot":0,"state":"battle"}`); m["ok"] != true {
		t.Fatalf("治疗后设出战应成功: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if got := dbPetHP(t, e, r.ID, 0); got != gamedata.PetMaxHP(r.Pets[0].Stats) {
			t.Errorf("治疗后数据库里的宠物 HP = %d，应为满血 %d", got, gamedata.PetMaxHP(r.Pets[0].Stats))
		}
	})
}

// dbPetHP 直接从数据库读宠物的 HP（不经过内存）。
func dbPetHP(t *testing.T, e *env, charID int64, slot int) int {
	t.Helper()
	pets, err := readPets(context.Background(), e.chars.db, charID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pets {
		if p.Slot == slot {
			return p.HP
		}
	}
	t.Fatalf("数据库里没有 %d 号宠物", slot)
	return 0
}
