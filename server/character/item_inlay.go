// 精工（STAGE-298，docs/protocol.md 5.73，docs/inlay.md 2.5）：item_inlay 的入口、写库和推送。
// 请求形状和镶嵌规则在 inlay_rules.go。一次请求一个事务：扣料和写目标的镶嵌状态一起提交，
// 提交后才更新内存并推送。中途镶满不回滚，回复 INLAY_FULL 并带 applied、consumed。

package character

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

type itemInlayRequest struct {
	Target    *int  `json:"target"`
	Materials []int `json:"materials"`
}

// handleItemInlay 是 item_inlay（docs/protocol.md 5.73）：把材料格的东西镶到目标格的武器或防具上。
// 校验顺序：控制权（Operate）→ 战斗中 → 请求形状 → planInlay 的各步。
func (s *Service) handleItemInlay(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in itemInlayRequest
	decodeErr := decodeStrict(req, &in)
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var (
		out    inlayOutcome
		item   inventorySlotView
		opErr  error
		target int
	)
	if err := l.Operate(func(r *record) {
		if perr := inBattle(l); perr != nil {
			opErr = perr
			return
		}
		if decodeErr != nil {
			opErr = decodeErr
			return
		}
		if in.Target == nil {
			opErr = protocol.Errorf(protocol.CodeBadRequest, "需要目标格（target）")
			return
		}
		target = *in.Target
		if opErr = checkInlayShape(target, in.Materials); opErr != nil {
			return
		}
		out, opErr = s.applyInlay(ctx, l.p, r, target, in.Materials)
		if opErr == nil {
			item = inventorySlotOf(*findItem(r.Items, target))
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	if out.Full { // 中途镶满：前面的已经镶上并扣掉，以 error.data 告诉客户端
		return nil, inlayFullError(out)
	}
	return map[string]any{"applied": out.Applied, "consumed": out.Consumed, "item": item}, nil
}

// applyInlay 在一个事务里做精工：加行锁读出物品，算出结果，写库，提交后才更新内存并推送。
// 一件也镶不上（第一件就镶满）时不写库，返回 INLAY_FULL。中途镶满返回 Full 为真的结果，已镶的部分照常提交。
func (s *Service) applyInlay(ctx context.Context, p *Presence, r *record, target int, materials []int) (inlayOutcome, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return inlayOutcome{}, err
	}
	defer tx.Rollback(ctx)
	items, err := readItems(ctx, tx, r.ID, true)
	if err != nil {
		return inlayOutcome{}, err
	}
	out, err := planInlay(items, target, materials)
	if err != nil {
		return out, err
	}
	if out.Applied == 0 {
		return out, inlayFullError(out)
	}
	if err := writeItemSlots(ctx, tx, r.ID, out.Next, changedSlots(items, out.Next)); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	before, magicBefore := r.status().Derived, r.magicSlots()
	r.Items = out.Next
	p.pushAll("inventory_changed", r.inventoryView())
	if after := r.status().Derived; after != before || r.magicSlots() != magicBefore {
		p.pushAll("status_changed", map[string]any{"status": r.status()})
	}
	return out, nil
}

// inlayFullError 是“镶满了”的回复：applied 和 consumed 说明前面已经镶上、扣掉了哪些。
func inlayFullError(out inlayOutcome) *protocol.Error {
	return protocol.Errorf(protocol.CodeInlayFull, "武器或防具已经镶满了。").
		WithData(map[string]any{"applied": out.Applied, "consumed": out.Consumed})
}

// decodeStrict 解析请求的 data：多出的字段和类型不对都回复 BAD_REQUEST，精工的字段拼错不能静默失效（5.73）。
func decodeStrict(req *protocol.Request, v any) *protocol.Error {
	dec := json.NewDecoder(bytes.NewReader(req.Data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return protocol.Errorf(protocol.CodeBadRequest, "请求的 data 字段不正确")
	}
	return nil
}
