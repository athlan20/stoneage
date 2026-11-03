package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/athlan20/stoneage/server/protocol"
)

// startWS 在随机端口上启动 WebSocket 入口，测试结束时关闭服务。返回 ws:// 地址。
func startWS(t *testing.T, opts Options) (*Server, string) {
	t.Helper()
	srv := New(opts)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.ServeWS(l)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv, "ws://" + l.Addr().String() + WSPath
}

// dialWS 用和 TCP 测试相同的 testClient 包装一条 WebSocket 连接（每次 Write 就是一个文本帧）。
func dialWS(t *testing.T, url string, header http.Header) *testClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow(); cancel() })
	return &testClient{t: t, ws: c, ctx: ctx}
}

func TestWSHelloAndPing(t *testing.T) {
	_, url := startWS(t, Options{})
	c := dialWS(t, url, nil)
	if m := c.hello("player"); m["data"].(map[string]any)["protocol"] == nil {
		t.Fatalf("hello 响应缺少 protocol: %v", m)
	}
	c.send(`{"id":2,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("ping 应成功: %v", m)
	}
}

// 帧里没有换行也应当能读：约定是一行一帧，换行由服务端补上。
func TestWSFrameWithoutNewline(t *testing.T) {
	_, url := startWS(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	for i, line := range []string{
		`{"id":1,"type":"hello","data":{"protocol":1,"client":"test","role":"player"}}`,
		`{"id":2,"type":"ping"}`,
	} {
		if err := c.Write(ctx, websocket.MessageText, []byte(line)); err != nil {
			t.Fatal(err)
		}
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("第 %d 条读取失败: %v", i, err)
		}
		if !strings.Contains(string(data), `"ok":true`) {
			t.Fatalf("第 %d 条应成功: %s", i, data)
		}
		if bytes.ContainsRune(data, '\n') || !json.Valid(data) {
			t.Fatalf("每个文本帧应是恰好一行 JSON，不带换行: %q", data)
		}
	}
}

func TestWSLineTooLong(t *testing.T) {
	_, url := startWS(t, Options{MaxLine: 200})
	c := dialWS(t, url, nil)
	c.hello("player")
	c.send(`{"id":2,"type":"ping","data":"` + strings.Repeat("x", 300) + `"}`)
	if m := c.recv(); errCode(m) != protocol.CodeBadRequest {
		t.Fatalf("超长的一行应回复 BAD_REQUEST: %v", m)
	}
	c.expectClosed()
}

func TestWSIdleTimeout(t *testing.T) {
	// 服务端的 ping 不算应用层的消息，空闲超时照常生效。
	_, url := startWS(t, Options{IdleTimeout: 700 * time.Millisecond, WSPingInterval: 100 * time.Millisecond})
	c := dialWS(t, url, nil)
	c.hello("player")
	m := c.recv()
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonIdleTimeout {
		t.Fatalf("应推送 IDLE_TIMEOUT: %v", m)
	}
	c.expectClosed()
}

// 服务端定时 ping 时，客户端正常读取（自动回 pong），连接在没有应用层消息的时候也保持畅通。
func TestWSServerPingKeepsConnectionAlive(t *testing.T) {
	_, url := startWS(t, Options{WSPingInterval: 50 * time.Millisecond, IdleTimeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	// 后台一直读，pong 才会被处理；这是真实客户端的行为。
	go func() {
		for {
			if _, _, err := c.Read(ctx); err != nil {
				return
			}
		}
	}()
	time.Sleep(600 * time.Millisecond)
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"id":1,"type":"ping"}`)); err != nil {
		t.Fatalf("空闲一段时间后连接应当仍然可用: %v", err)
	}
}

func TestWSShutdownPushesDisconnect(t *testing.T) {
	srv, url := startWS(t, Options{})
	c := dialWS(t, url, nil)
	c.hello("player")
	// 关服要等连接的关闭握手完成，客户端得一直读取（真实客户端如此），所以关服放到后台。
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done <- srv.Shutdown(ctx)
	}()
	m := c.recv()
	if m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonServerShutdown {
		t.Fatalf("关服应推送 SERVER_SHUTDOWN: %v", m)
	}
	c.expectClosed() // 读到关闭帧并回应，服务端的关闭握手才能完成
	if err := <-done; err != nil {
		t.Fatalf("关服应当正常结束: %v", err)
	}
}

func TestWSRemoteAddr(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	srv := New(Options{TrustedProxies: trusted})
	cases := []struct {
		name, peer, header, want string
	}{
		{"直连且没有代理头", "198.51.100.7:4000", "", "198.51.100.7"},
		{"不可信来源的代理头被忽略", "198.51.100.7:4000", "203.0.113.9", "198.51.100.7"},
		{"可信代理转来真实 IP", "127.0.0.1:4000", "203.0.113.9", "203.0.113.9"},
		{"可信代理头不是 IP 时用直连地址", "127.0.0.1:4000", "not-an-ip", "127.0.0.1"},
		{"可信代理转来 IPv6", "127.0.0.1:4000", "2001:db8::9", "2001:db8::9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, WSPath, nil)
			r.RemoteAddr = tc.peer
			if tc.header != "" {
				r.Header.Set(CloudflareIPHeader, tc.header)
			}
			addr := srv.wsRemoteAddr(r)
			host, _, err := net.SplitHostPort(addr.String())
			if err != nil {
				t.Fatalf("地址格式不对: %q: %v", addr.String(), err)
			}
			if host != tc.want {
				t.Fatalf("玩家地址应为 %s，实际 %s", tc.want, host)
			}
		})
	}
}

// 代理转来的真实 IP 要成为会话的远端地址，限流等按它计算。
func TestWSSessionUsesForwardedRemote(t *testing.T) {
	srv, url := startWS(t, Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	_ = srv
	c := dialWS(t, url, http.Header{CloudflareIPHeader: []string{"203.0.113.9"}})
	c.hello("player")
	sessions := srv.Hub().Sessions()
	if len(sessions) != 1 {
		t.Fatalf("应只有一条连接，实际 %d", len(sessions))
	}
	if host, _, _ := net.SplitHostPort(sessions[0].RemoteAddr().String()); host != "203.0.113.9" {
		t.Fatalf("会话的远端地址应是真实 IP，实际 %s", sessions[0].RemoteAddr())
	}
}
