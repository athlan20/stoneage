package gamedata

// STAGE-302 便利店素材集中化：萨姆吉尔（1009）与卡鲁它那（4009）两家便利店的 6 个素材店。
// 数据由 tools/gen-npcs.py 按 tools/npc_overlay/convenience_shops.json 生成（原版数据不动）。
// 每家店 1-10 级齐全（4 组 × 10 级，共 40 件），两家店同位置的 NPC 卖同样的东西，价格按基价 × 1.0。

import (
	"sort"
	"strings"
	"testing"
)

// 两家店相同的 6 个素材店位置（x 都是 27）。
var convenienceShopYs = []int{17, 22, 24, 28, 37, 39}

// convenienceShops 返回这张地图上的素材店，按 y 坐标索引。
func convenienceShops(t *testing.T, floor int) map[int]NPC {
	t.Helper()
	shops := map[int]NPC{}
	for _, n := range NPCsByFloor(floor) {
		if n.Type != "商店" || !strings.Contains(n.Name, "素材") {
			continue
		}
		if _, dup := shops[n.Y]; dup {
			t.Fatalf("地图 %d 的 y=%d 出现了两家素材店", floor, n.Y)
		}
		shops[n.Y] = n
	}
	return shops
}

func TestConvenienceShopsLayout(t *testing.T) {
	for _, floor := range []int{1009, 4009} {
		shops := convenienceShops(t, floor)
		if len(shops) != 6 {
			t.Fatalf("地图 %d 应有 6 个素材店，实际 %d 个", floor, len(shops))
		}
		for _, y := range convenienceShopYs {
			n, ok := shops[y]
			if !ok {
				t.Errorf("地图 %d 缺少 (27,%d) 的素材店", floor, y)
				continue
			}
			if n.X != 27 {
				t.Errorf("地图 %d 的素材店 y=%d 应在 x=27，实际 x=%d", floor, y, n.X)
			}
			shop := NpcShopOf(n.ID)
			if shop == nil {
				t.Errorf("NPC %d（%s）应是商店", n.ID, n.Name)
				continue
			}
			if len(shop.Items) != 40 {
				t.Errorf("NPC %d（%s）应有 40 件商品，实际 %d", n.ID, n.Name, len(shop.Items))
			}
			if shop.BuyRate != 1.0 {
				t.Errorf("NPC %d 的买价倍率应为 1.0（基础价），实际 %v", n.ID, shop.BuyRate)
			}
		}
	}
}

func TestConvenienceShopsSameStockBothTowns(t *testing.T) {
	// 两家店的同位置 NPC 商品完全相同（沿用原版「两村便利店同款」的设计）。
	sayaShops := convenienceShops(t, 1009)
	karuShops := convenienceShops(t, 4009)
	for _, y := range convenienceShopYs {
		a, b := NpcShopOf(sayaShops[y].ID), NpcShopOf(karuShops[y].ID)
		if a == nil || b == nil {
			t.Fatalf("y=%d 两家店都应有商店参数", y)
		}
		if len(a.Items) != len(b.Items) {
			t.Fatalf("y=%d 两家店商品数不同：%d/%d", y, len(a.Items), len(b.Items))
		}
		for i := range a.Items {
			if a.Items[i] != b.Items[i] {
				t.Errorf("y=%d 第 %d 件两家店不同：%d/%d", y, i+1, a.Items[i], b.Items[i])
			}
		}
	}
}

