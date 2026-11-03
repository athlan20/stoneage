package character

import (
	"context"
	"testing"
)

// STAGE-209：装备耐久的落库（需要 STONEAGE_TEST_DATABASE_URL）：迁移 0023 的三列能读写，
// 战斗写回的耐久、等级能保存，耗尽的装备删行。

func TestItemDurabilityPersisted(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	ctx := context.Background()

	var charID int64
	withRec(t, e, "alice", func(r *record) { charID = r.ID })
	if _, err := e.chars.db.Exec(ctx,
		`INSERT INTO character_items (character_id, slot, item_id, count, durability, max_durability, crush_level) VALUES ($1, 1, 0, 1, 150000, 200000, 1)`,
		charID); err != nil {
		t.Fatal(err)
	}
	items, err := readItems(ctx, e.chars.db, charID, false)
	if err != nil {
		t.Fatal(err)
	}
	it := findItem(items, 1)
	if it == nil || it.Durability != 150000 || it.MaxDurability != 200000 || it.CrushLevel != 1 {
		t.Fatalf("读出的装备 = %+v，要 150000/200000 受损 1", it)
	}

	// 战斗写回：耐久和等级更新。
	e.chars.saveItemCrush(charID, []itemCrushWrite{{Slot: 1, Durability: 100, CrushLevel: 2}})
	items, _ = readItems(ctx, e.chars.db, charID, false)
	if it = findItem(items, 1); it == nil || it.Durability != 100 || it.CrushLevel != 2 {
		t.Fatalf("写回后 = %+v，要 100 毁坏 2", it)
	}

	// 耗尽：删行。
	e.chars.saveItemCrush(charID, []itemCrushWrite{{Slot: 1, Gone: true}})
	items, _ = readItems(ctx, e.chars.db, charID, false)
	if findItem(items, 1) != nil {
		t.Fatalf("耗尽的装备应从库里删掉：%+v", items)
	}
}
