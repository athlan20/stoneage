package auth

import (
	"context"
	"net"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

type credentials struct {
	Account  *string `json:"account"`
	Password *string `json:"password"`
}

func decodeCredentials(req *protocol.Request) (account, password string, perr *protocol.Error) {
	var in credentials
	if perr := req.Decode(&in); perr != nil {
		return "", "", perr
	}
	if in.Account == nil || in.Password == nil {
		return "", "", protocol.Errorf(protocol.CodeBadRequest, "需要 account 和 password 字段")
	}
	return *in.Account, *in.Password, nil
}

func clientIP(sess *gateway.Session) string {
	host, _, err := net.SplitHostPort(sess.RemoteAddr().String())
	if err != nil {
		return sess.RemoteAddr().String()
	}
	return host
}

func (s *Service) handleRegister(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	account, password, perr := decodeCredentials(req)
	if perr != nil {
		return nil, perr
	}
	if err := s.Register(ctx, clientIP(sess), account, password); err != nil {
		return nil, err
	}
	// 注册后不会自动登录，连接仍处于“已握手”状态。
	return map[string]any{"account": NormalizeAccount(account)}, nil
}

func (s *Service) handleLogin(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	account, password, perr := decodeCredentials(req)
	if perr != nil {
		return nil, perr
	}
	id, name, err := s.Login(ctx, clientIP(sess), account, password)
	if err != nil {
		return nil, err
	}
	if s.beforeEnter != nil {
		s.beforeEnter()
	}
	if err := s.enter(ctx, sess, id, name); err != nil {
		return nil, err
	}
	return map[string]any{"account": name}, nil
}
