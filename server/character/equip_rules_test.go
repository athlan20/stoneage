package character

import (
	"errors"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// 物品编号（tools/gen-itemset.py 生成的表）：小斧头 0（斧，攻 +9 防 -3 敏 -3）、小棍棒 100（棍，攻 +4）、
// 粗杂的兜 800（头盔，防 +3）、大的棍棒 140（棍，要 25 级）、黄色双重手环 1200（饰品）、迷人香菇 1233（不能穿）。
const (
	itemAxe       = 0
	itemClub      = 100
	itemHelm      = 800
	itemBigClub   = 140
	itemBracelet  = 1200
	itemMushrooms = 1233
)

// itemErrCode 取出协议错误码；不是协议错误时返回空串。
func itemErrCode(err error) string {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return perr.Code
	}
	return ""
}

func TestEquipBonusCountsOnlyWornItems(t *testing.T) {
	items := []itemRec{{Slot: 2, ItemID: itemAxe, Count: 1}, {Slot: 5, ItemID: itemClub, Count: 1}}
	m := equipBonus(items)
	if m.Attack != 9 || m.Defence != -3 || m.Agility != -3 || m.Charm != 0 || m.Luck != 0 {
		t.Errorf("只算穿在身上的：%+v，应为攻击 9、防御 -3、敏捷 -3，魅力、运气为 0", m)
	}
}

// 小的枪（物品 200）穿在手上带魅力 +1（STAGE-61）；放在背包里不算。
func TestEquipBonusCharmOnlyWhenWorn(t *testing.T) {
	const itemGun = 200
	worn := equipBonus([]itemRec{{Slot: 2, ItemID: itemGun, Count: 1}})
	if worn.Charm != 1 {
		t.Errorf("穿着枪的魅力修正 = %d，应为 1", worn.Charm)
	}
	bag := equipBonus([]itemRec{{Slot: 5, ItemID: itemGun, Count: 1}})
	if bag.Charm != 0 {
		t.Errorf("背包里的枪不应加魅力，得到 %d", bag.Charm)
	}
}

func TestPlanMoveWearsIntoEmptySlot(t *testing.T) {
	items := []itemRec{{Slot: 5, ItemID: itemAxe, Count: 1}}
	next, err := planMove(items, 1, 5, 2)
	if err != nil {
		t.Fatalf("穿斧头应当成功：%v", err)
	}
	if len(next) != 1 || next[0].Slot != 2 || next[0].ItemID != itemAxe {
		t.Errorf("斧头应该在 2 号格，实际 %+v", next)
	}
}

func TestPlanMoveSwapsWornItemBackToBag(t *testing.T) {
	items := []itemRec{{Slot: 2, ItemID: itemAxe, Count: 1}, {Slot: 5, ItemID: itemClub, Count: 1}}
	next, err := planMove(items, 1, 5, 2)
	if err != nil {
		t.Fatalf("两件装备互换应当成功：%v", err)
	}
	if findItem(next, 2).ItemID != itemClub || findItem(next, 5).ItemID != itemAxe {
		t.Errorf("互换后 2 号应是小棍棒、5 号应是小斧头，实际 %+v", next)
	}
}

func TestPlanMoveRejectsWrongPlaceAndLevel(t *testing.T) {
	// 头盔不能放进手的格子。
	items := []itemRec{{Slot: 5, ItemID: itemHelm, Count: 1}}
	if _, err := planMove(items, 99, 5, 2); itemErrCode(err) != protocol.CodeNotEquipment {
		t.Errorf("头盔放进手的格子应回复 NOT_EQUIPMENT，实际 %v", err)
	}
	// 等级不够（大的棍棒要 25 级）。
	items = []itemRec{{Slot: 5, ItemID: itemBigClub, Count: 1}}
	if _, err := planMove(items, 10, 5, 2); itemErrCode(err) != protocol.CodeRequirementNotMet {
		t.Errorf("10 级穿 25 级的棍子应回复 REQUIREMENT_NOT_MET，实际 %v", err)
	}
	if _, err := planMove(items, 25, 5, 2); err != nil {
		t.Errorf("25 级应当能穿：%v", err)
	}
}

func TestPlanMoveRejectsStackAndEquipToEquip(t *testing.T) {
	stack := []itemRec{{Slot: 5, ItemID: itemAxe, Count: 2}}
	if _, err := planMove(stack, 1, 5, 2); itemErrCode(err) != protocol.CodeBadRequest {
		t.Errorf("两件叠在一格不能穿，应回复 BAD_REQUEST，实际 %v", err)
	}
	worn := []itemRec{{Slot: 2, ItemID: itemAxe, Count: 1}}
	if _, err := planMove(worn, 1, 2, 3); itemErrCode(err) != protocol.CodeBadRequest {
		t.Errorf("装备之间不能直接换，应回复 BAD_REQUEST，实际 %v", err)
	}
}

