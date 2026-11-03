package character

import (
	"context"
	"testing"
)

// 课程页的窗口号：110 是初级班，每级加 10，不是确认页的号返回 false（STAGE-122）。
func TestCourseWin(t *testing.T) {
	for i, want := range []int{110, 120, 130, 140} {
		idx, ok := courseWinOf(want)
		if !ok || idx != i {
			t.Fatalf("courseWinOf(%d) = %d, %v；want %d, true", want, idx, ok, i)
		}
	}
	for _, no := range []int{1, 100, 115, 150, riderMsgWin} {
		if _, ok := courseWinOf(no); ok {
			t.Fatalf("courseWinOf(%d) 不应该是确认页", no)
		}
	}
}

// 学课程的检查（docs/ride.md §2）：学过了说“不需要再学了”，没学前一级说“必须先学过”，都没有才能学。
func TestRiderCourseCheck(t *testing.T) {
	cases := []struct {
		level, idx int
		want       string
	}{
		{0, 0, ""}, // 新角色学初级班
		{40, 0, "你已经学会初级班了呀。\n不需要再学了。"}, // 学过初级班
		{0, 1, "你必须先学过初级班才能来中级班唷。"},     // 没学初级班就来中级班
		{40, 1, ""},  // 初级之后学中级班
		{80, 2, ""},  // 中级之后学高级班
		{120, 3, ""}, // 高级之后学特级班
		{200, 3, "你已经学会特级班了呀。\n不需要再学了。"},
		{80, 0, "你已经学会初级班了呀。\n不需要再学了。"}, // 等级已经超过的课程也算学过
	}
	for _, c := range cases {
		if got := riderCourseCheck(c.level, c.idx); got != c.want {
			t.Errorf("riderCourseCheck(等级 %d, 课程 %d) = %q，want %q", c.level, c.idx, got, c.want)
		}
	}
}

// 查询页按外形列出虎和雷龙（外形 3/2 是金格萨贝鲁和布拉奇多斯）。
func TestRiderQueryText(t *testing.T) {
	r := &record{Appearance: Appearance{Body: 3, Color: 2}}
	want := "你的外形能骑的宠物：\n虎：金格萨贝鲁\n雷龙：布拉奇多斯"
	if got := riderQueryText(r); got != want {
		t.Fatalf("riderQueryText = %q，want %q", got, want)
	}
}

// 主菜单的页面带选项，消息页没有选项，确认页是“是/否”（docs/protocol.md 5.49）。
func TestRenderRiderPage(t *testing.T) {
	r := &record{Appearance: Appearance{Body: 3, Color: 2}}
	main, ok := renderRiderPage(r, riderMainWin)
	if !ok || len(main.options) != 3 || main.targets[2] != 100 {
		t.Fatalf("主菜单 = %+v", main)
	}
	confirm, ok := renderRiderPage(r, 120)
	if !ok || confirm.text != "学习骑乘中级班不需要学费。你确定要学习吗？" ||
		len(confirm.options) != 2 || confirm.targets[0] != riderLearnTo || confirm.targets[1] != riderCloseTo {
		t.Fatalf("中级班确认页 = %+v", confirm)
	}
	if _, ok := renderRiderPage(r, riderMsgWin); ok {
		t.Fatal("消息页没有窗口定义，不应该能渲染")
	}
}

// 管理命令只接受 0/40/80/120/200：别的等级在碰数据库前就拒绝（db 为 nil 也不会 panic）。
func TestSetRideLevelRejectsOtherLevels(t *testing.T) {
	for _, level := range []int{-1, 1, 39, 100, 201} {
		if err := SetRideLevel(context.Background(), nil, "ch", "勇者", level); err == nil {
			t.Errorf("SetRideLevel(%d) 应该被拒绝", level)
		}
	}
}
