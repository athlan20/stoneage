// 战斗奖励的单元测试（STAGE-60）：等级差经验、金币上限、掉落分配与武器类别。
// 期望值按 gmsv/battle/battle.c（EXPGET_MAXLEVEL 5、EXPGET_DIV 15、_BATTLE_GOLD 分支、
// 战利品 getitem[3] 的分配）手算。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

func TestLevelDiffExp(t *testing.T) {
	cases := []struct {
		player, enemy int
		exp, want     int64
	}{
		{10, 10, 100, 100}, // 等级相同：全额
		{15, 10, 100, 100}, // 高 5 级以内：全额（b_level <= 5）
		{16, 10, 100, 93},  // 高 6 级：factor = 5+15-6 = 14，100*14/15 = 93
		{20, 10, 100, 66},  // 高 10 级：factor = 10，100*10/15 = 66
		{25, 10, 100, 33},  // 高 15 级：factor = 5，100*5/15 = 33
		{30, 10, 100, 1},   // 高 20 级：factor <= 0，固定 1
		{21, 10, 1, 1},     // 经验太少：下限 1
	}
	for _, c := range cases {
		if got := levelDiffExp(c.player, c.enemy, c.exp); got != c.want {
			t.Errorf("levelDiffExp(人物 %d, 敌人 %d, 经验 %d) = %d，期望 %d", c.player, c.enemy, c.exp, got, c.want)
		}
	}
}

func TestBattleGoldGain(t *testing.T) {
	const max = 10000000
	if MaxHaveGold != max {
		t.Fatalf("持有上限应统一为 1000 万，得 %d", MaxHaveGold)
	}
	cases := []struct {
		name       string
		level      int
		gold, want int64
	}{
		{"普通增加：等级 × 10", 10, 0, 100},
		{"补到 1000 万", 10, max - 50, 50},
		{"已经到上限：不加", 10, max, 0},
		{"超过上限：扣到上限（原版直接设为 MaxHaveGold）", 10, max + 100, -100},
	}
	for _, c := range cases {
		if got := battleGoldGain(c.level, c.gold); got != c.want {
			t.Errorf("%s: battleGoldGain(%d, %d) = %d，期望 %d", c.name, c.level, c.gold, got, c.want)
		}
	}
	if gamedata.BattleGold != 10 {
		t.Errorf("BATTLEGOLD 应是 10，得 %d", gamedata.BattleGold)
	}
}

func TestDistributeDropsKeepsThreeAndReplaces(t *testing.T) {
	slain := []*Unit{
		{Drops: []int{1, 2}},
		{Drops: []int{3, 4}},
		{Drops: []int{5}},
	}
	// 前三件直接入槽。第 4 件：RAND(0,1) 出 1 → RAND(0,2) 出 0，换掉槽 0。
	// 第 5 件同理再换槽 0。rnd 对 n==2 出 1、n==3 出 0。
	rnd := func(n int) int {
		if n == 2 {
			return 1
		}
		return 0
	}
	got := distributeDrops(slain, rnd)
	if want := []int{5, 2, 3}; !equalInts(got, want) {
		t.Fatalf("掉落分配 = %v，期望 %v", got, want)
	}
}

func TestDistributeDropsNoReplaceWhenRollZero(t *testing.T) {
	slain := []*Unit{{Drops: []int{1, 2, 3, 4}}}
	// RAND(0,1) 出 0：第 4 件直接丢弃（原版 ITEM_endExistItemsOne）。
	got := distributeDrops(slain, func(int) int { return 0 })
	if want := []int{1, 2, 3}; !equalInts(got, want) {
		t.Fatalf("掉落分配 = %v，期望 %v", got, want)
	}
}

func TestWeaponClassOf(t *testing.T) {
	cases := map[int]weaponClass{
		gamedata.ItemTypeFist:      weaponClaw,
		gamedata.ItemTypeAxe:       weaponAxe,
		gamedata.ItemTypeClub:      weaponClub,
		gamedata.ItemTypeBow:       weaponBow,
		gamedata.ItemTypeBoomerang: weaponThrou,
		-1:                         weaponNone,
	}
	for typ, want := range cases {
		if got := weaponClassOf(typ); got != want {
			t.Errorf("weaponClassOf(%d) = %d，期望 %d", typ, got, want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
