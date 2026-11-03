// 穿脱与整理的规则（STAGE-29，docs/protocol.md 5.25–5.28，docs/ui.md 背包页 B）：不碰数据库，
// 只算出改动后的物品列表和装备加成，写库和推送在 equip.go。

package character

import (
	"fmt"
	"slices"
	"sort"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// equipMods 是穿在身上的装备（0–4 号格）的加成合计（STAGE-61 起包含运气、魅力；STAGE-226 起包含抗魔）。
type equipMods struct {
	Attack, Defence, Agility int
	Luck, Charm              int
	// MagicDef 是抗魔加成（地水火风、QU），下标同 gamedata.MagicDefKeys；每件装备单项限幅 ±100（gamedata/equip_magicdef.go）。
	MagicDef [5]int
	// HP、MP、伤、吸（STAGE-298，精工累加进来；战斗里还没接入，docs/inlay.md 2.6）。
	HP, MP, OtherDamage, OtherDefc int
}

// equipBonus 算穿在身上的装备（0–4 号格）的加成合计：攻击、防御、敏捷，以及运气、魅力修正
// （item.c 的 ITEM_equipEffect，数值来自 tools/gen-itemset.py 的第 47、49 列）。地水火风的抗性本身不从装备推算，
// 装备只给抗魔加成 MagicDef（STAGE-226，原版 ITEM_MagicEquitWear）。
func equipBonus(items []itemRec) (m equipMods) {
	for _, it := range items {
		if !gamedata.IsEquipSlot(it.Slot) {
			continue
		}
		def, ok := gamedata.ItemByID(it.ItemID)
		if !ok {
			continue
		}
		def = effectiveTemplate(def, it.Inlay) // 镶过的装备用实例的数值（STAGE-298）
		m.Attack += def.Attack
		m.HP += def.HP
		m.MP += def.MP
		m.OtherDamage += def.OtherDamage
		m.OtherDefc += def.OtherDefc
		m.Defence += def.Defence
		m.Agility += def.Agility
		m.Luck += def.Luck
		m.Charm += def.Charm
		for i, v := range def.EquipMagicDef() {
			m.MagicDef[i] += v
		}
	}
	return m
}

// findItem 返回格子上的物品；没有就返回 nil。
func findItem(items []itemRec, slot int) *itemRec {
	for i := range items {
		if items[i].Slot == slot {
			return &items[i]
		}
	}
	return nil
}

// validItemSlot 检查格子编号是不是 0–19。
func validItemSlot(slot int) error {
	if slot < MinItemSlot || slot > MaxItemSlot {
		return protocol.Errorf(protocol.CodeBadRequest, "格子编号要在 %d–%d 之间", MinItemSlot, MaxItemSlot)
	}
	return nil
}

// checkWear 检查这件物品能不能穿在 slot 上：类型要能穿这个部位，等级要够。
func checkWear(it itemRec, slot, level int) error {
	def, ok := gamedata.ItemByID(it.ItemID)
	if !ok || !slices.Contains(gamedata.EquipPlaces(def.Type), slot) {
		return protocol.Errorf(protocol.CodeNotEquipment, "%s 不能穿在这个部位", itemName(it.ItemID))
	}
	if def.Level > level {
		return protocol.Errorf(protocol.CodeRequirementNotMet, "%s 需要 %d 级，现在 %d 级", def.Name, def.Level, level)
	}
	return nil
}

// itemName 返回物品的名字，表里没有时返回编号（不会发生：发物品时已经校验过编号）。
func itemName(itemID int) string {
	if def, ok := gamedata.ItemByID(itemID); ok {
		return def.Name
	}
	return fmt.Sprintf("物品 %d", itemID)
}

// planMove 把 from 的物品移到 to：to 为空就移过去，有东西就互换。
// 放进装备格要检查数量（只能一件）、类型、部位和等级；装备格里的东西换到背包时，背包里的那件要能穿在 from 上。
// 装备格之间不能直接换（先卸下）。返回改后的完整列表（按格子从小到大）。
func planMove(items []itemRec, level, from, to int) ([]itemRec, error) {
	if err := validItemSlot(from); err != nil {
		return nil, err
	}
	if err := validItemSlot(to); err != nil {
		return nil, err
	}
	if from == to {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "源格子和目标格子是同一个")
	}
	src := findItem(items, from)
	if src == nil {
		return nil, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子没有物品", from)
	}
	fromEquip, toEquip := gamedata.IsEquipSlot(from), gamedata.IsEquipSlot(to)
	if fromEquip && toEquip {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "装备之间不能直接换，先卸下再穿")
	}
	if toEquip {
		if src.Count != 1 {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "一次只能穿一件，这一格有 %d 个", src.Count)
		}
		if err := checkWear(*src, to, level); err != nil {
			return nil, err
		}
	}
	next := make([]itemRec, 0, len(items))
	for _, it := range items {
		if it.Slot != from && it.Slot != to {
			next = append(next, it)
		}
	}
	moved := *src
	moved.Slot = to
	next = append(next, moved)
	if dst := findItem(items, to); dst != nil {
		if fromEquip {
			if err := checkWear(*dst, from, level); err != nil {
				return nil, err
			}
		}
		back := *dst
		back.Slot = from
		next = append(next, back)
	}
	sort.Slice(next, func(i, j int) bool { return next[i].Slot < next[j].Slot })
	return next, nil
}

