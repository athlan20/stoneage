package character

// 存档点（STAGE-72，docs/protocol.md 5.40）：talk 存档点 NPC 弹记录窗口（Request 文案，
// 记录要交一件指定物品），选「记录」后收走物品、把存档点记在角色上（characters.savepoint，
// 数据库列，重新登录仍然有效）。战败送回时先看存档点的落点，没有记录过再回出生地
// （见 battle_apply 的 applyResult）。物品候选之间任一满足即可，候选里 `&` 连起来的几件
// 要一起有、一起收（原版 npc_savepoint.c 的 NPC_UsedCheck / NPC_AndReduce）；没有合格物品时
// 不记录、不消耗。没有设 GetItem 的存档点不要东西，直接记录（原版同）。
// 已经记录过这个存档点时只说 NomalMsg，不查也不收（原版同）。
// 存档点编号只在记录时写库（UPDATE），离线保存不用回写（save 不含这一列）。

import (
	"context"
	"slices"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

const savepointOptionRecord = 1

// openSavepoint 是 talk 成功后的记录窗口。调用时要持有 p.mu（在 talk 的 Operate 里）。
func (s *Service) openSavepoint(p *Presence, npc gamedata.NPC, sp *gamedata.NpcSavepoint) {
	p.npcWindow = &npcWindow{npc: npc.ID}
	text := sp.Request
	if text == "" {
		text = "要我帮你记录吗？" // 数据里没有 Request 文案时的占位（22 处都有，走不到）
	}
	if p.rec.Savepoint == npc.ID && sp.Normal != "" {
		// 已经记录过这里：原版 NomalMsg（“%s已经记录了，之后也请加油喔！”）。
		text = strings.ReplaceAll(sp.Normal, "%s", p.rec.Name)
	}
	p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": shopText(text), "options": []npcOption{{No: savepointOptionRecord, Label: "记录"}},
	})
}

// savepointChoose 处理记录窗口（npc_choose）。调用时要持有 p.mu。
func (s *Service) savepointChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, sp *gamedata.NpcSavepoint, no int) *protocol.Error {
	if no != savepointOptionRecord {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
	}
	name := func(text string) string { return strings.ReplaceAll(shopText(text), "%s", r.Name) }
	// 已经记录过这里：原版 NomalMsg 直接存档，不查物品、不收东西。
	if r.Savepoint == npc.ID {
		l.p.pushAll("npc_window", map[string]any{
			"npc": npc.ID, "name": npc.Name, "type": npc.Type,
			"text": name(sp.Normal), "options": []npcOption{},
		})
		return nil
	}
	group, slots, ok := pickSavepointGroup(r, sp)
	if !ok {
		// 没有合格物品：不记录、不消耗（原版就再说一遍 Request 的要求）。
		return protocol.Errorf(protocol.CodeActionNotAllowed, "%s", shopText(sp.Request))
	}
	for i, slot := range slots {
		if err := s.takeBagItem(ctx, r, slot, group[i].Count); err != nil {
			return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
		}
	}
	if _, err := s.db.Exec(ctx,
		`UPDATE characters SET savepoint = $2 WHERE id = $1`, r.ID, npc.ID); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Savepoint = npc.ID
	// 成功话（原版 OkMsg）；收了物品时 RealyMsg 一并给在前面。
	text := shopText(sp.Ok)
	if len(slots) > 0 {
		l.p.pushAll("inventory_changed", r.inventoryView())
		if sp.Really != "" {
			text = shopText(sp.Really) + "\n" + text
		}
	}
	l.p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": text, "options": []npcOption{},
	})
	return nil
}

// pickSavepointGroup 找第一组背包里齐全的候选（原版 OR 语义），返回这组物品和每件所在的格。
// 没有 GetItem 时返回空组（直接记录）；有 GetItem 但没有一组齐全时 ok 为 false。
func pickSavepointGroup(r *record, sp *gamedata.NpcSavepoint) (group []gamedata.SavepointItem, slots []int, ok bool) {
	if len(sp.Groups) == 0 {
		return nil, nil, true
	}
	for _, g := range sp.Groups {
		if found, ok := findGroupSlots(r, g); ok {
			return g, found, true
		}
	}
	return nil, nil, false
}

// findGroupSlots 找一组物品在背包里各自的一格（编号和数量都够；同组的几件不占同一格；
// 0–4 装备位不算）。全部找到才返回 ok，任何一件缺了整组都不算。
func findGroupSlots(r *record, group []gamedata.SavepointItem) (slots []int, ok bool) {
	for _, want := range group {
		slot := findItemSlotExcept(r, want.Item, want.Count, slots)
		if slot < 0 {
			return nil, false
		}
		slots = append(slots, slot)
	}
	return slots, true
}

// findItemSlot 找背包里物品编号和数量都够的一格（0–4 装备位不算），没有返回 -1。
// 传送员的通行条件（warpman.go）也用它。
func findItemSlot(r *record, itemID, count int) int {
	return findItemSlotExcept(r, itemID, count, nil)
}

// findItemSlotExcept 同 findItemSlot，但跳过 used 里已经占用的格。
func findItemSlotExcept(r *record, itemID, count int, used []int) int {
	for _, it := range r.Items {
		if it.ItemID == itemID && it.Count >= count && it.Slot >= firstBagSlot && !slices.Contains(used, it.Slot) {
			return it.Slot
		}
	}
	return -1
}

// takeBagItem 从一格里收走 count 件：这一格的数量不够剩下的就整格删行，多出来的减 count（写库）。
func (s *Service) takeBagItem(ctx context.Context, r *record, slot, count int) error {
	for i := range r.Items {
		if r.Items[i].Slot != slot {
			continue
		}
		if left := r.Items[i].Count - count; left > 0 {
			if _, err := s.db.Exec(ctx,
				`UPDATE character_items SET count = $3 WHERE character_id = $1 AND slot = $2`, r.ID, slot, left); err != nil {
				return err
			}
			r.Items[i].Count = left
			return nil
		}
		break
	}
	if _, err := s.db.Exec(ctx,
		`DELETE FROM character_items WHERE character_id = $1 AND slot = $2`, r.ID, slot); err != nil {
		return err
	}
	removeItemSlot(r, slot)
	return nil
}

// SavepointRespawn 返回战败送回的落点：记录过存档点用它，否则用出生地
// （STAGE-72；血量剩 1 等其余规则不变，见 applyResult）。
func SavepointRespawn(r *record) (floor, x, y int, ok bool) {
	if r.Savepoint != 0 {
		if sp := gamedata.NpcSavepointOf(r.Savepoint); sp != nil {
			if f, x, y, ok := sp.BornAt(); ok {
				return f, x, y, true
			}
		}
	}
	h := gamedata.Hometowns[r.Hometown]
	return h.Floor, h.X, h.Y, false
}
