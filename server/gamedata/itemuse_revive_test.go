// 复活类道具（STAGE-262 A 段，原版 battle_item.c 的 ITEM_useRessurect 与 ITEM_ResAndDef）的解析与目标类型。
// 用物品表里的真实编号检查，再用合成的参数覆盖百分比、防御关键字和回合数的分支。

package gamedata

import "testing"

func TestReviveItemsOfRealTemplates(t *testing.T) {
	cases := []struct {
		id      int
		power   int
		percent bool
		defend  string
		turns   int
	}{
		{1530, 100, false, "", 0},       // 复活药(100)
		{1531, 300, false, "", 0},       // 复活药(300)
		{1532, 500, false, "", 0},       // 复活药(500)
		{1839, 600, false, "", 0},       // 回魂之玉
		{2450, 500, false, "", 0},       // 萨顿家的秘药
		{1815, 100, false, "absorb", 1}, // 真诚的眼泪：复活 + 吸收 1 回合
		{1821, 500, false, "vanish", 2}, // 铁壁之石：复活 + 无效 2 回合
	}
	for _, c := range cases {
		it, ok := ItemByID(c.id)
		if !ok {
			t.Fatalf("物品 %d 不在表里", c.id)
		}
		eff, usable := it.UseEffect()
		if !usable || !eff.Revive {
			t.Fatalf("物品 %d（%s）应是可用的复活类: %+v usable=%v", c.id, it.Name, eff, usable)
		}
		if eff.RevivePower != c.power || eff.RevivePercent != c.percent {
			t.Errorf("物品 %d：复活量 %d（百分比 %v），want %d（%v）", c.id, eff.RevivePower, eff.RevivePercent, c.power, c.percent)
		}
		if eff.Defend != c.defend || (c.defend != "" && eff.DefendTurns != c.turns) {
			t.Errorf("物品 %d：防御 %q 回合 %d，want %q 回合 %d", c.id, eff.Defend, eff.DefendTurns, c.defend, c.turns)
		}
		if bt := it.BattleTarget(); bt != TargetAllyDown {
			t.Errorf("物品 %d：目标类型 %s，want ally_down", c.id, bt)
		}
	}
}

func TestReviveItemParsing(t *testing.T) {
	cases := []struct {
		name    string
		item    ItemTemplate
		power   int
		percent bool
		defend  string
		turns   int
	}{
		{"百分比", ItemTemplate{UseFunc: "ITEM_useRessurect", Argument: "50%", Target: 101}, 50, true, "", 0},
		{"定值", ItemTemplate{UseFunc: "ITEM_useRessurect", Argument: "600", Target: 101}, 600, false, "", 0},
		{"回满（参数为 0）", ItemTemplate{UseFunc: "ITEM_useRessurect", Argument: "0", Target: 101}, 0, false, "", 0},
		{"防御缺省 3 回合", ItemTemplate{UseFunc: "ITEM_ResAndDef", Argument: "100 反", Target: 101}, 100, false, "reflect", 3},
		{"防御写明回合", ItemTemplate{UseFunc: "ITEM_ResAndDef", Argument: "500 无 turn 2", Target: 101}, 500, false, "vanish", 2},
		{"没有防御关键字", ItemTemplate{UseFunc: "ITEM_ResAndDef", Argument: "100", Target: 101}, 100, false, "", 0},
	}
	for _, c := range cases {
		eff, usable := c.item.UseEffect()
		if !usable || !eff.Revive {
			t.Fatalf("%s：应是可用的复活类: %+v", c.name, eff)
		}
		if eff.RevivePower != c.power || eff.RevivePercent != c.percent || eff.Defend != c.defend ||
			(c.defend != "" && eff.DefendTurns != c.turns) {
			t.Errorf("%s：解析结果 %+v，want 威力 %d 百分比 %v 防御 %q 回合 %d", c.name, eff, c.power, c.percent, c.defend, c.turns)
		}
	}
}
