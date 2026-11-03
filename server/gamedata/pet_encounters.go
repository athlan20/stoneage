// STAGE-251：1 级宠物目录，把“能在野外遇到的 1 级宠物”按宠物列出，每只宠物带可遇点（区域代表落点）。
// 数据由 tools/gen-pet-encounters.py 生成（pet_encounters_gen.go），口径见 docs/encounter.md；
// 这里只放类型和只读访问，不做筛选。

package gamedata

// PetEncounterPoint 是地图格子坐标。
type PetEncounterPoint struct{ X, Y int }

// PetEncounterRect 是闭区间矩形（已裁到地图边界）。
type PetEncounterRect struct{ X1, Y1, X2, Y2 int }

// PetEncounterPet 是 1 级宠物目录的一行：一个宠物模板（一只宠物 = 一个模板）。
type PetEncounterPet struct {
	ID        int      // 模板编号
	Name      string   // 宠物名（同名模板允许重复，显示时靠编号区分）
	Aliases   []string // 民间俗称（pet_aliases.tsv），可空
	Series    string   // 系列标签（pet_series.tsv），可空
	Atoms     [4]int   // 地、水、火、风（只用于详情展示，不是分类依据）
	Size      int      // 体型
	Catch     int      // 捕捉难度
	SortKey   string   // 名字的 GBK 字节序（十六进制），近似拼音序；列表已按它排好，客户端不改序
	SpotCount int      // 可遇点个数
}

// PetEncounterSpot 是一个可遇点：某只宠物能在某个遇敌区域遇到，Anchor 是开战时的落点。
type PetEncounterSpot struct {
	PetID      int
	AreaID     string // 稳定区域 ID，与 encounter_areas 的 id 相同
	Number     int    // 区域原编号
	Floor      int
	MapName    string // 快捷传送目录里的显示名（同名已带 ` · floor`）
	Anchor     PetEncounterPoint
	Rect       PetEncounterRect
	MaxEnemies int      // 这一区域一场战斗的最多敌人数
	Steps      int      // 约多少格一场（CEP 期望步数，取整）
	AlsoLevel1 []string // 同一区域其他 1 级宠物的名字
}

// PetEncounterCatalogVersion 是 1 级宠物目录的版本（数据指纹）；数据变了客户端要重新取。
func PetEncounterCatalogVersion() string { return petEncounterVersion }

// PetEncounterPets 返回全部 1 级宠物，已按 SortKey 排好。返回的切片是共享的，调用方不要修改。
func PetEncounterPets() []PetEncounterPet { return petEncounterPetRows }

// PetEncounterSpots 返回全部可遇点，按宠物顺序、地图、区域 ID 排好。返回的切片是共享的，调用方不要修改。
func PetEncounterSpots() []PetEncounterSpot { return petEncounterSpotRows }

// PetEncounterSpotFor 按宠物模板与区域 ID 找可遇点；找不到返回 false（这只宠物在这个区域没有可遇点）。
func PetEncounterSpotFor(petID int, areaID string) (*PetEncounterSpot, bool) {
	for i := range petEncounterSpotRows {
		s := &petEncounterSpotRows[i]
		if s.PetID == petID && s.AreaID == areaID {
			return s, true
		}
	}
	return nil, false
}

// PetEncounterPetByID 按模板编号取宠物；找不到返回 false。
func PetEncounterPetByID(id int) (*PetEncounterPet, bool) {
	for i := range petEncounterPetRows {
		if petEncounterPetRows[i].ID == id {
			return &petEncounterPetRows[i], true
		}
	}
	return nil, false
}
