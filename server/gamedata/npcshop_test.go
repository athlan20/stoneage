package gamedata

// 抽查 npc-shop.json（STAGE-70，docs/protocol.md 5.38）：倍率计价、商品展开、
// 高价回收和台词都来自原版商店参数文件。

import "testing"

func TestNpcShopPricing(t *testing.T) {
	// 萨姆吉尔村的特产品贩卖员（198）：buy_rate=1.0、sell_rate=0.5（shop 参数文件原值）。
	shop := NpcShopOf(198)
	if shop == nil {
		t.Fatal("198 号 NPC 应是商店（特产品贩卖员）")
	}
	if shop.BuyRate != 1.0 || shop.SellRate != 0.5 {
		t.Errorf("倍率应来自参数文件（1.0/0.5），实际 %v/%v", shop.BuyRate, shop.SellRate)
	}
	if !shop.ShopSellsItems() || !shop.ShopBuysItems() {
		t.Errorf("这家店应既能买又能卖: items=%d sell=%v", len(shop.Items), shop.ShopBuysItems())
	}
	// 第一个商品是萨姆吉尔产的桃子（基价 25），买价 = 25 × 1.0。
	itemID, price, ok := shop.BuyPrice(0)
	if !ok || price != 25 {
		t.Fatalf("第一个商品应取价 25: %d %d %v", itemID, price, ok)
	}
	if def, ok := ItemByID(itemID); !ok || def.Name != "萨姆吉尔产的桃子" {
		t.Errorf("第一个商品应是萨姆吉尔产的桃子: %+v", def)
	}
	// 卖价 = 基价 × sell_rate 向下取整：25 × 0.5 = 12。
	if sell, ok := shop.SellPrice(itemID); !ok || sell != 12 {
		t.Errorf("桃子的回收价应是 12: %d %v", sell, ok)
	}
	if _, ok := shop.SellPrice(999999); ok {
		t.Error("物品表里没有的编号不应可卖")
	}
}

func TestNpcShopSpecialRate(t *testing.T) {
	// 有 special_item 的店（高价回收）：special_rate 覆盖 sell_rate。
	var found *NpcShop
	for id := 1; id <= 1181; id++ {
		if s := NpcShopOf(id); s != nil && len(s.SpecialItems) > 0 {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatal("原版数据里应有带 special_item 的商店")
	}
	itemID := found.SpecialItems[0]
	rate := found.SpecialRate
	if rate <= 0 {
		rate = 1.2 // 缺省高价倍率（原版 npc_itemshop.c）
	}
	def, ok := ItemByID(itemID)
	if !ok || def.Cost <= 0 {
		t.Fatalf("special 商品应有正的基价: %d", itemID)
	}
	want := int(float64(def.Cost) * rate)
	if got, ok := found.SellPrice(itemID); !ok || got != want {
		t.Errorf("高价回收价应是 %d，实际 %d", want, got)
	}
}

func TestNpcShopTalkLinesFromParams(t *testing.T) {
	// 台词逐字来自参数文件（welcome 语和 buy_main 等非空）。
	shop := NpcShopOf(198)
	if shop.Main == "" || shop.BuyMain == "" || shop.What == "" {
		t.Errorf("商店的欢迎语、引导语、数量提示应来自参数文件: %+v", shop)
	}
}
