package auth

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/store/storetest"
)

// kickEnv 是一个装好账号服务的服务端，时钟可以拨快。
type kickEnv struct {
	t      *testing.T
	svc    *Service
	srv    *gateway.Server
	addr   string
	offset atomic.Int64 // 拨快的秒数
	kicked atomic.Int32 // BeforeKick 被调用的次数
}

func newKickEnv(t *testing.T, tune func(*Service)) *kickEnv {
	t.Helper()
	e := &kickEnv{t: t}
	st := storetest.New(t)
	e.srv = gateway.New(gateway.Options{RegisterEnabled: true})
	svc, err := New(st.Pool, e.srv.Hub(), Options{RegisterEnabled: true, Hash: fastHash,
		Now:                func() time.Time { return time.Now().Add(time.Duration(e.offset.Load()) * time.Second) },
		BeforeKick:         func(*gateway.Session) { e.kicked.Add(1) },
		TokenCheckInterval: 50 * time.Millisecond,
		NotifyChannel:      fmt.Sprintf("account_disabled_%d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	if tune != nil {
		tune(svc)
	}
	svc.Install(e.srv)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	e.addr = ln.Addr().String()
	go e.srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.srv.Shutdown(ctx)
	})
	return e
}

func (e *kickEnv) dial(role string) *peer {
	e.t.Helper()
	c, err := net.Dial("tcp", e.addr)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { c.Close() })
	p := &peer{conn: c, r: bufio.NewReader(c)}
	p.send(e.t, `{"id":1,"type":"hello","data":{"protocol":1,"role":"`+role+`"}}`)
	p.recv(e.t)
	return p
}

func errCodeOf(m map[string]any) string {
	if e, ok := m["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

// STAGE-14：密码校验通过之后、连接登记到账号名下之前，账号被禁用（禁用通知刚好在这时处理完）：登录要失败，
// 不能留下一条被禁用账号的在线连接。
func TestDisableDuringLoginIsNotMissed(t *testing.T) {
	e := newKickEnv(t, nil)
	ctx := context.Background()
	if err := e.svc.Register(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	e.svc.beforeEnter = func() {
		if err := e.svc.SetDisabled(ctx, "alice", true, "测试"); err != nil {
			t.Error(err)
		}
		e.svc.kickAccount("alice") // 禁用通知此刻处理完：连接还没登记，找不到它
	}
	p := e.dial("player")
	p.send(t, `{"id":2,"type":"login","data":{"account":"alice","password":"secret1"}}`)
	if m := p.recv(t); errCodeOf(m) != CodeAccountDisabled {
		t.Fatalf("登录应回复 ACCOUNT_DISABLED: %v", m)
	}
	for _, s := range e.srv.Hub().Sessions() {
		if s.Account() != "" || s.State() == protocol.StateLoggedIn {
			t.Errorf("被禁用的账号不能留下登录状态: %q %v", s.Account(), s.State())
		}
	}
	// 不是被禁用的账号照常登录（钩子只对 alice 生效，这里用另一个账号确认钩子不会误伤）。
	e.svc.beforeEnter = nil
	if err := e.svc.Register(ctx, "1.1.1.2", "bob", "secret1"); err != nil {
		t.Fatal(err)
	}
	q := e.dial("player")
	q.send(t, `{"id":2,"type":"login","data":{"account":"bob","password":"secret1"}}`)
	if m := q.recv(t); m["ok"] != true {
		t.Fatalf("bob 应登录成功: %v", m)
	}
}

// STAGE-14：agent 令牌校验通过之后、连接登记之前令牌被吊销：agent 登录要失败。
func TestRevokeDuringAgentLoginIsNotMissed(t *testing.T) {
	e := newKickEnv(t, nil)
	ctx := context.Background()
	if err := e.svc.Register(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	id, name, err := e.svc.Login(ctx, "1.1.1.1", "alice", "secret1")
	if err != nil {
		t.Fatal(err)
	}
	token, info, err := e.svc.CreateToken(ctx, id, "测试", 0)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.beforeEnter = func() {
		if err := e.svc.RevokeToken(ctx, id, name, info.TokenID); err != nil {
			t.Error(err)
		}
	}
	p := e.dial("agent")
	p.send(t, fmt.Sprintf(`{"id":2,"type":"agent_login","data":{"token":%q}}`, token))
	if m := p.recv(t); errCodeOf(m) != protocol.CodeInvalidToken {
		t.Fatalf("令牌已被吊销，agent_login 应回复 INVALID_TOKEN: %v", m)
	}
	for _, s := range e.srv.Hub().Sessions() {
		if s.Account() != "" || s.TokenID() != "" || s.State() == protocol.StateLoggedIn {
			t.Errorf("被吊销的令牌不能留下登录状态: %q %q %v", s.Account(), s.TokenID(), s.State())
		}
	}
}

// STAGE-15：在线 agent 的令牌到期，服务端定时检查后先保存再推送 AGENT_TOKEN_REVOKED 断开；没到期的 agent 不受影响。
func TestExpiredTokenKicksOnlineAgent(t *testing.T) {
	e := newKickEnv(t, nil)
	ctx := context.Background()
	// 同一账号的第二个 agent 会挤掉第一个，所以两个 agent 用两个账号。
	loginAgent := func(account string, days int) *peer {
		if err := e.svc.Register(ctx, "1.1.1."+account[:1], account, "secret1"); err != nil {
			t.Fatal(err)
		}
		id, _, err := e.svc.Login(ctx, "1.1.1.1", account, "secret1")
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := e.svc.CreateToken(ctx, id, "测试", days)
		if err != nil {
			t.Fatal(err)
		}
		p := e.dial("agent")
		p.send(t, fmt.Sprintf(`{"id":2,"type":"agent_login","data":{"token":%q}}`, token))
		if m := p.recv(t); m["ok"] != true {
			t.Fatalf("agent 应登录成功: %v", m)
		}
		return p
	}
	short, long := loginAgent("alice", 1), loginAgent("bob", 30)
	if err := e.svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.svc.Stop)

	e.offset.Store(2 * 24 * 3600) // 两天后：短令牌到期，长令牌还有效
	m := short.recv(t)
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonAgentTokenRevoked {
		t.Fatalf("到期令牌的 agent 应收到 AGENT_TOKEN_REVOKED: %v", m)
	}
	if !short.closed() {
		t.Error("到期令牌的 agent 连接应被断开")
	}
	if e.kicked.Load() != 1 {
		t.Errorf("断开之前应先调用保存钩子 1 次，实际 %d", e.kicked.Load())
	}
	long.send(t, `{"id":3,"type":"ping"}`)
	if m := long.recv(t); m["ok"] != true {
		t.Errorf("没到期的 agent 应保持在线: %v", m)
	}
}
