package gateway

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-100：hello 的客户端版本检查（docs/protocol.md 5.1）。

func TestHelloRejectsClientTooOld(t *testing.T) {
	_, addr := startServer(t, Options{MinClientVersion: "0.2.0", LatestClientVersion: "0.3.0"})
	for _, role := range []string{"player", "agent"} {
		c := dial(t, addr)
		c.send(`{"id":1,"type":"hello","data":{"protocol":1,"client":"stoneage-cli/0.1.0","role":"` + role + `"}}`)
		m := c.recv()
		if errCode(m) != protocol.CodeClientTooOld {
			t.Fatalf("%s 版本过旧应回复 CLIENT_TOO_OLD: %v", role, m)
		}
		errObj := m["error"].(map[string]any)
		if msg, _ := errObj["message"].(string); msg != "当前版本 0.1.0 过旧，需要升级到 0.2.0 以上，运行 stoneage update" {
			t.Errorf("提示文字 = %q", msg)
		}
		if min := errObj["data"].(map[string]any)["min_client"]; min != "0.2.0" {
			t.Errorf("error.data.min_client = %v", min)
		}
		c.expectClosed()
	}
}

func TestHelloAllowsClientAtMinimumAndReturnsVersions(t *testing.T) {
	_, addr := startServer(t, Options{MinClientVersion: "0.1.0", LatestClientVersion: "0.2.0"})
	c := dial(t, addr)
	m := c.hello("player")
	data := m["data"].(map[string]any)
	if data["min_client"] != "0.1.0" || data["latest_client"] != "0.2.0" {
		t.Fatalf("握手响应应带上版本要求: %v", data)
	}
	// 握手成功后连接保持。
	c.send(`{"id":2,"type":"ping"}`)
	if m := c.recv(); m["ok"] != true {
		t.Fatalf("放行后连接应保持: %v", m)
	}
}

func TestHelloAllowsUnparseableClientVersion(t *testing.T) {
	_, addr := startServer(t, Options{MinClientVersion: "0.2.0"})
	c := dial(t, addr)
	c.send(`{"id":1,"type":"hello","data":{"protocol":1,"client":"dev","role":"player"}}`)
	m := c.recv()
	if m["ok"] != true {
		t.Fatalf("无法解析的客户端版本应放行: %v", m)
	}
	if _, ok := m["data"].(map[string]any)["latest_client"]; ok {
		t.Error("没配置 latest 时不应返回 latest_client")
	}
}

func TestHelloWithoutVersionConfigOmitsFields(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	data := c.hello("player")["data"].(map[string]any)
	for _, k := range []string{"min_client", "latest_client"} {
		if _, ok := data[k]; ok {
			t.Errorf("没配置版本要求时不应返回 %s", k)
		}
	}
}
