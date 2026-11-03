// 穿脱与整理（STAGE-29，docs/protocol.md 5.25–5.28，docs/ui.md 背包页 B）：item_equip 穿上、item_unequip 卸下、
// item_move 移动或互换、item_drop 丢弃。四者都是操作类，走同一条路径：加行锁读出物品，规则在 equip_rules.go 算出
// 改后的列表，在一个事务里写库（先删改动的格子，再插入新的，避免唯一约束冲突），提交后才更新内存并推送。
// 装备加成变了再推一次 status_changed，角色页立刻变化。

package character

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

type itemSlotRequest struct {
	Slot *int `json:"slot"`
}

type itemMoveRequest struct {
	From *int `json:"from"`
	To   *int `json:"to"`
}

type itemDropRequest struct {
	Slot    *int    `json:"slot"`
	Count   *int    `json:"count"`
	Confirm *string `json:"confirm"`
}

// itemPlan 是一次物品操作：拿到角色当前的物品和等级，返回改后的完整列表。
type itemPlan func(items []itemRec, level int) ([]itemRec, error)

// handleItemEquip 是 item_equip（docs/protocol.md 5.25）：背包里的装备穿到它该在的部位。
func (s *Service) handleItemEquip(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in itemSlotRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := requiredItemSlot(in.Slot)
	if err != nil {
		return nil, err
	}
	return s.operateItems(ctx, sess, func(items []itemRec, level int) ([]itemRec, error) {
		target, err := wearTarget(items, slot)
		if err != nil {
			return nil, err
		}
		return planMove(items, level, slot, target)
	})
}

// handleItemUnequip 是 item_unequip（docs/protocol.md 5.26）：装备脱下，放进第一个空的背包格。
func (s *Service) handleItemUnequip(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in itemSlotRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := requiredItemSlot(in.Slot)
	if err != nil {
		return nil, err
	}
	return s.operateItems(ctx, sess, func(items []itemRec, level int) ([]itemRec, error) {
		target, err := unequipTarget(items, slot)
		if err != nil {
			return nil, err
		}
		return planMove(items, level, slot, target)
	})
}

// handleItemMove 是 item_move（docs/protocol.md 5.27）：背包格之间移动或互换；与装备格之间的移动按穿脱的规则检查。
func (s *Service) handleItemMove(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in itemMoveRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	from, err := requiredItemSlot(in.From)
	if err != nil {
		return nil, err
	}
	to, err := requiredItemSlot(in.To)
	if err != nil {
		return nil, err
	}
	return s.operateItems(ctx, sess, func(items []itemRec, level int) ([]itemRec, error) {
		return planMove(items, level, from, to)
	})
}

// handleItemDrop 是 item_drop（docs/protocol.md 5.28）：丢弃（销毁）格子上的物品，需要 confirm 为 "yes"。
func (s *Service) handleItemDrop(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in itemDropRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := requiredItemSlot(in.Slot)
	if err != nil {
		return nil, err
	}
	if in.Confirm == nil || *in.Confirm != "yes" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "丢弃要确认：confirm 必须是 \"yes\"")
	}
	count := 0
	if in.Count != nil {
		if *in.Count < 1 {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "数量要是正整数")
		}
		count = *in.Count
	}
	return s.operateItems(ctx, sess, func(items []itemRec, _ int) ([]itemRec, error) {
		return planDrop(items, slot, count)
	})
}

// requiredItemSlot 检查请求里的格子编号：必须有，且是 0–19。
func requiredItemSlot(slot *int) (int, error) {
	if slot == nil {
		return 0, protocol.Errorf(protocol.CodeBadRequest, "需要格子编号（0–%d）", MaxItemSlot)
	}
	if err := validItemSlot(*slot); err != nil {
		return 0, err
	}
	return *slot, nil
}

// operateItems 是四个物品操作共用的入口：确认连接在世界中且是操控方（Live.Operate），角色没死，然后执行 plan。
func (s *Service) operateItems(ctx context.Context, sess *gateway.Session, plan itemPlan) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var opErr error
	if err := l.Operate(func(r *record) {
		if r.HP <= 0 {
			opErr = protocol.Errorf(protocol.CodeCharacterDead, "角色已经死亡，不能穿脱或整理物品")
			return
		}
		opErr = s.changeItems(ctx, l.p, r, plan)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return map[string]any{}, nil
}

// changeItems 在一个事务里改角色的物品。调用时要持有角色的锁（在 Live.Operate 里）。
// 提交成功后才更新内存：推送 inventory_changed；装备加成（攻击、防御、敏捷）或装备自带的精灵（STAGE-190）变了再推 status_changed。
// 变身中穿远距离武器被拒、卸下身体装备让变身失效（STAGE-203，metamo.go）。
func (s *Service) changeItems(ctx context.Context, p *Presence, r *record, plan itemPlan) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	items, err := readItems(ctx, tx, r.ID, true)
	if err != nil {
		return err
	}
	next, err := plan(items, r.Level)
	if err != nil {
		return err
	}
	changed := changedSlots(items, next)
	if len(changed) == 0 {
		return nil
	}
	if perr := checkMetamoWear(r, items, next); perr != nil { // 变身中不能穿远距离武器（STAGE-203）
		return perr
	}
	if err := writeItemSlots(ctx, tx, r.ID, next, changed); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	before, magicBefore := r.status().Derived, r.magicSlots()
	armourOff := r.metamo != nil && bodyArmourChanged(r.Items, next) // 卸下身体装备，变身失效（STAGE-203）
	r.Items = next
	p.pushAll("inventory_changed", r.inventoryView())
	if armourOff {
		p.endMetamoLocked(r, "变身失效了！") // 推送 status_changed（metamo 消失）和提示
		return nil
	}
	if after := r.status().Derived; after != before || r.magicSlots() != magicBefore {
		p.pushAll("status_changed", map[string]any{"status": r.status()})
	}
	return nil
}

// writeItemSlots 在事务里把改动的格子写回 character_items：先删掉这些格子，再按 items 里的内容插入（带 inlay）。
// 穿脱（changeItems）和精工（item_inlay.go）共用。
func writeItemSlots(ctx context.Context, tx pgx.Tx, charID int64, items []itemRec, slots []int) error {
	for _, slot := range slots {
		if _, err := tx.Exec(ctx, `DELETE FROM character_items WHERE character_id = $1 AND slot = $2`, charID, slot); err != nil {
			return err
		}
	}
	for _, slot := range slots {
		it := findItem(items, slot)
		if it == nil {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO character_items (character_id, slot, item_id, count, merged, durability, max_durability, crush_level, inlay) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			charID, slot, it.ItemID, it.Count, it.Merged, it.Durability, it.MaxDurability, it.CrushLevel, inlayJSON(it.Inlay)); err != nil {
			return fmt.Errorf("写入格子 %d 失败: %w", slot, err)
		}
	}
	return nil
}
