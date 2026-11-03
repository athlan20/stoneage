package character

// 道具仓库（STAGE-309，原版 npc_poolitemshop.c，docs/protocol.md 5.60）：便利店的道具仓库管理员。
// 「寄存」选一件背包里的道具（整件，堆叠数量、耐久、精工原样），每件收 50 石币；「领取」免费取回到
// 第一个空背包格。装备格（0–4）里的道具不能寄存（原版寄存列表从背包格开始）。
// 仓库固定 50 格（STAGE-347，不再随转生）；寄存时背包、仓库满或金币不够都不扣钱、不动物品。

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// poolItemKind 是道具仓库管理员的 NPC 类型（gen-npcs.py 的 kind）。
const poolItemKind = "npcgen_poolitemshop"

// 道具仓库窗口的阶段。
const (
	poolStageMenu = "menu"    // 菜单：寄存 / 领取
	poolStagePick = "pick"    // 寄存：背包物品列表（选项编号是背包格号）
	poolStageOK   = "pick_ok" // 寄存：确认页
	poolStageDraw = "draw"    // 领取：仓库物品列表（选项编号是仓库格号，0 起）
)

// poolItemSession 记录道具仓库对话的阶段和待定的格子（寄存是背包格，领取是仓库格）。
type poolItemSession struct {
	stage string
	slot  int
}

// openPoolItemShop 是 talk 成功后的菜单。调用时要持有 p.mu（在 talk 的 Operate 里）。
func (s *Service) openPoolItemShop(p *Presence, npc gamedata.NPC) {
	p.npcWindow = &npcWindow{npc: npc.ID, poolitem: &poolItemSession{stage: poolStageMenu}}
	p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text":    shopText("欢迎光临"),
		"options": []npcOption{{No: 1, Label: "寄存"}, {No: 2, Label: "领取"}},
	})
}

// poolItemChoose 推进道具仓库窗口（npc_choose）。调用时要持有 p.mu。
func (s *Service) poolItemChoose(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, no int) *protocol.Error {
	switch w.poolitem.stage {
	case poolStageMenu:
		switch no {
		case 1:
			w.poolitem.stage = poolStagePick
			s.petMenuView(l, w, npc, "要寄存什么呢？", bagItemOptions(r))
			return nil
		case 2:
			pool, err := readPoolItems(ctx, s.db, r.ID)
			if err != nil {
				return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
			}
			w.poolitem.stage = poolStageDraw
			s.petMenuView(l, w, npc, "要领取什么呢？", poolItemOptions(pool))
			return nil
		}
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)

	case poolStagePick:
		it := bagItemAt(r.Items, no)
		if it == nil {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "背包 %d 号没有道具", no)
		}
		w.poolitem.slot = no
		w.poolitem.stage = poolStageOK
		s.petMenuView(l, w, npc, fmt.Sprintf("寄存[%s]\n\n    寄存价格            %d stone\n真的要那个吗", poolItemLabel(*it), gamedata.PoolItemCost),
			[]npcOption{{No: 1, Label: "寄存"}})
		return nil

	case poolStageOK:
		return s.poolItemDeposit(ctx, l, r, w)

	case poolStageDraw:
		return s.poolItemWithdraw(ctx, l, r, no)
	}
	return protocol.Errorf(protocol.CodeWrongState, "窗口状态不对")
}

// poolItemDeposit 寄存待定的背包格（poolitem.slot）：仓库满回 poolfull，金币不够回 stone_msg，
// 都不扣钱不动物品；成功后背包少一件、仓库多一件，扣 50 石币，推 inventory_changed 并关窗。
func (s *Service) poolItemDeposit(ctx context.Context, l *Live, r *record, w *npcWindow) *protocol.Error {
	it := bagItemAt(r.Items, w.poolitem.slot)
	if it == nil {
		return protocol.Errorf(protocol.CodeWrongState, "这件道具已经不在背包里了")
	}
	deposit := *it
	pool, err := readPoolItems(ctx, s.db, r.ID)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	poolSlot := freePoolItemSlot(pool, gamedata.PoolItemLimit(r.Transmigration))
	if poolSlot < 0 {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "超过的无法处理喔")
	}
	if r.Gold < int64(gamedata.PoolItemCost) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "钱不够喔!")
	}
	next := withoutItemSlot(r.Items, deposit.Slot)
	err = s.commitPoolItems(ctx, r.ID, r.Items, next, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO character_pool_items (character_id, slot, item_id, count, merged, durability, max_durability, crush_level, inlay) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			r.ID, poolSlot, deposit.ItemID, deposit.Count, deposit.Merged, deposit.Durability, deposit.MaxDurability, deposit.CrushLevel, inlayJSON(deposit.Inlay))
		return err
	})
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Items = next
	r.Gold -= int64(gamedata.PoolItemCost)
	l.p.pushAll("inventory_changed", r.inventoryView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushWindowClosed(WindowReasonChosen)
	return nil
}

// poolItemWithdraw 把仓库 no 号格的道具取回到第一个空背包格（免费，原版如此）；背包满回 HaveItemFull。
func (s *Service) poolItemWithdraw(ctx context.Context, l *Live, r *record, no int) *protocol.Error {
	pool, err := readPoolItems(ctx, s.db, r.ID)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	got := poolItemAt(pool, no)
	if got == nil {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "仓库 %d 号没有道具", no)
	}
	free := firstEmptyBagSlot(r.Items)
	if free < 0 {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "道具不是已经满了吗")
	}
	back := *got
	back.Slot = free
	next := append(cloneItems(r.Items), back)
	err = s.commitPoolItems(ctx, r.ID, r.Items, next, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM character_pool_items WHERE character_id = $1 AND slot = $2`, r.ID, no)
		return err
	})
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Items = next
	l.p.pushAll("inventory_changed", r.inventoryView())
	l.p.pushWindowClosed(WindowReasonChosen)
	return nil
}

// bagItemAt 返回背包（5–19）里 slot 号格的道具，装备格或空格返回 nil。
func bagItemAt(items []itemRec, slot int) *itemRec {
	if slot < firstBagSlot || slot > MaxItemSlot {
		return nil
	}
	for i := range items {
		if items[i].Slot == slot {
			return &items[i]
		}
	}
	return nil
}

// withoutItemSlot 返回去掉 slot 号格的背包副本（不改原切片）。
func withoutItemSlot(items []itemRec, slot int) []itemRec {
	out := make([]itemRec, 0, len(items))
	for _, it := range items {
		if it.Slot != slot {
			out = append(out, it)
		}
	}
	return out
}

// bagItemOptions 是寄存列表：背包格里的道具，选项编号是背包格号。
func bagItemOptions(r *record) []npcOption {
	options := []npcOption{}
	for _, it := range r.Items {
		if it.Slot < firstBagSlot || it.Slot > MaxItemSlot {
			continue
		}
		options = append(options, npcOption{No: it.Slot, Label: poolItemLabel(it)})
	}
	return options
}

// poolItemOptions 是领取列表：仓库里的道具，选项编号是仓库格号（0 起）。
func poolItemOptions(pool []itemRec) []npcOption {
	options := []npcOption{}
	for _, it := range pool {
		options = append(options, npcOption{No: it.Slot, Label: poolItemLabel(it)})
	}
	return options
}

// poolItemLabel 是列表里的名字，堆叠超过 1 件时带数量。
func poolItemLabel(it itemRec) string {
	if it.Count > 1 {
		return fmt.Sprintf("%s x%d", itemName(it.ItemID), it.Count)
	}
	return itemName(it.ItemID)
}
