// 目标类型（STAGE-78，docs/protocol.md 10.5 的表，以及 5.8、5.17、5.19 里的 target_type）：
// 物品、精灵、宠物技能共用一套取值。服务端从各自的模板算出它；客户端只看它决定要不要选目标、能选哪一方。

package gamedata

// TargetType 是协议里的目标类型。
type TargetType string

const (
	TargetNone     TargetType = "none"      // 不需要目标，直接出招；战斗中不能用的物品也是这个取值
	TargetSelf     TargetType = "self"      // 只能对自己用，不需要选
	TargetAlly     TargetType = "ally"      // 己方一个活着的单位（自己或宠物），需要选
	TargetAllyAll  TargetType = "ally_all"  // 己方全体，不需要选
	TargetAllyDown TargetType = "ally_down" // 己方一个倒下的单位（人或宠物，复活精灵用），需要选
	TargetEnemy    TargetType = "enemy"     // 敌方一个活着的单位，需要选
	TargetEnemyAll TargetType = "enemy_all" // 敌方全体，不需要选
	TargetAny      TargetType = "any"       // 己方或敌方任意一个活着的单位，需要选（属性反转精灵，STAGE-200）
)

// NeedsTarget 报告这个类型要不要选一个单位（ally、ally_down、enemy、any），其余类型不带 target。
// 其中己方一个缺省时服务端落在自己身上（ally），其余必须带。
func (t TargetType) NeedsTarget() bool {
	return t == TargetAlly || t == TargetAllyDown || t == TargetEnemy || t == TargetAny
}

// BattleTarget 是消耗品在战斗中的目标类型。只能在战斗外用、没有可用的使用效果、或目标不是
// 自己、他人、全己方的物品（装备、材料等）战斗中不能用，返回 TargetNone；这与 battle 提交时的校验一致。
func (t ItemTemplate) BattleTarget() TargetType {
	if t.AbleUse == AbleUseField {
		return TargetNone
	}
	eff, ok := t.UseEffect()
	if !ok {
		return TargetNone
	}
	if eff.Revive { // 复活类：原版 Target=101（气绝目标），不看 0–4 的枚举，目标是己方倒下的单位
		return TargetAllyDown
	}
	return targetOfItem(t.Target)
}

// FieldTarget 是消耗品在世界中（背包里的「使用」）的目标类型（STAGE-326）。与 BattleTarget 的区别只在
// 只能在战斗外用的物品（AbleUse=2，如安抚之素、高级宠物食品）：战斗中的目标类型是 none，世界中按
// 物品表的目标字段换算（他人 → ally，要选宠物）。其余物品两者相同；只能在战斗中用的物品返回 none。
func (t ItemTemplate) FieldTarget() TargetType {
	if t.AbleUse != AbleUseField {
		return t.BattleTarget()
	}
	if _, ok := t.UseEffect(); !ok {
		return TargetNone
	}
	return targetOfItem(t.Target)
}

// targetOfItem 是物品表的目标字段（ITEM_TARGET）换成协议目标类型；没有列出的编号不能直接选。
func targetOfItem(target int) TargetType {
	switch target {
	case TargetMyself:
		return TargetSelf
	case TargetOther:
		return TargetAlly
	case TargetMySide:
		return TargetAllyAll
	}
	return TargetNone
}

// BattleTarget 是精灵的目标类型（函数和目标列，见 magic.go）：攻击和状态打敌方（单体要选，全体不用选），
// 自愈只治自己，治疗他人和解状态救己方一个（单体要选，全体救己方全体），复活选己方一个倒下的单位。
func (t MagicTemplate) BattleTarget() TargetType {
	switch t.Func {
	case MagicFuncAttack, MagicFuncStatus:
		if t.All {
			return TargetEnemyAll
		}
		return TargetEnemy
	case MagicFuncHeal:
		if t.All {
			return TargetAllyAll
		}
		return TargetSelf
	case MagicFuncHealOther, MagicFuncCure:
		if t.All {
			return TargetAllyAll
		}
		return TargetAlly
	case MagicFuncRevive, MagicFuncReviveDefend:
		return TargetAllyDown
	case MagicFuncReverse: // 彩虹选一个，极光选一个（落在它那一方全体）（STAGE-200）
		return TargetAny
	case MagicFuncResist: // 魔抗系（STAGE-228）：己方一个，或己方全体
		if t.All {
			return TargetAllyAll
		}
		return TargetAlly
	case MagicFuncDefend: // 己方一个（缺省落在自己身上）
		return TargetAlly
	case MagicFuncMetamo: // 变幻的精灵（STAGE-203）只能在战斗外对自己的宠物用，不走战斗的目标选择
		return TargetNone
	case MagicFuncDragon: // 召唤龙阵（STAGE-202）：奔龙阵选敌方一个，强恩阵、奔雷阵、GM 烟火是敌方全体
		if t.All {
			return TargetEnemyAll
		}
		return TargetEnemy
	case MagicFuncAttSkill: // 精灵版宠物技能（STAGE-202）：目标类型取这个技能自己的
		if s, ok := t.AttSkill(); ok {
			return s.BattleTarget()
		}
		return TargetNone
	}
	return TargetNone // 场地属性（STAGE-200）不用选，对整场生效
}

// BattleTarget 是宠物技能的目标类型，由原版的目标码换算（petskill.txt 第 9 列，原版 PETSKILL_TARGETTYPE）：
// 0 自己、1 己方一个、2 己方全体、3 敌方全体、5 不需要目标。6（他人，不含自己）和 7（不含自己和宠物）
// 在一期没有队友时只剩敌方，攻击类技能都是这一种；没有列出的编号一期数据里没有，也按敌方一个处理
// （服务端按普通攻击结算）。只能在战斗外用的技能（场合不是战斗，如加工、料理）返回 TargetNone。
func (s PetSkill) BattleTarget() TargetType {
	if s.Field != PetSkillFieldBattle {
		return TargetNone
	}
	switch s.Target {
	case 0:
		return TargetSelf
	case 1:
		return TargetAlly
	case 2:
		return TargetAllyAll
	case 3:
		return TargetEnemyAll
	case 5:
		return TargetNone
	}
	return TargetEnemy
}
