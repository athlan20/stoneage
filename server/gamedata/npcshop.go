package gamedata

// 商店买卖参数（STAGE-70，docs/protocol.md 5.38）：npcs.tsv 里每家商店（npcgen_shop、
// npcgen_limitshop）的倍率、商品和台词，由 tools/gen-npcs.py 从原版参数文件（data/npc 的
// family/、genout/、poru/ 等，shop1_1 之类）生成。规则出处是 gmsv/npc/npc_itemshop.c：
//   - 买价 = 物品基价（ItemTemplate.Cost）× buy_rate，向下取整；
//   - 卖价 = 物品基价 × sell_rate（special_items 里的按 special_rate 高价回收，缺省 1.2）；
//   - ItemList 已在生成时展开成编号列表（上限 60，STAGE-302 由原版 MAXSHOPITEM=33 提高）；
//   - 台词逐字保留参数文件里的原话（main_msg、buy_main、what_msg、level_msg 等）。
// 限量店的 LimitItemType/LimitItemNo 收购范围本期不实现：所有店统一按 sell_rate 收购
// 背包里基价大于 0 的物品（见STAGE-70 的评论）。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-shop.json
var npcShopJSON []byte

// NpcShop 是一家商店的买卖参数。
type NpcShop struct {
	BuyRate  float64 `json:"buy_rate,omitempty"`
	SellRate float64 `json:"sell_rate,omitempty"`
	Items    []int   `json:"items,omitempty"`     // 商品编号列表（已展开，≤60，STAGE-302）
	Main     string  `json:"main,omitempty"`      // 欢迎光临
	BuyMain  string  `json:"buy_main,omitempty"`  // 商品列表页的引导语
	SellMain string  `json:"sell_main,omitempty"` // 回收列表页的引导语
	What     string  `json:"what,omitempty"`      // 问数量
	Level    string  `json:"level,omitempty"`     // 等级不够
	Realy    string  `json:"realy,omitempty"`     // 确认
	ItemFull string  `json:"itemfull,omitempty"`  // 背包满
	Stone    string  `json:"stone,omitempty"`     // 金币超上限

	// 单价覆盖（STAGE-326）：某件商品的买价直接定死（物品编号 → 买价），没写的商品仍按 基价 × buy_rate。
	// 由 tools/npc_overlay 生成时写入（安抚之素在药师店 10000 石币），不是原版数据。
	Prices map[int]int `json:"prices,omitempty"`

	// 高价回收（原版 special_item/special_rate）：列表里的物品按 SpecialRate（缺省 1.2）回收。
	SpecialItems []int   `json:"special_items,omitempty"`
	SpecialRate  float64 `json:"special_rate,omitempty"`
}

var (
	shopOnce  sync.Once
	shopByID  map[int]*NpcShop
	shopItems map[int]bool // 有商品的店（菜单给「买」的依据）
)

// NpcShopOf 返回一家商店的参数；这个 NPC 不是商店（或没有参数）时返回 nil。
func NpcShopOf(id int) *NpcShop {
	shopOnce.Do(loadNpcShop)
	return shopByID[id]
}

// ShopSellsItems 报告这家店有没有商品可买（菜单要不要给「买」）。
func (s *NpcShop) ShopSellsItems() bool { return len(s.Items) > 0 }

// ShopBuysItems 报告这家店收不收购物品（菜单要不要给「卖」）：sell_rate 有值（大于 0）就收。
func (s *NpcShop) ShopBuysItems() bool { return s.SellRate > 0 }

// BuyPrice 是编号为 at（Items 的下标从 0 数）的商品单价（见 UnitPrice）。
// at 越界或物品表里没有时返回 false。
func (s *NpcShop) BuyPrice(at int) (int, int, bool) {
	if at < 0 || at >= len(s.Items) {
		return 0, 0, false
	}
	t, ok := ItemByID(s.Items[at])
	if !ok {
		return 0, 0, false
	}
	return s.Items[at], s.UnitPrice(s.Items[at], t.Cost), true
}

// UnitPrice 是这家店买一件物品的单价：有覆盖价（Prices）用覆盖价，否则基价 × buy_rate 向下取整。
// cost 是物品的基价（调用方已经查过物品表）。
func (s *NpcShop) UnitPrice(itemID, cost int) int {
	if p, ok := s.Prices[itemID]; ok {
		return p
	}
	return int(float64(cost) * s.BuyRate)
}

// SellPrice 是一件物品的回收价：基价 × sell_rate（高价回收列表里的按 special_rate，缺省 1.2），
// 向下取整。基价为 0 的物品不收（卖了没钱，原版 cost=0 同样无意义）。
func (s *NpcShop) SellPrice(itemID int) (int, bool) {
	t, ok := ItemByID(itemID)
	if !ok || t.Cost <= 0 {
		return 0, false
	}
	rate := s.SellRate
	if s.SpecialRate > 0 && specialContains(s.SpecialItems, itemID) {
		rate = s.SpecialRate
	}
	return int(float64(t.Cost) * rate), true
}

func specialContains(items []int, itemID int) bool {
	for _, id := range items {
		if id == itemID {
			return true
		}
	}
	return false
}

func loadNpcShop() {
	shopByID = map[int]*NpcShop{}
	shopItems = map[int]bool{}
	var list []struct {
		ID int `json:"id"`
		NpcShop
	}
	if err := json.Unmarshal(npcShopJSON, &list); err != nil {
		shopByID = map[int]*NpcShop{}
		return
	}
	for i := range list {
		r := list[i].NpcShop
		shopByID[list[i].ID] = &r
		if r.ShopSellsItems() {
			shopItems[list[i].ID] = true
		}
	}
}
