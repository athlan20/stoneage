package character

import "github.com/athlan20/stoneage/server/gamedata"

// newItemDurability 是新物品的初始耐久（STAGE-209，docs/durability.md）：取物品表的当前耐久和最大耐久，
// 与原版 item.c 直接把表值写进物品一致。表里查不到的物品返回 0、0（不损坏）。
func newItemDurability(itemID int) (durability, maxDurability int) {
	if t, ok := gamedata.ItemByID(itemID); ok {
		return t.Durability, t.MaxDurability
	}
	return 0, 0
}
