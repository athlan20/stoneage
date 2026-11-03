// 攻击精灵的命中范围（STAGE-218）：25 个精灵按原版 attmagic.bin 的掩码取格，对照 docs/magic-range.md 的“改后”列。
// 这里用满员的敌方（两排五列）和后排中间（单位 17：第 1 排第 2 列）做基准，边角裁剪另测一例。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// fullEnemyBattle 造一场人物对 10 只 1 级怪的仗，怪物占满敌方两排五列。
func fullEnemyBattle(t *testing.T) *Battle {
	t.Helper()
	var monsters []*Unit
	for i := 0; i < 10; i++ {
		monsters = append(monsters, &Unit{Kind: KindMonster, Name: "乌力", Level: 1, HP: 100, MaxHP: 100, Attack: 10})
	}
	return New("b_range", "test", testSide(), monsters, func(int) int { return 0 })
}

// 25 个攻击精灵在满员敌方里的命中数（目标选后排中间的单位 17）。
func TestAttackMagicRanges(t *testing.T) {
	b := fullEnemyBattle(t)
	you := b.PlayerUnit()
	cases := []struct {
		id   int
		name string
		want int
	}{
		{301, "岩石撞击", 1}, {302, "落石撞击", 1}, {303, "土石流", 5}, {304, "巨岩撞击", 4},
		{305, "地震", 10}, {306, "陨石冲击", 10}, {307, "水柱", 1}, {308, "水刃", 1},
		{309, "大冰山", 1}, {310, "沸点攻击", 1}, {311, "暴风雪", 5}, {312, "冰天雪地", 10},
		{313, "火球", 1}, {314, "火焰连弹", 1}, {315, "蒸汽喷射", 3}, {316, "火焰风爆", 3},
		{317, "熔岩爆发", 5}, {318, "火山爆发", 10}, {319, "风刃", 1}, {320, "真空刃", 1},
		{321, "龙卷风", 5}, {322, "暴风烈刃", 1}, {323, "暴风冰刃", 5}, {324, "南极飓风", 10},
		{325, "毁天灭地", 10},
	}
	if len(cases) != 25 {
		t.Fatalf("应有 25 个攻击精灵，实际列了 %d 个", len(cases))
	}
	for _, tc := range cases {
		tpl, ok := gamedata.MagicByID(tc.id)
		if !ok {
			t.Fatalf("%s（%d）不在精灵表里", tc.name, tc.id)
		}
		if tpl.Name != tc.name || tpl.Func != gamedata.MagicFuncAttack {
			t.Fatalf("%d 应是攻击精灵 %s，实际 %s（%s）", tc.id, tc.name, tpl.Name, tpl.Func)
		}
		got := b.magicAttackTargets(you, Cmd{Target: 17}, tpl)
		if len(got) != tc.want {
			t.Errorf("%s（%d）：命中 %d 个，应为 %d 个（目标码 %d，掩码 %v）",
				tc.name, tc.id, len(got), tc.want, tpl.Target, tpl.Mask)
		}
		for _, x := range got {
			if x.Side() == you.Side() || !x.Alive() {
				t.Errorf("%s（%d）：命中集合里不该有 %d 号（同方或已倒下）", tc.name, tc.id, x.No)
			}
		}
	}
}

// 边角裁剪：巨岩撞击（十字）选第 1 排第 0 列（单位 15），左、下两边出界，只命中 3 个。
func TestAttackMagicRangeClipsAtEdge(t *testing.T) {
	b := fullEnemyBattle(t)
	tpl, _ := gamedata.MagicByID(304)
	got := b.magicAttackTargets(b.PlayerUnit(), Cmd{Target: 15}, tpl)
	if len(got) != 3 {
		t.Fatalf("巨岩撞击在边角应命中 3 个，实际 %d 个", len(got))
	}
}

// 不用选目标的精灵（目标码 11）忽略 cmd.Target，按掩码取敌方两排：熔岩爆发只打第 1 排 5 个。
func TestAttackMagicAllIgnoresTarget(t *testing.T) {
	b := fullEnemyBattle(t)
	tpl, _ := gamedata.MagicByID(317)
	got := b.magicAttackTargets(b.PlayerUnit(), Cmd{Target: -1}, tpl)
	if len(got) != 5 {
		t.Fatalf("熔岩爆发应命中 5 个，实际 %d 个", len(got))
	}
	for _, x := range got {
		if unitLocalRow(x.No) != 1 {
			t.Errorf("熔岩爆发只该打第 1 排，实际命中 %d 号（第 %d 排）", x.No, unitLocalRow(x.No))
		}
	}
}
