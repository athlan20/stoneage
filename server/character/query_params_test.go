package character

import (
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-258：maps 的 keyword 过滤和 encounter_areas 的 offset/limit 分页（纯函数，不需要数据库）。

func TestFilterWarpMaps(t *testing.T) {
	all := gamedata.WarpableMaps()
	if got := filterWarpMaps(all, ""); len(got) != len(all) {
		t.Fatalf("空关键字不应过滤，得到 %d 条，应为 %d 条", len(got), len(all))
	}
	if got := filterWarpMaps(all, "  \t "); len(got) != len(all) {
		t.Fatalf("只有空白的关键字也不应过滤，得到 %d 条", len(got))
	}
	got := filterWarpMaps(all, " 萨姆吉尔村 ")
	if len(got) != 1 || got[0].Floor != 1000 || got[0].Name != "萨姆吉尔村" {
		t.Fatalf("按名字过滤应只剩 1000 萨姆吉尔村，实际 %v", got)
	}
	none := filterWarpMaps(all, "不存在的地方")
	if none == nil || len(none) != 0 {
		t.Fatalf("没有命中时应返回非 nil 的空切片，实际 %v", none)
	}
}

func TestPageAreaViews(t *testing.T) {
	var all []areaView
	for i := 0; i < 5; i++ {
		all = append(all, areaView{ID: strconv.Itoa(i)})
	}
	ids := func(vs []areaView) string {
		s := ""
		for _, v := range vs {
			s += v.ID
		}
		return s
	}
	i := func(n int) *int { return &n }
	cases := []struct {
		name          string
		offset, limit *int
		want          string
	}{
		{"缺省返回全部", nil, nil, "01234"},
		{"offset 和 limit", i(1), i(2), "12"},
		{"只给 offset", i(3), nil, "34"},
		{"offset 越界", i(10), nil, ""},
		{"limit 为 0", nil, i(0), ""},
		{"limit 超过剩余数量", i(4), i(9), "4"},
		{"limit 超过 100 按 100 处理", nil, i(500), "01234"},
	}
	for _, c := range cases {
		got := pageAreaViews(all, c.offset, c.limit)
		if got == nil {
			t.Fatalf("%s：结果应为非 nil 的切片", c.name)
		}
		if ids(got) != c.want {
			t.Fatalf("%s：want %q, got %q", c.name, c.want, ids(got))
		}
	}
	if len(pageAreaViews(nil, nil, nil)) != 0 {
		t.Fatal("没有区域时应返回空切片")
	}
}
