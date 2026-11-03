// 按区域遇敌的查询端（STAGE-152，docs/protocol/encounter-areas.md）：
// 把 encount 表里本图的区域整理成客户端能直接用的列表——代表落点、候选敌人、
// 有效人数上限、当前能不能开战。开战那一半在 encounter.go。

package character

import (
	"context"
	"sort"
	"strconv"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// areaEnemyView 是区域里的一个候选敌人（随机怪组已展开成具体候选）。
type areaEnemyView struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	LevelMin  int    `json:"level_min"`
	LevelMax  int    `json:"level_max"`
	CreateMax int    `json:"create_max"`
	Weight    int    `json:"weight"` // 区域权重 × 敌群权重，只用于看谁更常见
	GroupID   int    `json:"group_id"`
}

type rectView struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

type pointView struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// areaView 是 encounter_areas 的一项。
type areaView struct {
	ID                string          `json:"id"`
	Number            int             `json:"number"`
	Rect              rectView        `json:"rect"`
	Anchor            pointView       `json:"anchor"`
	Available         bool            `json:"available"`
	UnavailableReason *string         `json:"unavailable_reason"`
	MaxEnemies        int             `json:"max_enemies"`
	LevelMin          int             `json:"level_min"`
	LevelMax          int             `json:"level_max"`
	Enemies           []areaEnemyView `json:"enemies"`
	RequiredItems     []int           `json:"required_items"`
	ForbiddenItems    []int           `json:"forbidden_items"`
	Level1Pets        []string        `json:"level1_pets"` // STAGE-286：本区域能遇到的 1 级宠物名（无则 []）
}

// encounterAreasLimitMax 是显式 limit 的上限（STAGE-258）：超过它按它处理。
const encounterAreasLimitMax = 100

