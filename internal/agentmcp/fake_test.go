package agentmcp

// 测试用的假服务端（STAGE-300）：本地 TCP 监听，逐行收请求，按脚本回响应或推送。

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testToken = "sat_test_token_secret"

type fakeSrv struct {
	t      *testing.T
	ln     net.Listener
	docRev string
	// script 先于默认回复执行；返回 true 表示已处理。
	script func(fc *fakeConn, req map[string]any) bool

	mu    sync.Mutex
	conns int
	reqs  []map[string]any
}

type fakeConn struct {
	conn net.Conn
	mu   sync.Mutex
}

func newFakeSrv(t *testing.T) *fakeSrv {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSrv{t: t, ln: ln, docRev: "rev-a"}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns++
			s.mu.Unlock()
			go s.serve(c)
		}
	}()
	return s
}

func (s *fakeSrv) addr() string { return s.ln.Addr().String() }

func (s *fakeSrv) connCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

// count 返回收到的某类请求数。
func (s *fakeSrv) count(typ string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.reqs {
		if r["type"] == typ {
			n++
		}
	}
	return n
}

// waitType 等到收到某类请求（或超时），返回那条请求。
func (s *fakeSrv) waitType(typ string, d time.Duration) map[string]any {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, r := range s.reqs {
			if r["type"] == typ {
				s.mu.Unlock()
				return r
			}
		}
		s.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("等待 %s 超时", typ)
	return nil
}

func (s *fakeSrv) serve(c net.Conn) {
	fc := &fakeConn{conn: c}
	defer c.Close()
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var req map[string]any
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			continue
		}
		s.mu.Lock()
		s.reqs = append(s.reqs, req)
		s.mu.Unlock()
		if s.script != nil && s.script(fc, req) {
			continue
		}
		s.defaultReply(fc, req)
	}
}

// defaultReply 是正常服务端的回复：握手、登录、进世界都成功。
func (s *fakeSrv) defaultReply(fc *fakeConn, req map[string]any) {
	data := map[string]any{}
	switch req["type"] {
	case "hello":
		data = map[string]any{"protocol": 1, "server": "stoneage-server/test", "heartbeat_seconds": 60, "compress": "gzip", "docs_rev": s.docRev}
	case "agent_login":
		d, _ := req["data"].(map[string]any)
		if tok, _ := d["token"].(string); tok != testToken {
			fc.sendLine(errLine(req, "INVALID_TOKEN", "令牌无效"))
			return
		}
		data = map[string]any{"account": "alice", "token_id": "t_1"}
	case "characters":
		data = map[string]any{"max_slots": 2, "characters": []any{map[string]any{"slot": 0, "name": "勇者"}}}
	case "enter_world":
		data = map[string]any{"character": map[string]any{"slot": 0}, "position": map[string]any{"map_name": "加加村"}, "control": "agent"}
	case "status":
		data = map[string]any{"level": 1, "hp": 35}
	}
	fc.sendLine(okLine(req, data))
}

func okLine(req map[string]any, data any) map[string]any {
	return map[string]any{"id": req["id"], "type": req["type"], "ok": true, "data": data}
}

func errLine(req map[string]any, code, msg string) map[string]any {
	return map[string]any{"id": req["id"], "type": req["type"], "ok": false, "error": map[string]any{"code": code, "message": msg}}
}

// send 发一行 JSON（普通行）。
func (fc *fakeConn) send(v any) {
	b, _ := json.Marshal(v)
	fc.write(append(b, '\n'))
}

func (fc *fakeConn) sendLine(v any) { fc.send(v) }

// sendGzip 发一行压缩行（STAGE-246）。
func (fc *fakeConn) sendGzip(v any) {
	b, _ := json.Marshal(v)
	line, err := protocol.EncodeGzip(b)
	if err != nil {
		panic(err)
	}
	fc.write(line)
}

func (fc *fakeConn) write(b []byte) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	_, _ = fc.conn.Write(b)
}

// newTestServer 建一个指向假服务端的 MCP 服务。docs_rev 记录写到临时目录，不碰真实的家目录。
func newTestServer(t *testing.T, fake *fakeSrv, logBuf *testWriter) *Server {
	t.Helper()
	cfg := Config{
		Server:    fake.addr(),
		Token:     testToken,
		StatePath: filepath.Join(t.TempDir(), "docs-rev.json"),
	}
	if logBuf != nil {
		cfg.LogFile = logBuf
	}
	srv := New(cfg)
	t.Cleanup(srv.Close)
	return srv
}

// testWriter 是线程安全的内存日志。
type testWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *testWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

// findTool 按名字取工具定义。
func findTool(t *testing.T, name string) tool {
	t.Helper()
	for _, tl := range allTools() {
		if tl.name == name {
			return tl
		}
	}
	t.Fatalf("没有工具 %s", name)
	return tool{}
}

// callTool 直接执行一个工具，返回它的统一结果（解析后的 JSON）。
func callTool(t *testing.T, srv *Server, name, args string) map[string]any {
	t.Helper()
	tl := findTool(t, name)
	data, e := tl.run(context.Background(), srv.sess, json.RawMessage(args))
	res := srv.sess.result(data, e)
	return parseResult(t, res)
}

func parseResult(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	text := res.Content[0].(*mcp.TextContent).Text
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("工具结果不是 JSON：%v\n%s", err, text)
	}
	return out
}

// errCode 取结果里的错误码，没有错误时返回空串。
func errCode(out map[string]any) string {
	e, ok := out["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := e["code"].(string)
	return code
}
