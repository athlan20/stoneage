package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// startServer 在随机端口上启动网络层，测试结束时关闭。
func startServer(t *testing.T, opts Options) (*Server, string) {
	t.Helper()
	srv := New(opts)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(l)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv, l.Addr().String()
}

// testClient 是测试用的客户端。TCP 连接按行读写；WebSocket 连接（ws 不为空）按帧读写，
// 一帧就是一行 JSON，不带换行。
type testClient struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
	ws   *websocket.Conn
	ctx  context.Context
}

func dial(t *testing.T, addr string) *testClient {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &testClient{t: t, conn: c, r: bufio.NewReader(c)}
}

func (c *testClient) send(line string) {
	c.t.Helper()
	if c.ws != nil {
		if err := c.ws.Write(c.ctx, websocket.MessageText, []byte(line)); err != nil {
			c.t.Fatal(err)
		}
		return
	}
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		c.t.Fatal(err)
	}
}

// readLine 读一行（不含换行），超时 5 秒。
func (c *testClient) readLine() ([]byte, error) {
	if c.ws != nil {
		ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
		defer cancel()
		_, data, err := c.ws.Read(ctx)
		return data, err
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := c.r.ReadBytes('\n')
	return bytes.TrimSuffix(line, []byte("\n")), err
}

// recv 读一条消息，超时算失败。
func (c *testClient) recv() map[string]any {
	c.t.Helper()
	line, err := c.readLine()
	if err != nil {
		c.t.Fatalf("读取消息失败: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		c.t.Fatalf("服务端发来的不是 JSON: %q", line)
	}
	return m
}

// expectClosed 断言服务端已关闭连接。
func (c *testClient) expectClosed() {
	c.t.Helper()
	line, err := c.readLine()
	if err == nil {
		c.t.Fatalf("连接应当已关闭，却收到 %q", line)
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		c.t.Fatal("连接应当已关闭，实际仍然打开")
	}
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func (c *testClient) hello(role string) map[string]any {
	c.t.Helper()
	c.send(`{"id":1,"type":"hello","data":{"protocol":1,"client":"test","role":"` + role + `"}}`)
	m := c.recv()
	if m["ok"] != true {
		c.t.Fatalf("hello 失败: %v", m)
	}
	return m
}
