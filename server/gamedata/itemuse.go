// 使用物品的效果（STAGE-30，docs/protocol.md 5.29 与 10.5 的 item 指令）。
// 一期只做三类：回复 HP（“体N”）、回复 MP（“气N”）、解除异常状态（“毒”“麻”等），
// “全”是 HP 回满加 MP 回满（或解掉全部异常）。出处是原版服务端
// item_event.c 的 ITEM_useRecovery_Field / battle_item.c 的 ITEM_useRecovery_Battle、
// ITEM_useStatusRecovery_Battle，公式见 recoveryPower 与 RecoveryRate。

package gamedata

import "strings"

// 可使用场合（item.h 的 ITEM_FIELDTYPE）与目标（ITEM_TARGETTYPE）。
const (
	AbleUseAll    = 0 // 战斗内外都可以
	AbleUseBattle = 1 // 只能在战斗中使用
	AbleUseField  = 2 // 只能在战斗外（地图上）使用

	TargetMyself = 0 // 自己
	TargetOther  = 1 // 他人（己方的单位，包括自己）
	TargetMySide = 2 // 全己方
	TargetFoes   = 3 // 全敌方（回复类没有，防御性支持）
	TargetAll    = 4 // 全场
)

// UseEffect 是一件消耗品解析出来的效果。Recovery / Cure 两类按物品表的
// 使用函数（ITEM_USEFUNC）区分；两类都不成立的物品不能用（Handle 里回复 ITEM_UNUSABLE）。
type UseEffect struct {
	Recovery bool   // 回复类（ITEM_useRecovery）
	Full     bool   // 参数含“全”：HP 回满，MP 回满（只对人物）
	HP       int    // “体N”的 N，回复的基准量
	MP       int    // “气N”的 N
	Loyalty  int    // “忠N”的 N：宠物忠诚增量（STAGE-326，安抚之素 忠30、高级宠物食品 忠10），只给宠物用
	Cure     bool   // 解除异常状态类（ITEM_useStatusRecovery），只在战斗中生效（原版 Field 分支为空）
	Status   string // Cure 时要解除的状态（协议状态名）：“poison”等；“all”是全部
	// SkillUp 是英雄的祝福（ITEM_useSkup，STAGE-334）：只对自己用、只在战斗外，使用后未分配能力点 +1，扣 1 个物品。
	SkillUp bool
	// NoEnemy 是龙王的守护（ITEM_useNoenemy，STAGE-340）：只对自己用、只在战斗外，使用后一段时间不遇敌，扣 1 个物品。
	NoEnemy bool
	// Revive 是复活类（ITEM_useRessurect、ITEM_ResAndDef）：只对倒下的己方单位生效，目标类型是 ally_down。
	Revive        bool
	RevivePower   int    // 参数的第一个数字 N：复活量的基准；0 表示回满（原版 pow <= 0）
	RevivePercent bool   // 第一个数字带“%”：N 是最大血量的百分比（原版 par）
	Defend        string // ITEM_ResAndDef 附加的防御（“absorb”“reflect”“vanish”），没有则为空
	DefendTurns   int    // 防御的回合数（参数里的 “turn N”，缺省 3，原版同）
}

// reviveDefends 是 ITEM_ResAndDef 参数里的防御关键字，取值同原版 aszMagicDef（吸、反、无），
// 协议取值同防御精灵（battle/magic_def.go）。
var reviveDefends = []struct{ key, kind string }{
	{"吸", "absorb"},
	{"反", "reflect"},
	{"无", "vanish"},
}

// statusKeys 把参数里的状态关键字（battle_event.h 的 BATTLE_ST 枚举对应的双字节汉字）
// 换成协议状态名（docs/protocol.md 10.2）。
var statusKeys = map[string]string{
	"毒": "poison",
	"麻": "paralysis",
	"眠": "sleep",
	"石": "stone",
	"醉": "drunk",
	"乱": "confusion",
	"全": "all",
}

