package character

// 商店买卖（STAGE-70，docs/protocol.md 5.38）：talk 商店弹菜单，买（选商品 → reply 数量 →
// 扣金币入背包）、卖（回收列表选槽位 → 加金币删物品），错误路径（等级、金币、数量、
// 金币上限）都不改数据；战斗中和观察方被拒。

import (
	"fmt"
	"github.com/athlan20/stoneage/server/battle"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 萨姆吉尔村（1000）的「特产品贩卖员」是 198（npcgen_shop，buy_rate=1.0、sell_rate=0.5）；
// 村长家（1006）没有商店。编号由 gen-npcs.py 的批次排序保证（STAGE-68 起稳定）。
const shopNPC = 198

// withRec 在持有角色锁的情况下读写内存里的 record。
func withRec(t *testing.T, e *env, account string, f func(r *record)) {
	t.Helper()
	if err := e.chars.LiveOf(e.session(account)).With(f); err != nil {
		t.Fatal(err)
	}
}

// shopFirstItem 是测试店的第一个商品：返回物品编号和单价（buy_rate=1.0 时就是基价）。
func shopFirstItem(t *testing.T) (int, int) {
	t.Helper()
	shop := gamedata.NpcShopOf(shopNPC)
	if shop == nil || len(shop.Items) == 0 {
		t.Fatal("198 号 NPC 应是有商品的商店（萨姆吉尔的特产品贩卖员）")
	}
	itemID, price, ok := shop.BuyPrice(0)
	if !ok || price <= 0 {
		t.Fatalf("第一个商品应能取价: %d %d %v", itemID, price, ok)
	}
	return itemID, price
}

// openBuyList 走完 talk → 菜单 → 「买」→ 商品列表，返回商品列表窗口的 data。
func openBuyList(t *testing.T, c *client) map[string]any {
	t.Helper()
	m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, shopNPC))
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 商店应成功并推一个窗口: %v %v", m, pushes)
	}
	if opts := pushes[0]["data"].(map[string]any)["options"].([]any); len(opts) != 2 {
		t.Fatalf("商店菜单应有买/卖两个选项: %v", pushes[0])
	}
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true || len(pushes) != 1 || pushes[0]["type"] != "npc_window" {
		t.Fatalf("选买应成功并推商品列表: %v %v", m, pushes)
	} else {
		return pushes[0]["data"].(map[string]any)
	}
	return nil
}