// handleEncounterAreas 是 encounter_areas（查看类，协议 5.50）：只读角色当前地图。
//
// STAGE-258 起可选参数 offset、limit（非负整数，缺省 offset=0，缺省 limit=不限）用来分页，
// 响应里的 total 是这张图区域的总数（分页之前）。
func (s *Service) handleEncounterAreas(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Offset *int `json:"offset"`
		Limit  *int `json:"limit"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if (in.Offset != nil && *in.Offset < 0) || (in.Limit != nil && *in.Limit < 0) {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "offset 和 limit 不能是负数")
	}
	live, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := live.With(func(r *record) {
		all := encounterAreaViews(r)
		out = map[string]any{
			"floor":          r.Floor,
			"areas":          pageAreaViews(all, in.Offset, in.Limit),
			"total":          len(all),
			"server_time_ms": s.now().UnixMilli(),
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	return out, nil
}

// pageAreaViews 按 offset/limit 截取区域列表；越界时返回空列表（非 nil，JSON 里是 []）。
func pageAreaViews(all []areaView, offset, limit *int) []areaView {
	start := 0
	if offset != nil {
		start = min(*offset, len(all))
	}
	end := len(all)
	if limit != nil {
		end = min(start+min(*limit, encounterAreasLimitMax), len(all))
	}
	out := make([]areaView, 0, end-start)
	return append(out, all[start:end]...)
}

// encounterAreaViews 列出这张图所有可选的遇敌区域。排序按原编号、再按矩形，保证界面稳定。
func encounterAreaViews(r *record) []areaView {
	held := heldItemSet(r)
	pets := level1PetsOn(r.Floor)
	out := []areaView{}
	for _, a := range gamedata.EncounterAreasOn(r.Floor) {
		v, ok := buildAreaView(r.Floor, a, held)
		if ok {
			// STAGE-286：空列表也要序列化成 []，不能是 null。
			v.Level1Pets = append([]string{}, pets[v.ID]...)
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Number != out[j].Number {
			return out[i].Number < out[j].Number
		}
		a, b := out[i].Rect, out[j].Rect
		if a.X1 != b.X1 {
			return a.X1 < b.X1
		}
		if a.Y1 != b.Y1 {
			return a.Y1 < b.Y1
		}
		if a.X2 != b.X2 {
			return a.X2 < b.X2
		}
		return a.Y2 < b.Y2
	})
	return out
}

// buildAreaView 把一行区域整理成视图；ok 为 false 表示这条不该出现在列表里
// （安全区、被完全覆盖、没有任何有效敌人、找不到代表落点）。
func buildAreaView(floor int, a *gamedata.EncounterArea, held map[int]bool) (areaView, bool) {
	if a.ZOrder <= 0 || a.NeverEncounter() {
		return areaView{}, false
	}
	x1, y1, x2, y2, ok := gamedata.ClampToMap(floor, a.X1, a.Y1, a.X2, a.Y2)
	if !ok {
		return areaView{}, false
	}
	cand := collectAreaEnemies(a, held)
	if len(cand.all) == 0 {
		return areaView{}, false
	}
	x, y, ok := areaAnchor(floor, a, x1, y1, x2, y2)
	if !ok {
		return areaView{}, false
	}
	v := areaView{
		ID: a.Key, Number: a.ID,
		Rect: rectView{X1: x1, Y1: y1, X2: x2, Y2: y2}, Anchor: pointView{X: x, Y: y},
		MaxEnemies: candidateCap(a.MaxEnemies, cand.all),
		Enemies:    cand.usable, RequiredItems: cand.required, ForbiddenItems: cand.forbidden,
		LevelMin: 0, LevelMax: 0,
	}
	// 空切片要序列化成 []，不能是 null：客户端按非空数组解析（协议 5.50 约定）。
	if v.Enemies == nil {
		v.Enemies = []areaEnemyView{}
	}
	if v.RequiredItems == nil {
		v.RequiredItems = []int{}
	}
	if v.ForbiddenItems == nil {
		v.ForbiddenItems = []int{}
	}
	for _, e := range cand.usable {
		if v.LevelMin == 0 || e.LevelMin < v.LevelMin {
			v.LevelMin = e.LevelMin
		}
		if e.LevelMax > v.LevelMax {
			v.LevelMax = e.LevelMax
		}
	}
	if len(cand.usable) == 0 {
		v.Available = false
		v.UnavailableReason = strPtr(cand.blockReason())
		return v, true
	}
	if v.MaxEnemies < 1 {
		v.Available = false
		v.UnavailableReason = strPtr("这一群敌人当前最多只能出 0 只")
		return v, true
	}
	v.Available = true
	return v, true
}

// areaCandidates 是一行区域的候选敌人：all 用于算人数上限（含受道具条件挡住的），
// usable 是当前真正能遇到的，required/forbidden 是被道具条件挡住的线索。
type areaCandidates struct {
	all       []areaEnemyView
	usable    []areaEnemyView
	required  []int
	forbidden []int
}

// blockReason 说明为什么一个敌人都遇不到。
func (c areaCandidates) blockReason() string {
	switch {
	case len(c.required) > 0 && len(c.forbidden) > 0:
		return "需要持有道具且不能持有另一件道具，当前条件都不满足"
	case len(c.required) > 0:
		return "需要持有指定道具才能遇到这里的敌人"
	case len(c.forbidden) > 0:
		return "持有指定道具时遇不到这里的敌人"
	default:
		return "这里的敌人配置当前抽不出可用的敌人"
	}
}

// collectAreaEnemies 展开区域里的敌群与敌人，口径与开战（battle.BuildWild）一致：
// 缺表的敌群剔除、零权重跳过、模板不存在的变体跳过、随机怪组展开成候选。
func collectAreaEnemies(a *gamedata.EncounterArea, held map[int]bool) areaCandidates {
	var c areaCandidates
	for _, ref := range a.Groups {
		if ref.ID == -1 || ref.Weight <= 0 {
			continue
		}
		g, ok := gamedata.EnemyGroupByID(ref.ID)
		if !ok {
			continue
		}
		usableGroup := true
		if g.AppearItem != -1 && !held[g.AppearItem] {
			c.required = appendUnique(c.required, g.AppearItem)
			usableGroup = false
		}
		if g.NotAppearItem != -1 && held[g.NotAppearItem] {
			c.forbidden = appendUnique(c.forbidden, g.NotAppearItem)
			usableGroup = false
		}
		for _, er := range g.Enemies {
			if er.ID == -1 || er.Weight <= 0 {
				continue
			}
			e, ok := gamedata.EnemyByID(er.ID)
			if !ok {
				continue
			}
			weight := ref.Weight * er.Weight
			for _, v := range expandEnemy(e) {
				item := areaEnemyView{
					ID: v.ID, Name: enemyName(v), LevelMin: v.LVMin, LevelMax: v.LVMax,
					CreateMax: v.CreateMax, Weight: weight, GroupID: g.ID,
				}
				c.all = append(c.all, item)
				if usableGroup {
					c.usable = append(c.usable, item)
				}
			}
		}
	}
	return c
}

// expandEnemy 把随机怪组展开成具体候选；普通敌人原样返回。
func expandEnemy(e *gamedata.EnemyVariant) []*gamedata.EnemyVariant {
	if len(e.RandomOf) == 0 {
		return []*gamedata.EnemyVariant{e}
	}
	out := make([]*gamedata.EnemyVariant, 0, len(e.RandomOf))
	for _, id := range e.RandomOf {
		if v, ok := gamedata.EnemyByID(id); ok {
			out = append(out, v)
		}
	}
	return out
}

// enemyName 取模板名（原版战斗里显示的也是模板名）；模板缺失时退回编号。
func enemyName(e *gamedata.EnemyVariant) string {
	if t, ok := gamedata.PetByID(e.Template); ok && t.Name != "" {
		return t.Name
	}
	return "敌人 #" + strconv.Itoa(e.ID)
}

// candidateCap 算一场最多几只：区域上限与候选同场上限之和取小。
// 随机怪组的同场上限按候选里的最大值算（开战时抽到哪个都有可能）。
func candidateCap(areaMax int, all []areaEnemyView) int {
	byID := map[int]int{}
	order := []int{}
	for _, e := range all {
		if _, seen := byID[e.ID]; !seen {
			order = append(order, e.ID)
		}
		if e.CreateMax > byID[e.ID] {
			byID[e.ID] = e.CreateMax
		}
	}
	sum := 0
	for _, id := range order {
		sum += byID[id]
	}
	if sum < 1 {
		return 0
	}
	if areaMax < sum {
		return areaMax
	}
	return sum
}

// areaAnchor 取代表落点：区域内、确实由本区域生效、且不是普通出口触发格，按 x 再 y 升序取第一个。
func areaAnchor(floor int, a *gamedata.EncounterArea, x1, y1, x2, y2 int) (int, int, bool) {
	blocked := exitTriggerCells(floor)
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			if blocked[[2]int{x, y}] {
				continue
			}
			if got := areaAtOn(floor, x, y); got != nil && got.Key == a.Key {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// areaAtOn 是按地图过滤后的 EncounterAreaAt（重叠取最大 zorder，平局取文件靠前）。
func areaAtOn(floor, x, y int) *gamedata.EncounterArea {
	var best *gamedata.EncounterArea
	for _, a := range gamedata.EncounterAreasOn(floor) {
		if a.ZOrder <= 0 || !a.Contains(x, y) {
			continue
		}
		if best == nil || a.ZOrder > best.ZOrder {
			best = a
		}
	}
	return best
}

// exitTriggerCells 收集这张图所有普通出口的触发格（代表落点不能落在上面，否则一落地就被传走）。
func exitTriggerCells(floor int) map[[2]int]bool {
	cells := map[[2]int]bool{}
	for _, e := range gamedata.ExitsOf(floor) {
		for _, c := range e.Cells {
			cells[[2]int{c.X, c.Y}] = true
		}
	}
	return cells
}

func appendUnique(list []int, v int) []int {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func strPtr(s string) *string { return &s }
