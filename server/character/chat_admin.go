package character

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// STAGE-278：公开聊天禁言的管理命令（stoneage-admin mute / unmute / mutes，casetest 的 chat_mute / chat_unmute 行）。
// 时间都用数据库的 now() 算，服务端发言时读库比较，所以在线的角色立即生效。

// MutedCharacter 是仍在禁言中的角色（mutes 列出用）。
type MutedCharacter struct {
	Name  string
	Until time.Time
}

// ChatMuteFor 禁言角色 name 从现在起 d 这么长时间（不足 1 秒按 1 秒，超出的部分不限）。
func ChatMuteFor(ctx context.Context, db *pgxpool.Pool, name string, d time.Duration) (time.Time, error) {
	secs := d.Seconds()
	if secs < 1 {
		secs = 1
	}
	var until time.Time
	err := db.QueryRow(ctx, `UPDATE characters SET chat_muted_until = now() + ($2::float8 * interval '1 second')
		WHERE name = $1 RETURNING chat_muted_until`, name, secs).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, fmt.Errorf("找不到角色 %s", name)
	}
	return until, err
}

// ChatUnmute 解除角色 name 的禁言（立即生效）。
func ChatUnmute(ctx context.Context, db *pgxpool.Pool, name string) error {
	tag, err := db.Exec(ctx, `UPDATE characters SET chat_muted_until = NULL WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("找不到角色 %s", name)
	}
	return nil
}

// ListMutes 列出仍在禁言中的角色，按解除时间从早到晚排。
func ListMutes(ctx context.Context, db *pgxpool.Pool) ([]MutedCharacter, error) {
	rows, err := db.Query(ctx, `SELECT name, chat_muted_until FROM characters
		WHERE chat_muted_until > now() ORDER BY chat_muted_until, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MutedCharacter
	for rows.Next() {
		var m MutedCharacter
		if err := rows.Scan(&m.Name, &m.Until); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
