// STAGE-190：装备自带的精灵进入精灵栏位。

package character

import "testing"

func noMagics() [5]int { return [5]int{-1, -1, -1, -1, -1} }

// 1012 芙雷雅Lv1铠2 穿在身（装备位 1）：1 号栏位是滋润的精灵 Lv1（10），MP 取装备的 8，不是魔法表的 6。
func TestEquippedItemGivesMagic(t *testing.T) {
	r := &record{Magics: noMagics(), Items: []itemRec{{Slot: 1, ItemID: 1012, Count: 1}}}
	slots := r.magicSlots()
	if slots[1] != (magicSlot{ID: 10, MP: 8}) {
		t.Fatalf("1 号栏位应是滋润 Lv1（MP 8）: %+v", slots[1])
	}
	for _, i := range []int{0, 2, 3, 4} {
		if slots[i].ID != -1 {
			t.Errorf("%d 号栏位应为空: %+v", i, slots[i])
		}
	}
	view := r.magicView()
	if len(view) != 1 || view[0].Slot != 1 || view[0].ID != 10 || view[0].MP != 8 || view[0].Name != "滋润的精灵 Lv1" {
		t.Fatalf("status.magic 应带出装备的精灵: %+v", view)
	}
}

// 背包里（槽位 5 起）的装备不算；没穿就没有精灵。
func TestBagItemGivesNoMagic(t *testing.T) {
	r := &record{Magics: noMagics(), Items: []itemRec{{Slot: 5, ItemID: 1012, Count: 1}}}
	if got := r.magicView(); len(got) != 0 {
		t.Fatalf("背包里的装备不该给精灵: %+v", got)
	}
}

// 管理命令给的精灵占着同号栏位时以它为准；MP 是魔法表里的值。
func TestInnateMagicWinsSameSlot(t *testing.T) {
	r := &record{Magics: [5]int{-1, 0, -1, -1, -1}, Items: []itemRec{{Slot: 1, ItemID: 1012, Count: 1}}}
	slots := r.magicSlots()
	if slots[1].ID != 0 {
		t.Fatalf("1 号栏位应是自己的治愈 Lv1: %+v", slots[1])
	}
}
