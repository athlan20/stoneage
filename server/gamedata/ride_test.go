package gamedata

import (
	"testing"
)

// 骑乘表（docs/ride.md §3 第 10 条）：红行的虎是格鲁西斯，外形 3/2 落在黄行，虎是金格萨贝鲁。
func TestRidePairByBody(t *testing.T) {
	tiger, dragon, ok := RidePair(3, 2) // 图像 100070：黄行第 4 列
	if !ok || tiger != 194 || dragon != 253 {
		t.Fatalf("RidePair(3,2) = %d, %d, %v；want 194（金格萨贝鲁）, 253（布拉奇多斯）, true", tiger, dragon, ok)
	}
	if tiger, _, ok := RidePair(0, 0); !ok || tiger != 193 { // 图像 100000：红行第 1 列
		t.Fatalf("RidePair(0,0) 虎 = %d, %v；want 193（格鲁西斯）", tiger, ok)
	}
	if _, _, ok := RidePair(-1, 0); ok {
		t.Fatal("外形越界时不应该有骑乘表")
	}
}

// 每个外形都在表里（12 外形 × 4 颜色 = 48 种），每种都有一只虎和一只雷龙。
func TestRidePairCoversAllAppearances(t *testing.T) {
	for body := 0; body < BodyCount; body++ {
		for color := 0; color < ColorCount; color++ {
			if _, _, ok := RidePair(body, color); !ok {
				t.Fatalf("外形 %d/%d 不在骑乘表里", body, color)
			}
		}
	}
}

func TestRideAllowed(t *testing.T) {
	if !RideAllowed(3, 2, 194) || !RideAllowed(3, 2, 253) {
		t.Fatal("外形 3/2 应该能骑金格萨贝鲁和布拉奇多斯")
	}
	if RideAllowed(3, 2, 193) { // 格鲁西斯是红行的虎
		t.Fatal("外形 3/2 不应该能骑红行的格鲁西斯")
	}
	if RideAllowed(3, 2, 2) { // 乌力乌力是出生地宠物，不在骑乘表里
		t.Fatal("普通宠物不在骑乘表里")
	}
}

// 新角色出生配套宠的老规则不变：出生地宠物 + 配套虎 + 配套雷龙（STAGE-62）。
func TestNewPlayerPetIDsUsesRidePair(t *testing.T) {
	got := NewPlayerPetIDs(0, 3, 2)
	if len(got) != 3 || got[0] != 2 || got[1] != 194 || got[2] != 253 {
		t.Fatalf("NewPlayerPetIDs = %v，want [2 194 253]", got)
	}
}

func TestRideCourses(t *testing.T) {
	want := []int{40, 80, 120, 200}
	for i, c := range RideCourses {
		if c.Level != want[i] {
			t.Fatalf("课程 %d 的等级 = %d，want %d", i, c.Level, want[i])
		}
	}
	if i, ok := RideCourseIndex(120); !ok || i != 2 {
		t.Fatalf("RideCourseIndex(120) = %d, %v；want 2, true", i, ok)
	}
	if _, ok := RideCourseIndex(0); ok {
		t.Fatal("0 不是课程等级")
	}
}

// 训练师的窗口数据（npc-riderman.json）：主菜单、课程、确认页、两个说明页和查询的跳转都能走通。
func TestRidermanWindowsReachable(t *testing.T) {
	main, ok := RidermanWindowOf(1)
	if !ok || len(main.Options) != 3 {
		t.Fatalf("主菜单 = %+v，want 3 个选项", main)
	}
	if main.Options[2].GoTo != 100 {
		t.Fatalf("学习骑乘宠物应去课程页 100，实际 %d", main.Options[2].GoTo)
	}
	course, ok := RidermanWindowOf(100)
	if !ok || len(course.Options) != 4 {
		t.Fatalf("课程页 = %+v，want 4 个课程", course)
	}
	for i, o := range course.Options {
		if o.GoTo != 110+10*i {
			t.Fatalf("课程 %d 去 %d，want %d", i, o.GoTo, 110+10*i)
		}
	}
	for _, no := range []int{2, 1000, 2000} {
		if _, ok := RidermanWindowOf(no); !ok {
			t.Fatalf("窗口 %d 不在数据里", no)
		}
	}
	if _, ok := RidermanWindowOf(1010); ok {
		t.Fatal("家族成立的窗口走不到，不应该收录")
	}
}

func TestRideTexts(t *testing.T) {
	basic := RideCourses[0]
	if got := RideLearnedText(basic); got != "你已经学会初级班了呀。\n不需要再学了。" {
		t.Fatalf("学会的台词 = %q", got)
	}
	if got := RidePrereqText(RideCourses[0], RideCourses[1]); got != "你必须先学过初级班才能来中级班唷。" {
		t.Fatalf("没学前一级的台词 = %q", got)
	}
	if got := RideConfirmText(basic); got != "学习骑乘初级班不需要学费。你确定要学习吗？" {
		t.Fatalf("确认页 = %q", got)
	}
	if got := RideLearnedNotice(RideCourses[3]); got != "恭喜你！你已经完成骑乘特级班了。\n可以骑乘所有等级的宠物，\n请小心骑乘喔！" {
		t.Fatalf("特级班的恭喜话 = %q", got)
	}
}
