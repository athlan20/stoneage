package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type accountRow struct {
	id       int64
	hash     string
	disabled bool
}

func (s *Service) accountExists(ctx context.Context, name string) (bool, error) {
	var one int
	err := s.db.QueryRow(ctx, `SELECT 1 FROM accounts WHERE name = $1`, name).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) findAccount(ctx context.Context, name string) (accountRow, bool, error) {
	if !ValidAccount(name) {
		return accountRow{}, false, nil
	}
	var a accountRow
	err := s.db.QueryRow(ctx, `SELECT id, password_hash, disabled FROM accounts WHERE name = $1`, name).Scan(&a.id, &a.hash, &a.disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return accountRow{}, false, nil
	}
	return a, err == nil, err
}

// audit 写一条审计记录：账号、来源 IP、结果、时间。失败只记日志，不影响请求。
func (s *Service) audit(ctx context.Context, kind, account, ip string, success bool, result string) {
	_, err := s.db.Exec(ctx, `INSERT INTO auth_audit (at, kind, account, ip, success, result) VALUES ($1, $2, $3, $4, $5, $6)`,
		s.opts.Now(), kind, auditName(account), ip, success, result)
	if err != nil {
		s.opts.Logger.Error("写审计记录失败", "err", err)
	}
}

// countRegistrations 统计某 IP 在 since 之后的注册次数（成功或因账号已存在而失败的都算），并返回其中最早一次的时间。
func (s *Service) countRegistrations(ctx context.Context, ip string, since time.Time) (int, time.Time, error) {
	return s.countAudit(ctx, `kind = 'register' AND ip = $1 AND at > $2 AND result IN ('ok', 'ACCOUNT_EXISTS')`, ip, since)
}

// countIPFailures 统计某 IP 在 since 之后的密码错误和令牌无效次数（login 和 agent_login 共用同一套限制）。
func (s *Service) countIPFailures(ctx context.Context, ip string, since time.Time) (int, time.Time, error) {
	return s.countAudit(ctx, `kind IN ('login', 'agent_login') AND ip = $1 AND at > $2 AND result IN ('WRONG_CREDENTIALS', 'INVALID_TOKEN')`, ip, since)
}

func (s *Service) countAudit(ctx context.Context, where string, ip string, since time.Time) (int, time.Time, error) {
	var n int
	var oldest *time.Time
	err := s.db.QueryRow(ctx, `SELECT count(*), min(at) FROM auth_audit WHERE `+where, ip, since).Scan(&n, &oldest)
	if err != nil || oldest == nil {
		return n, time.Time{}, err
	}
	return n, *oldest, nil
}

func (s *Service) lockedUntil(ctx context.Context, name string, now time.Time) (time.Time, bool, error) {
	var until time.Time
	err := s.db.QueryRow(ctx, `SELECT locked_until FROM login_locks WHERE account = $1 AND locked_until > $2`, name, now).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return until, err == nil, err
}

// maybeLock 在窗口内的失败次数达到上限时锁定账号名。上一次成功登录之前的失败不再计数。
func (s *Service) maybeLock(ctx context.Context, name string, now time.Time) error {
	lim := s.opts.Limits
	var n int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM auth_audit
		WHERE kind = 'login' AND account = $1 AND result = 'WRONG_CREDENTIALS' AND at > $2
		  AND at > COALESCE((SELECT max(at) FROM auth_audit WHERE kind = 'login' AND account = $1 AND success), '-infinity')`,
		auditName(name), now.Add(-lim.LoginFailWindow)).Scan(&n)
	if err != nil || n < lim.LoginFailMax {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO login_locks (account, locked_until) VALUES ($1, $2)
		ON CONFLICT (account) DO UPDATE SET locked_until = EXCLUDED.locked_until`, auditName(name), now.Add(lim.LockDuration))
	return err
}
