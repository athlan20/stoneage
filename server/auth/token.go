package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/protocol"
)

// agent 令牌（docs/protocol.md 9.1、9.2）。令牌原文只在生成时返回一次，数据库里只存 SHA-256：
// 令牌是 256 位随机数，没法暴力猜，所以不需要像密码那样用慢哈希。
const (
	TokenMax         = 5   // 每个账号最多几个有效令牌
	DefaultTokenDays = 30  // 有效期默认天数
	MaxTokenDays     = 365 // 有效期最长天数
	tokenLabelMax    = 32  // 备注最长字符数
	tokenPrefix      = "sat_"
)

// TokenInfo 是令牌的公开信息（不含令牌原文）。
type TokenInfo struct {
	TokenID    string
	Label      string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomString(n int, alphabet string) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func validateLabel(label string) (string, *protocol.Error) {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > tokenLabelMax {
		return "", protocol.Errorf(protocol.CodeBadRequest, "备注最长 %d 个字符", tokenLabelMax)
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return "", protocol.Errorf(protocol.CodeBadRequest, "备注不能包含控制字符")
		}
	}
	return label, nil
}

// CreateToken 为账号生成一个令牌，返回令牌原文（之后再也取不到）和公开信息。
// days 为 0 时取默认值；超出 1–365 回复 BAD_REQUEST；有效令牌已满回复 TOKEN_LIMIT。
func (s *Service) CreateToken(ctx context.Context, accountID int64, label string, days int) (string, TokenInfo, error) {
	label, perr := validateLabel(label)
	if perr != nil {
		return "", TokenInfo{}, perr
	}
	if days == 0 {
		days = DefaultTokenDays
	}
	if days < 1 || days > MaxTokenDays {
		return "", TokenInfo{}, protocol.Errorf(protocol.CodeBadRequest, "有效期要在 1–%d 天之间", MaxTokenDays)
	}
	token, err := newToken()
	if err != nil {
		return "", TokenInfo{}, err
	}
	now := s.opts.Now()
	info := TokenInfo{Label: label, CreatedAt: now, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour)}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", TokenInfo{}, err
	}
	defer tx.Rollback(ctx)
	// 锁住账号行，让“数一数有几个、再插入”对同一账号串行，并发生成不会超过上限。
	if _, err := tx.Exec(ctx, `SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE`, accountID); err != nil {
		return "", TokenInfo{}, err
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM agent_tokens WHERE account_id = $1 AND revoked_at IS NULL AND expires_at > $2`,
		accountID, now).Scan(&active); err != nil {
		return "", TokenInfo{}, err
	}
	if active >= TokenMax {
		return "", TokenInfo{}, protocol.Errorf(protocol.CodeTokenLimit, "有效令牌已达上限（%d 个），请先吊销不用的令牌", TokenMax)
	}
	suffix, err := randomString(8, "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ")
	if err != nil {
		return "", TokenInfo{}, err
	}
	info.TokenID = "t_" + suffix
	if _, err := tx.Exec(ctx, `INSERT INTO agent_tokens (token_id, account_id, label, token_hash, created_at, expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		info.TokenID, accountID, label, hashToken(token), info.CreatedAt, info.ExpiresAt); err != nil {
		return "", TokenInfo{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", TokenInfo{}, err
	}
	return token, info, nil
}

// ListTokens 列出账号的有效令牌（没吊销、没过期），按生成顺序排。
func (s *Service) ListTokens(ctx context.Context, accountID int64) ([]TokenInfo, error) {
	rows, err := s.db.Query(ctx, `SELECT token_id, label, created_at, expires_at, last_used_at FROM agent_tokens
		WHERE account_id = $1 AND revoked_at IS NULL AND expires_at > $2 ORDER BY id`, accountID, s.opts.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TokenInfo
	for rows.Next() {
		var t TokenInfo
		if err := rows.Scan(&t.TokenID, &t.Label, &t.CreatedAt, &t.ExpiresAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken 吊销令牌，并让正在使用它的 agent 连接立即下线。只能吊销自己账号的令牌。
func (s *Service) RevokeToken(ctx context.Context, accountID int64, account, tokenID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE agent_tokens SET revoked_at = $3 WHERE account_id = $1 AND token_id = $2 AND revoked_at IS NULL`,
		accountID, tokenID, s.opts.Now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return protocol.Errorf(protocol.CodeTokenNotFound, "没有这个令牌")
	}
	s.kickToken(account, tokenID)
	return nil
}

// AgentLogin 校验令牌；成功时返回账号 id、账号名和令牌编号，调用方负责把连接标记为已登录。
// 失败次数的限制和 login 共用同一套：同一 IP 10 分钟内失败 20 次就限制。
func (s *Service) AgentLogin(ctx context.Context, ip, token string) (accountID int64, name, tokenID string, err error) {
	unlock, err := s.attempts.lockAll(ctx, "login-ip:"+ip)
	if err != nil {
		return 0, "", "", err
	}
	defer unlock()
	rctx := context.WithoutCancel(ctx)
	lim := s.opts.Limits
	now := s.opts.Now()
	fail := func(e *protocol.Error) (int64, string, string, error) {
		s.audit(rctx, "agent_login", "", ip, false, e.Code)
		return 0, "", "", e
	}

	n, oldest, err := s.countIPFailures(ctx, ip, now.Add(-lim.IPFailWindow))
	if err != nil {
		return 0, "", "", err
	}
	if n >= lim.IPFailMax {
		return fail(rateLimited(oldest.Add(lim.IPFailWindow).Sub(now), "登录失败次数过多"))
	}
	invalid := protocol.Errorf(protocol.CodeInvalidToken, "令牌无效、已过期或已被吊销")
	if !strings.HasPrefix(token, tokenPrefix) || len(token) > 200 {
		return fail(invalid)
	}
	var disabled bool
	err = s.db.QueryRow(ctx, `
		SELECT t.account_id, a.name, t.token_id, a.disabled
		FROM agent_tokens t JOIN accounts a ON a.id = t.account_id
		WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND t.expires_at > $2`,
		hashToken(token), now).Scan(&accountID, &name, &tokenID, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(invalid)
	}
	if err != nil {
		return 0, "", "", err
	}
	if disabled {
		s.audit(rctx, "agent_login", name, ip, false, CodeAccountDisabled)
		return 0, "", "", protocol.Errorf(CodeAccountDisabled, "账号已被禁用")
	}
	if _, err := s.db.Exec(rctx, `UPDATE agent_tokens SET last_used_at = $2 WHERE token_id = $1`, tokenID, now); err != nil {
		return 0, "", "", err
	}
	s.audit(rctx, "agent_login", name, ip, true, "ok")
	return accountID, name, tokenID, nil
}
