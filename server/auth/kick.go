package auth

import (
	"context"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// enter 把连接标记为已登录，并挤掉同一账号、同一身份的旧连接：新连接登录成功，
// 旧连接先保存数据，再收到 KICKED_DUPLICATE_LOGIN 推送并被断开。玩家连接和 agent 连接互不挤占。
//
// 先登记、再复查（STAGE-14）：连接登记到账号名下之后，才去数据库确认账号没被禁用、令牌没被吊销。
// 这样禁用或吊销发生在登记之前，复查能发现；发生在登记之后，踢人时能找到这条连接。两种情况都不会漏。
// 复查不通过时撤销登记，登录失败。
func (s *Service) enter(ctx context.Context, sess *gateway.Session, id int64, name string) error {
	s.kickMu.Lock()
	var old []*gateway.Session
	for _, other := range s.hub.ByAccount(name) {
		if other != sess && other.Role() == sess.Role() {
			old = append(old, other)
		}
	}
	sess.SetAccount(name)
	sess.SetAccountID(id)
	s.kickMu.Unlock()

	if perr := s.stillAllowed(ctx, id, sess.TokenID()); perr != nil {
		sess.SetAccount("")
		sess.SetAccountID(0)
		return perr
	}
	sess.SetState(protocol.StateLoggedIn)
	sess.SetLoginAt(s.opts.Now())
	// 保存和断开在锁外做：保存得慢时，只有这个账号的登录要等，别人的登录不受影响（STAGE-13）。
	s.kick(old, protocol.ReasonKickedDuplicateLogin)
	return nil
}

// stillAllowed 复查账号没被禁用、令牌（agent 登录时）没有被吊销或过期。
func (s *Service) stillAllowed(ctx context.Context, accountID int64, tokenID string) *protocol.Error {
	var disabled, tokenOK bool
	err := s.db.QueryRow(ctx, `
		SELECT a.disabled, t.token_id IS NOT NULL
		FROM accounts a LEFT JOIN agent_tokens t
		  ON t.token_id = $2 AND t.account_id = a.id AND t.revoked_at IS NULL AND t.expires_at > $3
		WHERE a.id = $1`, accountID, tokenID, s.opts.Now()).Scan(&disabled, &tokenOK)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, "登录失败，请稍后重试")
	}
	switch {
	case disabled:
		return protocol.Errorf(CodeAccountDisabled, "账号已被禁用")
	case tokenID != "" && !tokenOK:
		return protocol.Errorf(protocol.CodeInvalidToken, "令牌无效、已过期或已被吊销")
	}
	return nil
}

// kick 先保存这些连接的数据，再推送断开原因并断开。不持有全服共用的锁：保存可能很慢。
func (s *Service) kick(sessions []*gateway.Session, reason string) {
	for _, sess := range sessions {
		if s.opts.BeforeKick != nil {
			s.opts.BeforeKick(sess)
		}
		sess.Disconnect(reason, "")
	}
}

// kickAccount 断开这个账号的所有连接（玩家和 agent 都有）：先保存数据，再推送 ACCOUNT_DISABLED。
func (s *Service) kickAccount(name string) {
	s.kick(s.hub.ByAccount(name), protocol.ReasonAccountDisabled)
}

// kickToken 让正在使用这个令牌的 agent 连接下线：先保存数据，再推送 AGENT_TOKEN_REVOKED（吊销和过期共用）。
func (s *Service) kickToken(account, tokenID string) {
	var users []*gateway.Session
	for _, sess := range s.hub.ByAccount(account) {
		if sess.Role() == protocol.RoleAgent && sess.TokenID() == tokenID {
			users = append(users, sess)
		}
	}
	s.kick(users, protocol.ReasonAgentTokenRevoked)
}

// expiryLoop 定时检查在线 agent 用的令牌有没有到期（STAGE-15），以及在线时长有没有超过上限（STAGE-118）。
func (s *Service) expiryLoop(ctx context.Context) {
	defer close(s.expiryDone)
	t := time.NewTicker(s.opts.TokenCheckInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.kickOvertimeAgents(ctx)
			s.sweepExpiredTokens(ctx)
		}
	}
}

// sweepExpiredTokens 让所有“令牌已到期（或已被吊销）却仍在线”的 agent 下线，和吊销走同一条路径。
func (s *Service) sweepExpiredTokens(ctx context.Context) {
	online := map[string]string{} // token_id → 账号名
	for _, sess := range s.hub.Sessions() {
		if sess.Role() == protocol.RoleAgent && sess.TokenID() != "" && sess.Account() != "" {
			online[sess.TokenID()] = sess.Account()
		}
	}
	if len(online) == 0 {
		return
	}
	ids := make([]string, 0, len(online))
	for id := range online {
		ids = append(ids, id)
	}
	rows, err := s.db.Query(ctx, `SELECT token_id FROM agent_tokens WHERE token_id = ANY($1) AND (revoked_at IS NOT NULL OR expires_at <= $2)`,
		ids, s.opts.Now())
	if err != nil {
		s.opts.Logger.Warn("检查在线 agent 的令牌有效期失败", "error", err)
		return
	}
	var dead []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			dead = append(dead, id)
		}
	}
	rows.Close()
	for _, id := range dead {
		s.kickToken(online[id], id)
		s.audit(context.WithoutCancel(ctx), "token_expire", online[id], "", true, id)
	}
}
