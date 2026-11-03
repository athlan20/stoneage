package battle

import (
	"hash/fnv"
	"math/rand"
)

// STAGE-209：装备耐久接入战斗（原版 BATTLE_ItemCrushSeq，docs/durability.md）。
// physHurt 里伤害落地且 dmg>0 时调用：防守方（人物本人）的一件防具和攻击方（人物本人）的武器各扣一次耐久。
// 宠物和怪物不损坏，魔法伤害不调用（原版如此）。耐久用独立随机源，见 Battle.crushRnd。

// newCrushRand 按战斗编号播种一个装备耐久专用的随机源，同一场战斗结果可重现。
func newCrushRand(id string) func(n int) int {
	h := fnv.New64a()
	h.Write([]byte(id))
	return rand.New(rand.NewSource(int64(h.Sum64()))).Intn
}

// copyEquips 复制装备快照，战斗里扣耐久不改 SideInput 本身。
func copyEquips(src [crushSlotCount]*EquipInput) [crushSlotCount]*EquipInput {
	var out [crushSlotCount]*EquipInput
	for i, e := range src {
		if e != nil {
			c := *e
			out[i] = &c
		}
	}
	return out
}

// equipped 报告人物某栏有没有还在的装备（耗尽删除的不算）。
func equipped(u *Unit) func(slot int) bool {
	return func(slot int) bool { return u.crush[slot] != nil && !u.crush[slot].Gone }
}

// itemCrushSeq 先扣防守方的一件防具，再扣攻击方的武器（原版先防守方后攻击方，同一个伤害值）。
// 结算的提示挂在事件的 Crush 上。
func (b *Battle) itemCrushSeq(attacker, hit *Unit, dmg int, ev *Event) {
	if hit != nil && hit.crushOwner {
		if slot := pickDefenseSlot(b.crushRnd, equipped(hit)); slot >= 0 {
			ev.Crush = append(ev.Crush, applyCrush(hit.crush[slot], slot, dmg, b.crushRnd)...)
		}
	}
	if attacker != nil && attacker.crushOwner {
		if slot := pickAttackSlot(equipped(attacker)); slot >= 0 {
			ev.Crush = append(ev.Crush, applyCrush(attacker.crush[slot], slot, dmg, b.crushRnd)...)
		}
	}
}

// EquipState 是战斗结束时人物装备栏的耐久（写回背包用）。Gone 为真的是本场耗尽的；没有人物本人时全为 nil。
func (b *Battle) EquipState() [crushSlotCount]*EquipInput {
	if u := b.PlayerUnit(); u != nil && u.crushOwner {
		return copyEquips(u.crush)
	}
	return [crushSlotCount]*EquipInput{}
}

// EquipStateFor 是真人 no 的装备耐久（真人同场时每个队员各自写回；离场的也按离场时的状态给出）。
func (b *Battle) EquipStateFor(no int) [crushSlotCount]*EquipInput {
	if u := b.FinalHuman(no); u != nil && u.crushOwner {
		return copyEquips(u.crush)
	}
	return [crushSlotCount]*EquipInput{}
}
