package character

// 宠物店（STAGE-73，docs/protocol.md 5.41）：talk 宠物店弹菜单，卖（估价→确认→删宠加钱）、
// 寄放（收费→state=3 落库→随身列表消失）、领取（免费回随身）、买（占位商品清单→新宠入第一个
// 空栏位）；栏位满、寄放池满、金币不够都被拒且数据不变；战斗中与观察方被拒。

import (
	"fmt"
	"github.com/athlan20/stoneage/server/battle"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 萨姆吉尔村没有宠物店；地图 130 的「宠物店」是 74 号（pool=true，支持寄放）。
const petshopNPC = 74

func petshopData(t *testing.T) *gamedata.NpcPetshop {
	t.Helper()
	ps := gamedata.NpcPetshopOf(petshopNPC)
	if ps == nil {
		t.Fatal("74 号 NPC 应是宠物店")
	}
	return ps
}

func TestPetshopSell(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	// 挪到宠物店所在地图，给一只 1 级宠物（乌力乌力模板 2，稀有 0）。
	withRec(t, e, "alice", func(r *record) {
		r.Floor = 130
		r.Pets = append(r.Pets, petRec{Slot: 0, TemplateID: 2, Name: "乌力乌力", Level: 1, HP: 10,
			Stats: gamedata.Ability{Vital: 1000, Strength: 1000, Toughness: 1000, Dexterity: 1000}})
	})
	// talk → 菜单（寄放/领取/卖/买）→ 选卖 → 列表 → 选 0 号 → 确认页 → 确认。
	// 估价：1 级无稀有 = 10，魅力 60（新角色）、忠诚 66（乌力乌力 1 级 MODAI=100）→ 系数 63 → 6，同原版。
	if m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC)); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 应成功: %v %v", m, pushes)
	} else if opts := pushes[0]["data"].(map[string]any)["options"].([]any); len(opts) != 4 {
		t.Fatalf("菜单应有寄放/领取/卖/买四项: %v", pushes[0])
	}
	c.callp("npc_choose", `{"no":3}`) // 卖
	if m, pushes := c.callp("npc_choose", `{"no":0}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("选宠物应到确认页: %v %v", m, pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "是[乌力乌力]没错吧？") || !strings.Contains(text, "6 STONE") {
		t.Errorf("确认页应是原版卖宠格式、估价 6: %v", pushes[0])
	}
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("确认卖出应成功: %v", m)
	} else if len(pushes) != 3 {
		t.Fatalf("卖出应推 pets_changed、status_changed、closed: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if len(r.Pets) != 0 {
			t.Errorf("宠物应被卖掉，实际 %v", r.Pets)
		}
		if r.Gold != 1_000_000+6 {
			t.Errorf("金币应加卖价 6，实际 %d", r.Gold)
		}
	})
}

func TestPetshopPoolAndGet(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor = 130
		r.Pets = append(r.Pets, petRec{Slot: 0, TemplateID: 2, Name: "乌力乌力", Level: 1, HP: 10,
			Stats: gamedata.Ability{Vital: 1000, Strength: 1000, Toughness: 1000, Dexterity: 1000}})
	})

	// 寄放：菜单 1 → 选 0 号 → 确认（费 50+1×4=54）。
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":1}`)
	if m, pushes := c.callp("npc_choose", `{"no":0}`); m["ok"] != true {
		t.Fatalf("选寄放宠物应成功: %v", m)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "寄放[乌力乌力]") || !strings.Contains(text, "寄放价格            54 stone") {
		t.Errorf("确认页应是原版寄放格式、寄放费 50+等级×4=54: %v", pushes[0])
	}
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("确认寄放应成功: %v", m)
	} else if len(pushes) != 3 {
		t.Fatalf("寄放应推 pets_changed、status_changed、closed: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Pets[0].State != petStatePooled || r.Pets[0].Slot < gamedata.PooledSlotBase {
			t.Errorf("宠物应进入寄放池（state=3、slot≥50），实际 %+v", r.Pets[0])
		}
		if r.Gold != 1_000_000-54 {
			t.Errorf("金币应扣寄放费 54，实际 %d", r.Gold)
		}
		if n := len(r.petsView()["pets"].([]petView)); n != 3 {
			t.Errorf("寄放的乌力不应出现在随身列表（出生 3 只还在），实际 %d 只", n)
		}
	})

	// 领取：菜单 2 → 选池位 0 → 随身栏空，免费取回，属性不变。
	goldBefore := int64(0)
	withRec(t, e, "alice", func(r *record) { goldBefore = r.Gold })
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":2}`)
	// 领取列表选池位 0 后直接取回（没有确认页）。
	if m, _ := c.callp("npc_choose", `{"no":0}`); m["ok"] != true {
		t.Fatalf("领取应成功: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Pets[0].State == petStatePooled {
			t.Errorf("宠物应已取回: %+v", r.Pets[0])
		}
		if r.Pets[0].Level != 1 || r.Pets[0].TemplateID != 2 {
			t.Errorf("取回后属性应不变: %+v", r.Pets[0])
		}
		if r.Gold != goldBefore {
			t.Errorf("领取免费，金币不该动: %d → %d", goldBefore, r.Gold)
		}
	})
}

