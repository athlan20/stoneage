package gamedata

import "testing"

// 精灵表按 magic.txt 的函数收录：攻击、自愈、治疗他人（STAGE-48），状态、解状态、复活（STAGE-77）。
// 数量与 tools/gen-magic.py 的输出一致，抽查的几条对照原版 magic.txt 的参数。
func TestMagicTableFunctions(t *testing.T) {
	magicOnce.Do(loadMagic)
	count := map[string]int{}
	for _, m := range magicTab {
		count[m.Func]++
	}
	want := map[string]int{
		MagicFuncAttack: 25, MagicFuncHeal: 13, MagicFuncHealOther: 6,
		MagicFuncStatus: 59, MagicFuncCure: 23, MagicFuncRevive: 4,
		MagicFuncField: 21, MagicFuncReverse: 2, // STAGE-200：大地/水/火炎/疾风 Lv1–5，调和；彩虹、极光
		MagicFuncDefend: 3, MagicFuncReviveDefend: 3, MagicFuncMetamo: 5,
		MagicFuncDragon: 4, MagicFuncAttSkill: 5, // STAGE-202：奔龙阵、强恩阵、奔雷阵、GM 烟火；五个精灵版宠物技能
	}
	for fn, n := range want {
		if count[fn] != n {
			t.Fatalf("%s 应有 %d 个，实际 %d", fn, n, count[fn])
		}
	}
}

func TestMagicTableStatusAndRevive(t *testing.T) {
	cases := []struct {
		id   int
		want MagicTemplate
	}{
		// 猛毒的精灵 Lv1：MAGIC_StatusChange，毒 turn 3 成 20，单人。
		{130, MagicTemplate{Func: MagicFuncStatus, Status: "poison", Turns: 3, Success: 20, MP: 4}},
		// 毒雾的精灵 Lv6：毒 turn 6 成 25，一方全体。
		{412, MagicTemplate{Func: MagicFuncStatus, Status: "poison", Turns: 6, Success: 25, All: true, MP: 14}},
		// 高等净化精灵Lv1：解“全”，单人。
		{60, MagicTemplate{Func: MagicFuncCure, Status: "all", MP: 4}},
		// 破除沉默：解沉默，一方全体。
		{411, MagicTemplate{Func: MagicFuncCure, Status: "nocast", All: true, MP: 4}},
		// STAGE-201：虚弱（单人）、全体虚弱（敌方全体），挂上 3 回合（引擎里挂上时 +1，见 battle/magic_debuff.go）。
		{390, MagicTemplate{Func: MagicFuncStatus, Status: "weaken", Turns: 3, Success: 50, MP: 4}},
		{391, MagicTemplate{Func: MagicFuncStatus, Status: "weaken", Turns: 3, Success: 50, All: true, MP: 4}},
		// 魔障：敌方全体，1 回合（挂上 +1）；沉默：敌方全体，3 回合。原版目标编号 3（MAGIC_TARGET_ALLOTHERSIDE）。
		{397, MagicTemplate{Func: MagicFuncStatus, Status: "barrier", Turns: 1, Success: 50, All: true, MP: 4}},
		{398, MagicTemplate{Func: MagicFuncStatus, Status: "nocast", Turns: 3, Success: 50, All: true, MP: 4}},
		// 复活精灵 Lv1：气绝状态回复 100，MP 按威力推算。
		{50, MagicTemplate{Func: MagicFuncRevive, Power: 100, MP: 8}},
		{53, MagicTemplate{Func: MagicFuncRevive, Power: 700, MP: 44}},
		// STAGE-199：光的精灵（吸收 1 次，MP 8）、守护的精灵（无效 2 次，MP 12）、复活守护精灵 Lv1（复活 100 + 无效 2 次，MP 16）。
		{250, MagicTemplate{Func: MagicFuncDefend, Status: "absorb", Turns: 1, MP: 8}},
		{270, MagicTemplate{Func: MagicFuncDefend, Status: "vanish", Turns: 2, MP: 12}},
		{300, MagicTemplate{Func: MagicFuncReviveDefend, Status: "vanish", Turns: 2, Power: 100, MP: 16}},
		// STAGE-199 的六个精灵都能按编号取到：镜子的精灵（反射 1 次）、复活镜精灵 Lv1、复活光精灵 Lv1。
		{260, MagicTemplate{Func: MagicFuncDefend, Status: "reflect", Turns: 1, MP: 8}},
		{280, MagicTemplate{Func: MagicFuncReviveDefend, Status: "absorb", Turns: 1, Power: 100, MP: 12}},
		{290, MagicTemplate{Func: MagicFuncReviveDefend, Status: "reflect", Turns: 1, Power: 100, MP: 12}},
		// 治愈的精灵 Lv1 不受影响。
		{0, MagicTemplate{Func: MagicFuncHeal, Power: 80, MP: 6}},
	}
	for _, tc := range cases {
		got, ok := MagicByID(tc.id)
		if !ok {
			t.Fatalf("编号 %d 不在表里", tc.id)
		}
		if got.Func != tc.want.Func || got.Status != tc.want.Status || got.Turns != tc.want.Turns ||
			got.Success != tc.want.Success || got.All != tc.want.All || got.Power != tc.want.Power || got.MP != tc.want.MP {
			t.Fatalf("编号 %d = %+v，应为 %+v", tc.id, got, tc.want)
		}
	}
}

