package character

// 任务脚本的交易（STAGE-90，对照 npc_exchangeman.c 的 NPC_AcceptDel / NPC_EventAdd）：先在副本上算出
// 背包、金币和任务进度的结果，检查都通过再一个事务里落库、改内存、推送（要么都变，要么都不变）。

import (
	"context"
	"github.com/athlan20/stoneage/server/battle"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// eventResult 是一趟交易后的完整状态：背包（按槽位排序）、金币、魅力和事件进度（任务进度和不对应任务的事件）。
// refund 是接任务时记下的退款（nil 表示不动，见 npcevent_refund.go）。
type eventResult struct {
	items  []itemRec
	gold   int64
	charm  int
	prog   progress
	pets   []petRec // 宠物的新列表（STAGE-197）：每个调用方都要给，不改宠物时给 r.Pets
	notes  []string // 交易成功后消息栏的提示（交出、拿到宠物）
	refund *refundWrite
}

// eventPlan 是背包、金币和宠物的计算结果（任务进度和魅力由脚本的 EndSetFlg 等步骤另算）。taken 和 takenGold 是
// DelItem、DelStone 实际收走的东西，接任务时记下，放弃时退回。
type eventPlan struct {
	items     []itemRec
	gold      int64
	taken     []refundItem
	takenGold int64
	pets      []petRec
	notes     []string
}

// eventDelItems 是这一块要收的道具：DelItem 直接写的，或者 DelItem:EVDEL 里命中的那一组（STAGE-197）。
func eventDelItems(blk *gamedata.EventBlock, group int) []gamedata.ItemCount {
	if blk.DelItemEV != nil {
		if group >= 0 && group < len(blk.DelItemEV) {
			return blk.DelItemEV[group]
		}
		return nil
	}
	return blk.DelItem
}

// planEvent 按原版顺序算一趟交易：先删道具（DelItem），再算空位，再查金币（DelStone、GetStone），再收送宠物
// （DelPet、GetPet，见 npcevent_pet_act.go），最后给随机道具和道具（GetRandItem、GetItem）。空位不够给的数量
// （按删完之后算）、金币不够扣或装不下、宠物收不成或栏位满时整笔不做，返回对应的台词：ItemFullMsg（NPC_ItemFullCheck）、
// StoneLessMsg、StoneFullMsg、PetFullMsg（宠物收不成时没有台词）。
func planEvent(r *record, blk *gamedata.EventBlock, sel eventSel, rnd func(n int) int) (eventPlan, string, bool) {
	// STAGE-324：GetItem 引用了原版也不存在的道具时，原版 NPC_EventAddItem 的 ITEM_makeItemAndRegist 返回 -1，
	// 整个块交易失败、什么都不改（也不收 DelItem）。这里照原版处理，失败时不说话。
	if blk.MissingItem {
		return eventPlan{}, "", false
	}
	items := cloneItems(r.Items)
	var taken []refundItem
	for _, d := range eventDelItems(blk, sel.group) {
		before := countItem(items, d.ID)
		items = removeItems(items, d.ID, d.N, d.N == 0)
		if n := before - countItem(items, d.ID); n > 0 {
			taken = append(taken, refundItem{ID: d.ID, N: n})
		}
	}
	need := 0
	for _, g := range blk.GetItem {
		need += g.N
	}
	if len(blk.GetRand) > 0 {
		need++
	}
	if need > freeBagSlots(items) {
		return eventPlan{}, blk.Msg.ItemFull, false
	}
	gold := r.Gold
	if blk.DelStone != nil && r.Gold-int64(stoneCost(blk.DelStone, r.Level)) < 0 {
		return eventPlan{}, blk.Msg.StoneLess, false
	}
	if blk.GetStone > 0 && r.Gold+int64(blk.GetStone) >= battle.MaxHaveGold {
		return eventPlan{}, blk.Msg.StoneFull, false
	}
	pets, notes, petFail, ok := planPets(r, blk, sel, rnd)
	if !ok {
		return eventPlan{}, petFail, false
	}
	var takenGold int64
	if blk.DelStone != nil {
		takenGold = int64(stoneCost(blk.DelStone, r.Level))
		gold -= takenGold
	}
	gold += int64(blk.GetStone)
	if len(blk.GetRand) > 0 {
		items = addItem(items, blk.GetRand[rnd(len(blk.GetRand))])
	}
	for _, g := range blk.GetItem {
		for k := 0; k < g.N; k++ {
			items = addItem(items, g.ID)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Slot < items[j].Slot })
	return eventPlan{items: items, gold: gold, taken: taken, takenGold: takenGold, pets: pets, notes: notes}, "", true
}

// cloneItems 复制背包并按槽位排序（交易在副本上算）。
func cloneItems(items []itemRec) []itemRec {
	out := append([]itemRec(nil), items...)
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// removeItems 删掉背包里编号为 id 的道具：all 为真删光，否则删 n 件（堆叠数量相加），从小槽位开始
// （同 NPC_EventDelItem）。背包里不够时删到没有为止。
func removeItems(items []itemRec, id, n int, all bool) []itemRec {
	out := make([]itemRec, 0, len(items))
	for _, it := range items {
		if it.ItemID != id || (!all && n <= 0) {
			out = append(out, it)
			continue
		}
		if all {
			continue
		}
		take := it.Count
		if take > n {
			take = n
		}
		n -= take
		if it.Count -= take; it.Count > 0 {
			out = append(out, it)
		}
	}
	return out
}

// freeBagSlots 数背包里的空位。
func freeBagSlots(items []itemRec) int {
	taken := make(map[int]bool, len(items))
	for _, it := range items {
		taken[it.Slot] = true
	}
	n := 0
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		if !taken[slot] {
			n++
		}
	}
	return n
}

// addItem 在最小的空位上放一件道具（原版每件道具单独占一格，不堆叠）。调用前已经确认有空位。
func addItem(items []itemRec, id int) []itemRec {
	taken := make(map[int]bool, len(items))
	for _, it := range items {
		taken[it.Slot] = true
	}
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		if !taken[slot] {
			dur, maxDur := newItemDurability(id)
			return append(items, itemRec{Slot: slot, ItemID: id, Count: 1, Durability: dur, MaxDurability: maxDur})
		}
	}
	return items
}

