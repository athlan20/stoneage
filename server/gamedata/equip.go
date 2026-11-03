package gamedata

// 装备部位（STAGE-29，docs/ui.md 背包页 格子与编号）。出处：gmsv/item/item.c 的 ITEM_getEquipPlace、
// gmsv/include/item.h 的 ITEM_CATEGORY。盾牌在原版服务端数据里被注释掉了，一期不支持。

// 装备槽位（0–4，绝对编号，与背包页、协议一致）。
const (
	SlotHead           = 0 // 头
	SlotBody           = 1 // 身
	SlotHand           = 2 // 手
	SlotAccessory1     = 3 // 饰品 1
	SlotAccessory2     = 4 // 饰品 2
	MaxEquipSlot       = SlotAccessory2
	equipSlotCount     = MaxEquipSlot + 1
	itemTypeFist       = 0
	itemTypeAxe        = 1
	itemTypeClub       = 2
	itemTypeSpear      = 3
	itemTypeBow        = 4
	itemTypeHelm       = 6
	itemTypeArmour     = 7
	itemTypeBracelet   = 8
	itemTypeAmulet     = 15
	itemTypeBoomerang  = 17
	itemTypeBoundThrow = 18
	itemTypeBreakThrow = 19
)

// EquipPlaces 返回物品类型能穿的部位（槽位编号）；不能穿的类型返回 nil。
// 饰品类型（手镯到护身符）能穿 3 号或 4 号，先放 3 号（调用方决定）。
func EquipPlaces(itemType int) []int {
	switch {
	case itemType == itemTypeFist, itemType == itemTypeAxe, itemType == itemTypeClub,
		itemType == itemTypeSpear, itemType == itemTypeBow,
		itemType == itemTypeBoomerang, itemType == itemTypeBoundThrow, itemType == itemTypeBreakThrow:
		return []int{SlotHand}
	case itemType == itemTypeHelm:
		return []int{SlotHead}
	case itemType == itemTypeArmour:
		return []int{SlotBody}
	case itemType >= itemTypeBracelet && itemType <= itemTypeAmulet:
		return []int{SlotAccessory1, SlotAccessory2}
	}
	return nil
}

// IsEquipSlot 判断槽位是不是装备槽（0–4）。
func IsEquipSlot(slot int) bool {
	return slot >= 0 && slot < equipSlotCount
}
