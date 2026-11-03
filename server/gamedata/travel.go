package gamedata

//go:generate python ../../tools/build-travel-catalog.py

import (
	"sort"
	"sync"
)

// TravelNode 是快捷传送目录的一个节点（STAGE-161，docs/protocol.md 5.42）。
//
//   - ID 稳定不变：地图节点按 floor 命名，形如 map:1006；纯分组用 group: 前缀。
//   - ParentID nil 表示根节点。
//   - Floor nil 表示纯分组（只用来分层，不能传送）；有值的是可传送的正式地图。
//   - Order 是同级排序号，按审核清单里的顺序，从 0 开始。
//
// 节点表与落点表由 tools/build-travel-catalog.py 从 travel_catalog.json 生成，
// 每张正式地图恰好对应一个地图节点，能传入的 floor 集合与目录里的地图节点集合相等。
type TravelNode struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id"`
	Name     string  `json:"name"`
	Floor    *int    `json:"floor"`
	Order    int     `json:"order"`
}

// ip、sp 生成节点表里的指针字段（nil 表示「没有」），只在生成文件里用。
func ip(v int) *int          { return &v }
func sp(v string) *string    { return &v }

// TravelCatalogVersion 返回快捷传送目录的版本号：层级或落点配置变了它才会变。
func TravelCatalogVersion() string { return travelCatalogVersion }

// TravelCatalog 返回目录的全部节点，根在前、同级按 Order。返回的切片是共享的，调用方不要修改。
func TravelCatalog() []TravelNode { return travelCatalogNodes }

var (
	travelOnce      sync.Once
	travelFormal    map[int]string // floor -> 目录里的显示名
	travelReachable []int          // 正式地图的 floor，升序
)

func buildTravelIndex() {
	travelFormal = make(map[int]string, len(travelCatalogLandings))
	for _, node := range travelCatalogNodes {
		if node.Floor == nil {
			continue
		}
		travelFormal[*node.Floor] = node.Name
		travelReachable = append(travelReachable, *node.Floor)
	}
	sort.Ints(travelReachable)
}

// FormalFloor 判断 floor 是不是目录里的正式地图。快捷传送的准入只看这一条：
// 测试图、占位图、缺落点的图都不在目录里，因此不能经 warp、按名字搜索或跨图找 NPC 绕进来。
func FormalFloor(floor int) bool {
	travelOnce.Do(buildTravelIndex)
	_, ok := travelFormal[floor]
	return ok
}

// TravelNodeName 返回正式地图在目录里显示的名字（同名地图带 ` · floor` 消歧后缀）；不是正式地图时为空。
func TravelNodeName(floor int) string {
	travelOnce.Do(buildTravelIndex)
	return travelFormal[floor]
}

// FormalFloors 返回全部正式地图的 floor，升序。返回的切片是共享的，调用方不要修改。
func FormalFloors() []int {
	travelOnce.Do(buildTravelIndex)
	return travelReachable
}
