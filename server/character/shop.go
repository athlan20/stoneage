package character

// 商店买卖（STAGE-70，docs/protocol.md 5.38）：talk 商店 NPC 后的窗口状态机——菜单
// （买/卖）→ 商品或回收列表 → 买还要报数量。倍率、商品和台词全部来自店自己的参数
// （gamedata.NpcShop，npc_itemshop.c 的行为）：买价 = 基价 × buy_rate，卖价 = 基价 ×
// sell_rate（高价回收按 special_rate）；等级不够回 level_msg、背包满回 itemfull_msg、
// 卖出超金币上限回 stone_msg。买按背包空位截断（原版 kosuucnt < kosuu 时按空位算），
// 一格一件；卖按整行（管理命令可能给过 count>1 的物品，按 count 件计价）。
// 物品行同步写库（同 saveBattleItems 的锁内直写），金币在内存 record 上改、离线保存回写。

import (
	"context"
	"fmt"
	"github.com/athlan20/stoneage/server/battle"
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// shopSession 的阶段。
const (
	shopStageMenu     = "menu"         // 欢迎 + 买/卖
	shopStageBuyList  = "buy_list"     // 商品列表
	shopStageBuyQty   = "buy_quantity" // 报数量
	shopStageSellList = "sell_list"    // 回收列表
)

// shopSession 是商店对话的阶段与待定商品（npcWindow.shop）。
type shopSession struct {
	selected itemRec
	quote    *shopQuote
	stage    string
	itemID   int // buy_quantity 阶段已选中的商品编号
}

// shopText 是发给窗口的正文：参数文件里的字面 `\n` 转成真换行（同告示牌的 TalkText）。
func shopText(text string) string {
	return strings.ReplaceAll(text, `\n`, "\n")
}

// openShop 是 talk 成功后的商店菜单：欢迎语加买/卖选项（按这家店有什么生成）。
// 调用时要持有 p.mu（在 talk 的 Operate 里）。
func (s *Service) openShop(p *Presence, npc gamedata.NPC, shop *gamedata.NpcShop) {
	options := []npcOption{}
	if shop.ShopSellsItems() {
		options = append(options, npcOption{No: 1, Label: "买"})
	}
	if shop.ShopBuysItems() {
		options = append(options, npcOption{No: 2, Label: "卖"})
	}
	p.npcWindow = &npcWindow{npc: npc.ID, shop: &shopSession{stage: shopStageMenu}}
	p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": shopText(shop.Main), "options": options,
	})
}

// shopMenuView 组一个商店窗口的推送（列表页公用）。options 为 nil 时发空数组
// （协议 5.34：options 不能是 null）。
func (s *Service) shopMenuView(l *Live, w *npcWindow, npc gamedata.NPC, text string, options []npcOption) {
	if options == nil {
		options = []npcOption{}
	}
	w.options = options
	l.p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": shopText(text), "options": options,
	})
}

// shopChoose 推进商店窗口（npc_choose）。调用时要持有 p.mu。
func (s *Service) shopChoose(ctx context.Context, l *Live, r *record, w *npcWindow, no int) *protocol.Error {
	if l.p.npcMenuV2 && w.shop.stage == shopStageSellList {
		it := findItem(r.Items, no)
		if it == nil || it.Slot < firstBagSlot {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "物品已不存在")
		}
		shop := gamedata.NpcShopOf(w.npc)
		if _, ok := shop.SellPrice(it.ItemID); !ok {
			return protocol.Errorf(protocol.CodeActionNotAllowed, "这个物品不能卖")
		}
		w.shop.selected = *it
		w.shop.stage = shopStageSellQty
		npc, _ := s.npcs.ByID(w.npc)
		s.shopMenuView(l, w, npc, "请输入出售数量", nil)
		return nil
	}
	npc, _ := s.npcs.ByID(w.npc)
	shop := gamedata.NpcShopOf(w.npc)
	switch w.shop.stage {
	case shopStageMenu:
		switch {
		case no == 1 && shop.ShopSellsItems():
			w.shop.stage = shopStageBuyList
			options := make([]npcOption, 0, len(shop.Items))
			for i := range shop.Items {
				itemID, price, ok := shop.BuyPrice(i)
				if !ok {
					continue
				}
				options = append(options, npcOption{No: len(options) + 1, Label: fmt.Sprintf("%s（%d 金币）", itemName(itemID), price)})
			}
			s.shopMenuView(l, w, npc, shop.BuyMain, options)
			return nil
		case no == 2 && shop.ShopBuysItems():
			return s.openSellList(l, r, w, npc, shop)
		}
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)

	case shopStageBuyList:
		itemID, price, ok := shop.BuyPrice(no - 1)
		if !ok {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
		}
		w.shop.stage = shopStageBuyQty
		w.shop.itemID = itemID
		text := fmt.Sprintf("%s\n%s：%d 金币一个。输入 reply 数量 购买。", shop.What, itemName(itemID), price)
		s.shopMenuView(l, w, npc, text, nil)
		return nil

	case shopStageSellList:
		return s.sellItem(ctx, l, r, w, npc, shop, no)
	case shopStageBuyQty:
		// 数量输错后窗口停在这一步，选项是空的：说清楚现在该做什么（STAGE-178）。
		return protocol.Errorf(protocol.CodeWrongState, "现在等输入购买数量：用 reply 数量 购买，不买了就关掉窗口")
	}
	return protocol.Errorf(protocol.CodeWrongState, "窗口状态不对")
}