func TestPlanMoveRejectsEmptyAndBadSlots(t *testing.T) {
	items := []itemRec{{Slot: 5, ItemID: itemAxe, Count: 1}}
	if _, err := planMove(items, 1, 6, 7); itemErrCode(err) != protocol.CodeEmptySlot {
		t.Errorf("空格子应回复 EMPTY_SLOT，实际 %v", err)
	}
	if _, err := planMove(items, 1, 5, 5); itemErrCode(err) != protocol.CodeBadRequest {
		t.Errorf("同一个格子应回复 BAD_REQUEST，实际 %v", err)
	}
	if _, err := planMove(items, 1, 5, 20); itemErrCode(err) != protocol.CodeBadRequest {
		t.Errorf("格子 20 不存在，应回复 BAD_REQUEST，实际 %v", err)
	}
}

func TestPlanMoveDoesNotWearItemThatCannotGoBack(t *testing.T) {
	// 2 号格穿着斧头，5 号背包里是头盔；把斧头移到 5 号，头盔要穿到手上，不行。
	items := []itemRec{{Slot: 2, ItemID: itemAxe, Count: 1}, {Slot: 5, ItemID: itemHelm, Count: 1}}
	if _, err := planMove(items, 1, 2, 5); itemErrCode(err) != protocol.CodeNotEquipment {
		t.Errorf("换回背包时对面的头盔穿不到手上，应回复 NOT_EQUIPMENT，实际 %v", err)
	}
}

func TestWearTargetAccessoryOrder(t *testing.T) {
	bag := []itemRec{{Slot: 5, ItemID: itemBracelet, Count: 1}}
	if slot, err := wearTarget(bag, 5); err != nil || slot != gamedata.SlotAccessory1 {
		t.Errorf("饰品先放 3 号，实际 %d, %v", slot, err)
	}
	one := append([]itemRec{{Slot: gamedata.SlotAccessory1, ItemID: itemBracelet, Count: 1}}, bag...)
	if slot, err := wearTarget(one, 5); err != nil || slot != gamedata.SlotAccessory2 {
		t.Errorf("3 号满了放 4 号，实际 %d, %v", slot, err)
	}
	both := append([]itemRec{{Slot: gamedata.SlotAccessory2, ItemID: itemBracelet, Count: 1}}, one...)
	if slot, err := wearTarget(both, 5); err != nil || slot != gamedata.SlotAccessory2 {
		t.Errorf("两个都满时换 4 号，实际 %d, %v", slot, err)
	}
	mushrooms := []itemRec{{Slot: 5, ItemID: itemMushrooms, Count: 1}}
	if _, err := wearTarget(mushrooms, 5); itemErrCode(err) != protocol.CodeNotEquipment {
		t.Errorf("不能穿的物品应回复 NOT_EQUIPMENT，实际 %v", err)
	}
}

func TestUnequipTargetFirstEmptyBagSlot(t *testing.T) {
	items := []itemRec{{Slot: 2, ItemID: itemAxe, Count: 1}, {Slot: 5, ItemID: itemClub, Count: 1}}
	if slot, err := unequipTarget(items, 2); err != nil || slot != 6 {
		t.Errorf("卸下应放进第一个空背包格 6，实际 %d, %v", slot, err)
	}
	var full []itemRec
	full = append(full, itemRec{Slot: 2, ItemID: itemAxe, Count: 1})
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: itemClub, Count: 1})
	}
	if _, err := unequipTarget(full, 2); itemErrCode(err) != protocol.CodeBagFull {
		t.Errorf("背包满了应回复 BAG_FULL，实际 %v", err)
	}
	if _, err := unequipTarget(items, 5); itemErrCode(err) != protocol.CodeBadRequest {
		t.Errorf("背包里的东西不能用卸下，应回复 BAD_REQUEST，实际 %v", err)
	}
}

func TestPlanDropPartialAndWhole(t *testing.T) {
	items := []itemRec{{Slot: 5, ItemID: itemClub, Count: 3}}
	next, err := planDrop(items, 5, 2)
	if err != nil || findItem(next, 5).Count != 1 {
		t.Errorf("丢 2 个应剩 1 个，实际 %+v, %v", next, err)
	}
	next, err = planDrop(items, 5, 0)
	if err != nil || findItem(next, 5) != nil {
		t.Errorf("数量为 0 表示全部丢弃，实际 %+v, %v", next, err)
	}
	if _, err := planDrop(items, 5, 4); itemErrCode(err) != protocol.CodeBadRequest {
		t.Errorf("丢得比有的多应回复 BAD_REQUEST，实际 %v", err)
	}
	if _, err := planDrop(items, 6, 1); itemErrCode(err) != protocol.CodeEmptySlot {
		t.Errorf("空格子应回复 EMPTY_SLOT，实际 %v", err)
	}
}

func TestChangedSlots(t *testing.T) {
	before := []itemRec{{Slot: 2, ItemID: itemAxe, Count: 1}, {Slot: 5, ItemID: itemClub, Count: 1}}
	after := []itemRec{{Slot: 2, ItemID: itemClub, Count: 1}, {Slot: 5, ItemID: itemAxe, Count: 1}}
	if got := changedSlots(before, after); len(got) != 2 || got[0] != 2 || got[1] != 5 {
		t.Errorf("互换影响 2 和 5 号格，实际 %v", got)
	}
	if got := changedSlots(before, before); len(got) != 0 {
		t.Errorf("没有变化时应为空，实际 %v", got)
	}
}
