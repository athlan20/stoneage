package gamedata

// STAGE-279：练级传送目录（docs/encounter.md「练级传送口径」、docs/map.md）。
// 数据由 tools/gen-leveling-spots.py 生成到 leveling_spots_gen.go，这里只提供类型与查询。

import "sync"

// LevelingSegmentWidth 是练级分段的宽度（10 级一段）；分段下限为 1、11、…、121。
const LevelingSegmentWidth = 10

// LevelingSegmentCount 是练级分段的段数（1–10 到 121–130）。
const LevelingSegmentCount = 13

// LevelingPoint 是落点坐标。
type LevelingPoint struct{ X, Y int }

// LevelingRect 是区域矩形（已裁到地图边界）。
type LevelingRect struct{ X1, Y1, X2, Y2 int }

// LevelingMonster 是练级点里的一种怪物：模板名 + 段内裁切后的等级区间。
type LevelingMonster struct {
	Name     string
	LevelMin int
	LevelMax int
}

// LevelingSpot 是练级点的一个落点（一个遇敌区域）。
type LevelingSpot struct {
	AreaID     string
	Number     int
	Floor      int
	MapName    string
	Anchor     LevelingPoint
	Rect       LevelingRect
	MaxEnemies int
	Steps      int
}

// LevelingEntry 是一个练级点：同段内怪物组合相同的区域合并在一起。
type LevelingEntry struct {
	Segment  int // 段下限（1、11、…、121）
	Monsters []LevelingMonster
	Spots    []LevelingSpot
}

var (
	levelingIndexOnce sync.Once
	levelingByArea    map[string]LevelingSpot
)

// LevelingCatalogVersion 是练级目录的数据指纹。
func LevelingCatalogVersion() string { return levelingSpotVersion }

// LevelingEntries 返回全部练级点（按段、段内等级升序）。调用方不要修改返回的切片。
func LevelingEntries() []LevelingEntry { return levelingSpotRows }

// LevelingSegmentOf 返回段下限（1、11、…）所在的段下标；不是合法段下限时返回 false。
func LevelingSegmentOf(min int) (int, bool) {
	if min < 1 || min > LevelingSegmentWidth*LevelingSegmentCount || (min-1)%LevelingSegmentWidth != 0 {
		return 0, false
	}
	return (min - 1) / LevelingSegmentWidth, true
}

// LevelingSpotFor 按区域 ID 查练级目录里的落点（同一区域在各段的条目里落点相同，取第一次出现的）。
func LevelingSpotFor(areaID string) (LevelingSpot, bool) {
	levelingIndexOnce.Do(buildLevelingIndex)
	s, ok := levelingByArea[areaID]
	return s, ok
}

func buildLevelingIndex() {
	levelingByArea = map[string]LevelingSpot{}
	for _, e := range levelingSpotRows {
		for _, s := range e.Spots {
			if _, dup := levelingByArea[s.AreaID]; !dup {
				levelingByArea[s.AreaID] = s
			}
		}
	}
}
