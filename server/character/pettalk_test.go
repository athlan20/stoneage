package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 玛蕾（718）模板上限 10，个体上限没改过（NULL）时用模板的。
func TestPetLimitTemplateDefault(t *testing.T) {
	tmpl, _ := gamedata.PetByID(718)
	p := petRec{TemplateID: 718, Level: 10}
	if got := petLimit(p, tmpl); got != 10 {
		t.Fatalf("没改过的玛蕾上限 = %d，要 10（模板值）", got)
	}
	v := 79
	p.LimitLevel = &v
	if got := petLimit(p, tmpl); got != 79 {
		t.Fatalf("个体上限 79 时有效上限 = %d", got)
	}
}

// 不对话则不升：上限是 10 时，给再多经验也停在 10，经验清零。
func TestPetNoTalkStaysAtLimit(t *testing.T) {
	zero := func(n int) int { return 0 }
	p := petRec{TemplateID: 718, Level: 10}
	gainPetExp(&p, 1e12, zero)
	if p.Level != 10 || p.Exp != 0 {
		t.Fatalf("不对话的玛蕾应停在 10 级、经验清零，得到 %d 级、经验 %d", p.Level, p.Exp)
	}
}

// 逐站对话：10、15…75 各对话一次，上限依次 15…79；每次对话后经验能把宠物升到新上限，不会多升；
// 75 站以后不对话就停在 79，不升到 80。
func TestPetTalkChainToMaxLevel(t *testing.T) {
	zero := func(n int) int { return 0 }
	stations := []struct{ level, floor, limit int }{
		{10, 1000, 15}, {15, 1400, 20}, {20, 1200, 25}, {25, 5542, 30}, {30, 4000, 35},
		{35, 20301, 40}, {40, 3300, 45}, {45, 21201, 50}, {50, 20105, 55}, {55, 6000, 60},
		{60, 31901, 65}, {65, 30703, 70}, {70, 31201, 75}, {75, 40, 79},
	}
	p := petRec{TemplateID: 718, Level: 10}
	for _, s := range stations {
		if got := gainPetExp(&p, 1e12, zero); got != s.level {
			t.Fatalf("要升到上限前停在 %d 级，得到 %d 级", s.level, got)
		}
		text, changed := talkPet(&p, s.floor, zero, func([]int) bool { return true })
		if !changed || text == "" || p.LimitLevel == nil || *p.LimitLevel != s.limit {
			t.Fatalf("%d 级在 %d 楼对话：changed=%v 上限=%v，要 %d", s.level, s.floor, changed, p.LimitLevel, s.limit)
		}
		if got := gainPetExp(&p, 1e12, zero); got != s.limit {
			t.Fatalf("对话后上限 %d，宠物应升到 %d，得到 %d", s.limit, s.limit, got)
		}
	}
	if p.Level != 79 {
		t.Fatalf("全部对话后应停在 79 级，得到 %d", p.Level)
	}
	// 不再对话：79 级以后不升到 80
	gainPetExp(&p, 1e12, zero)
	if p.Level != 79 {
		t.Fatalf("没有再对话时不该升到 80，得到 %d", p.Level)
	}
}

// 在错误楼层对话不改上限；AddItem 的道具发不出去（栏位满）时整块失败，说“．．．．！”，不改上限。
func TestPetTalkWrongFloorAndFullBag(t *testing.T) {
	zero := func(n int) int { return 0 }
	p := petRec{TemplateID: 718, Level: 10}
	if _, changed := talkPet(&p, 1, zero, nil); changed || p.LimitLevel != nil {
		t.Fatal("在 1 楼对话不该改上限")
	}
	p = petRec{TemplateID: 718, Level: 65}
	text, changed := talkPet(&p, 30703, zero, func([]int) bool { return false })
	if changed || text != petTalkBusy || p.LimitLevel != nil {
		t.Fatalf("栏位满时应整块失败：text=%q changed=%v 上限=%v", text, changed, p.LimitLevel)
	}
	var got []int
	text, changed = talkPet(&p, 30703, zero, func(items []int) bool { got = items; return true })
	if !changed || text == petTalkBusy || p.LimitLevel == nil || *p.LimitLevel != 70 {
		t.Fatalf("道具给得出时应改上限到 70：text=%q changed=%v", text, changed)
	}
	if len(got) != 3 || got[0] != 19688 {
		t.Fatalf("AddItem 应给 3 件 19688，得到 %v", got)
	}
}