func TestConvenienceShopsCoverage(t *testing.T) {
	// 23 类 × 10 级 = 230 种唯一物品（G1 与 G2 都含「牙」，所以 24 个分组位只有 23 类）；
	// 向日草（11980-11989）、捏鼻草（11990-11999）全图没人卖，本次不补。
	shops := convenienceShops(t, 1009)
	unique := map[int]bool{}
	for _, y := range convenienceShopYs {
		shop := NpcShopOf(shops[y].ID)
		seen := map[int]bool{}
		for _, id := range shop.Items {
			if seen[id] {
				t.Errorf("y=%d 的商店商品重复：%d", y, id)
			}
			seen[id] = true
			unique[id] = true
		}
	}
	if len(unique) != 230 {
		t.Errorf("素材店唯一物品应为 230 种，实际 %d", len(unique))
	}
	var ids []int
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if id >= 11980 && id <= 11999 {
			t.Errorf("向日草/捏鼻草不应在素材店里：%d", id)
		}
		if _, ok := ItemByID(id); !ok {
			t.Errorf("素材店商品 %d 不在物品表里", id)
		}
	}
	for id := 11800; id <= 12049; id++ {
		if (id < 11980 || id > 11999) && !unique[id] {
			t.Errorf("合成素材 %d 没有任何素材店在卖", id)
		}
	}
}

func TestConvenienceShopsPrice(t *testing.T) {
	// 买价 = 基价 × 1.0，不做溢价；单件最贵的是 10 级石（27000）。
	shops := convenienceShops(t, 4009)
	for _, y := range convenienceShopYs {
		shop := NpcShopOf(shops[y].ID)
		for i := range shop.Items {
			itemID, price, ok := shop.BuyPrice(i)
			def, found := ItemByID(itemID)
			if !ok || !found {
				t.Fatalf("y=%d 第 %d 件（%d）取价失败", y, i+1, itemID)
			}
			if price != def.Cost {
				t.Errorf("y=%d 第 %d 件（%s）买价 %d 应等于基价 %d", y, i+1, def.Name, price, def.Cost)
			}
		}
	}
	shop := NpcShopOf(shops[17].ID)
	if _, price, _ := shop.BuyPrice(9); price != 27000 {
		t.Errorf("石的 10 级（11809）买价应为 27000，实际 %d", price)
	}
}

func TestConvenienceShopsRemovedSources(t *testing.T) {
	// 原版源 NPC（3109/1102/1302/1402/10408 的旧素材店）已删除，编号留空洞，不再有任何素材在售。
	for _, id := range []int{268, 317, 318, 345, 346, 490, 762} {
		if _, ok := NPCByID(id); ok {
			t.Errorf("编号 %d 应已删除（编号留空洞）", id)
		}
		if NpcShopOf(id) != nil {
			t.Errorf("编号 %d 的商店参数应已删除", id)
		}
	}
	for _, floor := range []int{3109, 1102, 1302, 1402} {
		if n := convenienceShops(t, floor); len(n) != 0 {
			t.Errorf("地图 %d 不应再有素材店：%v", floor, n)
		}
	}
	for _, n := range NPCsByFloor(10408) {
		if n.Name == "“欺骗”的商人" {
			t.Errorf("10408 的“欺骗”的商人应已删除：%+v", n)
		}
	}
}

func TestConvenienceShopsKeepsOtherNPCs(t *testing.T) {
	// 1009/4009 的非素材 NPC 位置与名字不变；新增店的编号接在最后（1802-1805，STAGE-309 的道具仓库占了 1798-1801）。
	keep := []struct {
		id, x, y int
		name     string
	}{
		{228, 16, 12, "工作介绍中心"},
		{229, 20, 12, "委托书贩售员"},
		{230, 27, 15, "收集破烂的商店"},
		{1239, 18, 12, "委托管理员"},
		{580, 17, 12, "工作介绍中心"},
		{581, 20, 12, "委托书贩售员"},
		{582, 27, 15, "收集破烂的商店"},
		{1301, 18, 12, "委托管理员"},
	}
	for _, k := range keep {
		n, ok := NPCByID(k.id)
		if !ok || n.X != k.x || n.Y != k.y || n.Name != k.name {
			t.Errorf("编号 %d 应是 (%d,%d)「%s」，实际 %+v（%v）", k.id, k.x, k.y, k.name, n, ok)
		}
	}
	for _, id := range []int{1802, 1803, 1804, 1805} {
		if n, ok := NPCByID(id); !ok || n.Type != "商店" || !strings.Contains(n.Name, "素材") {
			t.Errorf("新增素材店编号 %d 不对：%+v（%v）", id, n, ok)
		}
	}
}
