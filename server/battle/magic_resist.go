// STAGE-228：魔法防御（魔抗）与冰、电、火之精灵的挂法和魔抗的效果，对照原版 gmsv/battle/battle_magic.c 的
// MAGIC_MagicStatusChange_Battle、BATTLE_MultiMagicStatusChange，battle_event.c 的 MagicTbl，battle.c 的
// BATTLE_MagicStatusSeq，以及 BATTLE_MultiAttMagic 里对被打方抗性的加成（_MAGIC_DEFMAGICATT）。
// 挂载不掷骰、不看异常状态，只与其他魔抗系状态互斥（一个单位同时只带一种）；回合数照精灵的 turn（不 +1），
// 和其他状态一起在回合末递减。魔抗的加成只在攻击精灵的伤害里生效（magic_fix.go 的 magicPower 用 resistOf）；
// 火、电、冰抗只占槽、不改数值（原版如此，见 docs/magic-resist.md 的裁定）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// resistField 返回魔抗系状态的剩余回合字段，key 是协议 10.2 的状态名；不是魔抗系的返回 nil。
func (s *StatusSet) resistField(key string) *int {
	switch key {
	case "magic_resist":
		return &s.ResistMagic
	case "fire_resist":
		return &s.ResistFire
	case "thunder_resist":
		return &s.ResistThunder
	case "ice_resist":
		return &s.ResistIce
	}
	return nil
}

// hasResist 表示已经带着任何一种魔抗系状态（原版 BATTLE_MultiMagicStatusChange 检查 MagicTbl[1..5] 是否有值）。
func (s StatusSet) hasResist() bool {
	return s.ResistMagic > 0 || s.ResistFire > 0 || s.ResistThunder > 0 || s.ResistIce > 0
}

// resistOf 是守方这一属性的魔法抗性（原版 BATTLE_MultiAttMagic 的 def_magic_resist）：带着魔抗时，抗性大于 0 的
// 按 抗性 + 抗性 × 数值 / 100 加成（原版是 float 算完再存进 int，截断，这里整数算得到同样的结果）。
func resistOf(target *Unit, f int) int {
	r := target.MagicResist[f]
	if target.Status.ResistMagic > 0 && r > 0 {
		r += r * target.Status.ResistPct / 100
	}
	return r
}

// resistMagic 是魔抗系精灵对一个己方单位的结算：已经带着任何魔抗系状态就不中（记 miss，状态不变）；
// 否则挂上，回合数是精灵的 turn，加成百分比是精灵的威力（原版的第 3 段数值）。
func (b *Battle) resistMagic(target *Unit, t gamedata.MagicTemplate, ev *Event) {
	field := target.Status.resistField(t.Status)
	if field == nil || target.Status.hasResist() {
		b.effect(ev, target, "miss", 0)
		return
	}
	*field = t.Turns
	target.Status.ResistPct = t.Power
	ev.Effects = append(ev.Effects, b.stateEffect(target, []string{t.Status}, nil))
}