func TestShopMenuAndBuyList(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`) // 萨姆吉尔村

	data := openBuyList(t, c)
	first := data["options"].([]any)[0].(map[string]any)
	if first["no"] != float64(1) {
		t.Errorf("商品列表从 1 号开始: %v", first)
	}

	// 选 1 号商品：进入数量询问，正文带单价，没有选项（等 reply）。
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("选商品应成功: %v %v", m, pushes)
	} else if w := pushes[0]["data"].(map[string]any); len(w["options"].([]any)) != 0 {
		t.Errorf("数量询问窗口不应有选项: %v", w)
	}
}

func TestShopBuyHappyPath(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	itemID, price := shopFirstItem(t)
	openBuyList(t, c)
	c.callp("npc_choose", `{"no":1}`)
	if m, pushes := c.callp("npc_reply", `{"text":"2"}`); m["ok"] != true {
		t.Fatalf("reply 数量应成功: %v", m)
	} else if len(pushes) != 3 {
		t.Fatalf("成交应推 inventory_changed、status_changed、npc_window_closed: %v", pushes)
	}
	var gold int64
	var n int
	withRec(t, e, "alice", func(r *record) {
		gold = r.Gold
		for _, it := range r.Items {
			if it.ItemID == itemID {
				n += it.Count
			}
		}
	})
	if gold != int64(1_000_000)-int64(price*2) {
		t.Errorf("金币应减 %d，实际剩 %d", price*2, gold)
	}
	if n != 2 {
		t.Errorf("背包应有 2 个商品，实际 %d", n)
	}
	// 成交后窗口关了：再 reply 回 WRONG_STATE。
	c.wantErr(t, "npc_reply", `{"text":"1"}`, "WRONG_STATE")
}

func TestShopBuyValidation(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	_, price := shopFirstItem(t)
	openBuyList(t, c)
	c.callp("npc_choose", `{"no":1}`)

	// 数量非法：回提示文案，数据不动。
	if m := c.call("npc_reply", `{"text":"abc"}`); m["ok"] != false {
		t.Fatalf("非数字应失败: %v", m)
	} else if msg, _ := m["error"].(map[string]any)["message"].(string); msg == "" {
		t.Errorf("应有提示文案: %v", m)
	}
	// 金币不够：把角色的钱减到买不起一件。
	withRec(t, e, "alice", func(r *record) { r.Gold = int64(price) - 1 })
	if m := c.call("npc_reply", `{"text":"1"}`); m["ok"] != false {
		t.Fatalf("金币不够应失败: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "NOT_ENOUGH_GOLD" {
		t.Errorf("应回 NOT_ENOUGH_GOLD: %v", m)
	}
	withRec(t, e, "alice", func(r *record) { r.Gold = 1_000_000 })

	// 等级不够的物品（Level > 1 的商品）：REQUIREMENT_NOT_MET + 店里的 level_msg。
	shop := gamedata.NpcShopOf(shopNPC)
	high := -1
	for i := range shop.Items {
		itemID, _, ok := shop.BuyPrice(i)
		if !ok {
			continue
		}
		if def, ok := gamedata.ItemByID(itemID); ok && def.Level > 1 {
			high = i
			break
		}
	}
	if high >= 0 {
		c.callp("talk", fmt.Sprintf(`{"npc":%d}`, shopNPC))
		c.callp("npc_choose", `{"no":1}`)
		if m, _ := c.callp("npc_choose", fmt.Sprintf(`{"no":%d}`, high+1)); m["ok"] != true {
			t.Fatalf("选高级商品应能进入数量询问: %v", m)
		}
		if m := c.call("npc_reply", `{"text":"1"}`); m["ok"] != false {
			t.Fatalf("等级不够应失败: %v", m)
		} else if code := m["error"].(map[string]any)["code"]; code != "REQUIREMENT_NOT_MET" {
			t.Errorf("应回 REQUIREMENT_NOT_MET: %v", m)
		}
	} else {
		t.Log("这家店没有等级 >1 的商品，跳过等级检查分支")
	}

	// 上面所有失败都不该动金币和背包。
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != 1_000_000 {
			t.Errorf("失败的购买不该动金币，实际 %d", r.Gold)
		}
		if len(r.Items) != 0 {
			t.Errorf("失败的购买不该给物品，实际 %v", r.Items)
		}
	})
}

func TestShopSell(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	// 直接放进背包一件可卖的物品（小斧头，基价 65；卖出写库时这行不存在，DELETE 无害）。
	const sellItemID = 0
	shop := gamedata.NpcShopOf(shopNPC)
	sellPrice, ok := shop.SellPrice(sellItemID)
	if !ok || sellPrice <= 0 {
		t.Fatalf("小斧头应可卖: %d %v", sellPrice, ok)
	}
	withRec(t, e, "alice", func(r *record) { r.Items = append(r.Items, itemRec{Slot: 5, ItemID: sellItemID, Count: 1}) })

	if m, _ := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, shopNPC)); m["ok"] != true {
		t.Fatalf("talk 应成功: %v", m)
	} else if m, pushes := c.callp("npc_choose", `{"no":2}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("选卖应成功并推回收列表: %v %v", m, pushes)
	} else {
		opts := pushes[0]["data"].(map[string]any)["options"].([]any)
		if len(opts) == 0 {
			t.Fatalf("背包有物品时应列出来: %v", pushes[0])
		}
		slot := int(opts[0].(map[string]any)["no"].(float64))
		if m, pushes := c.callp("npc_choose", fmt.Sprintf(`{"no":%d}`, slot)); m["ok"] != true {
			t.Fatalf("卖出应成功: %v", m)
		} else if len(pushes) != 3 {
			t.Fatalf("卖出应推 inventory_changed、status_changed、closed: %v", pushes)
		}
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != int64(1_000_000)+int64(sellPrice) {
			t.Errorf("金币应加 %d，实际 %d", sellPrice, r.Gold)
		}
		for _, it := range r.Items {
			if it.ItemID == sellItemID {
				t.Errorf("卖掉的物品还在背包: %v", r.Items)
			}
		}
	})
}

// TestShopSellRejectsWhenGoldFull 是 stone_msg 的场景：金币到上限时卖出被拒，物品不消失。
func TestShopSellRejectsWhenGoldFull(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)
	withRec(t, e, "alice", func(r *record) {
		r.Gold = battle.MaxHaveGold
		r.Items = append(r.Items, itemRec{Slot: 5, ItemID: 0, Count: 1})
	})

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, shopNPC))
	c.callp("npc_choose", `{"no":2}`)
	if m := c.call("npc_choose", `{"no":5}`); m["ok"] != false {
		t.Fatalf("金币满时应拒绝: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "GOLD_FULL" {
		t.Errorf("应回 GOLD_FULL: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != battle.MaxHaveGold || len(r.Items) != 1 {
			t.Errorf("被拒的卖出不该动数据: gold=%d items=%v", r.Gold, r.Items)
		}
	})
}

// TestShopBlockedInBattle 是验收 4 的一半：战斗中 talk 被拒（IN_BATTLE）。
func TestShopBlockedInBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, shopNPC), "IN_BATTLE")
	c.drainToEnd()
}

// TestShopBlockedWhileObserving 是验收 4 的另一半：观察方 talk/choose 被拒，agent 自己可以买卖。
func TestShopBlockedWhileObserving(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed")
	c.recv()
	c.drainToEnd()
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, shopNPC), "CONTROLLED_BY_AGENT")
	if m, pushes := agent.callp("talk", fmt.Sprintf(`{"npc":%d}`, shopNPC)); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("agent 应能打开商店: %v %v", m, pushes)
	}
	c.recv()                              // 玩家连接收到 agent 打开的菜单窗口
	agent.callp("npc_choose", `{"no":1}`) // agent 自己推进到商品列表
	c.recv()                              // 玩家连接收到商品列表
	c.wantErr(t, "npc_choose", `{"no":1}`, "CONTROLLED_BY_AGENT")
	agent.callp("npc_close", `{}`) // npc_close 自带关闭推送，用 callp 消费
}
