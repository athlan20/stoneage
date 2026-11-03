package character

import (
	"context"
	"testing"
)

// STAGE-207：合成品标记（itemRec.Merged）写库后重读还在，不合成的物品读出来是假。
func TestMergedFlagSavedAndReloaded(t *testing.T) {
	e, _ := enterAt(t, 1006, 15, 22, false)
	p := e.presence(t, "勇者小明")
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx := context.Background()
	if err := e.chars.changeItems(ctx, p, &p.rec, func(items []itemRec, _ int) ([]itemRec, error) {
		return append(items,
			itemRec{Slot: 5, ItemID: 1, Count: 1, Merged: true},
			itemRec{Slot: 6, ItemID: 2, Count: 1}), nil
	}); err != nil {
		t.Fatal(err)
	}
	items, err := e.chars.loadItems(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !items[0].Merged || items[1].Merged {
		t.Fatalf("读回的合成品标记不对: %+v", items)
	}
	// 只改数量（扣掉一件）不能丢掉标记，同一格的合成品仍是合成品。
	if err := e.chars.changeItems(ctx, p, &p.rec, func(items []itemRec, _ int) ([]itemRec, error) {
		next := make([]itemRec, 0, len(items))
		for _, it := range items {
			if it.Slot == 6 {
				continue
			}
			it.Count = 1
			it.Merged = it.Merged && it.Slot == 5
			next = append(next, it)
		}
		return next, nil
	}); err != nil {
		t.Fatal(err)
	}
	items, err = e.chars.loadItems(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Slot != 5 || !items[0].Merged {
		t.Fatalf("删掉一格后标记应留在 5 号格: %+v", items)
	}
}
