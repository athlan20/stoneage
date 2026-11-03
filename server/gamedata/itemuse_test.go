// 使用物品的效果解析与回复公式（STAGE-30，docs/protocol.md 5.29）的单元测试。

package gamedata

import "testing"

func mustItem(t *testing.T, id int) ItemTemplate {
	t.Helper()
	def, ok := ItemByID(id)
	if !ok {
		t.Fatalf("物品 %d 不在表里", id)
	}
	return def
}

func TestUseEffectRecovery(t *testing.T) {
	// 普通的肉：体40（itemset.txt 的 1235），给自己或宠物回血。
	def := mustItem(t, 1235)
	if def.UseFunc != "ITEM_useRecovery" || def.Argument != "体40" {
		t.Fatalf("普通的肉的使用字段不对: %+v", def)
	}
	eff, ok := def.UseEffect()
	if !ok || !eff.Recovery || eff.HP != 40 || eff.MP != 0 || eff.Full {
		t.Fatalf("体40 解析不对: %+v ok=%v", eff, ok)
	}
	if def.AbleUse != AbleUseAll || def.Target != TargetOther {
		t.Fatalf("普通的肉的场合/目标不对: %d/%d", def.AbleUse, def.Target)
	}

	// 小块气力肉：气5（1240）。
	eff, ok = mustItem(t, 1240).UseEffect()
	if !ok || eff.HP != 0 || eff.MP != 5 {
		t.Fatalf("气5 解析不对: %+v ok=%v", eff, ok)
	}

	// 结婚蛋糕：体600、战斗专用、全己方（1232）。
	def = mustItem(t, 1232)
	if def.AbleUse != AbleUseBattle || def.Target != TargetMySide {
		t.Fatalf("结婚蛋糕的场合/目标不对: %d/%d", def.AbleUse, def.Target)
	}
	if eff, _ := def.UseEffect(); !eff.Recovery || eff.HP != 600 {
		t.Fatalf("体600 解析不对: %+v", eff)
	}
}

// STAGE-334：英雄的祝福（18543，ITEM_useSkup）解析为“加未分配点”，只对自己，只在战斗外。
func TestUseEffectSkillUpHeroBlessing(t *testing.T) {
	def := mustItem(t, 18543)
	eff, ok := def.UseEffect()
	if !ok || !eff.SkillUp || eff.HP != 0 || eff.MP != 0 || eff.Loyalty != 0 || eff.Cure || eff.Revive {
		t.Fatalf("英雄的祝福解析不对: %+v ok=%v", eff, ok)
	}
	if def.AbleUse != AbleUseField || def.Target != TargetMyself {
		t.Fatalf("英雄的祝福应只在战斗外、目标为自己: 场合 %d 目标 %d", def.AbleUse, def.Target)
	}
	if def.BattleTarget() != TargetNone {
		t.Fatalf("英雄的祝福战斗中不能用，战斗目标应为 none: %s", def.BattleTarget())
	}
}

func TestUseEffectCureAndUnsupported(t *testing.T) {
	// 治疗中毒的肉：解“毒”（1247），只战斗中。
	eff, ok := mustItem(t, 1247).UseEffect()
	if !ok || !eff.Cure || eff.Status != "poison" {
		t.Fatalf("毒 解析不对: %+v ok=%v", eff, ok)
	}
	// 莉儿的爱心糖果：解“全”（2013）。
	if eff, _ := mustItem(t, 2013).UseEffect(); !eff.Cure || eff.Status != "all" {
		t.Fatalf("全 解析不对: %+v", eff)
	}
	// 小斧头没有使用函数；迷人香菇（魅力）不属于一期的三类。
	if _, ok := mustItem(t, 0).UseEffect(); ok {
		t.Fatal("小斧头不应该可用")
	}
	for id, name := range map[int]string{1211: "迷人香菇"} {
		if _, ok := mustItem(t, id).UseEffect(); ok {
			t.Fatalf("%s 不应该可用（一期只做回血/回魔/解状态）", name)
		}
	}
}

func TestRecoveryPower(t *testing.T) {
	zero := func(int) int { return 0 }      // 随机数总取 0：回复量是下限 0.9N
	full := func(n int) int { return n - 1 } // 随机数总取上限：回复量是 1.1N
	// N=40：0.9N=36、1.1N=44。
	if got := RecoveryPower(40, 0, false, false, zero); got != 36 {
		t.Fatalf("下限回复量不对: %d", got)
	}
	if got := RecoveryPower(40, 0, false, false, full); got != 44 {
		t.Fatalf("上限回复量不对: %d", got)
	}
	// 体力 8 的人物恢复率 1+0.0001×8=1.0008：44×1.0008 取整仍是 44。
	if got := RecoveryPower(40, 8, false, true, full); got != 44 {
		t.Fatalf("带恢复率的回复量不对: %d", got)
	}
	// 宠物（体力点数 400，恢复率 1.02）：110×1.02=112.2 取整 112。
	if got := RecoveryPower(100, 400, true, true, full); got != 112 {
		t.Fatalf("宠物的回复量不对: %d", got)
	}
	// MP 不乘恢复率：体力再高也是 44。
	if got := RecoveryPower(40, 800, false, false, full); got != 44 {
		t.Fatalf("MP 的回复量不该乘恢复率: %d", got)
	}
}
