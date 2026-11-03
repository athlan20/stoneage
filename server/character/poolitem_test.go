package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-309：道具仓库的纯函数（格子查找、寄存列表只含背包格、取出后去掉原格）。
func TestFreePoolItemSlot(t *testing.T) {
	pool := []itemRec{{Slot: 0}, {Slot: 1}}
	if got := freePoolItemSlot(pool, 10); got != 2 {
		t.Errorf("空格 = %d，要 2", got)
	}
	if got := freePoolItemSlot(pool, 2); got != -1 {
		t.Errorf("上限 2 格已满，空格 = %d，要 -1", got)
	}
	if got := freePoolItemSlot(nil, 0); got != -1 {
		t.Errorf("上限 0 格，空格 = %d，要 -1", got)
	}
}

func TestBagItemOptionsSkipsEquipAndEmpty(t *testing.T) {
	r := &record{Items: []itemRec{{Slot: 0, ItemID: 1, Count: 1}, {Slot: 4, ItemID: 2, Count: 1}, {Slot: 5, ItemID: 3, Count: 1}, {Slot: 19, ItemID: 4, Count: 2}}}
	opts := bagItemOptions(r)
	if len(opts) != 2 || opts[0].No != 5 || opts[1].No != 19 {
		t.Fatalf("寄存列表 = %+v，要只有背包格 5 和 19", opts)
	}
	if bagItemAt(r.Items, 0) != nil || bagItemAt(r.Items, 6) != nil {
		t.Errorf("装备格或空格不能寄存")
	}
}

func TestWithoutItemSlotCopies(t *testing.T) {
	items := []itemRec{{Slot: 5, ItemID: 3, Count: 1}, {Slot: 6, ItemID: 4, Count: 1}}
	out := withoutItemSlot(items, 5)
	if len(out) != 1 || out[0].Slot != 6 || len(items) != 2 {
		t.Fatalf("去掉格 5 后 = %+v，原切片被改了 %+v", out, items)
	}
}

func TestPoolItemLabelShowsCount(t *testing.T) {
	if got := poolItemLabel(itemRec{ItemID: 999999, Count: 1}); got != "物品 999999" {
		t.Errorf("单件名字 = %q", got)
	}
	if got := poolItemLabel(itemRec{ItemID: 999999, Count: 3}); got != "物品 999999 x3" {
		t.Errorf("堆叠名字 = %q", got)
	}
}

// STAGE-347：道具仓库固定 50 格，0 转也一样；第 51 件没有空格（寄存时由 poolItemDeposit 回 poolfull）。
func TestPoolItemHasFiftySlotsForEveryone(t *testing.T) {
	for _, tensei := range []int{0, 1, 5, 9} {
		if got := gamedata.PoolItemLimit(tensei); got != 50 {
			t.Fatalf("转生 %d 的仓库格数 = %d，要 50", tensei, got)
		}
	}
	pool := make([]itemRec, 0, 50)
	for slot := 0; slot < 49; slot++ {
		pool = append(pool, itemRec{Slot: slot, ItemID: 1, Count: 1})
	}
	if got := freePoolItemSlot(pool, gamedata.PoolItemLimit(0)); got != 49 {
		t.Fatalf("存了 49 件时下一格 = %d，要 49", got)
	}
	pool = append(pool, itemRec{Slot: 49, ItemID: 1, Count: 1})
	if got := freePoolItemSlot(pool, gamedata.PoolItemLimit(0)); got != -1 {
		t.Fatalf("存满 50 件时应没有空格，得到 %d", got)
	}
}
