package character

import (
	"context"
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-191：物品表里没有的编号（掉落表漏了的）。1732 是 enemies.tsv 里引用、itemset.tsv 里没有的真实例子。
const unknownItemID = 1732

// 背包里有查不到模板的格子：仍然显示（占位名），不能静默跳过，否则格子看起来是空的。
func TestInventoryShowsUnknownItemPlaceholder(t *testing.T) {
	if _, ok := gamedata.ItemByID(unknownItemID); ok {
		t.Fatalf("测试用的 %d 应当不在物品表里", unknownItemID)
	}
	r := &record{Items: []itemRec{{Slot: 6, ItemID: unknownItemID, Count: 1}}}
	slots := r.inventoryView()["slots"].([]inventorySlotView)
	if len(slots) != 1 || slots[0].Slot != 6 || slots[0].Name != "未知物品 #1732" {
		t.Fatalf("未知物品应显示成占位格子，实际 %+v", slots)
	}
}

// 战斗掉落的编号在物品表里查不到：不发放、不占格（已知的物品照常放进第一个空格）。
func TestPlaceRewardItemsSkipsUnknownItem(t *testing.T) {
	r := &record{}
	kept, placed := placeRewardItems(r, []battle.RewardItem{
		{ItemID: unknownItemID, Count: 1},
		{ItemID: 1234, Count: 1},
	})
	if len(kept) != 1 || kept[0].ItemID != 1234 {
		t.Fatalf("只应保留已知物品，实际 %+v", kept)
	}
	if len(placed) != 1 || placed[0].Slot != firstBagSlot || len(r.Items) != 1 {
		t.Fatalf("已知物品应放进第一个空格，实际 placed=%+v items=%+v", placed, r.Items)
	}
}

// 占位的幽灵物品玩家能丢掉：item_drop 不依赖物品模板。
func TestUnknownItemCanBeDropped(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, 6, $2, 1)`, r.ID, unknownItemID); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: 6, ItemID: unknownItemID, Count: 1}}
	})
	if m, _ := c.callp("item_drop", `{"slot":6,"confirm":"yes"}`); m["ok"] != true {
		t.Fatalf("丢弃未知物品应成功: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if findItem(r.Items, 6) != nil {
			t.Fatalf("丢弃后 6 号格应空，实际 %+v", r.Items)
		}
	})
}
