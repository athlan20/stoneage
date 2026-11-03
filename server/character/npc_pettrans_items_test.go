package character

import "testing"

// STAGE-237：转生删道具的范围（原版 DelItem 表）和不查数量、整格删除。
func TestPetTransItemDeleted(t *testing.T) {
	cases := []struct {
		id   int
		want bool
	}{
		{19624, false}, // 事件 70 给的项链，不在删除范围
		{19625, true},  // 地灵石
		{19628, true},  // 风灵石
		{19629, true},  // 地灵项链
		{19632, true},  // 风灵项链
		{19633, false}, // 范围之间的空档
		{19671, false},
		{19672, true},
		{19687, true},
		{19688, false},
		{100, false},
	}
	for _, c := range cases {
		if got := petTransItemDeleted(c.id); got != c.want {
			t.Errorf("petTransItemDeleted(%d) = %v，要 %v", c.id, got, c.want)
		}
	}
}

func TestPlanPetTransItems(t *testing.T) {
	items := []itemRec{
		{Slot: 5, ItemID: 19625, Count: 1},
		{Slot: 6, ItemID: 100, Count: 3},
		{Slot: 7, ItemID: 19687, Count: 1},
		{Slot: 8, ItemID: 19629, Count: 1},
	}
	got := planPetTransItems(items)
	if len(got) != 1 || got[0].ItemID != 100 || got[0].Slot != 6 || got[0].Count != 3 {
		t.Fatalf("删完之后背包 = %+v，只应剩 100 号道具（6 号格，3 件）", got)
	}
	if len(items) != 4 {
		t.Fatalf("传入的切片被改了：%+v", items)
	}
	if got := planPetTransItems(nil); len(got) != 0 {
		t.Fatalf("空背包删完还是 %+v", got)
	}
}
