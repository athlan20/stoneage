package character

import (
	"context"
	"fmt"
	"github.com/athlan20/stoneage/server/battle"
	"strconv"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

const shopStageSellQty = "sell_quantity"
const shopStageConfirm = "confirm"

type shopQuote struct {
	direction                 string
	itemID, slot, unit, count int
	total                     int64
	expires                   time.Time
	items                     []itemRec
	revision                  uint64
	note                      string // 按空位截断时的说明（STAGE-192），确认页写在交易行前面
}

func (q *shopQuote) view() map[string]any {
	return map[string]any{"direction": q.direction, "item_id": q.itemID, "slot": q.slot,
		"name": itemName(q.itemID), "unit_price": q.unit, "count": q.count, "total": q.total}
}

func (s *Service) quoteShop(l *Live, r *record, w *npcWindow, text string) *protocol.Error {
	sh := w.shop
	if sh.stage != shopStageBuyQty && sh.stage != shopStageSellQty {
		return protocol.Errorf(protocol.CodeWrongState, "当前窗口不接受数量")
	}
	n, err := strconv.Atoi(text)
	max := 15
	if sh.stage == shopStageSellQty {
		max = sh.selected.Count
	}
	if err != nil || n < 1 || n > max {
		return protocol.Errorf(protocol.CodeBadRequest, "数量必须为 1–%d 的整数", max)
	}
	shop := gamedata.NpcShopOf(w.npc)
	q := &shopQuote{direction: "buy", itemID: sh.itemID, slot: -1, count: n,
		expires: s.opts.Now().Add(60 * time.Second), items: append([]itemRec(nil), r.Items...), revision: l.p.inventoryRevision}
	if sh.stage == shopStageSellQty {
		if it := findItem(r.Items, sh.selected.Slot); it == nil || *it != sh.selected {
			return protocol.Errorf(protocol.CodeWrongState, "物品已变化，重新选择")
		}
		q.direction, q.itemID, q.slot = "sell", sh.selected.ItemID, sh.selected.Slot
		q.unit, _ = shop.SellPrice(q.itemID)
	} else {
		tmpl, ok := gamedata.ItemByID(q.itemID)
		if !ok {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "商品已不存在")
		}
		q.unit = shop.UnitPrice(q.itemID, tmpl.Cost)
	}
	if q.direction == "buy" {
		// 按空位截断（STAGE-192，原版 npc_itemshop.c）：空位不够就买得下几件是几件，一格都没有才拒绝。
		// 报价时就截断，确认页的 count、total 是截断后的数；提交时背包没变（见 sameShopItems），空位数不会更少。
		free := freeBagSlots(r.Items)
		if free == 0 {
			return protocol.Errorf(protocol.CodeBagFull, "%s", shop.ItemFull)
		}
		if n > free {
			q.count = free
			q.note = fmt.Sprintf("背包只剩 %d 格，按空位买 %d 件", free, free)
		}
	}
	q.total = int64(q.unit) * int64(q.count)
	if q.unit <= 0 || q.total > battle.MaxHaveGold {
		return protocol.Errorf(protocol.CodeBadRequest, "交易金额超出范围")
	}
	sh.quote, sh.stage = q, shopStageConfirm
	npc, _ := s.npcs.ByID(w.npc)
	s.shopMenuView(l, w, npc, "请核对交易", nil)
	return nil
}

func (s *Service) handleNpcConfirm(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		WindowID string `json:"window_id"`
		Confirm  *bool  `json:"confirm"`
	}
	if e := req.Decode(&in); e != nil {
		return nil, e
	}
	if in.Confirm == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 confirm 布尔值")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var perr *protocol.Error
	err = l.Operate(func(r *record) {
		if perr = l.p.checkWindow(in.WindowID); perr != nil {
			return
		}
		if perr = inBattle(l); perr != nil {
			return
		}
		w := l.p.npcWindow
		if w.shop == nil || w.shop.quote == nil {
			perr = protocol.Errorf(protocol.CodeWrongState, "没有待确认的报价")
			return
		}
		q := w.shop.quote
		npc, _ := s.npcs.ByID(w.npc)
		if !*in.Confirm {
			w.shop.quote = nil
			if q.direction == "sell" {
				perr = s.openSellList(l, r, w, npc, gamedata.NpcShopOf(w.npc))
			} else {
				w.shop.stage = shopStageMenu
				perr = s.shopChoose(ctx, l, r, w, 1)
			}
			return
		}
		if !s.opts.Now().Before(q.expires) || npc.Floor != r.Floor || q.revision != l.p.inventoryRevision || !sameShopItems(q.items, r.Items) {
			perr = protocol.Errorf(protocol.CodeWrongState, "报价已过期或物品已变化，请重新选择")
			return
		}
		perr = s.commitShop(ctx, l, r, w, q)
	})
	if err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{}, nil
}

