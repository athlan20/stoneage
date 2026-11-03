package character

// 道具仓库的存储（STAGE-309，迁移 0032 的 character_pool_items）：寄存的物品整件存在这张表里，
// 格子 0 起，不和背包的 character_items 混用，背包的列表、交易、合成都不用改。

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// readPoolItems 读角色道具仓库里的物品，按格子从小到大。
func readPoolItems(ctx context.Context, q dbQuerier, charID int64) ([]itemRec, error) {
	rows, err := q.Query(ctx, `SELECT slot, item_id, count, merged, durability, max_durability, crush_level, inlay FROM character_pool_items WHERE character_id = $1 ORDER BY slot`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []itemRec
	for rows.Next() {
		var it itemRec
		var inlay []byte
		if err := rows.Scan(&it.Slot, &it.ItemID, &it.Count, &it.Merged, &it.Durability, &it.MaxDurability, &it.CrushLevel, &inlay); err != nil {
			return nil, err
		}
		st, err := decodeInlay(inlay)
		if err != nil {
			return nil, err
		}
		it.Inlay = st
		out = append(out, it)
	}
	return out, rows.Err()
}

// poolItemAt 返回仓库里 slot 号格子的物品，没有时返回 nil。
func poolItemAt(pool []itemRec, slot int) *itemRec {
	for i := range pool {
		if pool[i].Slot == slot {
			return &pool[i]
		}
	}
	return nil
}

// freePoolItemSlot 返回仓库里第一个空格（只看前 limit 格），满了返回 -1。
func freePoolItemSlot(pool []itemRec, limit int) int {
	for slot := 0; slot < limit; slot++ {
		if poolItemAt(pool, slot) == nil {
			return slot
		}
	}
	return -1
}

// commitPoolItems 在一个事务里改背包（writeItemDiff）并执行仓库表的改动 f，全部成功才提交。
func (s *Service) commitPoolItems(ctx context.Context, charID int64, before, after []itemRec, f func(tx pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := writeItemDiff(ctx, tx, charID, before, after); err != nil {
		return err
	}
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
