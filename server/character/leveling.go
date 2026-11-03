// STAGE-279：练级传送目录（docs/protocol.md 5.70，目录口径见 docs/encounter.md「练级传送口径」）。
// leveling_catalog 按段返回：不带参数只给 13 段摘要；带 segment（段下限）再给这一段的练级点。
// 整目录超过 64 KiB 的线上上限，所以分段取。开战见 leveling_start.go。

package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// levelingSegmentView 是一段的摘要。
type levelingSegmentView struct {
	Min        int `json:"min"`
	Max        int `json:"max"`
	EntryCount int `json:"entry_count"`
	MapCount   int `json:"map_count"`
}

// levelingMonsterView 是练级点里的一种怪物（段内裁切后的等级）。
type levelingMonsterView struct {
	Name     string `json:"name"`
	LevelMin int    `json:"level_min"`
	LevelMax int    `json:"level_max"`
}

// levelingSpotView 是练级点的一个落点。
type levelingSpotView struct {
	AreaID     string    `json:"area_id"`
	Number     int       `json:"number"`
	Floor      int       `json:"floor"`
	MapName    string    `json:"map_name"`
	Anchor     pointView `json:"anchor"`
	Rect       rectView  `json:"rect"`
	MaxEnemies int       `json:"max_enemies"`
	Steps      int       `json:"steps"`
}

// levelingEntryView 是一个练级点。
type levelingEntryView struct {
	Monsters []levelingMonsterView `json:"monsters"`
	Spots    []levelingSpotView    `json:"spots"`
}

// handleLevelingCatalog 是 leveling_catalog（查看类，协议 5.70）：静态目录，与角色无关。
func (s *Service) handleLevelingCatalog(_ context.Context, _ *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Segment *int `json:"segment"` // 指针：缺省表示只要摘要
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	var idx = -1
	if in.Segment != nil {
		i, ok := gamedata.LevelingSegmentOf(*in.Segment)
		if !ok {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "segment 要是练级段的下限：1、11、21、…、121")
		}
		idx = i
	}
	return levelingCatalogView(idx), nil
}

// levelingCatalogView 组装目录响应；idx 为段下标，小于 0 时不带段内容。
func levelingCatalogView(idx int) map[string]any {
	segs := levelingSegmentViews()
	out := map[string]any{
		"catalog_version": gamedata.LevelingCatalogVersion(),
		"segments":        segs,
	}
	if idx >= 0 {
		out["segment"] = segs[idx]
		out["entries"] = levelingEntryViews(idx)
	}
	return out
}

// levelingSegmentViews 给出全部 13 段的摘要（空段也在）。
func levelingSegmentViews() []levelingSegmentView {
	segs := make([]levelingSegmentView, gamedata.LevelingSegmentCount)
	maps := make([]map[int]bool, gamedata.LevelingSegmentCount)
	for i := range segs {
		lo := i*gamedata.LevelingSegmentWidth + 1
		segs[i] = levelingSegmentView{Min: lo, Max: lo + gamedata.LevelingSegmentWidth - 1}
		maps[i] = map[int]bool{}
	}
	for _, e := range gamedata.LevelingEntries() {
		i, _ := gamedata.LevelingSegmentOf(e.Segment)
		segs[i].EntryCount++
		for _, sp := range e.Spots {
			maps[i][sp.Floor] = true
		}
	}
	for i := range segs {
		segs[i].MapCount = len(maps[i])
	}
	return segs
}

// levelingEntryViews 列出一段的练级点；段内顺序就是目录里的顺序（等级升序）。
func levelingEntryViews(idx int) []levelingEntryView {
	out := []levelingEntryView{}
	lo := idx*gamedata.LevelingSegmentWidth + 1
	for _, e := range gamedata.LevelingEntries() {
		if e.Segment != lo {
			continue
		}
		v := levelingEntryView{Monsters: []levelingMonsterView{}, Spots: []levelingSpotView{}}
		for _, m := range e.Monsters {
			v.Monsters = append(v.Monsters, levelingMonsterView{Name: m.Name, LevelMin: m.LevelMin, LevelMax: m.LevelMax})
		}
		for _, sp := range e.Spots {
			v.Spots = append(v.Spots, levelingSpotView{
				AreaID: sp.AreaID, Number: sp.Number, Floor: sp.Floor, MapName: sp.MapName,
				Anchor:     pointView{X: sp.Anchor.X, Y: sp.Anchor.Y},
				Rect:       rectView{X1: sp.Rect.X1, Y1: sp.Rect.Y1, X2: sp.Rect.X2, Y2: sp.Rect.Y2},
				MaxEnemies: sp.MaxEnemies, Steps: sp.Steps,
			})
		}
		out = append(out, v)
	}
	return out
}
