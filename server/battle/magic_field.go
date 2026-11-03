// 场地属性与属性反转（STAGE-200，对照 gmsv/battle/battle_magic.c 的 BATTLE_FieldAttChange、
// BATTLE_MultiAttReverse，battle_event.c 的 BATTLE_FieldAttAdjust，battle.c 的 BATTLE_AttReverse 与回合末的
// att_count 递减）。场地：一场战斗只有一个，对双方都生效，属性、威力和剩余回合整体覆盖，回合末递减，
// 减到 0 恢复成无属性。反转：单位的标志翻转，见 reverseUnit。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// 场地加成的两个常数（BATTLE_FieldAttAdjust 的 AJ_BOTTOM、AJ_PLUS）。
const (
	fieldBottom = 0.5
	fieldPlus   = 0.5
)

// fieldState 是场地：Attr 是 earth、water、fire、wind，空串表示无属性；Pow 是威力（0–100），
// Left 是剩余回合。无属性时 Pow 和 Left 都没有意义。
type fieldState struct {
	Attr string
	Pow  int
	Left int
}

// FieldView 是场地的协议表示（docs/protocol.md 10.4、10.7）：attr 为 earth/water/fire/wind，无属性时是 none。
type FieldView struct {
	Attr string `json:"attr"`
	Pow  int    `json:"pow"`
	Left int    `json:"left"`
}

// setField 是场地属性精灵的施放（BATTLE_FieldAttChange）：属性、威力、回合整体换成新的；
// 调和的精灵（属性“无”）把场地设成无属性。返回一个 field 事件，告诉客户端场地变了。
func (b *Battle) setField(t gamedata.MagicTemplate) []Event {
	b.field = fieldState{}
	if attr := elemWire(t.Elem); attr != "" {
		b.field = fieldState{Attr: attr, Pow: t.Power, Left: t.Turns}
	}
	return []Event{b.fieldEvent()}
}

// tickField 是回合末的场地递减（battle.c 回合结算的收尾，状态递减之后）：场地有属性时剩余回合减一，
// 减到 0 恢复成无属性，返回一个 field 事件；无属性时什么都不做。
func (b *Battle) tickField() []Event {
	if b.field.Attr == "" {
		return nil
	}
	b.field.Left--
	if b.field.Left > 0 {
		return nil
	}
	b.field = fieldState{}
	return []Event{b.fieldEvent()}
}

// fieldEvent 是场地变化或恢复的事件（kind field）：attr 写 none 表示无属性。
func (b *Battle) fieldEvent() Event {
	f := FieldView{Attr: "none"}
	if b.field.Attr != "" {
		f = FieldView{Attr: b.field.Attr, Pow: b.field.Pow, Left: b.field.Left}
	}
	return Event{Kind: "field", Field: &f}
}

// fieldView 是 battle_start、battle_turn 等推送里的当前场地：无属性时返回 nil，字段省略。
func (b *Battle) fieldView() *FieldView {
	if b.field.Attr == "" {
		return nil
	}
	f := FieldView{Attr: b.field.Attr, Pow: b.field.Pow, Left: b.field.Left}
	return &f
}

// fieldPower 是单位在当前场地上的加成（BATTLE_FieldAttAdjust）：0.5 + 它在场地属性上的值 × 威力 × 0.0001 × 0.5。
// 无属性场地是 0.5，于是攻守两边相除得 1，伤害不变。
func (b *Battle) fieldPower(u *Unit) float64 {
	return b.fieldPowerOf(elemVector(u.Elem))
}

// fieldPowerOf 是场地加成的本体：v 是攻方（或守方）地水火风无五个元素的值，取场地属性那一格（STAGE-227 起攻击精灵
// 用换过属性的值调用，见 magic_fix.go）。
func (b *Battle) fieldPowerOf(v [5]float64) float64 {
	var i int // elemVector 的下标：0 地、1 水、2 火、3 风
	switch b.field.Attr {
	case "earth":
		i = 0
	case "water":
		i = 1
	case "fire":
		i = 2
	case "wind":
		i = 3
	default:
		return fieldBottom
	}
	return fieldBottom + v[i]*float64(b.field.Pow)*0.01*0.01*fieldPlus
}

// reverseUnit 是属性反转对一个单位的结算（BATTLE_MultiAttReverse 的单位部分）：反转标志翻转一次；
// 标志开时四属性互换（BATTLE_AttReverse：新地 = 旧火、新水 = 旧风、新火 = 旧地、新风 = 旧水），
// 标志关时属性不动。于是第一次用是互换，第二次只关标志、属性仍是换过的，第三次再互换换回来。
// 这是原版的行为，照做，不要“修正”。战斗结束不恢复（单位每场从角色快照重建）。
func (b *Battle) reverseUnit(u *Unit) Effect {
	u.Reversed = !u.Reversed
	if u.Reversed {
		e := u.Elem
		u.Elem = Elements{Earth: e.Fire, Water: e.Wind, Fire: e.Earth, Wind: e.Water}
		return b.stateEffect(u, []string{"reversed"}, nil)
	}
	return b.stateEffect(u, nil, []string{"reversed"})
}

// reverseTargets 是属性反转的目标：所选单位（活着的，提交时已校验过）；全体（极光）时是它那一方活着的单位。
func (b *Battle) reverseTargets(cmd Cmd, all bool) []*Unit {
	x := b.Unit(cmd.Target)
	if x == nil || !x.Alive() {
		return nil
	}
	if !all {
		return []*Unit{x}
	}
	var out []*Unit
	for _, y := range b.units {
		if y != nil && y.Alive() && y.Side() == x.Side() {
			out = append(out, y)
		}
	}
	return out
}

// statusList 是单位的状态标记（协议 10.2 的 status），属性反转的标志也在里面（reversed）。
func (u *Unit) statusList() []string {
	out := unitStatus(u) // 异常、防御次数（STAGE-199）在里面，再接上反转标志
	if u.Reversed {
		out = append(out, "reversed")
	}
	return out
}
