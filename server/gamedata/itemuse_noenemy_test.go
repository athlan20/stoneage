package gamedata

import "testing"

// STAGE-340：龙王的守护（18544，ITEM_useNoenemy）解析为“不遇敌”效果，只能在战斗外对自己用。
func TestNoEnemyItemUseEffect(t *testing.T) {
	def, ok := ItemByID(18544)
	if !ok {
		t.Fatal("物品表里应有 18544 龙王的守护")
	}
	eff, usable := def.UseEffect()
	if !usable || !eff.NoEnemy {
		t.Fatalf("18544 应解析为 NoEnemy 效果，实际 usable=%v eff=%+v", usable, eff)
	}
	if def.AbleUse != AbleUseField {
		t.Fatalf("18544 应只能战斗外使用，实际 AbleUse=%d", def.AbleUse)
	}
	if def.BattleTarget() != TargetNone {
		t.Fatalf("战斗中不能用，BattleTarget 应为 none，实际 %v", def.BattleTarget())
	}
	if def.FieldTarget() != TargetSelf {
		t.Fatalf("世界中目标应为自己，实际 %v", def.FieldTarget())
	}
}
