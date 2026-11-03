package character

// 传送员（STAGE-74，docs/protocol.md 5.42）：talk 传送员 NPC 按通行条件（FREE）与费用
// （MONEY）判定——条件满足免费传送，不满足按费用确认后付费传送（MoneyMsg 收钱不够的
// 原话），不能付费的不响应。传送成功收走门票（DelItem，持有时收第一件）、改位置、
// 推 position_changed，遇敌开关自动关闭（同 warp，协议 5.16）。依赖任务/庄园/家族条件
// 的传送员（unsupported）直接跳过，不放行。一期没有组队，PartyMsg 只保留原文。

import (
	"context"
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

const warpmanOptionGo = 1

// openWarpman 是 talk 的传送窗口：条件满足时免费确认，不满足时报价；免费不满足又不能付费时回复
// NPC_NOT_TALKABLE，不弹窗、也不动玩家已经开着的窗口（STAGE-177：客户端按 talk 成功会一直等窗口）。
// 调用时要持有 p.mu（在 talk 的 Operate 里）。
func (s *Service) openWarpman(p *Presence, npc gamedata.NPC, wm *gamedata.NpcWarpman) *protocol.Error {
	var text string
	options := []npcOption{}
	if wm.PassFree(warpStateOf(s, &p.rec)) {
		text = firstLine(wm.FreeMsg, "要送你过去吗？")
		options = append(options, npcOption{No: warpmanOptionGo, Label: "传送"})
	} else if fee := wm.Fee(p.rec.Level); fee > 0 {
		// 免费条件不满足：付费路径（PayMsg 里的 %8d 是费用、%4d 是原版的人数占位，去掉）。
		text = replaceFee(wm.PayMsg, fee)
		options = append(options, npcOption{No: warpmanOptionGo, Label: fmt.Sprintf("付费传送（%d 金币）", fee)})
	} else {
		// 免费条件不满足且不能付费：原版此时不响应，这里回错误让客户端知道，不弹窗。
		return protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 现在不能送你去别处", npcDisplayName(npc))
	}
	p.npcWindow = &npcWindow{npc: npc.ID}
	p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": shopText(text), "options": options,
	})
	return nil
}

// hasItemFn 是「背包里有没有某编号物品」的判定（FREE 条件的 ITEM 用）。
func (s *Service) hasItemFn(r *record) func(int) bool {
	return func(itemID int) bool {
		return findItemSlot(r, itemID, 1) >= 0
	}
}

// firstLine 取台词的第一行（原版 FreeMsg 里常有 %4d 人数占位的多行文案）。
func firstLine(text, fallback string) string {
	if text == "" {
		return fallback
	}
	return text
}

// replaceFee 把台词里的 %8d 换成费用、%4d 占位去掉（原版用它填在场人数，一期没有）。
func replaceFee(text string, fee int) string {
	out := ""
	for i := 0; i < len(text); i++ {
		if i+3 <= len(text) && text[i:i+4] == "%8d" {
			out += fmt.Sprintf("%d", fee)
			i += 3
			continue
		}
		if i+3 <= len(text) && text[i:i+4] == "%4d" {
			continue
		}
		out += string(text[i])
	}
	return out
}

// warpmanChoose 处理传送窗口（npc_choose）。调用时要持有 p.mu。
func (s *Service) warpmanChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, wm *gamedata.NpcWarpman, no int) *protocol.Error {
	if perr := followerError(l.p); perr != nil { // 队员不能自己传送（STAGE-274）
		return perr
	}
	if no != warpmanOptionGo {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
	}
	free := wm.PassFree(warpStateOf(s, r))
	fee := 0
	if !free {
		fee = wm.Fee(r.Level)
		if fee <= 0 {
			return protocol.Errorf(protocol.CodeActionNotAllowed, "%s", shopText(firstLine(wm.FreeMsg, "现在还不能过去。")))
		}
	}
	if r.Gold < int64(fee) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "%s", shopText(firstLine(wm.MoneyMsg, "你钱不够哦。")))
	}
	floor, x, y, ok := wm.BornAt()
	if !ok {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	// 收门票（DelItem 里角色持有的第一件，各一张）。
	if slot := s.takeDelItem(ctx, r, wm); slot >= 0 {
		l.p.pushAll("inventory_changed", r.inventoryView())
	}
	r.Floor, r.X, r.Y = floor, x, y
	l.p.players.moved(l.p) // STAGE-273：换图
	r.Gold -= int64(fee)
	if fee > 0 {
		l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	}
	// 位置推给两边连接；传送走时遇敌开关自动关闭（同 warp，协议 5.16、5.29）。
	l.p.pushPosition(r)
	l.p.pushWindowClosed(WindowReasonMoved)
	if l.p.encounterOn {
		l.p.stopEncounter(encounterOffWarp)
	}
	return nil
}

// takeDelItem 收走 DelItem 列表里角色持有的第一件（一张），返回槽位（没收返回 -1）。
func (s *Service) takeDelItem(ctx context.Context, r *record, wm *gamedata.NpcWarpman) int {
	for _, itemID := range wm.DelItem {
		if slot := findItemSlot(r, itemID, 1); slot >= 0 {
			if _, err := s.db.Exec(ctx,
				`DELETE FROM character_items WHERE character_id = $1 AND slot = $2`, r.ID, slot); err != nil {
				return -1
			}
			removeItemSlot(r, slot)
			return slot
		}
	}
	return -1
}

// warpStateOf 是传送条件要看的角色状态：等级、转生、道具，以及任务进度（ENDEV、NOWEV，STAGE-90）。
func warpStateOf(s *Service, r *record) gamedata.WarpState {
	return gamedata.WarpState{Level: r.Level, Transmigration: r.Transmigration,
		HasItem: s.hasItemFn(r), Done: r.eventDone, Doing: r.eventDoing}
}
