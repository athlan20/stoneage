package gamedata

// 精灵的效果文字（STAGE-116，docs/protocol.md 5.8 的 magic[].effect）：服务端按函数、威力、属性、状态和全体标记算好，
// 客户端只显示，和物品的 effect 一样。写法见协议文档，这里只负责把模板换算成文字。

import "fmt"

// magicStatusNames 是状态精灵与解状态精灵的状态名（协议 10.2 的取值 → 中文，与协议 5.8 的写法一致）。
// 表里出现但这里没有的（deep_poison 等）按“一种异常状态”处理。
var magicStatusNames = map[string]string{
	"poison":         "中毒",
	"paralysis":      "麻痹",
	"sleep":          "睡眠",
	"stone":          "石化",
	"drunk":          "醉酒",
	"confusion":      "混乱",
	"weaken":         "虚弱", // STAGE-201
	"barrier":        "魔障", // STAGE-201
	"nocast":         "沉默", // STAGE-201
	"magic_resist":   "魔抗", // STAGE-228
	"fire_resist":    "火抗",
	"thunder_resist": "电抗",
	"ice_resist":     "冰抗",
}

// Effect 是精灵的效果文字，见 docs/protocol.md 5.8。
func (t MagicTemplate) Effect() string {
	switch t.Func {
	case MagicFuncAttack:
		text := fmt.Sprintf("攻击%s，威力 %d", t.attackTargetText(), t.Power)
		if t.Elem != "-" && t.Elem != "" {
			text += "，" + t.Elem + "属性"
		}
		return text
	case MagicFuncHeal:
		if t.All {
			return fmt.Sprintf("回复己方全体 HP %d", t.Power)
		}
		return fmt.Sprintf("回复自己 HP %d", t.Power)
	case MagicFuncHealOther:
		if t.All {
			return fmt.Sprintf("回复己方全体 HP %d", t.Power)
		}
		return fmt.Sprintf("回复己方一个 HP %d", t.Power)
	case MagicFuncStatus:
		target := "敌方一个"
		if t.All {
			target = "敌方全体"
		}
		name, ok := magicStatusNames[t.Status]
		if !ok {
			return fmt.Sprintf("使%s陷入异常状态，持续 %d 回合", target, t.Turns)
		}
		return fmt.Sprintf("使%s%s，持续 %d 回合", target, name, t.Turns)
	case MagicFuncCure:
		if t.All {
			return "解除己方全体各一种异常状态"
		}
		name, ok := magicStatusNames[t.Status]
		if !ok {
			return "解除己方一个的一种异常状态"
		}
		return "解除己方一个的" + name
	case MagicFuncResist: // 魔抗系（STAGE-228）：魔抗是加成百分比；火、电、冰抗原版只占槽、不改数值，文字里写明
		target := "己方一个"
		if t.All {
			target = "己方全体"
		}
		if t.Status == "magic_resist" {
			return fmt.Sprintf("使%s魔抗提升 %d%%，持续 %d 回合", target, t.Power, t.Turns)
		}
		return fmt.Sprintf("使%s%s，持续 %d 回合（原版未生效，只占状态槽）", target, magicStatusNames[t.Status], t.Turns)
	case MagicFuncRevive:
		return fmt.Sprintf("复活己方一个倒下的单位，HP 约 %d", t.Power)
	case MagicFuncDragon: // 召唤龙阵（STAGE-202）：威力是这一下的攻击力
		if t.All {
			return fmt.Sprintf("召唤龙攻击敌方全体，威力 %d", t.Power)
		}
		return fmt.Sprintf("召唤龙攻击敌方一个，威力 %d", t.Power)
	case MagicFuncAttSkill: // 精灵版宠物技能（STAGE-202）：名字取 petskill.txt
		if s, ok := t.AttSkill(); ok {
			return "使用宠物技能 " + s.Name
		}
		return ""
	case MagicFuncField: // 场地属性（STAGE-200）
		if t.Elem == "无" {
			return "场地恢复成无属性"
		}
		return fmt.Sprintf("整场战斗的场地变成%s属性，威力 %d%%，持续 %d 回合", t.Elem, t.Power, t.Turns)
	case MagicFuncReverse: // 属性反转（STAGE-200）：开关切换，开时才互换
		if t.All {
			return "切换所选一方全体的属性反转，开启时地↔火、水↔风互换"
		}
		return "切换所选单位的属性反转，开启时地↔火、水↔风互换"
	case MagicFuncDefend:
		return defendText(t.Status, t.Turns)
	case MagicFuncReviveDefend:
		return fmt.Sprintf("复活己方一个倒下的单位，HP 约 %d，", t.Power) + defendText(t.Status, t.Turns)
	case MagicFuncMetamo: // 变幻的精灵（STAGE-203）
		return "变身成自己的一只宠物，持续 " + metamoDuration(t.Power)
	}
	return ""
}

// defendText 是防御精灵的效果文字（STAGE-199）：吸收、反射、无效各按次数写。
func defendText(status string, n int) string {
	switch status {
	case "absorb":
		return fmt.Sprintf("己方一个挨下的 %d 次物理攻击改为吸收（回复伤害值的血量）", n)
	case "reflect":
		return fmt.Sprintf("己方一个接下的 %d 次物理攻击反弹给攻击者", n)
	case "vanish":
		return fmt.Sprintf("己方一个接下的 %d 次物理攻击无效", n)
	}
	return ""
}

// attackTargetText 是攻击精灵的范围文字（STAGE-218）：按 attmagic.bin 的命中掩码写，
// 不用 All（目标码 11 里熔岩爆发、暴风冰刃也只打一排 5 个）。
func (t MagicTemplate) attackTargetText() string {
	n := 0
	for _, hit := range t.Mask {
		if hit {
			n++
		}
	}
	switch {
	case n == 0 && t.All: // 合成的模板（测试）或没有掩码的旧数据
		return "敌方全体"
	case n == 0:
		return "敌方一个"
	case n == 15:
		return "敌方全体"
	case n == 1:
		return "敌方一个"
	case t.rowOnly():
		return "敌方一排"
	case n == 5:
		return "敌方十字范围"
	default:
		return fmt.Sprintf("敌方 %d 个", n)
	}
}

// rowOnly 报告掩码是不是只有中间一行（3 格或 5 格）。
func (t MagicTemplate) rowOnly() bool {
	for i, hit := range t.Mask {
		if !hit {
			continue
		}
		if i/5 != 1 {
			return false
		}
	}
	return true
}

// metamoDuration 把变身的秒数写成“N 分钟”或“N 天”（原版参数是秒，一分钟、十分钟、一天三种以外按秒写）。
func metamoDuration(sec int) string {
	switch {
	case sec > 0 && sec%86400 == 0:
		return fmt.Sprintf("%d 天", sec/86400)
	case sec > 0 && sec%60 == 0:
		return fmt.Sprintf("%d 分钟", sec/60)
	}
	return fmt.Sprintf("%d 秒", sec)
}
