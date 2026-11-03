package character

import (
	"reflect"
	"testing"
)

// STAGE-308：强者表的分页和翻页选项是纯函数，不依赖数据库。
func TestDuelRankPaging(t *testing.T) {
	for _, tc := range []struct {
		total, pages int
	}{{0, 1}, {1, 1}, {10, 1}, {11, 2}, {20, 2}, {21, 3}} {
		if got := duelRankPages(tc.total); got != tc.pages {
			t.Errorf("duelRankPages(%d) = %d，要 %d", tc.total, got, tc.pages)
		}
	}
	for _, tc := range []struct{ rank, page int }{{1, 1}, {10, 1}, {11, 2}, {152, 16}} {
		if got := duelRankPageOf(tc.rank); got != tc.page {
			t.Errorf("duelRankPageOf(%d) = %d，要 %d", tc.rank, got, tc.page)
		}
	}
}

func TestDuelRankOptions(t *testing.T) {
	for _, tc := range []struct {
		page, pages int
		want        []npcOption
	}{
		{1, 1, []npcOption{}},
		{1, 3, []npcOption{{No: 1, Label: "下一页"}}},
		{2, 3, []npcOption{{No: 1, Label: "下一页"}, {No: 2, Label: "上一页"}}},
		{3, 3, []npcOption{{No: 1, Label: "上一页"}}},
	} {
		if got := duelRankOptions(tc.page, tc.pages); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("duelRankOptions(%d, %d) = %+v，要 %+v", tc.page, tc.pages, got, tc.want)
		}
	}
}
