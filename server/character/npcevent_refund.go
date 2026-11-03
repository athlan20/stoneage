package character

// 任务 NPC 的撤销（STAGE-94，docs/quest.md「放弃时撤销」）：接任务（REQUEST）时 DelItem 实际收走的道具和
// DelStone 收走的石币，按事件号记进 character_event_refunds（迁移 0014）；放弃（StopMsg 选是）时反过来做：
// 收回 GetItem 给的道具，退回记下的道具和石币。背包放不下退回的道具时说 ItemFullMsg，放弃不成立（不丢东西）。
// 原版 NPC_EventAdd(mode 1) 放弃时会把 DelItem 再删一遍、DelStone 再扣一次，这里不照搬（见 docs/quest.md）。

import (
	"context"
	"encoding/json"
	"github.com/athlan20/stoneage/server/battle"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// refundItem 是退回的一种道具和件数（每件占一格）。
type refundItem struct {
	ID int `json:"id"`
	N  int `json:"n"`
}

// eventRefund 是接任务时收走的东西：道具（按实际删掉的件数）和石币。
type eventRefund struct {
	Items []refundItem
	Gold  int64
}

// refundWrite 是 commitEvent 要写的退款记录（按事件号）。道具和石币都为空时删掉这一行。
type refundWrite struct {
	Event int
	eventRefund
}

// countItem 数背包里编号为 id 的道具件数（堆叠数量相加）。
func countItem(items []itemRec, id int) int {
	n := 0
	for _, it := range items {
		if it.ItemID == id {
			n += it.Count
		}
	}
	return n
}

// loadRefund 读角色在这个事件上收走的东西；没有记录时返回空的退款（放弃时什么都不退）。
func (s *Service) loadRefund(ctx context.Context, charID int64, event int) (eventRefund, error) {
	var raw []byte
	var out eventRefund
	err := s.db.QueryRow(ctx, `SELECT items, gold FROM character_event_refunds WHERE character_id = $1 AND event_no = $2`,
		charID, event).Scan(&raw, &out.Gold)
	if err == pgx.ErrNoRows {
		return eventRefund{}, nil
	}
	if err != nil {
		return eventRefund{}, err
	}
	if err := json.Unmarshal(raw, &out.Items); err != nil {
		return eventRefund{}, err
	}
	return out, nil
}

// writeRefundDiff 把退款记录写进同一个事务：道具和石币都为空就删掉这一行，否则覆盖。w 为 nil 时什么都不做。
func writeRefundDiff(ctx context.Context, tx pgx.Tx, charID int64, w *refundWrite) error {
	if w == nil {
		return nil
	}
	if len(w.Items) == 0 && w.Gold == 0 {
		_, err := tx.Exec(ctx, `DELETE FROM character_event_refunds WHERE character_id = $1 AND event_no = $2`, charID, w.Event)
		return err
	}
	raw, err := json.Marshal(w.Items)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO character_event_refunds (character_id, event_no, items, gold) VALUES ($1, $2, $3, $4)
		ON CONFLICT (character_id, event_no) DO UPDATE SET items = EXCLUDED.items, gold = EXCLUDED.gold, updated_at = now()`,
		charID, w.Event, raw, w.Gold)
	return err
}

// eventGiveUp 是放弃（原版 MsgDisp 7）：撤销接任务的交易。先收回 GetItem 给的道具（背包里没有就算了），再退回
// 接任务时收走的道具和石币；退回的道具放不下说 ItemFullMsg，石币超过上限说 StoneFullMsg，这两种情况都不放弃。
// 放弃成功后清进行中，魅力减 1（大于 0 时），说 EndStopMsg。
func (s *Service) eventGiveUp(ctx context.Context, l *Live, r *record, npc gamedata.NPC, blk *gamedata.EventBlock) *protocol.Error {
	if blk.Msg.EndStop == "" {
		return nil
	}
	refund, err := s.loadRefund(ctx, r.ID, blk.Event)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	items := cloneItems(r.Items)
	for _, g := range blk.GetItem {
		items = removeItems(items, g.ID, g.N, false)
	}
	need := 0
	for _, it := range refund.Items {
		need += it.N
	}
	if need > freeBagSlots(items) {
		openEventText(l.p, npc, blk.Msg.ItemFull)
		return nil
	}
	gold := r.Gold + refund.Gold
	if refund.Gold > 0 && gold >= battle.MaxHaveGold {
		openEventText(l.p, npc, blk.Msg.StoneFull)
		return nil
	}
	for _, it := range refund.Items {
		for k := 0; k < it.N; k++ {
			items = addItem(items, it.ID)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Slot < items[j].Slot })
	prog := progressOf(r)
	applyEventEdit(prog, editClrDoing, blk.Event)
	charm := r.Charm
	if charm > 0 {
		charm--
	}
	res := eventResult{items: items, gold: gold, charm: charm, prog: prog, pets: r.Pets, refund: &refundWrite{Event: blk.Event}}
	if perr := s.commitEvent(ctx, l, r, res); perr != nil {
		return perr
	}
	openEventText(l.p, npc, blk.Msg.EndStop)
	return nil
}
