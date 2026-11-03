package character

import "testing"

// STAGE-209：装备耐久的搬运与视图（不连数据库）：进战斗的快照带上耐久，背包视图只对有耐久的物品下发。

func TestSideInputEquipDurability(t *testing.T) {
	r := &record{Name: "测试", Level: 1, Items: []itemRec{
		{Slot: 1, ItemID: 0, Count: 1, Durability: 199000, MaxDurability: 200000, CrushLevel: 0},
		{Slot: 5, ItemID: 0, Count: 1}, // 背包里的同一件物品不进装备栏
	}}
	side := sideInput(r)
	e := side.Equips[1]
	if e == nil || e.Name != "小斧头" || e.Durability != 199000 || e.MaxDurability != 200000 {
		t.Fatalf("装备栏 1 = %+v", e)
	}
	if side.Equips[0] != nil || side.Equips[2] != nil {
		t.Errorf("空栏应为 nil：%+v %+v", side.Equips[0], side.Equips[2])
	}
}

func TestInventoryViewDurabilityOnlyWhenDurable(t *testing.T) {
	r := &record{Items: []itemRec{
		{Slot: 1, ItemID: 0, Count: 1, Durability: 150000, MaxDurability: 200000, CrushLevel: 1},
		{Slot: 5, ItemID: 0, Count: 1},
	}}
	slots := r.inventoryView()["slots"].([]inventorySlotView)
	if slots[0].Durability != 150000 || slots[0].MaxDurability != 200000 || slots[0].CrushLevel != 1 {
		t.Errorf("有耐久的物品视图 = %+v", slots[0])
	}
	if slots[1].MaxDurability != 0 || slots[1].Durability != 0 {
		t.Errorf("无耐久的物品不应带耐久：%+v", slots[1])
	}
}
