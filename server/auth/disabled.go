package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// DefaultNotifyChannel 是“账号被禁用”的 PostgreSQL 通知频道（docs/protocol.md 5.4）。
const DefaultNotifyChannel = "account_disabled"

// Start 开始监听“账号被禁用”的通知：stoneage-admin 和服务端是两个进程，
// 禁用时 stoneage-admin 在同一个事务里 NOTIFY，服务端收到后立即断开这个账号的所有连接。
// LISTEN 建立之后才返回；连接断了会自动重连，重连后再核对一遍，不漏掉断线期间被禁用的账号。
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	conn, err := s.listen(ctx)
	if err != nil {
		cancel()
		return err
	}
	s.stopListen = cancel
	s.listenDone = make(chan struct{})
	s.expiryDone = make(chan struct{})
	go s.listenLoop(ctx, conn)
	go s.expiryLoop(ctx)
	return nil
}

// Stop 停止监听并等待后台协程退出。
func (s *Service) Stop() {
	if s.stopListen != nil {
		s.stopListen()
		<-s.listenDone
		<-s.expiryDone
		s.stopListen = nil
	}
}

// listen 建立专用连接并 LISTEN，然后核对一次已禁用却仍在线的账号（先 LISTEN 再核对，中间不会漏）。
func (s *Service) listen(ctx context.Context) (*pgx.Conn, error) {
	pc, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	// 专用连接不还给连接池，用完直接关闭，避免 LISTEN 状态留在池里。
	conn := pc.Hijack()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{s.opts.NotifyChannel}.Sanitize()); err != nil {
		_ = conn.Close(context.Background())
		return nil, err
	}
	s.sweepDisabled(ctx)
	return conn, nil
}

func (s *Service) listenLoop(ctx context.Context, conn *pgx.Conn) {
	defer close(s.listenDone)
	backoff := time.Second
	for {
		for {
			n, err := conn.WaitForNotification(ctx)
			if err != nil {
				break
			}
			s.kickAccount(NormalizeAccount(n.Payload))
		}
		_ = conn.Close(context.Background())
		for {
			if ctx.Err() != nil {
				return
			}
			s.opts.Logger.Warn("账号禁用通知的监听连接断开，正在重连")
			var err error
			if conn, err = s.listen(ctx); err == nil {
				backoff = time.Second
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

// sweepDisabled 断开所有“已被禁用却仍在线”的账号。
func (s *Service) sweepDisabled(ctx context.Context) {
	online := map[string]bool{}
	for _, sess := range s.hub.Sessions() {
		if name := sess.Account(); name != "" {
			online[name] = true
		}
	}
	if len(online) == 0 {
		return
	}
	names := make([]string, 0, len(online))
	for n := range online {
		names = append(names, n)
	}
	rows, err := s.db.Query(ctx, `SELECT name FROM accounts WHERE disabled AND name = ANY($1)`, names)
	if err != nil {
		s.opts.Logger.Warn("核对被禁用的账号失败", "error", err)
		return
	}
	defer rows.Close()
	var disabled []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			disabled = append(disabled, n)
		}
	}
	rows.Close()
	for _, n := range disabled {
		s.kickAccount(n)
	}
}
