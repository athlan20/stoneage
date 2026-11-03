package gamedata

// 地图格子尺寸（STAGE-152）：遇敌区域要把矩形裁到地图边界内，代表落点才不会落到地图外。
// 数据由 tools/gen-mapsizes.py 从 LS2MAP 文件头生成到 mapsizes_gen.go。

// MapSize 返回地图的格子宽高；没有这张地图的数据时 ok 为 false。
// 注意：LS2MAP 的坐标是 0..w-1 / 0..h-1，宽高本身不是合法坐标。
func MapSize(floor int) (w, h int, ok bool) {
	s, ok := mapSizes[floor]
	if !ok {
		return 0, 0, false
	}
	return s[0], s[1], true
}

// ClampToMap 把一个闭区间矩形裁到地图边界内；地图没有尺寸数据时原样返回。
// 裁完为空（矩形完全在地图外）时 ok 为 false。
func ClampToMap(floor, x1, y1, x2, y2 int) (nx1, ny1, nx2, ny2 int, ok bool) {
	w, h, has := MapSize(floor)
	if !has {
		return x1, y1, x2, y2, x1 <= x2 && y1 <= y2
	}
	nx1, ny1 = max(x1, 0), max(y1, 0)
	nx2, ny2 = min(x2, w-1), min(y2, h-1)
	return nx1, ny1, nx2, ny2, nx1 <= nx2 && ny1 <= ny2
}
