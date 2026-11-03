package character

// STAGE-207：加工、料理的合成（协议 5.55，docs/synthesis.md）。这里是不碰数据库的部分：参数检查、材料检查、
// 合成的结果、背包的改动。写库和推送在 merge.go。

import (
	"errors"
	"fmt"
	"sort"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/synthesis"
)

// 材料件数（原版 cnt > 1 的下限；上限取背包格数 15，docs/synthesis.md 第 7.2 节）。
const (
	mergeMinItems = 2
	mergeMaxItems = 15
)

// 协议 5.55 的 message，原文照原版。
const (
	mergeCooldownMsg = "合成料理过於频繁，休息一下比较好哟。"
	mergeIllegalMsg  = "非法的合成方法"
)

// mergeResult 是一次合成的结果（协议 5.55 的 result，不含背包）。
type mergeResult struct {
	Kind    string // created、fallback、cooldown、illegal
	Placed  bool   // 有产物放进背包（illegal 没有）
	Product int    // 产物（或还回的材料）的物品编号
	Merged  bool   // 合成品标记：只有 created 为真（原版 ITEM_MERGEFLG 对还回的复制品也置位，这里按任务书只给配方产物）
}

// checkMergeSlots 检查 items 参数：必填，件数 2–15，格号在背包（5–19）之内且不重复。
func checkMergeSlots(items *[]int) ([]int, *protocol.Error) {
	if items == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 items 字段，是背包格号（5–19）的列表，2–15 件")
	}
	slots := *items
	if n := len(slots); n < mergeMinItems || n > mergeMaxItems {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "合成要 2–15 件材料，现在是 %d 件", n)
	}
	seen := make(map[int]bool, len(slots))
	for _, slot := range slots {
		if slot < firstBagSlot || slot > MaxItemSlot {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "items 里的格号要在 5–19 之间（背包格），%d 不在范围内", slot)
		}
		if seen[slot] {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "items 里的格号 %d 重复了", slot)
		}
		seen[slot] = true
	}
	return slots, nil
}

// firstEmptyBagSlot 返回背包（5–19）里第一个空格，没有时返回 -1。
func firstEmptyBagSlot(items []itemRec) int {
	taken := [MaxItemSlot + 1]bool{}
	for _, it := range items {
		if it.Slot >= 0 && it.Slot <= MaxItemSlot {
			taken[it.Slot] = true
		}
	}
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		if !taken[slot] {
			return slot
		}
	}
	return -1
}

// materialIDs 检查要合成的格子：每格都要有物品，而且能作材料（CanMergeFrom）。按请求的顺序返回物品编号。
func materialIDs(items []itemRec, slots []int) ([]int, *protocol.Error) {
	ids := make([]int, 0, len(slots))
	for _, slot := range slots {
		it := findItem(items, slot)
		if it == nil {
			return nil, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子是空的，没有东西可以合成", slot)
		}
		def, ok := gamedata.ItemByID(it.ItemID)
		if !ok {
			return nil, protocol.Errorf(protocol.CodeItemUnusable, "不认识的物品编号 %d（%d 号格）", it.ItemID, slot)
		}
		if !def.CanMergeFrom {
			return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s（%d 号格）不能作合成的材料", def.Name, slot)
		}
		ids = append(ids, it.ItemID)
	}
	return ids, nil
}

// runMerge 决定合成的结果（不扣料、不放入）。mats 是参与材料的物品编号（已检查都能作材料），
// petTemplate 是宠物的模板号，cooling 是冷却中。冷却中还回一件随机材料（原版 item_gen.c:747）；
// 否则交给引擎：配方匹配成功是 created，5 次都失败是 fallback（还回一件随机材料），食物与道具混合是 illegal。
func runMerge(mats []int, petTemplate int, cooling bool, rnd synthesis.Rand) (mergeResult, error) {
	if cooling {
		return mergeResult{Kind: "cooldown", Placed: true, Product: mats[rnd(len(mats))]}, nil
	}
	out, err := synthesis.NewEngine().Merge(mats, petTemplate, rnd)
	switch {
	case errors.Is(err, synthesis.ErrMixed):
		return mergeResult{Kind: "illegal"}, nil
	case err != nil:
		return mergeResult{}, err
	case out.OK:
		return mergeResult{Kind: "created", Placed: true, Product: out.RecipeID, Merged: true}, nil
	default:
		return mergeResult{Kind: "fallback", Placed: true, Product: mats[rnd(len(mats))]}, nil
	}
}

// consumeAndPlace 按合成的结果改背包（不改动入参）：参与的格子各扣 1 件，扣光的格子空出来；
// 有产物时放进扣料之后的第一个空格（原版 CHAR_addItemSpecificItemIndex 在扣料之后放入）。
// 返回新的背包和放进去的那件（没有产物时为 nil）。
func consumeAndPlace(items []itemRec, slots []int, res mergeResult) ([]itemRec, *itemRec, error) {
	used := make(map[int]bool, len(slots))
	for _, slot := range slots {
		used[slot] = true
	}
	next := make([]itemRec, 0, len(items)+1)
	for _, it := range items {
		if used[it.Slot] {
			if it.Count > 1 {
				it.Count--
				next = append(next, it)
			}
			continue
		}
		next = append(next, it)
	}
	if !res.Placed {
		return next, nil, nil
	}
	slot := firstEmptyBagSlot(next)
	if slot < 0 { // 调用方已经检查过有空格，扣料只会多出空格，走到这里是程序错误
		return nil, nil, fmt.Errorf("合成放入产物时背包没有空格")
	}
	dur, maxDur := newItemDurability(res.Product)
	placed := itemRec{Slot: slot, ItemID: res.Product, Count: 1, Merged: res.Merged, Durability: dur, MaxDurability: maxDur}
	next = append(next, placed)
	sort.Slice(next, func(i, j int) bool { return next[i].Slot < next[j].Slot })
	return next, &placed, nil
}

// mergeView 是协议 5.55 成功时的 data。
func mergeView(res mergeResult, placed *itemRec, slots []int) map[string]any {
	out := map[string]any{"result": res.Kind, "consumed": slots}
	if placed != nil {
		def, _ := gamedata.ItemByID(placed.ItemID)
		out["item"] = map[string]any{"slot": placed.Slot, "item_id": placed.ItemID, "name": def.Name}
	}
	switch res.Kind {
	case "cooldown":
		out["message"] = mergeCooldownMsg
	case "illegal":
		out["message"] = mergeIllegalMsg
	}
	return out
}
