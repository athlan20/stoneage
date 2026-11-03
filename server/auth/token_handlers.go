package auth

import (
	"context"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

type tokenView struct {
	TokenID    string  `json:"token_id"`
	Label      string  `json:"label"`
	CreatedAt  string  `json:"created_at"`
	ExpiresAt  string  `json:"expires_at"`
	LastUsedAt *string `json:"last_used_at"`
}

func viewOf(t TokenInfo) tokenView {
	v := tokenView{TokenID: t.TokenID, Label: t.Label, CreatedAt: rfc3339(t.CreatedAt), ExpiresAt: rfc3339(t.ExpiresAt)}
	if t.LastUsedAt != nil {
		s := rfc3339(*t.LastUsedAt)
		v.LastUsedAt = &s
	}
	return v
}

func (s *Service) handleCreateToken(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Label         string `json:"label"`
		ExpiresInDays *int   `json:"expires_in_days"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	days := 0
	if in.ExpiresInDays != nil {
		days = *in.ExpiresInDays
		if days == 0 { // 显式写 0 不算“没填”
			return nil, protocol.Errorf(protocol.CodeBadRequest, "有效期要在 1–%d 天之间", MaxTokenDays)
		}
	}
	token, info, err := s.CreateToken(ctx, sess.AccountID(), in.Label, days)
	if err != nil {
		return nil, err
	}
	s.audit(context.WithoutCancel(ctx), "token_create", sess.Account(), clientIP(sess), true, info.TokenID)
	return map[string]any{"token_id": info.TokenID, "token": token, "label": info.Label, "expires_at": rfc3339(info.ExpiresAt)}, nil
}

func (s *Service) handleTokens(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	list, err := s.ListTokens(ctx, sess.AccountID())
	if err != nil {
		return nil, err
	}
	views := make([]tokenView, 0, len(list))
	for _, t := range list {
		views = append(views, viewOf(t))
	}
	return map[string]any{"tokens": views}, nil
}

func (s *Service) handleRevokeToken(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		TokenID *string `json:"token_id"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.TokenID == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 token_id 字段")
	}
	if err := s.RevokeToken(ctx, sess.AccountID(), sess.Account(), *in.TokenID); err != nil {
		return nil, err
	}
	s.audit(context.WithoutCancel(ctx), "token_revoke", sess.Account(), clientIP(sess), true, *in.TokenID)
	return map[string]any{}, nil
}

func (s *Service) handleAgentLogin(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Token *string `json:"token"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Token == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 token 字段")
	}
	id, name, tokenID, err := s.AgentLogin(ctx, clientIP(sess), *in.Token)
	if err != nil {
		return nil, err
	}
	sess.SetTokenID(tokenID)
	if s.beforeEnter != nil {
		s.beforeEnter()
	}
	if err := s.enter(ctx, sess, id, name); err != nil {
		sess.SetTokenID("")
		return nil, err
	}
	return map[string]any{"account": name, "token_id": tokenID}, nil
}