// wearTarget 返回穿这件物品时应该放进的部位：先放空的，都满了放最后一个（饰品是 4 号）。
// 只能用于背包里的物品；不能穿的返回 NOT_EQUIPMENT。
func wearTarget(items []itemRec, from int) (int, error) {
	if err := validItemSlot(from); err != nil {
		return 0, err
	}
	if gamedata.IsEquipSlot(from) {
		return 0, protocol.Errorf(protocol.CodeBadRequest, "%d 号格子的装备已经穿在身上", from)
	}
	src := findItem(items, from)
	if src == nil {
		return 0, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子没有物品", from)
	}
	def, ok := gamedata.ItemByID(src.ItemID)
	if !ok {
		return 0, protocol.Errorf(protocol.CodeNotEquipment, "%s 不能穿戴", itemName(src.ItemID))
	}
	places := gamedata.EquipPlaces(def.Type)
	if len(places) == 0 {
		return 0, protocol.Errorf(protocol.CodeNotEquipment, "%s 不能穿戴", itemName(src.ItemID))
	}
	for _, p := range places {
		if findItem(items, p) == nil {
			return p, nil
		}
	}
	return places[len(places)-1], nil
}

// unequipTarget 返回卸下时应该放进的背包格：第一个空的背包格；背包满了返回 BAG_FULL。
func unequipTarget(items []itemRec, from int) (int, error) {
	if err := validItemSlot(from); err != nil {
		return 0, err
	}
	if !gamedata.IsEquipSlot(from) {
		return 0, protocol.Errorf(protocol.CodeBadRequest, "只能卸下装备格（0–4）里的东西")
	}
	if findItem(items, from) == nil {
		return 0, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子没有装备", from)
	}
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		if findItem(items, slot) == nil {
			return slot, nil
		}
	}
	return 0, protocol.Errorf(protocol.CodeBagFull, "背包满了（15 格都有物品），先丢掉或移走一件")
}

// planDrop 丢弃格子上的 count 个物品（count 为 0 表示全部）。丢弃就是销毁，没有局部地图，不会掉在地上。
func planDrop(items []itemRec, slot, count int) ([]itemRec, error) {
	if err := validItemSlot(slot); err != nil {
		return nil, err
	}
	src := findItem(items, slot)
	if src == nil {
		return nil, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子没有物品", slot)
	}
	if count == 0 {
		count = src.Count
	}
	if count < 1 || count > src.Count {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "丢弃数量要在 1 到 %d 之间", src.Count)
	}
	next := make([]itemRec, 0, len(items))
	for _, it := range items {
		if it.Slot != slot {
			next = append(next, it)
			continue
		}
		if left := it.Count - count; left > 0 {
			it.Count = left
			next = append(next, it)
		}
	}
	return next, nil
}

// changedSlots 返回改动前后内容不同的格子（用来决定写库的范围）。
func changedSlots(before, after []itemRec) []int {
	var out []int
	for slot := MinItemSlot; slot <= MaxItemSlot; slot++ {
		b, a := findItem(before, slot), findItem(after, slot)
		if (b == nil) != (a == nil) || (b != nil && (b.ItemID != a.ItemID || b.Count != a.Count || b.Merged != a.Merged || b.Durability != a.Durability || b.CrushLevel != a.CrushLevel || !sameInlay(b.Inlay, a.Inlay))) {
			out = append(out, slot)
		}
	}
	return out
}