func TestPetshopBuy(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) { r.Floor = 130 })

	// 买：菜单 4 → 商品列表（三只）→ 选 1 → 确认 → 第一个空栏位（0）出现新宠。
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	if m, pushes := c.callp("npc_choose", `{"no":4}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("选买应成功: %v %v", m, pushes)
	} else if opts := pushes[0]["data"].(map[string]any)["options"].([]any); len(opts) != len(gamedata.ShopPetIDs) {
		t.Fatalf("商品列表应是全部在售: %v", pushes[0])
	}
	price := petBuyCost(petshopData(t), gamedata.ShopPetIDs[0])
	c.callp("npc_choose", `{"no":1}`)
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("确认买入应成功: %v", m)
	} else if len(pushes) != 3 {
		t.Fatalf("买入应推 pets_changed、status_changed、closed: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if len(r.Pets) != 1 || r.Pets[0].Slot != 0 || r.Pets[0].TemplateID != gamedata.ShopPetIDs[0] {
			t.Errorf("新宠应出现在 0 号栏位: %v", r.Pets)
		}
		if r.Gold != 1_000_000-int64(price) {
			t.Errorf("金币应扣买价 %d，实际 %d", price, r.Gold)
		}
	})
}

func TestPetshopRejections(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	// 没有宠物：卖列表为空，选任何槽位都 NPC_BAD_CHOICE。
	withRec(t, e, "alice", func(r *record) { r.Floor = 130 })
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":3}`)
	c.wantErr(t, "npc_choose", `{"no":0}`, "NPC_BAD_CHOICE")
	c.callp("npc_close", `{}`) // npc_close 自带关闭推送，用 callp 消费

	// 买时钱不够：cost_msg。
	withRec(t, e, "alice", func(r *record) { r.Gold = 0 })
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":4}`)
	c.callp("npc_choose", `{"no":1}`)
	if m := c.call("npc_choose", `{"no":1}`); m["ok"] != false {
		t.Fatalf("钱不够应失败: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "NOT_ENOUGH_GOLD" {
		t.Errorf("应回 NOT_ENOUGH_GOLD: %v", m)
	}
}

// petshopPet 是测试用的一只乌力乌力（模板 2），放在给定的槽位和状态。
func petshopPet(slot, state int) petRec {
	return petRec{Slot: slot, TemplateID: 2, Name: "乌力乌力", Level: 1, HP: 10, State: state,
		Stats: gamedata.Ability{Vital: 1000, Strength: 1000, Toughness: 1000, Dexterity: 1000}}
}

