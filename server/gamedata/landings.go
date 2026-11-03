package gamedata

import "sync"

// LandingOf 返回 floor 的落点（格子坐标）：warp 到这张地图时站的位置（STAGE-82，docs/protocol.md 5.16）。
//
// STAGE-161 起落点来自审核过的快捷传送目录（travel_catalog.json → travel_catalog_gen.go）：
// 只有目录里的正式地图才有落点，测试图、占位图和没有可站立地形的图都不在其中。
// 每个落点要么沿用原版传送目标，要么是经地图数据核验的固定位置，来源见 build/travel-catalog-report.md。
func LandingOf(floor int) (x, y int, ok bool) {
	// 先过目录准入：即使落点表被人为塞了一条，非正式地图也不该拿到落点。
	if !FormalFloor(floor) {
		return 0, 0, false
	}
	xy, ok := travelCatalogLandings[floor]
	if !ok {
		return 0, 0, false
	}
	return xy[0], xy[1], true
}

// WarpableFloor 判断 floor 能不能用 warp 传到：必须是在快捷传送目录里的正式地图。
// 目录本身已经保证了「有名字、有落点」，所以这里只看目录（STAGE-161）。
func WarpableFloor(floor int) bool { return FormalFloor(floor) }

// WarpMap 是 maps 列表里的一张地图（docs/protocol.md 5.42）：能传到、并且有名字的地图。
type WarpMap struct {
	Floor int    `json:"floor"`
	Name  string `json:"name"`
}

var (
	warpMapsOnce sync.Once
	warpMaps     []WarpMap
)

// WarpableMaps 返回目录里的全部正式地图，按 floor 升序。返回的切片是共享的，调用方不要修改。
//
// 名字用地图的原始名（和 `position.map_name` 一致），不带目录节点上的消歧后缀：
// 平面表用来按名字搜索和补全，重名的地图各占一条、名字可能重复，靠 floor 区分。
func WarpableMaps() []WarpMap {
	warpMapsOnce.Do(func() {
		for _, floor := range FormalFloors() {
			warpMaps = append(warpMaps, WarpMap{Floor: floor, Name: mapNames[floor]})
		}
	})
	return warpMaps
}
