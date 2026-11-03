package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/store/storetest"
)

type peer struct {
	conn net.Conn
	r    *bufio.Reader
}

func (p *peer) send(t *testing.T, line string) {
	t.Helper()
	if _, err := p.conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
}

func (p *peer) recv(t *testing.T) map[string]any {
	t.Helper()
	_ = p.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := p.r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(line, &m)
	return m
}

func (p *peer) closed() bool {
	_ = p.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err := p.r.ReadBytes('\n')
	return err != nil
}

// 同一账号、同一身份再次登录：旧连接收到 KICKED_DUPLICATE_LOGIN 后被断开，新连接成功。
// 玩家连接和 agent 连接互不挤占。
func TestDuplicateLoginKicksSameRoleOnly(t *testing.T) {
	st := storetest.New(t)
	srv := gateway.New(gateway.Options{RegisterEnabled: true})
	var saved []uint64
	svc, err := New(st.Pool, srv.Hub(), Options{RegisterEnabled: true, Hash: fastHash,
		BeforeKick: func(old *gateway.Session) { saved = append(saved, old.ID()) }})
	if err != nil {
		t.Fatal(err)
	}
	svc.Install(srv)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	if err := svc.Register(context.Background(), "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	dial := func(role string) *peer {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		p := &peer{conn: c, r: bufio.NewReader(c)}
		p.send(t, `{"id":1,"type":"hello","data":{"protocol":1,"role":"`+role+`"}}`)
		p.recv(t)
		return p
	}
	login := `{"id":2,"type":"login","data":{"account":"alice","password":"secret1"}}`

	first := dial("player")
	first.send(t, login)
	if m := first.recv(t); m["ok"] != true {
		t.Fatalf("%v", m)
	}
	second := dial("player")
	second.send(t, login)
	if m := second.recv(t); m["ok"] != true {
		t.Fatalf("新连接应登录成功: %v", m)
	}
	m := first.recv(t)
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonKickedDuplicateLogin {
		t.Fatalf("旧连接应收到 KICKED_DUPLICATE_LOGIN: %v", m)
	}
	if !first.closed() {
		t.Error("旧连接应被断开")
	}
	if len(saved) != 1 {
		t.Errorf("断开旧连接之前应先调用保存钩子，实际 %d 次", len(saved))
	}
	second.send(t, `{"id":3,"type":"ping"}`)
	if m := second.recv(t); m["ok"] != true {
		t.Errorf("新连接应保持在线: %v", m)
	}

	// agent 连接和玩家连接互不挤占（agent 的令牌登录是 STAGE-6，这里直接标记已登录来验证规则）。
	agent := dial("agent")
	for _, s := range srv.Hub().Sessions() {
		if s.Role() == protocol.RoleAgent {
			_ = svc.enter(context.Background(), s, 1, "alice")
		}
	}
	agent.send(t, `{"id":2,"type":"ping"}`)
	if m := agent.recv(t); m["ok"] != true {
		t.Fatalf("agent 连接应在线: %v", m)
	}
	second.send(t, `{"id":4,"type":"ping"}`)
	if m := second.recv(t); m["ok"] != true {
		t.Errorf("agent 登录不应挤掉玩家连接: %v", m)
	}
}

// 账号被禁用：这个账号在线的玩家连接和 agent 连接都立即收到 ACCOUNT_DISABLED 并被断开，
// 断开前先调用保存钩子；别的账号不受影响；之后再登录被拒绝。
func TestDisableKicksAllConnections(t *testing.T) {
	st := storetest.New(t)
	srv := gateway.New(gateway.Options{RegisterEnabled: true})
	var mu sync.Mutex
	saved := map[uint64]bool{}
	svc, err := New(st.Pool, srv.Hub(), Options{RegisterEnabled: true, Hash: fastHash,
		NotifyChannel: fmt.Sprintf("account_disabled_%d", time.Now().UnixNano()),
		BeforeKick:    func(old *gateway.Session) { mu.Lock(); saved[old.ID()] = true; mu.Unlock() }})
	if err != nil {
		t.Fatal(err)
	}
	svc.Install(srv)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Stop)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	ctx := context.Background()
	for _, name := range []string{"alice", "bob"} {
		if err := svc.Register(ctx, "1.1.1.1", name, "secret1"); err != nil {
			t.Fatal(err)
		}
	}
	dial := func(role string) *peer {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		p := &peer{conn: c, r: bufio.NewReader(c)}
		p.send(t, `{"id":1,"type":"hello","data":{"protocol":1,"role":"`+role+`"}}`)
		p.recv(t)
		return p
	}
	loginAs := func(name string) *peer {
		p := dial("player")
		p.send(t, `{"id":2,"type":"login","data":{"account":"`+name+`","password":"secret1"}}`)
		if m := p.recv(t); m["ok"] != true {
			t.Fatalf("%v", m)
		}
		return p
	}
	player, bob := loginAs("alice"), loginAs("bob")
	agent := dial("agent")
	var agentID uint64
	for _, s := range srv.Hub().Sessions() {
		if s.Role() == protocol.RoleAgent {
			agentID = s.ID()
			_ = svc.enter(context.Background(), s, 1, "alice")
		}
	}

	if err := svc.SetDisabled(ctx, "Alice", true, "测试"); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]*peer{"玩家": player, "agent": agent} {
		m := p.recv(t)
		if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonAccountDisabled {
			t.Fatalf("%s连接应收到 ACCOUNT_DISABLED: %v", name, m)
		}
		if !p.closed() {
			t.Errorf("%s连接应被断开", name)
		}
	}
	mu.Lock()
	if len(saved) != 2 || !saved[agentID] {
		t.Errorf("断开前应对 alice 的两条连接都调用保存钩子，实际 %v", saved)
	}
	mu.Unlock()
	bob.send(t, `{"id":3,"type":"ping"}`)
	if m := bob.recv(t); m["ok"] != true {
		t.Errorf("别的账号不应受影响: %v", m)
	}
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); code(err) != CodeAccountDisabled {
		t.Errorf("之后再登录应被拒绝为 ACCOUNT_DISABLED，实际 %q", code(err))
	}
}

// 监听启动时，已经被禁用却仍在线的账号会被核对出来踢掉（覆盖断线重连期间错过通知的情况）。
func TestStartSweepsDisabledOnline(t *testing.T) {
	st := storetest.New(t)
	srv := gateway.New(gateway.Options{RegisterEnabled: true})
	svc, err := New(st.Pool, srv.Hub(), Options{RegisterEnabled: true, Hash: fastHash})
	if err != nil {
		t.Fatal(err)
	}
	svc.Install(srv)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	ctx := context.Background()
	if err := svc.Register(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	c, _ := net.Dial("tcp", ln.Addr().String())
	t.Cleanup(func() { c.Close() })
	p := &peer{conn: c, r: bufio.NewReader(c)}
	p.send(t, `{"id":1,"type":"hello","data":{"protocol":1,"role":"player"}}`)
	p.recv(t)
	p.send(t, `{"id":2,"type":"login","data":{"account":"alice","password":"secret1"}}`)
	p.recv(t)
	// 监听还没启动时禁用：没人收到通知。
	if err := svc.SetDisabled(ctx, "alice", true, "测试"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Stop)
	if m := p.recv(t); m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonAccountDisabled {
		t.Fatalf("启动核对应踢掉已被禁用的在线账号: %v", m)
	}
}
