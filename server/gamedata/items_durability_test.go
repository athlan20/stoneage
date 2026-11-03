package gamedata

import "testing"

// STAGE-209：物品表的耐久两列（itemset.txt 第 30、31 列，docs/durability.md）。
// 小斧头是第 0 行（编号 0），耐久 200000、最大耐久 200000，见 items_test.go。
func TestItemDurabilityCount(t *testing.T) {
	itemOnce.Do(loadItems)
	n := 0
	for id, it := range itemTable {
		if it.MaxDurability > 0 {
			n++
			if it.Durability != it.MaxDurability {
				t.Fatalf("物品 %d 当前耐久 %d != 最大耐久 %d", id, it.Durability, it.MaxDurability)
			}
		}
	}
	// 有耐久的物品共 1547 条，与 docs/durability.md 第 4 节的统计一致。
	if n != 1547 {
		t.Fatalf("有耐久的物品 = %d，要 1547", n)
	}
}