// 场地属性与属性反转（STAGE-200）：23 个精灵都取得到，属性、威力、回合和目标范围对照 magic.txt。
func TestMagicTableFieldAndReverse(t *testing.T) {
	cases := []struct {
		id   int
		want MagicTemplate
	}{
		// 大地的精灵 Lv1：地 60，持续 3 回合，MP 按等级推算为 4。
		{190, MagicTemplate{Func: MagicFuncField, Elem: "地", Power: 60, Turns: 3, MP: 4}},
		// 大地的精灵 Lv5：地 100，持续 5 回合。
		{194, MagicTemplate{Func: MagicFuncField, Elem: "地", Power: 100, Turns: 5, MP: 12}},
		// 火炎的精灵 Lv3：火 80，持续 4 回合。
		{212, MagicTemplate{Func: MagicFuncField, Elem: "火", Power: 80, Turns: 4, MP: 8}},
		// 调和的精灵：只写“无”，威力缺省 30、回合缺省 3。
		{230, MagicTemplate{Func: MagicFuncField, Elem: "无", Power: 30, Turns: 3, MP: 4}},
		// 彩虹的精灵：单人反转。
		{240, MagicTemplate{Func: MagicFuncReverse, Elem: "-", All: false, MP: 10}},
		// 极光的精灵：一方全体反转。
		{241, MagicTemplate{Func: MagicFuncReverse, Elem: "-", All: true, MP: 16}},
	}
	for _, tc := range cases {
		got, ok := MagicByID(tc.id)
		if !ok {
			t.Fatalf("编号 %d 不在表里", tc.id)
		}
		if got.Func != tc.want.Func || got.Elem != tc.want.Elem || got.Power != tc.want.Power ||
			got.Turns != tc.want.Turns || got.All != tc.want.All || got.MP != tc.want.MP {
			t.Fatalf("编号 %d = %+v，应为 %+v", tc.id, got, tc.want)
		}
	}
	reachable := 0
	for _, id := range []int{190, 191, 192, 193, 194, 200, 201, 202, 203, 204, 210, 211, 212, 213, 214,
		220, 221, 222, 223, 224, 230, 240, 241} {
		if _, ok := MagicByID(id); ok {
			reachable++
		}
	}
	if reachable != 23 {
		t.Fatalf("场地与反转精灵应有 23 个，实际取到 %d", reachable)
	}
}