// TestPetshopBuyRejectsWhenPetsFull：随身 5 个栏位都满时买宠按原话拒绝（getfull_msg），数据不变。
func TestPetshopBuyRejectsWhenPetsFull(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor = 130
		for slot := 0; slot < MaxPetSlots; slot++ {
			r.Pets = append(r.Pets, petshopPet(slot, petStateRest))
		}
	})

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":4}`)
	c.callp("npc_choose", `{"no":1}`)
	m := c.call("npc_choose", `{"no":1}`)
	if m["ok"] != false {
		t.Fatalf("栏位满时买宠应失败: %v", m)
	}
	if em := m["error"].(map[string]any); em["code"] != "ACTION_NOT_ALLOWED" || em["message"] != petshopData(t).GetFull {
		t.Errorf("应回 ACTION_NOT_ALLOWED 和 getfull_msg 原话: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if len(r.Pets) != MaxPetSlots {
			t.Errorf("被拒的买入不该动数据，实际 %d 只", len(r.Pets))
		}
	})
}

// TestPetshopSellRejectsWhenGoldFull：卖出后金币超上限（over_msg + GOLD_FULL），宠物还在。
func TestPetshopSellRejectsWhenGoldFull(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor = 130
		r.Gold = battle.MaxHaveGold
		r.Pets = append(r.Pets, petshopPet(0, petStateRest))
	})

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":3}`)
	c.callp("npc_choose", `{"no":0}`)
	m := c.call("npc_choose", `{"no":1}`)
	if m["ok"] != false {
		t.Fatalf("金币满时卖宠应失败: %v", m)
	}
	if code := m["error"].(map[string]any)["code"]; code != "GOLD_FULL" {
		t.Errorf("应回 GOLD_FULL: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if len(r.Pets) != 1 || r.Gold != battle.MaxHaveGold {
			t.Errorf("被拒的卖出不该动数据: pets=%d gold=%d", len(r.Pets), r.Gold)
		}
	})
}

// TestPetshopPoolRejectsWhenPoolFull：寄放池 15 格都满时寄放按原话拒绝（poolfull_msg），宠物还在随身。
func TestPetshopPoolRejectsWhenPoolFull(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor = 130
		r.Pets = append(r.Pets, petshopPet(0, petStateRest))
		for i := 0; i < gamedata.MaxPoolPets; i++ {
			r.Pets = append(r.Pets, petshopPet(gamedata.PooledSlotBase+i, petStatePooled))
		}
	})

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":1}`)
	c.callp("npc_choose", `{"no":0}`)
	m := c.call("npc_choose", `{"no":1}`)
	if m["ok"] != false {
		t.Fatalf("寄放池满时寄放应失败: %v", m)
	}
	if em := m["error"].(map[string]any); em["code"] != "ACTION_NOT_ALLOWED" || em["message"] != petshopData(t).PoolFull {
		t.Errorf("应回 ACTION_NOT_ALLOWED 和 poolfull_msg 原话: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Pets[0].Slot != 0 || r.Pets[0].State != petStateRest {
			t.Errorf("被拒的寄放不该动数据: %+v", r.Pets[0])
		}
	})
}

// TestPetshopBlockedInBattle：战斗中 talk 宠物店被拒（IN_BATTLE）。
func TestPetshopBlockedInBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC), "IN_BATTLE")
	c.drainToEnd()
}

// TestPetshopBlockedWhileObserving：agent 操控时玩家连接 talk/choose 宠物店被拒，agent 自己可以卖。
func TestPetshopBlockedWhileObserving(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) { r.Floor = 130 }) // 宠物店在 130 号地图

	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed")
	c.recv()
	c.drainToEnd()
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC), "CONTROLLED_BY_AGENT")
	if m, pushes := agent.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC)); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("agent 应能打开宠物店: %v %v", m, pushes)
	}
	c.recv()                              // 玩家连接收到 agent 打开的菜单窗口
	agent.callp("npc_choose", `{"no":3}`) // agent 自己进卖宠列表
	c.recv()
	c.wantErr(t, "npc_choose", `{"no":0}`, "CONTROLLED_BY_AGENT")
	agent.callp("npc_close", `{}`) // npc_close 自带关闭推送，用 callp 消费
}
