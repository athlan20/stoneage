package gamedata

import "testing"

// 目标类型从模板换算：用数据表里的真实编号检查，再用合成的模板覆盖各个分支。
func TestBattleTargetOfRealTemplates(t *testing.T) {
	cases := []struct {
		name string
		id   int
		want TargetType
	}{
		{"小斧头（装备，战斗中不能用）", 0, TargetNone},
		{"小块肉（己方一个）", 1234, TargetAlly},
		{"结婚蛋糕（己方全体，只能战斗中用）", 1232, TargetAllyAll},
		{"迷人香菇（只能战斗外用）", 1233, TargetNone},
		{"爱情巧克力（只能对自己）", 1230, TargetSelf},
	}
	for _, c := range cases {
		it, ok := ItemByID(c.id)
		if !ok {
			t.Fatalf("%s：物品 %d 不在表里", c.name, c.id)
		}
		if got := it.BattleTarget(); got != c.want {
			t.Errorf("%s：目标类型 %s，want %s", c.name, got, c.want)
		}
	}
	magic := map[int]TargetType{
		0:   TargetSelf,     // 治愈的精灵 Lv1：自愈
		10:  TargetAlly,     // 滋润的精灵 Lv1：治疗己方一个
		20:  TargetAllyAll,  // 恩惠的精灵 Lv1：己方全体
		301: TargetEnemy,    // 岩石撞击：攻击敌方一个
		305: TargetEnemyAll, // 地震：攻击敌方全体
		190: TargetNone,     // 大地的精灵 Lv1：场地属性，对整场生效
		240: TargetAny,      // 彩虹的精灵：反转一个单位
		241: TargetAny,      // 极光的精灵：选一个单位，落在它那一方全体
	}
	for id, want := range magic {
		m, ok := MagicByID(id)
		if !ok {
			t.Fatalf("精灵 %d 不在表里", id)
		}
		if got := m.BattleTarget(); got != want {
			t.Errorf("精灵 %s（%d）：目标类型 %s，want %s", m.Name, id, got, want)
		}
	}
	skills := map[int]TargetType{
		0:   TargetNone,  // 待机
		1:   TargetEnemy, // 攻击
		2:   TargetNone,  // 防御
		20:  TargetEnemy, // 忠犬：攻击，替主人挨打
		150: TargetNone,  // 不防守战法：不出手
		200: TargetNone,  // 加工：只能在战斗外用
	}
	for id, want := range skills {
		s, ok := PetSkillByID(id)
		if !ok {
			t.Fatalf("技能 %d 不在表里", id)
		}
		if got := s.BattleTarget(); got != want {
			t.Errorf("技能 %s（%d）：目标类型 %s，want %s", s.Name, id, got, want)
		}
	}
}

func TestItemBattleTargetRules(t *testing.T) {
	recovery := ItemTemplate{UseFunc: "ITEM_useRecovery", Argument: "体100", AbleUse: AbleUseAll}
	cases := []struct {
		name string
		item ItemTemplate
		want TargetType
	}{
		{"对自己", withTarget(recovery, TargetMyself), TargetSelf},
		{"他人", withTarget(recovery, TargetOther), TargetAlly},
		{"全己方", withTarget(recovery, TargetMySide), TargetAllyAll},
		{"全敌方（战斗中不能用）", withTarget(recovery, TargetFoes), TargetNone},
		{"全场（战斗中不能用）", withTarget(recovery, TargetAll), TargetNone},
		{"只能战斗外用", ItemTemplate{UseFunc: "ITEM_useRecovery", Argument: "体100", AbleUse: AbleUseField, Target: TargetOther}, TargetNone},
		{"没有可用的效果", ItemTemplate{UseFunc: "ITEM_useWarp", Target: TargetMyself}, TargetNone},
	}
	for _, c := range cases {
		if got := c.item.BattleTarget(); got != c.want {
			t.Errorf("%s：目标类型 %s，want %s", c.name, got, c.want)
		}
	}
}

