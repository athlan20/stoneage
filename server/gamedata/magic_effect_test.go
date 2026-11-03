package gamedata

import "testing"

// 效果文字按协议 5.8 的写法生成（STAGE-116）：每种函数各抽一条表里的精灵对照。
func TestMagicEffectText(t *testing.T) {
	cases := []struct {
		id   int
		want string
	}{
		{301, "攻击敌方一个，威力 100，地属性"},
		{302, "攻击敌方一个，威力 150，地属性"},
		{303, "攻击敌方一排，威力 165，地属性"},
		{304, "攻击敌方十字范围，威力 185，地属性"},
		{305, "攻击敌方全体，威力 200，地属性"},
		{317, "攻击敌方一排，威力 200，火属性"},
		{0, "回复自己 HP 80"},
		{20, "回复己方全体 HP 50"},
		{130, "使敌方一个中毒，持续 3 回合"},
		{70, "解除己方一个的中毒"},
		{60, "解除己方一个的一种异常状态"},
		{50, "复活己方一个倒下的单位，HP 约 100"},
		{350, "变身成自己的一只宠物，持续 1 分钟"}, // STAGE-203
		{410, "变身成自己的一只宠物，持续 10 分钟"},
		{355, "变身成自己的一只宠物，持续 1 天"},
	}
	magicOnce.Do(loadMagic)
	for _, c := range cases {
		tpl, ok := MagicByID(c.id)
		if !ok {
			t.Fatalf("精灵 %d 不在表里", c.id)
		}
		if got := tpl.Effect(); got != c.want {
			t.Errorf("精灵 %d（%s）效果 = %q，want %q", c.id, tpl.Name, got, c.want)
		}
	}
}

// 表里没有出现的组合（全体攻击、无属性攻击、治疗他人单体、解状态的未知状态）也要有稳定的文字。
func TestMagicEffectTextVariants(t *testing.T) {
	cases := []struct {
		name string
		tpl  MagicTemplate
		want string
	}{
		{"全体攻击", MagicTemplate{Func: MagicFuncAttack, Elem: "火", Power: 60, All: true}, "攻击敌方全体，威力 60，火属性"},
		{"无属性攻击", MagicTemplate{Func: MagicFuncAttack, Elem: "-", Power: 60}, "攻击敌方一个，威力 60"},
		{"治疗他人单体", MagicTemplate{Func: MagicFuncHealOther, Power: 160}, "回复己方一个 HP 160"},
		{"治疗他人全体", MagicTemplate{Func: MagicFuncHealOther, Power: 160, All: true}, "回复己方全体 HP 160"},
		{"全体状态", MagicTemplate{Func: MagicFuncStatus, Status: "sleep", Turns: 2, All: true}, "使敌方全体睡眠，持续 2 回合"},
		{"未知状态", MagicTemplate{Func: MagicFuncStatus, Status: "deep_poison", Turns: 1}, "使敌方一个陷入异常状态，持续 1 回合"},
		{"魔障", MagicTemplate{Func: MagicFuncStatus, Status: "barrier", Turns: 1, All: true}, "使敌方全体魔障，持续 1 回合"},
		{"沉默解除", MagicTemplate{Func: MagicFuncCure, Status: "nocast"}, "解除己方一个的沉默"},
		{"全体解状态", MagicTemplate{Func: MagicFuncCure, Status: "all", All: true}, "解除己方全体各一种异常状态"},
		{"解“全”", MagicTemplate{Func: MagicFuncCure, Status: "all"}, "解除己方一个的一种异常状态"},
		{"未知函数", MagicTemplate{Func: "none"}, ""},
	}
	for _, c := range cases {
		if got := c.tpl.Effect(); got != c.want {
			t.Errorf("%s：效果 = %q，want %q", c.name, got, c.want)
		}
	}
}

// 召唤龙阵与精灵版宠物技能的效果文字（STAGE-202，协议 5.8）。
func TestMagicEffectTextSummon(t *testing.T) {
	cases := []struct {
		id   int
		want string
	}{
		{394, "召唤龙攻击敌方一个，威力 300"},
		{395, "召唤龙攻击敌方全体，威力 300"}, // 强恩阵：实际等同全体
		{396, "召唤龙攻击敌方全体，威力 400"},
		{326, "使用宠物技能 破除防御"},
		{330, "使用宠物技能 不防守战法"},
	}
	magicOnce.Do(loadMagic)
	for _, c := range cases {
		tpl, ok := MagicByID(c.id)
		if !ok {
			t.Fatalf("精灵 %d 不在表里", c.id)
		}
		if got := tpl.Effect(); got != c.want {
			t.Errorf("精灵 %d（%s）效果 = %q，want %q", c.id, tpl.Name, got, c.want)
		}
	}
}
