// 人物的精灵（STAGE-48，docs/protocol.md 10.5）：管理命令 givemagic 写库并通知，
// 运行中的服务端收到通知后重读这一列并推送 status_changed（status 里多了 magic 字段）。

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
)

// DefaultMagicNotifyChannel 是“角色精灵变了”的 PostgreSQL 通知频道：stoneage-admin givemagic
// 在同一个事务里 NOTIFY，运行中的服务端收到后重读并推送 status_changed。
const DefaultMagicNotifyChannel = "character_magics_changed"

// GiveMagic 是管理命令（stoneage-admin givemagic 与协议用例的 admin 行）：
// 把精灵放进角色第一个空的精灵栏位，同一个事务里 NOTIFY。
// 角色不在线时只写库，等下次进入世界时可见。
func GiveMagic(ctx context.Context, db *pgxpool.Pool, channel, character string, magicID int) error {
	t, ok := gamedata.MagicByID(magicID)
	if !ok {
		return fmt.Errorf("精灵编号 %d 不在魔法表里", magicID)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	var magics []int32
	err = tx.QueryRow(ctx, `SELECT id, magics FROM characters WHERE name = $1`, character).Scan(&charID, &magics)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return err
	}
	free := -1
	for i, id := range magics {
		if id < 0 {
			free = i
			break
		}
	}
	if free < 0 {
		return fmt.Errorf("%s 的精灵栏位已满（%d 个）", character, gamedata.MaxMagicSlots)
	}
	magics[free] = int32(magicID)
	if _, err := tx.Exec(ctx, `UPDATE characters SET magics = $2 WHERE id = $1`, charID, magics); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Default().Info("管理命令给了精灵", "character", character, "magic", t.Name, "slot", free)
	return nil
}

// reloadMagics 重读角色的精灵栏位并推送 status_changed（数值全在 status 里，magic 字段跟着走）。
func (s *Service) reloadMagics(ctx context.Context, payload string) {
	charID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.opts.Logger.Warn("精灵通知的 payload 不是角色编号", "payload", payload)
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	var magics []int32
	err = s.db.QueryRow(ctx, `SELECT magics FROM characters WHERE id = $1`, charID).Scan(&magics)
	if err != nil {
		s.opts.Logger.Warn("重读角色精灵失败", "character", charID, "err", err)
		return
	}
	p.mu.Lock()
	if !p.leaving {
		for i := range p.rec.Magics {
			p.rec.Magics[i] = -1
			if i < len(magics) {
				p.rec.Magics[i] = int(magics[i])
			}
		}
		p.pushAll("status_changed", map[string]any{"status": p.rec.status()})
	}
	p.mu.Unlock()
}
