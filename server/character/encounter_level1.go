// STAGE-286：遇敌列标出能遇到 1 级宠物的区域（docs/protocol/encounter-areas.md 的 level1_pets）。
// 数据只来自 1 级宠物目录（gamedata.PetEncounterSpots），不另扫敌群表；索引按地图与区域 ID 预建一次。

package character

import (
	"sync"

	"github.com/athlan20/stoneage/server/gamedata"
)

// areaPetKey 是一个遇敌区域：地图加稳定区域 ID。
type areaPetKey struct {
	floor int
	area  string
}

var (
	level1PetsOnce  sync.Once
	level1PetsIndex map[int]map[string][]string // floor → 区域 ID → 1 级宠物名
)

// level1PetsOn 返回这张图上每个区域的 1 级宠物名（只读，不要修改）；没有宠物点的图返回 nil。
func level1PetsOn(floor int) map[string][]string {
	level1PetsOnce.Do(func() { level1PetsIndex = buildLevel1PetIndex() })
	return level1PetsIndex[floor]
}

// buildLevel1PetIndex 按可遇点汇总：每个可遇点的宠物本身，加上同区域其他 1 级宠物（AlsoLevel1）。
// 名字去重，顺序跟随目录（目录已按宠物顺序排好），所以结果稳定。
func buildLevel1PetIndex() map[int]map[string][]string {
	names := make(map[int]string, len(gamedata.PetEncounterPets()))
	for _, p := range gamedata.PetEncounterPets() {
		names[p.ID] = p.Name
	}
	lists := map[areaPetKey][]string{}
	seen := map[areaPetKey]map[string]bool{}
	add := func(k areaPetKey, name string) {
		if name == "" {
			return
		}
		if seen[k] == nil {
			seen[k] = map[string]bool{}
		}
		if seen[k][name] {
			return
		}
		seen[k][name] = true
		lists[k] = append(lists[k], name)
	}
	for _, sp := range gamedata.PetEncounterSpots() {
		k := areaPetKey{floor: sp.Floor, area: sp.AreaID}
		add(k, names[sp.PetID])
		for _, n := range sp.AlsoLevel1 {
			add(k, n)
		}
	}
	idx := map[int]map[string][]string{}
	for k, list := range lists {
		if idx[k.floor] == nil {
			idx[k.floor] = map[string][]string{}
		}
		idx[k.floor][k.area] = list
	}
	return idx
}