// writeItemDiff 把背包的变化写进 character_items（同一个事务）：槽位没了就删，变了就改，新的就插入。
func writeItemDiff(ctx context.Context, tx pgx.Tx, charID int64, before, after []itemRec) error {
	oldBy := make(map[int]itemRec, len(before))
	for _, it := range before {
		oldBy[it.Slot] = it
	}
	newBy := make(map[int]itemRec, len(after))
	for _, it := range after {
		newBy[it.Slot] = it
	}
	for slot := range oldBy {
		if _, ok := newBy[slot]; ok {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM character_items WHERE character_id = $1 AND slot = $2`, charID, slot); err != nil {
			return err
		}
	}
	for slot, it := range newBy {
		prev, ok := oldBy[slot]
		var err error
		switch {
		case !ok:
			_, err = tx.Exec(ctx, `INSERT INTO character_items (character_id, slot, item_id, count, merged, durability, max_durability, crush_level, inlay) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				charID, slot, it.ItemID, it.Count, it.Merged, it.Durability, it.MaxDurability, it.CrushLevel, inlayJSON(it.Inlay))
		case prev != it:
			_, err = tx.Exec(ctx, `UPDATE character_items SET item_id = $3, count = $4, merged = $5, durability = $6, max_durability = $7, crush_level = $8, inlay = $9 WHERE character_id = $1 AND slot = $2`,
				charID, slot, it.ItemID, it.Count, it.Merged, it.Durability, it.MaxDurability, it.CrushLevel, inlayJSON(it.Inlay))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func sameStates(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for no, st := range a {
		if b[no] != st {
			return false
		}
	}
	return true
}

func sameItems(a, b []itemRec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// commitEvent 把一趟交易落库（背包、任务进度，一个事务），成功后才改内存并推送变化。
// 数据库失败时什么都不变，返回内部错误。调用时要持有 p.mu。
func (s *Service) commitEvent(ctx context.Context, l *Live, r *record, next eventResult) *protocol.Error {
	before := progressOf(r)
	if sameItems(r.Items, next.items) && r.Gold == next.gold && r.Charm == next.charm && samePets(r.Pets, next.pets) &&
		sameStates(before.quests, next.prog.quests) && sameStates(before.events, next.prog.events) && next.refund == nil {
		return nil // 什么都没变（比如只说一句话的 MESSAGE 块）：不开事务
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	defer tx.Rollback(ctx)
	if err := writeItemDiff(ctx, tx, r.ID, r.Items, next.items); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err := writeStateDiff(ctx, tx, questTable, r.ID, before.quests, next.prog.quests); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err := writeStateDiff(ctx, tx, eventTable, r.ID, before.events, next.prog.events); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err := writeRefundDiff(ctx, tx, r.ID, next.refund); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err := writePetDiff(ctx, tx, r.ID, r.Pets, next.pets); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err := tx.Commit(ctx); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	oldQuests, oldPets := r.Quests, r.Pets
	itemsChanged := !sameItems(r.Items, next.items)
	statusChanged := r.Gold != next.gold || r.Charm != next.charm
	r.Items, r.Gold, r.Charm = next.items, next.gold, next.charm
	r.Pets = next.pets
	r.Quests, r.EventFlags = stateRecs(next.prog.quests), stateRecs(next.prog.events)
	if itemsChanged {
		l.p.pushAll("inventory_changed", r.inventoryView())
	}
	if statusChanged {
		l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	}
	if changed := changedQuests(oldQuests, r.Quests); len(changed) > 0 {
		l.p.pushAll("quests_changed", map[string]any{"quests": r.questsView(), "changed": changed})
	}
	if !samePets(oldPets, r.Pets) {
		l.p.pushAll("pets_changed", r.petsView())
	}
	for _, text := range next.notes { // 交出、拿到宠物（STAGE-197）：消息栏的系统消息
		l.p.pushAll("notice", map[string]string{"level": "info", "text": text})
	}
	return nil
}
