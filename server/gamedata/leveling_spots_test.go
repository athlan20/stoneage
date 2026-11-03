package gamedata

import "testing"

// STAGE-279：练级目录的结构与需求表（13 段、323 条、各段条目数）一致。
func TestLevelingCatalogShape(t *testing.T) {
	want := []int{47, 63, 34, 24, 30, 52, 27, 8, 13, 16, 9, 0, 1} // STAGE-289：121-130 段新增漆黑洞窟20层的 125-130 蓝雷
	got := make([]int, LevelingSegmentCount)
	total := 0
	for _, e := range LevelingEntries() {
		i, ok := LevelingSegmentOf(e.Segment)
		if !ok {
			t.Fatalf("条目段下限 %d 不合法", e.Segment)
		}
		got[i]++
		total++
		if len(e.Spots) == 0 {
			t.Errorf("段 %d 有没有落点的条目", e.Segment)
		}
		lo := e.Segment
		hi := lo + LevelingSegmentWidth - 1
		for _, m := range e.Monsters {
			if m.LevelMin < lo || m.LevelMax > hi || m.LevelMin > m.LevelMax {
				t.Errorf("段 %d 的怪 %s 等级 %d-%d 超出本段", lo, m.Name, m.LevelMin, m.LevelMax)
			}
			if m.LevelMin == 1 && m.LevelMax == 1 {
				t.Errorf("段 %d 出现 Lv1 刷点 %s", lo, m.Name)
			}
		}
	}
	if total != 324 {
		t.Fatalf("练级点 %d 条，应为 324", total)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("段 %d 条目 %d，应为 %d", i*LevelingSegmentWidth+1, got[i], want[i])
		}
	}
}

// 段内顺序：等级升序（同段内前一条的最低等级不大于后一条）。
func TestLevelingEntriesSortedByLevel(t *testing.T) {
	prev := -1
	prevSeg := -1
	for _, e := range LevelingEntries() {
		if e.Segment != prevSeg {
			prev, prevSeg = -1, e.Segment
		}
		lo := e.Monsters[0].LevelMin
		for _, m := range e.Monsters {
			if m.LevelMin < lo {
				lo = m.LevelMin
			}
		}
		if lo < prev {
			t.Fatalf("段 %d 的条目等级顺序乱了：%d 排在 %d 后面", e.Segment, lo, prev)
		}
		prev = lo
	}
}

func TestLevelingSpotForTypicalArea(t *testing.T) {
	s, ok := LevelingSpotFor("200-189-521-435-527-454")
	if !ok {
		t.Fatal("200-189 应在练级目录里")
	}
	if s.Floor != 200 || s.Anchor != (LevelingPoint{X: 521, Y: 435}) || s.Number != 189 {
		t.Fatalf("200-189 的落点不对：%+v", s)
	}
	if _, ok := LevelingSpotFor("no-such-area"); ok {
		t.Fatal("不存在的区域不应查到")
	}
}

func TestLevelingSegmentOf(t *testing.T) {
	for _, c := range []struct {
		min int
		ok  bool
		idx int
	}{{1, true, 0}, {11, true, 1}, {121, true, 12}, {0, false, 0}, {5, false, 0}, {131, false, 0}, {-9, false, 0}} {
		idx, ok := LevelingSegmentOf(c.min)
		if ok != c.ok || (ok && idx != c.idx) {
			t.Errorf("LevelingSegmentOf(%d) = %d,%v", c.min, idx, ok)
		}
	}
}
