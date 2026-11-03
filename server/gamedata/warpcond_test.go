package gamedata

import "testing"

func hasNone(int) bool    { return false }
func has2597(id int) bool { return id == 2597 }

func TestPassCheck(t *testing.T) {
	// 候选组 OR：LV>30,ITEM=2597 —— 31 级或持票都行。
	free := "LV>30,ITEM=2597"
	if !passCheck(free, WarpState{Level: 31, Transmigration: 0, HasItem: hasNone}) {
		t.Error("31 级应通过 LV>30")
	}
	if !passCheck(free, WarpState{Level: 1, Transmigration: 0, HasItem: has2597}) {
		t.Error("持票应通过 ITEM=2597")
	}
	if passCheck(free, WarpState{Level: 1, Transmigration: 0, HasItem: hasNone}) {
		t.Error("1 级无票不应通过")
	}
	// 组内 AND：ITEM=2414&LV>79 要同时满足。
	if passCheck("ITEM=2414&LV>79", WarpState{Level: 80, Transmigration: 0, HasItem: hasNone}) {
		t.Error("只满足等级不应通过 AND 组")
	}
	// ALLFREE 与空条件恒真。
	if !passCheck("ALLFREE", WarpState{Level: 1, Transmigration: 0, HasItem: hasNone}) || !passCheck("", WarpState{Level: 1, Transmigration: 0, HasItem: hasNone}) {
		t.Error("ALLFREE 与空条件应恒真")
	}
	// 不支持的标记一律不通过（任务/庄园/家族）。
	for _, bad := range []string{"ENDEV=1002", "NOWEV>0", "MANOR=1", "FM=1"} {
		if passCheck(bad, WarpState{Level: 80, Transmigration: 5, HasItem: has2597}) {
			t.Errorf("%s 不应通过（本期不支持）", bad)
		}
	}
	// 范围写法 LV=30-40。
	if !passCheck("LV=30-40", WarpState{Level: 35, Transmigration: 0, HasItem: hasNone}) || passCheck("LV=30-40", WarpState{Level: 41, Transmigration: 0, HasItem: hasNone}) {
		t.Error("LV=30-40 的范围判定不对")
	}
	// 转生。
	if !passCheck("TRANS>0", WarpState{Level: 1, Transmigration: 1, HasItem: hasNone}) || passCheck("TRANS>0", WarpState{Level: 1, Transmigration: 0, HasItem: hasNone}) {
		t.Error("TRANS 条件判定不对")
	}
}

func TestWarpFee(t *testing.T) {
	if got := (&NpcWarpman{Money: "300"}).Fee(50); got != 300 {
		t.Errorf("固定费用应是 300，实际 %d", got)
	}
	if got := (&NpcWarpman{Money: "LV*10"}).Fee(50); got != 500 {
		t.Errorf("LV*10 应按等级算（500），实际 %d", got)
	}
	if got := (&NpcWarpman{Money: "-1"}).Fee(50); got != -1 {
		t.Errorf("-1 应表示不能付费，实际 %d", got)
	}
	if got := (&NpcWarpman{}).Fee(50); got != 0 {
		t.Errorf("没有 MONEY 时费用应为 0，实际 %d", got)
	}
}

// 任务条件（STAGE-90）：ENDEV 按完成判断（= 已完成，!= 未完成），NOWEV 只有 = 有意义，ITEM!= 是没有这个道具。
func TestPassCheckQuestAndItemNotConditions(t *testing.T) {
	done := map[int]bool{4: true}
	doing := map[int]bool{40: true}
	has := map[int]bool{2609: true}
	st := WarpState{
		Level:   70,
		HasItem: func(id int) bool { return has[id] },
		Done:    func(ev int) bool { return done[ev] },
		Doing:   func(ev int) bool { return doing[ev] },
	}
	for free, want := range map[string]bool{
		"ENDEV=4": true, "ENDEV=5": false, "ENDEV!=4": false, "ENDEV!=5": true,
		"NOWEV=40": true, "NOWEV=41": false, "NOWEV!=40": false,
		"LV>64&ENDEV=4": true, "LV>79&ENDEV=4": false,
		"ENDEV=4&NOWEV=40": true, "ENDEV=5,NOWEV=40": true, "ENDEV=5,NOWEV=41": false,
		"LV>0&ITEM!=2609": false, "LV>0&ITEM!=2610": true, "ITEM=2609": true,
	} {
		if got := passCheck(free, st); got != want {
			t.Errorf("FREE %q：want %v，got %v", free, want, got)
		}
	}
	// 没有任务回调（比如角色没有进度可查）时，任务条件一律不通过。
	if passCheck("ENDEV=4", WarpState{Level: 70}) || passCheck("NOWEV=40", WarpState{Level: 70}) {
		t.Error("没有回调时任务条件不应通过")
	}
}
