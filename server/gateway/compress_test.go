package gateway

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-246：连接级压缩的测试。

// bigTypeLine 是一条超过 10 KiB 回复的请求：服务端对未知类型回复 UNKNOWN_TYPE，并把类型名原样放进回复，
// 所以回复的 type 和 error.message 都是这么长，合起来超过阈值。
func bigTypeLine(id int) string {
	return `{"id":` + strconv.Itoa(id) + `,"type":"` + strings.Repeat("abcdefghij", 600) + `","data":{}}`
}

// recvWire 读一行线上的数据，返回线上的字节和解压后的消息。
func recvWire(t *testing.T, c *testClient) (wire string, msg map[string]any) {
	t.Helper()
	line, err := c.readLine()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}
	plain, perr := protocol.DecodeLine(line)
	if perr != nil {
		t.Fatalf("服务端发来的压缩行无法解压: %v", perr)
	}
	if err := json.Unmarshal(plain, &msg); err != nil {
		t.Fatalf("解压后不是 JSON: %q", plain)
	}
	return string(line), msg
}

// helloWith 发送带 compress 字段的 hello，返回响应。
func (c *testClient) helloWith(compress string) map[string]any {
	c.t.Helper()
	c.send(`{"id":1,"type":"hello","data":{"protocol":1,"client":"test","role":"player"` + compress + `}}`)
	m := c.recv()
	if m["ok"] != true {
		c.t.Fatalf("hello 失败: %v", m)
	}
	return m
}

func TestCompressNegotiatedBigReplyIsGzip(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	hello := c.helloWith(`,"compress":["gzip"]`)
	if data, _ := hello["data"].(map[string]any); data["compress"] != "gzip" {
		t.Fatalf("hello 响应应确认 compress=gzip，实际 %v", hello["data"])
	}
	c.send(bigTypeLine(2))
	wire, msg := recvWire(t, c)
	if !strings.HasPrefix(wire, `{"z":"gzip","d":"`) {
		t.Fatalf("超过 10 KiB 的回复应压缩，线上却是原样: %.80s", wire)
	}
	if len(wire) >= 10*1024 {
		t.Fatalf("压缩后线上仍有 %d 字节，应明显变小", len(wire))
	}
	if msg["type"] != strings.Repeat("abcdefghij", 600) || errCode(msg) != "UNKNOWN_TYPE" {
		t.Fatalf("解压后的回复不对: type 长 %d, code %s", len(msg["type"].(string)), errCode(msg))
	}
	// 压缩不影响连接：之后的小消息仍是原样。
	c.send(`{"id":3,"type":"ping","data":{}}`)
	line, err := c.readLine()
	if err != nil || !strings.HasPrefix(string(line), `{"id":3`) {
		t.Fatalf("小消息应原样发送，得到 %q, %v", line, err)
	}
}

func TestCompressNotNegotiatedStaysPlain(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player")
	c.send(bigTypeLine(2))
	line, err := c.readLine()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(line), `"z"`) || !strings.HasPrefix(string(line), `{"id":2`) {
		t.Fatalf("未协商的连接必须收到原样 JSON，实际前 80 字节: %.80s", line)
	}
	if len(line) <= 10*1024 {
		t.Fatalf("测试消息应超过 10 KiB，实际 %d 字节", len(line))
	}
}

func TestCompressUnknownCompressValueIsIgnored(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	hello := c.helloWith(`,"compress":["zstd"]`)
	if data, _ := hello["data"].(map[string]any); data["compress"] != nil {
		t.Fatalf("不认识的压缩方式不应被确认，实际 %v", data)
	}
	c.send(bigTypeLine(2))
	line, _ := c.readLine()
	if strings.Contains(string(line), `"z"`) {
		t.Fatalf("没有协商成功的连接不应压缩")
	}
}

func TestCompressedRequestIsAccepted(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player")
	// 服务端认得压缩的请求行（对称实现，客户端本期不压缩请求）。
	req := `{"id":2,"type":"ping","data":{}}`
	line, err := protocol.EncodeGzip([]byte(req))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.conn.Write(line)
	_, msg := recvWire(t, c)
	if msg["ok"] != true || msg["type"] != "ping" {
		t.Fatalf("压缩的 ping 应得到正常回复，实际 %v", msg)
	}
}

func TestBadCompressedRequestClosesWithBadRequest(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player")
	c.send(`{"z":"gzip","d":"这不是 base64"}`)
	m := c.recv()
	if errCode(m) != "BAD_REQUEST" || m["id"] != float64(0) {
		t.Fatalf("坏的压缩行应回复 id 为 0 的 BAD_REQUEST，实际 %v", m)
	}
	c.expectClosed()
}

// WebSocket 入口和 TCP 用同一套压缩实现：这里只验证协商和超大回复。
func TestWSCompressNegotiatedBigReplyIsGzip(t *testing.T) {
	_, url := startWS(t, Options{})
	c := dialWS(t, url, nil)
	c.helloWith(`,"compress":["gzip"]`)
	c.send(bigTypeLine(2))
	wire, msg := recvWire(t, c)
	if !strings.HasPrefix(wire, `{"z":"gzip","d":"`) || errCode(msg) != "UNKNOWN_TYPE" {
		t.Fatalf("WebSocket 上超过 10 KiB 的回复应压缩，实际前 80 字节: %.80s", wire)
	}
}
