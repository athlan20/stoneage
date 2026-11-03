// 怪物 AI（STAGE-60，gmsv/battle/battle_ai.c 的 BATTLE_ai_normal 移植）：
// 怪物按 enemy.txt 的战术串行动——按权重抽一种模式（攻击 / 防御 / 精灵 / 逃跑 / 技能槽），
// 攻击和技能再按选项选目标（随机 / 血最多 / 血最少，只打人物或宠物）。
// 原版只实现了模式 1（ai_normal）；精灵模式抽中后没有实现（不出招）；
// 战术串为空或全部权重为 0 时同样不出招（原版打印“无指定任何的攻击方式”后放弃）。

package battle

import (
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
)

// AI 选项串里的子选项个数（battle_ai.c 的 B_AI_NORMAL_*SUBOPTIONNUM）。
const (
	aiAtSubs = 3 // at：权重、目标类型、选择方式
	aiWaSubs = 7 // wa：七个技能槽的权重
)

// 目标类型与选择方式（battle_ai.c 的 B_AI_NORMAL_*）。
const (
	aiTargetAll    = 1 // 全部（人物和宠物）
	aiTargetPlayer = 2 // 只打人物
	aiTargetPet    = 3 // 只打宠物

	aiSelectRandom = 1 // 随机
	aiSelectHPMax  = 2 // 血最多
	aiSelectHPMin  = 3 // 血最少
)

// aiOptions 是解析好的战术串。
type aiOptions struct {
	at [aiAtSubs]int // 攻击：at[0] 权重、at[1] 目标类型、at[2] 选择方式
	gu int           // 防御权重
	ma int           // 精灵权重（原版没实现，抽中不出招）
	es int           // 逃跑权重
	wa [aiWaSubs]int // 技能槽权重（0–6 槽）
	rn int           // 选血最多/最少时按 1/(rn+1) 的概率随机换目标（_ENEMY_ATTACK_AI，默认 1）
}

// defaultAIOptions 是没有战术串时的行为：普攻随机目标（绝大多数怪物的默认）。
func defaultAIOptions() aiOptions {
	return aiOptions{at: [aiAtSubs]int{1, aiTargetAll, aiSelectRandom}, rn: 1}
}

// parseAIOptions 解析 "at:1;1;1|gu:0|es:0|rn:|wa:0;0;0;0;0;0;0;"。
// 键不存在或子选项残缺时按 0（rn 按 1）；整串为空时返回零值（怪物不出招，同原版）。
func parseAIOptions(s string) aiOptions {
	var o aiOptions
	o.rn = 1
	for _, kv := range strings.Split(s, "|") {
		key, val, ok := strings.Cut(kv, ":")
		if !ok {
			continue
		}
		subs := strings.Split(val, ";")
		num := func(i int) int {
			if i >= len(subs) {
				return 0
			}
			n, _ := strconv.Atoi(strings.TrimSpace(subs[i]))
			return n
		}
		switch key {
		case "at":
			for i := range o.at {
				o.at[i] = num(i)
			}
		case "gu":
			o.gu = num(0)
		case "ma":
			o.ma = num(0)
		case "es":
			o.es = num(0)
		case "wa":
			for i := range o.wa {
				o.wa[i] = num(i)
			}
		case "rn":
			// "rn:" 空值在原版按键不存在处理（默认 1）。
			if v := strings.TrimSpace(val); v != "" {
				o.rn = num(0)
			}
		}
	}
	return o
}

// aiAction 是怪物这一回合的决定。
type aiAction struct {
	kind   string // attack / guard / escape / skill / none
	target int    // attack 与 skill 的目标单位号
	skill  int    // skill 的技能栏（1–7，同宠物指令）
}