func withTarget(it ItemTemplate, target int) ItemTemplate {
	it.Target = target
	return it
}

func TestMagicBattleTargetByFunction(t *testing.T) {
	cases := []struct {
		name string
		m    MagicTemplate
		want TargetType
	}{
		{"攻击单体", MagicTemplate{Func: "attack"}, TargetEnemy},
		{"攻击全体", MagicTemplate{Func: "attack", All: true}, TargetEnemyAll},
		{"自愈", MagicTemplate{Func: "heal"}, TargetSelf},
		{"自愈全体", MagicTemplate{Func: "heal", All: true}, TargetAllyAll},
		{"治疗单人", MagicTemplate{Func: "heal_other"}, TargetAlly},
		{"治疗全体", MagicTemplate{Func: "heal_other", All: true}, TargetAllyAll},
		{"状态单人打对方", MagicTemplate{Func: "status"}, TargetEnemy},
		{"状态全体打对方", MagicTemplate{Func: "status", All: true}, TargetEnemyAll},
		{"解状态单人救己方", MagicTemplate{Func: "cure"}, TargetAlly},
		{"解状态全体救己方", MagicTemplate{Func: "cure", All: true}, TargetAllyAll},
		{"复活选倒下的己方", MagicTemplate{Func: "revive"}, TargetAllyDown},
		// STAGE-199：防御精灵选己方一个（缺省落在自己），复活防御选倒下的己方。
		{"防御单人", MagicTemplate{Func: MagicFuncDefend}, TargetAlly},
		{"复活防御选倒下的己方", MagicTemplate{Func: MagicFuncReviveDefend}, TargetAllyDown},
		{"未知函数", MagicTemplate{Func: "unknown"}, TargetNone},
	}
	for _, c := range cases {
		if got := c.m.BattleTarget(); got != c.want {
			t.Errorf("%s：目标类型 %s，want %s", c.name, got, c.want)
		}
	}
}

func TestPetSkillBattleTargetByCode(t *testing.T) {
	cases := []struct {
		name  string
		skill PetSkill
		want  TargetType
	}{
		{"目标码 0 自己", PetSkill{Field: PetSkillFieldBattle, Target: 0}, TargetSelf},
		{"目标码 1 己方一个", PetSkill{Field: PetSkillFieldBattle, Target: 1}, TargetAlly},
		{"目标码 2 己方全体", PetSkill{Field: PetSkillFieldBattle, Target: 2}, TargetAllyAll},
		{"目标码 3 敌方全体", PetSkill{Field: PetSkillFieldBattle, Target: 3}, TargetEnemyAll},
		{"目标码 5 不需要目标", PetSkill{Field: PetSkillFieldBattle, Target: 5}, TargetNone},
		{"目标码 6 他人，一期只剩敌方", PetSkill{Field: PetSkillFieldBattle, Target: 6}, TargetEnemy},
		{"目标码 7 不含自己和宠物，一期只剩敌方", PetSkill{Field: PetSkillFieldBattle, Target: 7}, TargetEnemy},
		{"战斗外的技能不能在战斗中用", PetSkill{Field: 2, Target: 0}, TargetNone},
	}
	for _, c := range cases {
		if got := c.skill.BattleTarget(); got != c.want {
			t.Errorf("%s：目标类型 %s，want %s", c.name, got, c.want)
		}
	}
}

func TestNeedsTargetOnlyForOneSide(t *testing.T) {
	want := map[TargetType]bool{
		TargetNone: false, TargetSelf: false, TargetAlly: true,
		TargetAllyAll: false, TargetAllyDown: true, TargetEnemy: true, TargetEnemyAll: false,
		TargetAny: true,
	}
	for tt, need := range want {
		if got := tt.NeedsTarget(); got != need {
			t.Errorf("%s.NeedsTarget() = %v，want %v", tt, got, need)
		}
	}
}
