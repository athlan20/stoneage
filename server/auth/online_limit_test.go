package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

type fakeConn struct {
	role    protocol.Role
	account string
	loginAt time.Time
}

func (f fakeConn) Role() protocol.Role { return f.role }
func (f fakeConn) Account() string     { return f.account }
func (f fakeConn) TokenID() string     { return "t_x" }
func (f fakeConn) LoginAt() time.Time  { return f.loginAt }

// STAGE-118：只断开登录满上限的 agent；玩家、未登录、没有登录时间的不算。
func TestOverdueAgents(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	limit := time.Minute
	conns := []fakeConn{
		{protocol.RoleAgent, "alice", now.Add(-time.Minute)},    // 刚好满：断开
		{protocol.RoleAgent, "bob", now.Add(-30 * time.Second)}, // 没满：保留
		{protocol.RolePlayer, "carol", now.Add(-time.Hour)},     // 玩家：不管
		{protocol.RoleAgent, "", now.Add(-time.Hour)},           // 未登录：不管
		{protocol.RoleAgent, "dave", time.Time{}},               // 没有登录时间：不管
	}
	got := overdueAgents(conns, now, limit)
	if len(got) != 1 || got[0].account != "alice" {
		t.Fatalf("应只断开 alice，实际 %+v", got)
	}
}

// STAGE-118：在线满上限的 agent 被断开（走 AGENT_TOKEN_REVOKED 的路径），没满的保持在线。
func TestOnlineLimitKicksAgent(t *testing.T) {
	e := newKickEnv(t, func(s *Service) { s.opts.AgentMaxOnline = time.Minute })
	ctx := context.Background()
	if err := e.svc.Register(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	id, _, err := e.svc.Login(ctx, "1.1.1.1", "alice", "secret1")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := e.svc.CreateToken(ctx, id, "测试", 30)
	if err != nil {
		t.Fatal(err)
	}
	p := e.dial("agent")
	p.send(t, fmt.Sprintf(`{"id":2,"type":"agent_login","data":{"token":%q}}`, token))
	if m := p.recv(t); m["ok"] != true {
		t.Fatalf("agent 应登录成功: %v", m)
	}
	if err := e.svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.svc.Stop)

	e.offset.Store(30) // 半分钟：没满，照常在线
	p.send(t, `{"id":3,"type":"ping"}`)
	if m := p.recv(t); m["ok"] != true {
		t.Fatalf("没满一分钟的 agent 应保持在线: %v", m)
	}

	e.offset.Store(61) // 满一分钟
	m := p.recv(t)
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonAgentTokenRevoked {
		t.Fatalf("满上限的 agent 应收到 AGENT_TOKEN_REVOKED: %v", m)
	}
	if !p.closed() {
		t.Error("满上限的 agent 连接应被断开")
	}
	if e.kicked.Load() != 1 {
		t.Errorf("断开之前应先调用保存钩子 1 次，实际 %d", e.kicked.Load())
	}
}