// decide 按战术串决定本回合的行动（BATTLE_ai_normal 的主干）。
// 返回 ok=false 表示怪物不出招（战术串为空、全零，或精灵模式）。
func (o aiOptions) decide(b *Battle, u *Unit) (aiAction, bool) {
	if o.none() {
		return aiAction{kind: "none"}, false
	}
	mode, slot := o.pick(b.Rnd(o.total()))

	switch mode {
	case "guard":
		return aiAction{kind: "guard"}, true
	case "escape":
		return aiAction{kind: "escape"}, true
	case "magic": // 原版的 ai_normal 没有实现精灵模式（抽中等于不出招）
		return aiAction{kind: "none"}, false
	}

	// 选目标：先按 at[1] 过滤（全部 / 只人物 / 只宠物），筛空了退回全部。
	targets := b.aiTargets(u, o.at[1])
	if len(targets) == 0 {
		if o.at[1] == aiTargetAll {
			return aiAction{kind: "none"}, false
		}
		targets = b.aiTargets(u, aiTargetAll)
		if len(targets) == 0 {
			return aiAction{kind: "none"}, false
		}
	}
	var t *Unit
	switch o.at[2] {
	case aiSelectRandom:
		t = targets[b.Rnd(len(targets))]
	case aiSelectHPMax, aiSelectHPMin:
		t = targets[0]
		for _, cand := range targets[1:] {
			if (o.at[2] == aiSelectHPMax && cand.HP > t.HP) ||
				(o.at[2] == aiSelectHPMin && cand.HP < t.HP) {
				t = cand
			}
		}
		// _ENEMY_ATTACK_AI 的随机化：!RAND(0, rn) 为真时随机挑（概率 1/(rn+1)；rn 为 0 时每次都随机）。
		if b.Rnd(o.rn+1) == 0 {
			t = targets[b.Rnd(len(targets))]
		}
	default:
		return aiAction{kind: "none"}, false
	}
	if mode == "attack" {
		return aiAction{kind: "attack", target: t.No}, true
	}
	// 技能槽（wa 第 slot 个权重被抽中）：栏位没有技能时不出招（PETSKILL_Use 失败）。
	if _, ok := gamedata.PetSkillByID(u.Skills[slot]); !ok {
		return aiAction{kind: "none"}, false
	}
	return aiAction{kind: "skill", target: t.No, skill: slot + 1}, true
}

// none 表示战术串没有可用的行动（原版“无指定任何的攻击方式”）。
func (o aiOptions) none() bool {
	return o.at[0] == 0 && o.gu == 0 && o.ma == 0 && o.es == 0 && o.wa == [aiWaSubs]int{}
}

// total 是全部权重之和。
func (o aiOptions) total() int {
	sum := o.at[0] + o.gu + o.ma + o.es
	for _, w := range o.wa {
		sum += w
	}
	return sum
}

// pick 按累计权重取模式（BATTLE_ai_normal 的 while(!mode) 循环，一次到位）。
// 抽中技能时同时给出槽号。
func (o aiOptions) pick(r int) (mode string, slot int) {
	if o.at[0] != 0 && r < o.at[0] {
		return "attack", 0
	}
	if o.gu != 0 && r < o.at[0]+o.gu {
		return "guard", 0
	}
	if o.ma != 0 && r < o.at[0]+o.gu+o.ma {
		return "magic", 0
	}
	if o.es != 0 && r < o.at[0]+o.gu+o.ma+o.es {
		return "escape", 0
	}
	work := o.at[0] + o.gu + o.ma + o.es
	for i, w := range o.wa {
		work += w
		if w != 0 && r < work {
			return "skill", i
		}
	}
	return "attack", 0 // 全零不会到这（none 已提前返回）
}

// aiTargets 收集 actor 对面还活着的单位，按 want 过滤（全部 / 只人物 / 只宠物）。
// 阵营按单位号算（人物逃走离场后 PlayerUnit 为 nil，不能解引用）。
func (b *Battle) aiTargets(actor *Unit, want int) []*Unit {
	var out []*Unit
	for _, t := range b.units {
		if t == nil || !t.targetable() || t.Side() == actor.Side() {
			continue
		}
		switch want {
		case aiTargetPlayer:
			if t.Kind != KindPlayer {
				continue
			}
		case aiTargetPet:
			if t.Kind != KindPet {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}
