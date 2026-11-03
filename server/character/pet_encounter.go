// STAGE-251：1 级宠物目录（docs/protocol.md 5.62 / 5.63，目录口径见 docs/encounter.md）。
// pet_encounter_catalog 一次给全量宠物与可遇点，客户端缓存后在本地筛选；
// pet_encounter_start 跨图定位到可遇点的代表落点，并立刻开战（once / infinite），或只传送（travel）。
// 开战那一半复用 STAGE-152 的 encounter_start 语义，这里只负责把宠物和区域对上、校验，然后交给它。

package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// petAtomsView 是宠物的四元素（地水火风），只用于详情展示。
type petAtomsView struct {
	Earth int `json:"earth"`
	Water int `json:"water"`
	Fire  int `json:"fire"`
	Wind  int `json:"wind"`
}

// petCatalogPetView 是 pet_encounter_catalog 的一只宠物。
type petCatalogPetView struct {
	PetID     int          `json:"pet_id"`
	Name      string       `json:"name"`
	Aliases   []string     `json:"aliases"`
	Series    string       `json:"series"`
	Atoms     petAtomsView `json:"atoms"`
	Size      int          `json:"size"`
	Catch     int          `json:"catch_difficulty"`
	SortKey   string       `json:"sort_key"`
	SpotCount int          `json:"spot_count"`
}

// petCatalogSpotView 是 pet_encounter_catalog 的一个可遇点。
type petCatalogSpotView struct {
	PetID      int       `json:"pet_id"`
	AreaID     string    `json:"area_id"`
	Number     int       `json:"number"`
	Floor      int       `json:"floor"`
	MapName    string    `json:"map_name"`
	Anchor     pointView `json:"anchor"`
	Rect       rectView  `json:"rect"`
	MaxEnemies int       `json:"max_enemies"`
	Steps      int       `json:"steps"`
	AlsoLevel1 []string  `json:"also_level1"`
}

// handlePetEncounterCatalog 是 pet_encounter_catalog（查看类，协议 5.62）：与角色无关的静态目录。
func (s *Service) handlePetEncounterCatalog(_ context.Context, _ *gateway.Session, _ *protocol.Request) (any, error) {
	return petEncounterCatalogView(), nil
}

func petEncounterCatalogView() map[string]any {
	pets := []petCatalogPetView{}
	for _, p := range gamedata.PetEncounterPets() {
		aliases := append([]string{}, p.Aliases...)
		pets = append(pets, petCatalogPetView{
			PetID: p.ID, Name: p.Name, Aliases: aliases, Series: p.Series,
			Atoms: petAtomsView{Earth: p.Atoms[0], Water: p.Atoms[1], Fire: p.Atoms[2], Wind: p.Atoms[3]},
			Size:  p.Size, Catch: p.Catch, SortKey: p.SortKey, SpotCount: p.SpotCount,
		})
	}
	spots := []petCatalogSpotView{}
	for _, s := range gamedata.PetEncounterSpots() {
		spots = append(spots, petCatalogSpotView{
			PetID: s.PetID, AreaID: s.AreaID, Number: s.Number, Floor: s.Floor, MapName: s.MapName,
			Anchor:     pointView{X: s.Anchor.X, Y: s.Anchor.Y},
			Rect:       rectView{X1: s.Rect.X1, Y1: s.Rect.Y1, X2: s.Rect.X2, Y2: s.Rect.Y2},
			MaxEnemies: s.MaxEnemies, Steps: s.Steps, AlsoLevel1: append([]string{}, s.AlsoLevel1...),
		})
	}
	return map[string]any{
		"catalog_version": gamedata.PetEncounterCatalogVersion(),
		"pets":            pets,
		"spots":           spots,
	}
}

// handlePetEncounterStart 是 pet_encounter_start（操作类，协议 5.63）：客户端只给宠物编号和区域 ID，
// 坐标与地图由服务端从目录和区域数据里解析。
func (s *Service) handlePetEncounterStart(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		PetID  *int   `json:"pet_id"` // 指针：缺省要报 BAD_REQUEST，不能当成 0 号模板去比对
		AreaID string `json:"area_id"`
		Mode   string `json:"mode"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.PetID == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要整数字段 pet_id（宠物模板编号，见 pet_encounter_catalog）")
	}
	if in.AreaID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "area_id 不能为空")
	}
	if in.Mode != "once" && in.Mode != "infinite" && in.Mode != "travel" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "mode 要是 once、infinite 或 travel（只传送不遇敌）")
	}
	live, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var perr *protocol.Error
	if err := live.Operate(func(r *record) {
		if perr = followerError(live.p); perr != nil { // 队员不能自己遇敌（STAGE-274）
			return
		}
		out, perr = s.petEncounterStartLocked(live, r, *in.PetID, in.AreaID, in.Mode)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return out, nil
}

// petEncounterStartLocked 在角色操作锁内校验并执行。所有失败都发生在改动位置之前，所以没有副作用。
func (s *Service) petEncounterStartLocked(live *Live, r *record, petID int, areaID, mode string) (map[string]any, *protocol.Error) {
	p := live.p
	if p.battle != nil {
		return nil, protocol.Errorf(protocol.CodeInBattle, "战斗中不能换遇敌区域，先打完这一场")
	}
	spot, ok := gamedata.PetEncounterSpotFor(petID, areaID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodePetEncounterNotFound, "这只宠物在这个区域没有可遇点，请重新读取 1 级宠物目录")
	}
	area, ok := gamedata.EncounterAreaByKey(areaID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodePetEncounterNotFound, "没有这个遇敌区域，请重新读取 1 级宠物目录")
	}
	view, ok := buildAreaView(spot.Floor, area, heldItemSet(r))
	if !ok || !view.Available {
		reason := "这个区域当前不能遇敌"
		if ok && view.UnavailableReason != nil {
			reason = *view.UnavailableReason
		}
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "%s", reason)
	}
	if !areaHasTemplate(view, petID) {
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "这个区域当前遇不到这只宠物（道具条件变了）")
	}
	if view.Anchor != (pointView{X: spot.Anchor.X, Y: spot.Anchor.Y}) {
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "目录和服务端的区域数据对不上，请重新读取 1 级宠物目录")
	}
	if mode == "travel" {
		s.teleport(live, r, spot.Floor, view.Anchor.X, view.Anchor.Y)
		return encounterChangedData(p, ""), nil
	}
	if perr := s.checkNoEnemyEncounter(p); perr != nil { // 龙王的守护生效中（STAGE-340）：传送不算遇敌，照常
		return nil, perr
	}
	if !s.startWildBattleOn(p, spot.Floor, view.Anchor.X, view.Anchor.Y) {
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "这个区域现在抽不出敌人")
	}
	p.encounterOn = true
	p.plan = &encounterPlan{AreaID: area.Key, Number: area.ID, Mode: mode}
	p.pushAll("encounter_changed", encounterChangedData(p, ""))
	return encounterChangedData(p, ""), nil
}

// areaHasTemplate 报告区域当前可遇的敌人里有没有这个宠物模板。
func areaHasTemplate(view areaView, tpl int) bool {
	for _, e := range view.Enemies {
		if v, ok := gamedata.EnemyByID(e.ID); ok && v.Template == tpl {
			return true
		}
	}
	return false
}
