package character

// STAGE-310：管理命令 stoneage-admin grant --kind gold 批量发石币（在线、离线都能发）。
// 写库和 NOTIFY 在同一个事务里（FOR UPDATE 锁住角色行，按持有上限算实发数）；运行中的服务端
// 收到通知后，在内存上加上实发数并推送 status_changed。持有上限统一用 battle.MaxHaveGold
// （与交易、战斗奖励同一口径），超过上限封顶，实发数和应发数都回给调用方写进发放记录。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/athlan20/stoneage/server/battle"
)

// DefaultGoldNotifyChannel 是“角色石币变了”的 PostgreSQL 通知频道：stoneage-admin grant --kind gold。
// payload 是 “角色编号:实发数”。
const DefaultGoldNotifyChannel = "character_gold_changed"

// GoldGrant 是一次发石币的结果：Requested 是要发的数，Granted 是按持有上限实际加进去的数，
// Cap 是发放时的持有上限（封顶时写进发放记录的说明用）。
type GoldGrant struct {
	Requested, Granted, Cap int64
}

// Capped 表示因为持有上限少发了。
func (g GoldGrant) Capped() bool { return g.Granted < g.Requested }

// grantGold 算按持有上限最多能再加多少石币：gold 是现有石币，amount 是要加的数，maxHave 是上限。
// 已经到上限（或超过，比如旧数据）时返回 0。
func grantGold(gold, amount, maxHave int64) int64 {
	room := maxHave - gold
	if room <= 0 {
		return 0
	}
	if amount > room {
		return room
	}
	return amount
}

// GiveGold 是管理命令（stoneage-admin grant --kind gold）：给角色加 amount 石币，超过持有上限的部分不发。
// 不经过协议，直接操作数据库；角色不在线时只写库，等下次进入世界时可见。
func GiveGold(ctx context.Context, db *pgxpool.Pool, channel, character string, amount int) (GoldGrant, error) {
	if amount < 1 {
		return GoldGrant{}, errors.New("石币数量至少是 1")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return GoldGrant{}, err
	}
	defer tx.Rollback(ctx)
	var charID, gold int64
	err = tx.QueryRow(ctx,
		`SELECT id, gold FROM characters WHERE name = $1 FOR UPDATE`, character,
	).Scan(&charID, &gold)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoldGrant{}, fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return GoldGrant{}, err
	}
	maxHave := battle.MaxHaveGold
	res := GoldGrant{Requested: int64(amount), Granted: grantGold(gold, int64(amount), maxHave), Cap: maxHave}
	if _, err := tx.Exec(ctx, `UPDATE characters SET gold = gold + $2 WHERE id = $1`, charID, res.Granted); err != nil {
		return GoldGrant{}, err
	}
	payload := fmt.Sprintf("%d:%d", charID, res.Granted)
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, payload); err != nil {
		return GoldGrant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GoldGrant{}, err
	}
	slog.Default().Info("管理命令发了石币", "character", character, "requested", res.Requested, "granted", res.Granted)
	return res, nil
}

// parseGoldPayload 解析石币通知的 payload “角色编号:实发数”。
func parseGoldPayload(payload string) (charID, granted int64, err error) {
	idText, grantedText, ok := strings.Cut(payload, ":")
	if !ok {
		return 0, 0, fmt.Errorf("石币通知的 payload 格式不对: %q", payload)
	}
	if charID, err = strconv.ParseInt(idText, 10, 64); err != nil {
		return 0, 0, err
	}
	if granted, err = strconv.ParseInt(grantedText, 10, 64); err != nil {
		return 0, 0, err
	}
	return charID, granted, nil
}

// reloadGold 收到通知后在内存上加上实发数，推送 status_changed。不整行重读：在线角色身上可能有
// 还没存盘的石币变化（比如刚在商店买了东西），重读会把它们盖掉。角色不在世界中时跳过。
func (s *Service) reloadGold(payload string) {
	charID, granted, err := parseGoldPayload(payload)
	if err != nil {
		s.opts.Logger.Warn("石币通知的 payload 不对", "payload", payload, "err", err)
		return
	}
	if granted == 0 {
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.leaving {
		return
	}
	p.rec.Gold += grantGold(p.rec.Gold, granted, battle.MaxHaveGold)
	p.pushAll("status_changed", map[string]any{"status": statusLive(p, &p.rec)})
}

// SetCharacterGold 是测试用的管理命令（casetest 的 set_gold 行，STAGE-328）：直接设石币（0–持有上限）。
func SetCharacterGold(ctx context.Context, db *pgxpool.Pool, character string, gold int) error {
	if gold < 0 || int64(gold) > battle.MaxHaveGold {
		return fmt.Errorf("石币要在 0–%d 之间，不能是 %d", battle.MaxHaveGold, gold)
	}
	tag, err := db.Exec(ctx, `UPDATE characters SET gold = $2 WHERE name = $1`, character, gold)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.Join(pgx.ErrNoRows, fmt.Errorf("找不到角色 %s", character))
	}
	return nil
}
