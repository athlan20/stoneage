package gamedata

import (
	"strings"
	"testing"
)

// STAGE-234：玛蕾（718）的对话表：10→79 各站（EVENTRUN，FLOOR 与 PET:LV 对上）都在表里，上限依次 15…79。
func TestPetTalkStations(t *testing.T) {
	tbl, ok := PetTalkOf(718)
	if !ok {
		t.Fatal("718 没有对话表")
	}
	want := map[int]struct{ floor, limit int }{
		10: {1000, 15}, 15: {1400, 20}, 20: {1200, 25}, 25: {5542, 30}, 30: {4000, 35},
		35: {20301, 40}, 40: {3300, 45}, 45: {21201, 50}, 50: {20105, 55}, 55: {6000, 60},
		60: {31901, 65}, 65: {30703, 70}, 70: {31201, 75}, 75: {40, 79},
	}
	for lv, w := range want {
		b, _, ok := PetTalkPick(tbl, lv, lv, w.floor, func(n int) int { return 0 })
		if !ok || b == nil || b.Limit != w.limit || b.Tag != "EVENTRUN" {
			t.Fatalf("等级 %d 楼层 %d：挑出的块 = %+v，要 上限 %d", lv, w.floor, b, w.limit)
		}
	}
}

// 不在站点楼层上，或等级不对，就不改上限（同原版 FLOOR 与 PET:LV 的条件）。
func TestPetTalkNoStationElsewhere(t *testing.T) {
	tbl, _ := PetTalkOf(718)
	zero := func(n int) int { return 0 }
	if b, _, _ := PetTalkPick(tbl, 10, 10, 1, zero); b != nil && b.Limit != 0 {
		t.Fatalf("不在 1000 楼的 10 级玛蕾不该改上限，挑出 %+v", b)
	}
	if b, _, _ := PetTalkPick(tbl, 14, 14, 1400, zero); b != nil && b.Limit != 0 {
		t.Fatalf("14 级不是站点等级，不该改上限，挑出 %+v", b)
	}
}

// 等级低于上限时看 TALKRUN 块。原版 TALKRUN17（PET:LV>0&LV<10）带 LimitLevel:79：对低于 10 级的宠物说话时，
// 有可能把上限改成 79（原版如此，docs/quest.md「宠物对话」说明）。这里断言它确实在候选里。
func TestPetTalkLowPetTalkRunQuirk(t *testing.T) {
	tbl, _ := PetTalkOf(718)
	seen := false
	for k := 0; k < 8; k++ {
		b, _, ok := PetTalkPick(tbl, 5, 10, 1000, func(n int) int { return k % n })
		if ok && b != nil && b.Tag == "TALKRUN" && b.Limit == 79 {
			seen = true
		}
	}
	if !seen {
		t.Fatal("5 级的宠物对话应有 TALKRUN17（上限 79）作为候选")
	}
}

// 没有块命中时说 OTHER 的台词，且不改上限。
func TestPetTalkOtherFallback(t *testing.T) {
	tbl, _ := PetTalkOf(718)
	// 12 级、上限 10：走 EVENTRUN，但没有 12 级的站点块，所以落到 OTHER
	b, text, ok := PetTalkPick(tbl, 12, 10, 1000, func(n int) int { return 0 })
	if !ok || b != nil || text != tbl.Other[0] {
		t.Fatalf("应说 OTHER 台词，得到 block=%v text=%q ok=%v", b, text, ok)
	}
}

// 没有对话表的模板不说话。
func TestPetTalkMissingTable(t *testing.T) {
	if _, ok := PetTalkOf(1); ok {
		t.Fatal("乌力（1）不该有对话表")
	}
}

// STAGE-239：台词里的字面 \n 已在 tools/pettalk.py 转成换行，运行时的台词里不应再有反斜杠加 n。
func TestPetTalkTextHasNoLiteralNewline(t *testing.T) {
	for _, tbl := range pettalkTables {
		for _, b := range tbl.Blocks {
			if strings.Contains(b.Text, `\n`) {
				t.Fatalf("台词里还有字面 \\n：%q", b.Text)
			}
		}
		for _, o := range tbl.Other {
			if strings.Contains(o, `\n`) {
				t.Fatalf("OTHER 台词里还有字面 \\n：%q", o)
			}
		}
	}
}