// openSellList 列出背包里能卖的物品（5–19 格，回收价大于 0 的），选项编号就是格子号
// （原版卖列表的 select 也是槽位）。
func (s *Service) openSellList(l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcShop) *protocol.Error {
	w.shop.stage = shopStageSellList
	options := []npcOption{}
	for _, it := range r.Items {
		price, ok := shop.SellPrice(it.ItemID)
		if !ok || it.Slot < firstBagSlot {
			continue
		}
		label := fmt.Sprintf("%s（回收 %d 金币）", itemName(it.ItemID), price)
		if it.Count > 1 {
			label = fmt.Sprintf("%s×%d（回收 %d 金币）", itemName(it.ItemID), it.Count, price*it.Count)
		}
		options = append(options, npcOption{No: it.Slot, Label: label})
	}
	s.shopMenuView(l, w, npc, shop.SellMain, options)
	return nil
}

// sellItem 卖出一个槽位里的物品（整行，count 件一起）：金币不能超过携带上限（stone_msg）。
// 调用时要持有 p.mu。
func (s *Service) sellItem(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcShop, slot int) *protocol.Error {
	var found *itemRec
	for i := range r.Items {
		if r.Items[i].Slot == slot {
			found = &r.Items[i]
			break
		}
	}
	if found == nil || found.Slot < firstBagSlot {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "背包 %d 号格没有能卖的物品", slot)
	}
	price, ok := shop.SellPrice(found.ItemID)
	if !ok {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "这个物品不能卖")
	}
	total := price * found.Count
	if r.Gold+int64(total) > battle.MaxHaveGold {
		return protocol.Errorf(protocol.CodeGoldFull, "%s（金币上限 %d）", shop.Stone, battle.MaxHaveGold)
	}
	if _, err := s.db.Exec(ctx,
		`DELETE FROM character_items WHERE character_id = $1 AND slot = $2`, r.ID, slot); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	removeItemSlot(r, slot)
	r.Gold += int64(total)
	l.p.pushAll("inventory_changed", r.inventoryView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushWindowClosed(WindowReasonChosen)
	_ = npc
	return nil
}

// removeItemSlot 从背包里删掉一个槽位（内存）。
func removeItemSlot(r *record, slot int) {
	out := r.Items[:0]
	for _, it := range r.Items {
		if it.Slot != slot {
			out = append(out, it)
		}
	}
	r.Items = out
}

// shopReply 处理窗口的文字回复（npc_reply）：目前只有买数量的输入。调用时要持有 p.mu。
func (s *Service) shopReply(ctx context.Context, l *Live, r *record, w *npcWindow, text string) *protocol.Error {
	if l.p.npcMenuV2 {
		return s.quoteShop(l, r, w, text)
	}
	if w.shop.stage != shopStageBuyQty {
		return protocol.Errorf(protocol.CodeBadRequest, "这个窗口不接受文字回复")
	}
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		shop := gamedata.NpcShopOf(w.npc)
		return protocol.Errorf(protocol.CodeBadRequest, "%s（输入 reply 数量）", shop.What)
	}
	return s.buyItem(ctx, l, r, w, n)
}

// buyItem 买入待定的商品（shopStageBuyQty 里的 itemID）：依次检查等级、金币、背包空位，
// 全过才落库、扣金币、推推送并关窗——要么都变要么都不变。
func (s *Service) buyItem(ctx context.Context, l *Live, r *record, w *npcWindow, count int) *protocol.Error {
	shop := gamedata.NpcShopOf(w.npc)
	npc, _ := s.npcs.ByID(w.npc)
	tmpl, ok := gamedata.ItemByID(w.shop.itemID)
	if !ok {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这家店已经不卖这个物品了，重新打开试试")
	}
	if tmpl.Level > r.Level {
		return protocol.Errorf(protocol.CodeRequirementNotMet, "%s", shop.Level)
	}
	unit := shop.UnitPrice(w.shop.itemID, tmpl.Cost)
	total := unit * count
	if r.Gold < int64(total) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "金币不够：买 %d 个「%s」需要 %d 金币，你只有 %d", count, tmpl.Name, total, r.Gold)
	}
	// 空位不足按原版截断（kosuucnt < kosuu 时按空位算），一个空位都没有回 itemfull_msg。
	taken := [MaxItemSlot + 1]bool{}
	for _, it := range r.Items {
		taken[it.Slot] = true
	}
	free := []int{}
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		if !taken[slot] {
			free = append(free, slot)
		}
	}
	if len(free) == 0 {
		return protocol.Errorf(protocol.CodeBagFull, "%s", shop.ItemFull)
	}
	if count > len(free) {
		count = len(free)
	}
	for _, slot := range free[:count] {
		dur, maxDur := newItemDurability(w.shop.itemID)
		if _, err := s.db.Exec(ctx,
			`INSERT INTO character_items (character_id, slot, item_id, count, durability, max_durability) VALUES ($1, $2, $3, $4, $5, $6)`,
			r.ID, slot, w.shop.itemID, 1, dur, maxDur); err != nil {
			return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
		}
		r.Items = append(r.Items, itemRec{Slot: slot, ItemID: w.shop.itemID, Count: 1, Durability: dur, MaxDurability: maxDur})
	}
	r.Gold -= int64(unit * count)
	l.p.pushAll("inventory_changed", r.inventoryView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushWindowClosed(WindowReasonChosen)
	_ = npc
	return nil
}
