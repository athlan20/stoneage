package gateway

import (
	"strconv"
	"strings"
	"testing"
)

// STAGE-258：线上超过 64 KiB 的消息不能发，也不能断连。

// oversizeTypeLine 是一条请求：未知类型名很长（40000 字节），服务端回复 UNKNOWN_TYPE，类型名在回复的 type 和
// error.message 里各出现一次，回复原文超过 64 KiB；请求行本身不超过 64 KiB。
func oversizeTypeLine(id int) (line string, typ string) {
	typ = strings.Repeat("abcdefghij", 4000)
	return `{"id":` + strconv.Itoa(id) + `,"type":"` + typ + `","data":{}}`, typ
}

func TestOversizeReplyBecomesResponseTooLargeAndConnStays(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.hello("player") // 不声明 gzip
	line, typ := oversizeTypeLine(2)
	c.send(line)
	m := c.recv()
	if errCode(m) != "RESPONSE_TOO_LARGE" {
		t.Fatalf("超过 64 KiB 的响应应改发 RESPONSE_TOO_LARGE，实际 %v", m)
	}
	if m["id"] != float64(2) || m["type"] != typ || m["ok"] != false {
		t.Fatalf("错误响应要保留同一个 id 和 type，实际 id=%v ok=%v", m["id"], m["ok"])
	}
	// 连接仍活：之后的请求照常得到回复。
	c.send(`{"id":3,"type":"ping","data":{}}`)
	if m := c.recv(); m["id"] != float64(3) || m["ok"] != true {
		t.Fatalf("超限之后连接应仍可用，实际 %v", m)
	}
}

func TestOversizeReplyFitsWhenGzipNegotiated(t *testing.T) {
	_, addr := startServer(t, Options{})
	c := dial(t, addr)
	c.helloWith(`,"compress":["gzip"]`)
	line, typ := oversizeTypeLine(2)
	c.send(line)
	wire, msg := recvWire(t, c)
	if errCode(msg) != "UNKNOWN_TYPE" || msg["type"] != typ {
		t.Fatalf("声明 gzip 后同样大小的回复应正常发出，实际 code=%s", errCode(msg))
	}
	if !strings.HasPrefix(wire, `{"z":"gzip"`) || lineBytes([]byte(wire)) > 64*1024 {
		t.Fatalf("压缩后应在 64 KiB 以内，实际 %d 字节", len(wire))
	}
}

func TestOversizePushIsDroppedAndConnStays(t *testing.T) {
	s, _, logs := newDiagSession(t, 0)
	big := strings.Repeat("x", 70*1024)
	s.Push("maps", map[string]string{"blob": big})
	if len(s.out) != 0 {
		t.Fatalf("超限的推送不应入队，队列里有 %d 条", len(s.out))
	}
	select {
	case <-s.done:
		t.Fatal("超限的推送不应断开连接")
	default:
	}
	if !strings.Contains(logs.String(), "推送超过 64 KiB，已丢弃") {
		t.Fatalf("应记一条 Warn 日志，实际日志:\n%s", logs.String())
	}
	s.Push("ping", map[string]string{})
	if len(s.out) != 1 {
		t.Fatalf("之后的小推送应正常入队，队列里有 %d 条", len(s.out))
	}
}
