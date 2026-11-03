package auth

import (
	"context"
	"fmt"
	"time"
)

// AccountInfo 是账号管理视图（不含密码哈希）。
type AccountInfo struct {
	Name           string
	Disabled       bool
	DisabledReason string
	CreatedAt      time.Time
	LastLoginAt    *time.Time
	LockedUntil    *time.Time
}

// SetDisabled 禁用或启用账号。禁用时在同一个事务里 NOTIFY，正在运行的服务端收到后
// 立即断开这个账号的所有连接（见 Start）；服务端没在运行时，下次登录自然被拒。
func (s *Service) SetDisabled(ctx context.Context, account string, disabled bool, reason string) error {
	if !disabled {
		reason = ""
	}
	name := NormalizeAccount(account)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE accounts SET disabled = $2, disabled_reason = $3 WHERE name = $1`, name, disabled, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("账号 %q 不存在", account)
	}
	if disabled {
		if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, s.opts.NotifyChannel, name); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Unlock 解除登录锁定。
func (s *Service) Unlock(ctx context.Context, account string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM login_locks WHERE account = $1`, NormalizeAccount(account))
	return err
}

// ListAccounts 列出所有账号，按 id 排序。
func (s *Service) ListAccounts(ctx context.Context) ([]AccountInfo, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.name, a.disabled, a.disabled_reason, a.created_at, a.last_login_at,
		       (SELECT l.locked_until FROM login_locks l WHERE l.account = a.name AND l.locked_until > $1)
		FROM accounts a ORDER BY a.id`, s.opts.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountInfo
	for rows.Next() {
		var a AccountInfo
		if err := rows.Scan(&a.Name, &a.Disabled, &a.DisabledReason, &a.CreatedAt, &a.LastLoginAt, &a.LockedUntil); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RunAdminAction 执行协议用例里的 admin 行和管理命令：disable_account、enable_account、unlock_account。
func (s *Service) RunAdminAction(ctx context.Context, do, account string) error {
	switch do {
	case "disable_account":
		return s.SetDisabled(ctx, account, true, "管理员禁用")
	case "enable_account":
		return s.SetDisabled(ctx, account, false, "")
	case "unlock_account":
		return s.Unlock(ctx, account)
	}
	return fmt.Errorf("不认识的管理操作 %q", do)
}
