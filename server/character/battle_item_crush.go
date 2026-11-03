package character

import (
	"context"
	"time"

	"github.com/athlan20/stoneage/server/battle"
)

// STAGE-209：战斗结束时把人物装备的耐久写回背包（原版 BATTLE_ItemCrush 的写回部分，docs/durability.md）。

// itemCrushWrite 是战斗后一件装备的变化，写库用。Gone 为真时删掉这一格。
type itemCrushWrite struct {
	Slot       int
	Gone       bool
	Durability int
	CrushLevel int
}

// applyItemCrush 把战斗里装备的耐久和受损等级写回 r.Items；耗尽的装备从背包移除。返回有变化的栏位。
func applyItemCrush(r *record, b *battle.Battle) []itemCrushWrite {
	var out []itemCrushWrite
	for slot, e := range b.EquipState() {
		if e == nil {
			continue
		}
		it := findItem(r.Items, slot)
		if it == nil {
			continue
		}
		if !e.Gone && it.Durability == e.Durability && it.CrushLevel == e.CrushLevel {
			continue
		}
		out = append(out, itemCrushWrite{Slot: slot, Gone: e.Gone, Durability: e.Durability, CrushLevel: e.CrushLevel})
		if e.Gone {
			r.Items = removeSlot(r.Items, slot)
			continue
		}
		it.Durability, it.CrushLevel = e.Durability, e.CrushLevel
	}
	return out
}

// removeSlot 删掉某个栏位的物品（不改其他格子的顺序）。
func removeSlot(items []itemRec, slot int) []itemRec {
	out := items[:0]
	for _, it := range items {
		if it.Slot != slot {
			out = append(out, it)
		}
	}
	return out
}

// saveItemCrush 把装备耐久的变化写进 character_items：耗尽的删行，其余更新耐久和等级。
func (s *Service) saveItemCrush(charID int64, writes []itemCrushWrite) {
	if len(writes) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, w := range writes {
		var err error
		if w.Gone {
			_, err = s.db.Exec(ctx, `DELETE FROM character_items WHERE character_id = $1 AND slot = $2`, charID, w.Slot)
		} else {
			_, err = s.db.Exec(ctx, `UPDATE character_items SET durability = $3, crush_level = $4 WHERE character_id = $1 AND slot = $2`,
				charID, w.Slot, w.Durability, w.CrushLevel)
		}
		if err != nil {
			s.opts.Logger.Warn("保存装备耐久失败", "character", charID, "slot", w.Slot, "err", err)
		}
	}
}
