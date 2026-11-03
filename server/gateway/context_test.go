package gateway

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-8：正在处理请求时对端断开，请求的 ctx 要立即取消，排队等待的操作才能放弃。
func TestContextCancelledWhenClientDisconnects(t *testing.T) {
	srv, addr := startServer(t, Options{})
	started := make(chan struct{})
	result := make(chan error, 1)
	srv.Handle("characters", func(ctx context.Context, _ *Session, _ *protocol.Request) (any, error) {
		close(started)
		select {
		case <-ctx.Done():
			result <- ctx.Err()
		case <-time.After(5 * time.Second):
			result <- nil
		}
		return nil, ctx.Err()
	})
	c := dial(t, addr)
	c.hello("player")
	for _, s := range srv.Hub().Sessions() {
		s.SetState(protocol.StateLoggedIn)
	}
	c.send(`{"id":2,"type":"characters"}`)
	<-started
	_ = c.conn.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("对端断开后 ctx 没有取消")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("等待超时")
	}
}

// STAGE-8：被踢（Disconnect）或关服时，正在处理的请求 ctx 也取消。
func TestContextCancelledOnShutdown(t *testing.T) {
	srv, addr := startServer(t, Options{})
	started := make(chan struct{})
	result := make(chan error, 1)
	srv.Handle("characters", func(ctx context.Context, _ *Session, _ *protocol.Request) (any, error) {
		close(started)
		<-ctx.Done()
		result <- ctx.Err()
		return nil, ctx.Err()
	})
	c := dial(t, addr)
	c.hello("player")
	for _, s := range srv.Hub().Sessions() {
		s.SetState(protocol.StateLoggedIn)
	}
	c.send(`{"id":2,"type":"characters"}`)
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-result:
	default:
		t.Fatal("关服后处理中的请求 ctx 没有取消")
	}
}

// STAGE-8：关服的同时不停有新连接进来，Shutdown 不能卡到超时：
// 每条连接要么被通知后断开，要么在关服开始后被直接关掉。
func TestShutdownWithConnectionsArrivingConcurrently(t *testing.T) {
	for round := 0; round < 20; round++ {
		srv := New(Options{})
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go srv.Serve(l)
		var wg sync.WaitGroup
		stop := make(chan struct{})
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					c, err := net.Dial("tcp", l.Addr().String())
					if err != nil {
						return
					}
					defer c.Close()
				}
			}()
		}
		time.Sleep(10 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = srv.Shutdown(ctx)
		cancel()
		close(stop)
		wg.Wait()
		if err != nil {
			t.Fatalf("第 %d 轮：关服等待超时: %v", round, err)
		}
	}
}
