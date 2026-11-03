package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-351：按连接限速的集成测试，走真实的 TCP 连接。burst 调小、rate 调很小，不依赖真实时间。
// 每次只发一条并读回它的回复，避免服务端还有没读的数据时关连接，把尾部的 disconnect 冲掉。

// startRateServer 起一个网络层，登录请求（计入限速，握手后可发）有成功的处理函数。
func startRateServer(t *testing.T, mode string) string {
	t.Helper()
	srv, addr := startServer(t, Options{RateLimitMode: mode, RateLimitRate: 0.01, RateLimitBurst: 5})
	srv.Handle("login", func(context.Context, *Session, *protocol.Request) (any, error) {
		return map[string]any{"done": true}, nil
	})
	return addr
}

func loginLine(id int) string {
	return fmt.Sprintf(`{"id":%d,"type":"login","data":{}}`, id)
}

func TestRateLimitEnforceRejectsAfterBurst(t *testing.T) {
	c := dial(t, startRateServer(t, "enforce"))
	c.hello("player")
	for i := 0; i < 100; i++ {
		c.send(loginLine(i + 2))
		m := c.recv()
		if i < 5 {
			if m["ok"] != true {
				t.Fatalf("第 %d 个请求在 burst 内，应当成功：%v", i+1, m)
			}
			continue
		}
		if errCode(m) != protocol.CodeRateLimited {
			t.Fatalf("第 %d 个请求应回复 RATE_LIMITED：%v", i+1, m)
		}
		if m["id"] != float64(i+2) || m["type"] != "login" {
			t.Fatalf("限速回复要保留 id 和 type：%v", m)
		}
		data := m["error"].(map[string]any)["data"].(map[string]any)
		if n, ok := data["retry_after_seconds"].(float64); !ok || n < 1 {
			t.Fatalf("应带 retry_after_seconds 且不小于 1：%v", m)
		}
	}
	// 95 次拒绝，没到断开的门槛，连接仍然可用（心跳照常）。
	c.send(`{"id":900,"type":"ping","data":{}}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("限速后连接应仍可用：%v", m)
	}
}

func TestRateLimitHeartbeatNotCounted(t *testing.T) {
	c := dial(t, startRateServer(t, "enforce"))
	c.hello("player")
	for i := 0; i < 5; i++ {
		c.send(loginLine(i + 2))
		c.recv() // 用完 burst
	}
	for i := 0; i < 50; i++ {
		c.send(fmt.Sprintf(`{"id":%d,"type":"ping","data":{}}`, 100+i))
		if m := c.recv(); m["ok"] != true {
			t.Fatalf("心跳不应被限速：%v", m)
		}
	}
	c.send(loginLine(200))
	if errCode(c.recv()) != protocol.CodeRateLimited {
		t.Fatal("心跳之后的计入请求仍应被限速")
	}
}

func TestRateLimitSustainedViolationDisconnects(t *testing.T) {
	c := dial(t, startRateServer(t, "enforce"))
	c.hello("player")
	for i := 0; i < 5; i++ {
		c.send(loginLine(i + 2))
		c.recv()
	}
	rejected := 0
	for i := 0; i < 2*violationLimit; i++ {
		c.send(loginLine(100 + i))
		m := c.recv()
		if m["type"] == "disconnect" {
			if reason := m["data"].(map[string]any)["reason"]; reason != protocol.ReasonRateLimited {
				t.Fatalf("断开原因应是 RATE_LIMITED：%v", m)
			}
			if rejected != violationLimit {
				t.Fatalf("应在第 %d 次拒绝时断开，实际拒绝 %d 次", violationLimit, rejected)
			}
			c.expectClosed()
			return
		}
		if errCode(m) != protocol.CodeRateLimited {
			t.Fatalf("超限的请求应回复 RATE_LIMITED：%v", m)
		}
		rejected++
	}
	t.Fatal("持续超限后应当被断开")
}

func TestRateLimitLogModeNeverRejects(t *testing.T) {
	// 不设模式即取默认 log：超限只记日志。
	c := dial(t, startRateServer(t, ""))
	c.hello("player")
	for i := 0; i < 100; i++ {
		c.send(loginLine(i + 2))
		if m := c.recv(); m["ok"] != true {
			t.Fatalf("log 模式下第 %d 个请求应照常成功：%v", i+1, m)
		}
	}
}

func TestRateLimitOffModeNeverRejects(t *testing.T) {
	c := dial(t, startRateServer(t, "off"))
	c.hello("player")
	for i := 0; i < 100; i++ {
		c.send(loginLine(i + 2))
		if m := c.recv(); m["ok"] != true {
			t.Fatalf("off 模式下第 %d 个请求应照常成功：%v", i+1, m)
		}
	}
}