// 事务先锁库中物品，再校验快照；成功落库后才更新内存和推送。
func (s *Service) commitShop(ctx context.Context, l *Live, r *record, w *npcWindow, q *shopQuote) *protocol.Error {
	shop := gamedata.NpcShopOf(w.npc)
	next := append([]itemRec(nil), r.Items...)
	gold := r.Gold
	if q.direction == "buy" {
		tmpl, ok := gamedata.ItemByID(q.itemID)
		if !ok || shop.UnitPrice(q.itemID, tmpl.Cost) != q.unit {
			return protocol.Errorf(protocol.CodeWrongState, "价格已变化")
		}
		if tmpl.Level > r.Level {
			return protocol.Errorf(protocol.CodeRequirementNotMet, "%s", shop.Level)
		}
		if gold < q.total {
			return protocol.Errorf(protocol.CodeNotEnoughGold, "金币不足")
		}
		free := []int{}
		for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
			if findItem(next, slot) == nil {
				free = append(free, slot)
			}
		}
		if len(free) < q.count {
			return protocol.Errorf(protocol.CodeBagFull, "背包空格不足，需要 %d 格", q.count)
		}
		for _, slot := range free[:q.count] {
			dur, maxDur := newItemDurability(q.itemID)
			next = append(next, itemRec{Slot: slot, ItemID: q.itemID, Count: 1, Durability: dur, MaxDurability: maxDur})
		}
		gold -= q.total
	} else {
		unit, ok := shop.SellPrice(q.itemID)
		if !ok || unit != q.unit {
			return protocol.Errorf(protocol.CodeWrongState, "回收价格已变化")
		}
		if gold+q.total > battle.MaxHaveGold {
			return protocol.Errorf(protocol.CodeGoldFull, "金币达到上限")
		}
		it := findItem(next, q.slot)
		if it == nil || it.ItemID != q.itemID || it.Count < q.count {
			return protocol.Errorf(protocol.CodeWrongState, "物品已变化")
		}
		it.Count -= q.count
		if it.Count == 0 {
			out := next[:0]
			for _, v := range next {
				if v.Slot != q.slot {
					out = append(out, v)
				}
			}
			next = out
		}
		gold += q.total
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	defer tx.Rollback(ctx)
	items, err := readItems(ctx, tx, r.ID, true)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if !sameShopItems(items, q.items) {
		return protocol.Errorf(protocol.CodeWrongState, "物品已变化，重新选择")
	}
	for _, slot := range changedSlots(items, next) {
		if _, err = tx.Exec(ctx, `DELETE FROM character_items WHERE character_id=$1 AND slot=$2`, r.ID, slot); err != nil {
			return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
		}
		if it := findItem(next, slot); it != nil {
			if _, err = tx.Exec(ctx, `INSERT INTO character_items(character_id,slot,item_id,count,merged,durability,max_durability,crush_level,inlay) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.ID, it.Slot, it.ItemID, it.Count, it.Merged, it.Durability, it.MaxDurability, it.CrushLevel, inlayJSON(it.Inlay)); err != nil {
				return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE characters SET gold=$1 WHERE id=$2`, gold, r.ID); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err = tx.Commit(ctx); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Items, r.Gold = next, gold
	w.shop.quote = nil
	l.p.pushAll("inventory_changed", r.inventoryView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	npc, _ := s.npcs.ByID(w.npc)
	s.openShop(l.p, npc, shop)
	return nil
}

func sameShopItems(a, b []itemRec) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		it := findItem(b, v.Slot)
		if it == nil || *it != v {
			return false
		}
	}
	return true
}
