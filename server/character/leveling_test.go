package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-279：练级目录的每个落点都要过服务端真正的 buildAreaView（落点、矩形、同场上限、怪物都对得上）。
func TestLevelingSpotsMatchServerAreaView(t *testing.T) {
	for _, e := range gamedata.LevelingEntries() {
		for _, sp := range e.Spots {
			area, ok := gamedata.EncounterAreaByKey(sp.AreaID)
			if !ok {
				t.Fatalf("区域 %s 不在遇敌表里", sp.AreaID)
			}
			view, ok := buildAreaView(sp.Floor, area, map[int]bool{})
			if !ok || !view.Available {
				t.Fatalf("区域 %s 服务端判定不可用", sp.AreaID)
			}
			if view.Anchor != (pointView{X: sp.Anchor.X, Y: sp.Anchor.Y}) {
				t.Errorf("区域 %s 锚点 %+v，目录是 %+v", sp.AreaID, view.Anchor, sp.Anchor)
			}
			if view.Rect != (rectView{X1: sp.Rect.X1, Y1: sp.Rect.Y1, X2: sp.Rect.X2, Y2: sp.Rect.Y2}) {
				t.Errorf("区域 %s 矩形 %+v，目录是 %+v", sp.AreaID, view.Rect, sp.Rect)
			}
			if view.MaxEnemies != sp.MaxEnemies {
				t.Errorf("区域 %s 同场上限 %d，目录是 %d", sp.AreaID, view.MaxEnemies, sp.MaxEnemies)
			}
			for _, m := range e.Monsters {
				if !viewHasMonster(view, m.Name, m.LevelMin, m.LevelMax) {
					t.Errorf("区域 %s 服务端没有怪 %s %d-%d", sp.AreaID, m.Name, m.LevelMin, m.LevelMax)
				}
			}
		}
	}
}

// 落点数和怪物列表的几个典型条目（口径核对用）。
func TestLevelingTypicalEntries(t *testing.T) {
	var found bool
	for _, e := range gamedata.LevelingEntries() {
		if e.Segment != 11 || len(e.Monsters) != 3 || !hasMonsterName(e.Monsters, "乌卡鲁") {
			continue
		}
		found = true
		for _, m := range e.Monsters {
			if m.LevelMin != 11 || m.LevelMax != 12 {
				t.Errorf("%s 段内等级 %d-%d，应为 11-12", m.Name, m.LevelMin, m.LevelMax)
			}
		}
		if len(e.Spots) != 14 {
			t.Errorf("乌卡鲁/乌宝宝/威威 11-12 的落点 %d 个，应为 14", len(e.Spots))
		}
	}
	if !found {
		t.Fatal("没找到 11-20 段的乌卡鲁/乌宝宝/威威条目")
	}
}

func hasMonsterName(ms []gamedata.LevelingMonster, name string) bool {
	for _, m := range ms {
		if m.Name == name {
			return true
		}
	}
	return false
}

func viewHasMonster(view areaView, name string, lo, hi int) bool {
	for _, en := range view.Enemies {
		if en.Name == name && en.LevelMin <= hi && en.LevelMax >= lo {
			return true
		}
	}
	return false
}

// 段摘要：13 段、条目数与 map_count 之和一致，空段保留。
func TestLevelingSegmentViews(t *testing.T) {
	segs := levelingSegmentViews()
	if len(segs) != 13 {
		t.Fatalf("段数 %d，应为 13", len(segs))
	}
	if segs[0].Min != 1 || segs[0].Max != 10 || segs[12].Min != 121 || segs[12].Max != 130 {
		t.Fatalf("段边界不对：%+v … %+v", segs[0], segs[12])
	}
	// 111-120 段仍然为空；121-130 段自 STAGE-289 起有 1 条（漆黑洞窟20层的 125-130 蓝雷）。
	if segs[11].EntryCount != 0 || segs[11].MapCount != 0 || segs[12].EntryCount != 1 || segs[12].MapCount != 1 {
		t.Fatalf("段条目数不对（111-120 应为空、121-130 应为 1 条）：%+v %+v", segs[11], segs[12])
	}
	if segs[0].EntryCount != 47 || segs[10].EntryCount != 9 {
		t.Fatalf("段条目数不对：%+v %+v", segs[0], segs[10])
	}
}