// UseEffect 解析一件物品的使用效果；第二个返回值为假表示这个物品不能使用
// （没有使用函数，或效果不属于一期支持的类别，如传送、投掷毒药）。
func (t ItemTemplate) UseEffect() (UseEffect, bool) {
	switch t.UseFunc {
	case "ITEM_useRecovery":
		eff := UseEffect{Recovery: true}
		arg := t.Argument
		if strings.Contains(arg, "全") {
			eff.Full = true
			return eff, true
		}
		if n, ok := argValue(arg, "体"); ok {
			eff.HP = n
		}
		if n, ok := argValue(arg, "气"); ok {
			eff.MP = n
		}
		// “忠N”只在战斗外有效（原版 battle_item.c 不认“忠”）：AbleUse 不是 Field 的物品不算忠诚效果。
		if n, ok := argValue(arg, "忠"); ok && t.AbleUse == AbleUseField {
			eff.Loyalty = n
		}
		// “魅N”（魅力）一期不做：数值都取不到时这个物品不能用。
		if eff.HP == 0 && eff.MP == 0 && eff.Loyalty == 0 {
			return UseEffect{}, false
		}
		return eff, true
	case "ITEM_useStatusRecovery":
		for key, name := range statusKeys {
			if strings.Contains(t.Argument, key) {
				return UseEffect{Cure: true, Status: name}, true
			}
		}
	case "ITEM_useRessurect", "ITEM_ResAndDef":
		return reviveEffect(t.UseFunc, t.Argument), true
	case "ITEM_useSkup": // STAGE-334：英雄的祝福，出处是 item_event.c 的 ITEM_useSkup，原版没有别的限制
		return UseEffect{SkillUp: true}, true
	case "ITEM_useNoenemy": // STAGE-340：龙王的守护（18544）等，出处是 item_event.c 的 ITEM_useNoenemy
		return UseEffect{NoEnemy: true}, true
	}
	return UseEffect{}, false
}

// reviveEffect 解析复活类道具的参数（battle_item.c 的 ITEM_useRessurect / ITEM_ResAndDef）：
// 第一个数字是复活量，带 % 时按最大血量算；ITEM_ResAndDef 再看“吸/反/无”和 “turn N”挂防御。
func reviveEffect(useFunc, arg string) UseEffect {
	eff := UseEffect{Revive: true}
	if fields := strings.Fields(arg); len(fields) > 0 {
		eff.RevivePower, _ = argValue(fields[0], "")
		eff.RevivePercent = strings.Contains(fields[0], "%")
	}
	if useFunc != "ITEM_ResAndDef" {
		return eff
	}
	for _, d := range reviveDefends {
		if strings.Contains(arg, d.key) {
			eff.Defend = d.kind
			break
		}
	}
	if eff.Defend != "" {
		eff.DefendTurns = 3
		if n, ok := argValue(arg, "turn"); ok {
			eff.DefendTurns = n
		}
	}
	return eff
}

// argValue 取“前缀+数字”里的数字（item_event.c 的 sscanf(p+2, "%d", &work)）。
func argValue(arg, prefix string) (int, bool) {
	i := strings.Index(arg, prefix)
	if i < 0 {
		return 0, false
	}
	rest := strings.TrimSpace(arg[i+len(prefix):])
	digits := rest
	for j, r := range rest {
		if r < '0' || r > '9' {
			digits = rest[:j]
			break
		}
	}
	if digits == "" {
		return 0, false
	}
	n := 0
	for _, r := range digits {
		n = n*10 + int(r-'0')
	}
	return n, true
}

// RecoveryRate 是回复量的恢复率（char.c 的 GetRecoveryRate）：人物 1+0.0001×体力，
// 宠物 1+0.00005×体力。battle 里 Unit 的 Vital 已经是点数。
func RecoveryRate(vital int, pet bool) float64 {
	if pet {
		return 1.0 + 0.00005*float64(vital)
	}
	return 1.0 + 0.00010*float64(vital)
}

// RecoveryPower 按 itemset 参数 N 算回复量：RAND(0.9N, 1.1N) 再乘恢复率
// （HP 才乘，MP 不乘：item_event.c 的 Field 与 battle_magic.c 的 MultiRecovery 一致）。
// rnd 是随机源，取 0–99 的整数。
func RecoveryPower(n, vital int, pet bool, withRate bool, rnd func(int) int) int {
	base := n * 9 / 10
	if span := n / 5; span > 0 { // 0.9N–1.1N
		base += rnd(span + 1)
	}
	power := float64(base)
	if withRate {
		power *= RecoveryRate(vital, pet)
	}
	return int(power)
}