// 变幻的精灵（STAGE-203，原版 MAGIC_Metamo）：5 项，参数是变身的秒数，MP 按秒数推算，目标类型为 none。
func TestMagicTableMetamo(t *testing.T) {
	cases := []struct {
		id    int
		power int
		mp    int
	}{
		{350, 60, 8},     // 变幻的精灵 Lv1：一分钟
		{351, 120, 12},   // Lv2：二分钟
		{352, 180, 16},   // Lv3：三分钟
		{410, 600, 44},   // Lv4：十分钟
		{355, 86400, 60}, // Lv10：一天，MP 封顶 60
	}
	for _, tc := range cases {
		got, ok := MagicByID(tc.id)
		if !ok {
			t.Fatalf("编号 %d 不在表里", tc.id)
		}
		if got.Func != MagicFuncMetamo || got.Power != tc.power || got.MP != tc.mp {
			t.Fatalf("编号 %d = %+v，应为函数 metamo、秒数 %d、MP %d", tc.id, got, tc.power, tc.mp)
		}
		if got.BattleTarget() != TargetNone {
			t.Fatalf("编号 %d 的目标类型应为 none，得到 %s", tc.id, got.BattleTarget())
		}
	}
}

// 召唤龙阵与精灵版宠物技能（STAGE-202，原版 MAGIC_ToCallDragon、MAGIC_AttSkill）：参数列取得到，
// 龙阵的图像号和威力、精灵版技能的技能编号对得上 magic.txt；目标类型按协议 10.5 的规定。
func TestMagicTableSummon(t *testing.T) {
	dragons := []struct {
		id    int
		arg   string
		power int
		mp    int
		all   bool
	}{
		{394, "100373", 300, 40, false},
		{395, "100401", 300, 40, true},
		{396, "101422", 400, 50, true},
		{399, "101901", 1000, 0, true},
	}
	for _, tc := range dragons {
		got, ok := MagicByID(tc.id)
		if !ok {
			t.Fatalf("龙阵 %d 不在表里", tc.id)
		}
		if got.Func != MagicFuncDragon || got.Arg != tc.arg || got.Power != tc.power || got.MP != tc.mp || got.All != tc.all {
			t.Fatalf("龙阵 %d = %+v，应为图像号 %s、威力 %d、MP %d", tc.id, got, tc.arg, tc.power, tc.mp)
		}
	}
	skills := []struct {
		id    int
		skill string
	}{
		{326, "3"},   // 破除防御
		{327, "30"},  // 突击
		{328, "40"},  // 一击必杀
		{329, "50"},  // 背水之战其之１
		{330, "150"}, // 不防守战法
	}
	for _, tc := range skills {
		got, ok := MagicByID(tc.id)
		if !ok {
			t.Fatalf("精灵版技能 %d 不在表里", tc.id)
		}
		if got.Func != MagicFuncAttSkill || got.Arg != tc.skill || got.MP != 10 || got.Power != 0 {
			t.Fatalf("精灵版技能 %d = %+v，应为技能编号 %s、MP 10", tc.id, got, tc.skill)
		}
	}
	// 其余精灵的参数列都是空的（TSV 里写的 -）。
	if got, _ := MagicByID(301); got.Arg != "" {
		t.Fatalf("攻击精灵 301 的参数应为空，得到 %q", got.Arg)
	}
}

// 召唤龙阵与精灵版宠物技能的目标类型（STAGE-202）：奔龙阵选敌方一个，强恩阵、奔雷阵、GM 烟火是敌方全体；
// 精灵版技能取技能自己的目标类型（破除防御、突击、一击必杀、背水之战是敌方一个，不防守战法不用目标）。
func TestMagicSummonTargetType(t *testing.T) {
	cases := []struct {
		id   int
		want TargetType
	}{
		{394, TargetEnemy}, {395, TargetEnemyAll}, {396, TargetEnemyAll}, {399, TargetEnemyAll},
		{326, TargetEnemy}, {327, TargetEnemy}, {328, TargetEnemy}, {329, TargetEnemy}, {330, TargetNone},
	}
	for _, tc := range cases {
		got, ok := MagicByID(tc.id)
		if !ok {
			t.Fatalf("精灵 %d 不在表里", tc.id)
		}
		if bt := got.BattleTarget(); bt != tc.want {
			t.Fatalf("精灵 %d（%s）的目标类型 = %s，应为 %s", tc.id, got.Name, bt, tc.want)
		}
	}
	// 技能编号查不到的精灵版技能按 none 处理（不会因为表里的参数坏了而选目标）。
	if bt := (MagicTemplate{Func: MagicFuncAttSkill, Arg: "99999"}).BattleTarget(); bt != TargetNone {
		t.Fatalf("查不到的技能应为 none，得到 %s", bt)
	}
}
