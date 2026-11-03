// 背包（STAGE-28，docs/protocol.md 5.17、5.18）：inventory 查询、GiveItem 管理命令，
// 以及 character_items_changed 通知后的重读。槽位 0–4 装备、5–19 背包（docs/ui.md 背包页）。
// 通知的监听在 notify.go。

package character

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// DefaultItemNotifyChannel 是“角色物品变了”的 PostgreSQL 通知频道：stoneage-admin give
// 在同一个事务里 NOTIFY，运行中的服务端收到后重读该角色的物品并推送 inventory_changed。
const DefaultItemNotifyChannel = "character_items_changed"

// 背包格数与编号（docs/ui.md 背包页：0–4 装备，5–19 背包）。
const (
	MinItemSlot  = 0
	MaxItemSlot  = 19
	firstBagSlot = 5
)

// itemRec 是 character_items 表的一行。
type itemRec struct {
	Slot   int
	ItemID int
	Count  int
	Merged bool // 合成品标记（STAGE-207，原版 ITEM_MERGEFLG）：只有加工、料理的产物为真，效果见 STAGE-209
	// 装备耐久（STAGE-209，原版 ITEM_DAMAGECRUSHE / ITEM_MAXDAMAGECRUSHE / ITEM_CRUSHLEVEL，docs/durability.md）：
	// 最大耐久为 0 的物品不损坏；CrushLevel 0 完好、1 受损、2 毁坏、3 不堪使用。
	Durability    int
	MaxDurability int
	CrushLevel    int
	// 精工的实例覆盖（STAGE-298，docs/inlay.md 2.4）；没镶过为 nil。
	Inlay *inlayState
}

// loadItems 读角色现有的物品，按槽位从小到大。
func (s *Service) loadItems(ctx context.Context, charID int64) ([]itemRec, error) {
	return readItems(ctx, s.db, charID, false)
}

// dbQuerier 是连接池和事务共有的查询方法，物品的读取既可以在池上做，也可以在事务里加锁做。
type dbQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// readItems 读角色的物品，按槽位从小到大；lock 为真时加行锁（在事务里用，见 equip.go）。
func readItems(ctx context.Context, q dbQuerier, charID int64, lock bool) ([]itemRec, error) {
	query := `SELECT slot, item_id, count, merged, durability, max_durability, crush_level, inlay FROM character_items WHERE character_id = $1 ORDER BY slot`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, query, charID)
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

// handleInventory 是查看类请求（docs/protocol.md 5.17）：从内存里的角色数据出背包快照。
func (s *Service) handleInventory(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := l.With(func(r *record) { out = r.inventoryView() }); err != nil {
		return nil, asProtocolError(err)
	}
	return out, nil
}

// GiveItem 是管理命令（stoneage-admin give 与协议用例的 admin 行）：把物品放进角色第一个
// 空的背包格，在同一个事务里 NOTIFY，让正在运行的服务端重读并推送。不经过协议，直接操作数据库；
// 角色不在线时只写库，等下次进入世界时可见。
func GiveItem(ctx context.Context, db *pgxpool.Pool, channel, character string, itemID, count int) error {
	def, ok := gamedata.ItemByID(itemID)
	if !ok {
		return fmt.Errorf("物品编号 %d 不存在", itemID)
	}
	if count < 1 {
		return errors.New("数量至少是 1")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, character).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT slot FROM character_items WHERE character_id = $1 AND slot >= $2`, charID, firstBagSlot)
	if err != nil {
		return err
	}
	taken := [MaxItemSlot + 1]bool{}
	for rows.Next() {
		var slot int
		if err := rows.Scan(&slot); err != nil {
			rows.Close()
			return err
		}
		taken[slot] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	free := -1
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		if !taken[slot] {
			free = slot
			break
		}
	}
	if free < 0 {
		return fmt.Errorf("%s 的背包已满（15 格都有物品）", character)
	}
	durability, maxDurability := newItemDurability(itemID)
	if _, err := tx.Exec(ctx,
		`INSERT INTO character_items (character_id, slot, item_id, count, durability, max_durability) VALUES ($1, $2, $3, $4, $5, $6)`,
		charID, free, itemID, count, durability, maxDurability); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Default().Info("管理命令发了物品", "character", character, "item", def.Name, "slot", free, "count", count)
	return nil
}

// reloadItems 重读角色的物品并推送 inventory_changed。角色不在世界中时没有可通知的连接，跳过。
func (s *Service) reloadItems(ctx context.Context, payload string) {
	charID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.opts.Logger.Warn("物品通知的 payload 不是角色编号", "payload", payload)
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	items, err := s.loadItems(ctx, charID)
	if err != nil {
		s.opts.Logger.Warn("重读角色物品失败", "character", charID, "err", err)
		return
	}
	p.mu.Lock()
	if !p.leaving {
		magicBefore := p.rec.magicSlots()
		p.rec.Items = items
		p.pushAll("inventory_changed", p.rec.inventoryView())
		if p.rec.magicSlots() != magicBefore { // 管理命令改了装备位上的东西，装备自带的精灵跟着变（STAGE-190）
			p.pushAll("status_changed", map[string]any{"status": p.rec.status()})
		}
	}
	p.mu.Unlock()
}
