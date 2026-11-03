package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

func registered(t *testing.T, svc *Service, name string) int64 {
	t.Helper()
	ctx := context.Background()
	if err := svc.Register(ctx, "9.9.9.9", name, "secret1"); err != nil {
		t.Fatal(err)
	}
	id, _, err := svc.Login(ctx, "9.9.9.9", name, "secret1")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTokenCreateListRevoke(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	id := registered(t, svc, "alice")

	token, info, err := svc.CreateToken(ctx, id, "  我的 Claude  ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "sat_") || len(token) < 40 {
		t.Errorf("令牌格式不对: %q", token)
	}
	if !strings.HasPrefix(info.TokenID, "t_") || info.Label != "我的 Claude" {
		t.Errorf("令牌信息不对: %+v", info)
	}
	if got := info.ExpiresAt.Sub(clk.now()); got != 30*24*time.Hour {
		t.Errorf("默认有效期应是 30 天，实际 %v", got)
	}
	list, err := svc.ListTokens(ctx, id)
	if err != nil || len(list) != 1 || list[0].TokenID != info.TokenID || list[0].LastUsedAt != nil {
		t.Fatalf("列表不对: %+v %v", list, err)
	}

	// 登录成功后记录最后使用时间。
	accID, name, tokenID, err := svc.AgentLogin(ctx, "1.1.1.1", token)
	if err != nil || accID != id || name != "alice" || tokenID != info.TokenID {
		t.Fatalf("AgentLogin = %d %q %q %v", accID, name, tokenID, err)
	}
	if list, _ = svc.ListTokens(ctx, id); list[0].LastUsedAt == nil {
		t.Error("登录后应记录最后使用时间")
	}

	// 吊销：之后登录不了，列表里也没有；再吊销一次、吊销不存在的，都是 TOKEN_NOT_FOUND。
	if err := svc.RevokeToken(ctx, id, "alice", info.TokenID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.AgentLogin(ctx, "1.1.1.1", token); code(err) != protocol.CodeInvalidToken {
		t.Errorf("吊销后应 INVALID_TOKEN，实际 %q", code(err))
	}
	if list, _ = svc.ListTokens(ctx, id); len(list) != 0 {
		t.Errorf("吊销后列表应为空: %+v", list)
	}
	for _, tid := range []string{info.TokenID, "t_nonexistent"} {
		if err := svc.RevokeToken(ctx, id, "alice", tid); code(err) != protocol.CodeTokenNotFound {
			t.Errorf("吊销 %s 应 TOKEN_NOT_FOUND，实际 %q", tid, code(err))
		}
	}
}

func TestTokenValidation(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := registered(t, svc, "alice")
	for name, tc := range map[string]struct {
		label string
		days  int
	}{
		"0 天以下": {"", -1},
		"366 天": {"", 366},
		"备注过长":  {strings.Repeat("字", 33), 30},
		"备注含控制": {"a\x07b", 30},
	} {
		if _, _, err := svc.CreateToken(ctx, id, tc.label, tc.days); code(err) != protocol.CodeBadRequest {
			t.Errorf("%s 应 BAD_REQUEST，实际 %q", name, code(err))
		}
	}
	for _, days := range []int{1, 365} {
		if _, info, err := svc.CreateToken(ctx, id, strings.Repeat("字", 32), days); err != nil || info.ExpiresAt.Sub(info.CreatedAt) != time.Duration(days)*24*time.Hour {
			t.Errorf("%d 天应合法: %v", days, err)
		}
	}
}

// 每个账号最多 5 个有效令牌；吊销或过期的不占名额；别的账号不受影响；并发生成也不会超过上限。
func TestTokenLimit(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	id := registered(t, svc, "alice")
	other := registered(t, svc, "bob_x")
	var first TokenInfo
	for i := 0; i < TokenMax; i++ {
		days := 1 + i // 1、2、3、4、5 天；第 2 个改成 1 天，后面测过期
		if i == 1 {
			days = 1
		}
		_, info, err := svc.CreateToken(ctx, id, fmt.Sprint(i), days)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = info
		}
	}
	if _, _, err := svc.CreateToken(ctx, id, "多余", 30); code(err) != protocol.CodeTokenLimit {
		t.Errorf("第 6 个应 TOKEN_LIMIT，实际 %q", code(err))
	}
	if _, _, err := svc.CreateToken(ctx, other, "别人", 30); err != nil {
		t.Errorf("别的账号不受限制: %v", err)
	}
	if err := svc.RevokeToken(ctx, id, "alice", first.TokenID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateToken(ctx, id, "补上", 30); err != nil {
		t.Errorf("吊销一个后应能再生成: %v", err)
	}
	if _, _, err := svc.CreateToken(ctx, id, "多余", 30); code(err) != protocol.CodeTokenLimit {
		t.Errorf("又满了应 TOKEN_LIMIT，实际 %q", code(err))
	}
	clk.advance(25 * time.Hour) // 有效期 1 天的令牌过期了，名额空出来
	if _, _, err := svc.CreateToken(ctx, id, "过期后", 30); err != nil {
		t.Errorf("过期的令牌不占名额: %v", err)
	}

	counts := run(20, func(i int) error {
		_, _, err := svc.CreateToken(ctx, other, "并发", 30)
		return err
	})
	if list, _ := svc.ListTokens(ctx, other); len(list) != TokenMax {
		t.Errorf("并发生成后应正好 %d 个有效令牌，实际 %d（%v）", TokenMax, len(list), counts)
	}
}

func TestAgentLoginRejects(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	id := registered(t, svc, "alice")
	token, _, err := svc.CreateToken(ctx, id, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	for name, tok := range map[string]string{
		"空": "", "没有前缀": "abc", "格式对但不存在": "sat_" + strings.Repeat("A", 43), "超长": "sat_" + strings.Repeat("A", 500),
		"改了一位": token[:len(token)-1] + "x",
	} {
		if _, _, _, err := svc.AgentLogin(ctx, "2.2.2.2", tok); code(err) != protocol.CodeInvalidToken {
			t.Errorf("%s 应 INVALID_TOKEN，实际 %q", name, code(err))
		}
	}
	// 过期。
	clk.advance(25 * time.Hour)
	if _, _, _, err := svc.AgentLogin(ctx, "3.3.3.3", token); code(err) != protocol.CodeInvalidToken {
		t.Errorf("过期的令牌应 INVALID_TOKEN，实际 %q", code(err))
	}
	// 账号被禁用：令牌有效，回复 ACCOUNT_DISABLED。
	token2, _, _ := svc.CreateToken(ctx, id, "", 30)
	if err := svc.SetDisabled(ctx, "alice", true, "测试"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.AgentLogin(ctx, "4.4.4.4", token2); code(err) != CodeAccountDisabled {
		t.Errorf("禁用的账号应 ACCOUNT_DISABLED，实际 %q", code(err))
	}
}

// 令牌登录的失败和密码登录共用 IP 限制：同一 IP 10 分钟内失败 20 次就限制，之后连正确的令牌也不行。
func TestAgentLoginSharesIPLimit(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := registered(t, svc, "alice")
	token, _, _ := svc.CreateToken(ctx, id, "", 30)
	for i := 0; i < 10; i++ {
		_, _, _, err := svc.AgentLogin(ctx, "5.5.5.5", "sat_bad")
		if code(err) != protocol.CodeInvalidToken {
			t.Fatalf("第 %d 次应 INVALID_TOKEN，实际 %q", i, code(err))
		}
		if _, _, err := svc.Login(ctx, "5.5.5.5", "alice", "wrongpw"); i < 5 && code(err) != CodeWrongCredentials {
			// 账号会被锁定，之后是 ACCOUNT_LOCKED，不影响 IP 计数（只数 WRONG_CREDENTIALS 和 INVALID_TOKEN）
			t.Fatalf("第 %d 次密码错误: %q", i, code(err))
		}
	}
	// 20 次失败（10 次令牌 + 至少 5 次密码）还没到？补到 20。
	for i := 0; i < 20; i++ {
		_, _, _, _ = svc.AgentLogin(ctx, "5.5.5.5", "sat_bad")
	}
	if _, _, _, err := svc.AgentLogin(ctx, "5.5.5.5", token); code(err) != protocol.CodeRateLimited {
		t.Errorf("失败太多后应 RATE_LIMITED，实际 %q", code(err))
	}
	if _, _, _, err := svc.AgentLogin(ctx, "6.6.6.6", token); err != nil {
		t.Errorf("别的 IP 不受影响: %v", err)
	}
}

// 令牌原文不落库：表里只有哈希，没有任何一列等于或包含令牌；审计里记了生成和登录。
func TestTokenNotStoredInPlaintext(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := registered(t, svc, "alice")
	token, info, _ := svc.CreateToken(ctx, id, "x", 30)
	if _, _, _, err := svc.AgentLogin(ctx, "1.1.1.1", token); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"agent_tokens", "auth_audit"} {
		rows, err := svc.db.Query(ctx, fmt.Sprintf("SELECT t::text FROM %s t", q))
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			if strings.Contains(s, token) || strings.Contains(s, strings.TrimPrefix(token, "sat_")) {
				t.Errorf("%s 里出现了令牌原文: %s", q, s)
			}
		}
		rows.Close()
	}
	var hash string
	if err := svc.db.QueryRow(ctx, `SELECT token_hash FROM agent_tokens WHERE token_id = $1`, info.TokenID).Scan(&hash); err != nil || len(hash) != 64 {
		t.Errorf("应保存 SHA-256 哈希（64 位十六进制），实际 %q %v", hash, err)
	}
	var n int
	_ = svc.db.QueryRow(ctx, `SELECT count(*) FROM auth_audit WHERE kind IN ('agent_login') AND success AND account = 'alice'`).Scan(&n)
	if n != 1 {
		t.Errorf("应有 1 条成功的 agent_login 审计，实际 %d", n)
	}
}
