package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-246：诊断日志的单元测试。

// lockedBuffer 是可以并发写读的缓冲区：写循环在另一个 goroutine 里写日志。
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// newDiagSession 造一个只用于测写循环的会话。对端是 net.Pipe，由测试决定怎么读写；日志写进返回的缓冲区。
func newDiagSession(t *testing.T, writeTimeout time.Duration) (*Session, net.Conn, *lockedBuffer) {
	t.Helper()
	logs := &lockedBuffer{}
	srv := New(Options{WriteTimeout: writeTimeout, Logger: slog.New(slog.NewTextHandler(logs, nil))})
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	return newSession(srv, a, 42, &net.TCPAddr{}), b, logs
}

// waitForLog 等日志里出现 want，最多 3 秒；超时返回当前全部日志并失败。
func waitForLog(t *testing.T, logs *lockedBuffer, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if out := logs.String(); strings.Contains(out, want) {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("没有等到日志 %q，实际日志:\n%s", want, logs.String())
	return ""
}

func TestWriteFailReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"上下文超时", context.DeadlineExceeded, "超时"},
		{"写截止时间到了", os.ErrDeadlineExceeded, "超时"},
		{"对端读到结束", io.EOF, "对端关闭"},
		{"连接被对端重置", syscall.ECONNRESET, "对端关闭"},
		{"管道写端已关", syscall.EPIPE, "对端关闭"},
		{"其他错误", errors.New("磁盘坏了"), "其他"},
	}
	for _, c := range cases {
		if got := writeFailReason(c.err); got != c.want {
			t.Errorf("%s：writeFailReason(%v) = %q，应为 %q", c.name, c.err, got, c.want)
		}
	}
}

func TestLineHelpers(t *testing.T) {
	if n := lineBytes([]byte("abc\n")); n != 3 {
		t.Errorf("lineBytes 应不计换行，得到 %d", n)
	}
	if n := lineBytes([]byte("abc")); n != 3 {
		t.Errorf("没有换行的行 lineBytes 应为 3，得到 %d", n)
	}
	if typ := lineType([]byte(`{"type":"room_state","data":{}}` + "\n")); typ != "room_state" {
		t.Errorf("lineType 取到 %q", typ)
	}
	if typ := lineType([]byte("不是 JSON")); typ != protocol.UnknownType {
		t.Errorf("无法解析时 lineType 应为 %q，得到 %q", protocol.UnknownType, typ)
	}
}

func TestWriteLoopLogsWriteTimeout(t *testing.T) {
	sess, _, logs := newDiagSession(t, 50*time.Millisecond)
	go sess.writeLoop()
	// 对端不读，写会一直阻塞，直到写超时。
	sess.sendRaw(protocol.EncodePush("room_state", map[string]any{"x": 1}))
	out := waitForLog(t, logs, "写消息失败")
	for _, want := range []string{"session=42", "type=room_state", "bytes=", "reason=超时"} {
		if !strings.Contains(out, want) {
			t.Errorf("写超时日志缺少 %q:\n%s", want, out)
		}
	}
}

func TestWriteLoopLogsOtherWriteError(t *testing.T) {
	sess, peer, logs := newDiagSession(t, time.Second)
	_ = peer.Close() // 对端先关，写会失败
	go sess.writeLoop()
	sess.sendRaw(protocol.EncodePush("ping", map[string]any{}))
	out := waitForLog(t, logs, "写消息失败")
	if !strings.Contains(out, "reason=其他") {
		t.Errorf("对端已关闭的写失败应归为其他，日志:\n%s", out)
	}
}

func TestWriteLoopLogsBigMessageOnce(t *testing.T) {
	sess, peer, logs := newDiagSession(t, 5*time.Second)
	go func() { _, _ = io.Copy(io.Discard, peer) }() // 对端一直读
	go sess.writeLoop()
	// 先发小消息再发大消息：写循环按顺序写，大消息的日志出现时小消息已经写完。
	sess.sendRaw(protocol.EncodePush("small_push", map[string]any{"x": 1}))
	big := strings.Repeat("x", 11*1024)
	sess.sendRaw(protocol.EncodePush("big_push", map[string]any{"text": big}))
	out := waitForLog(t, logs, "单条消息超过 10 KiB")
	if !strings.Contains(out, "type=big_push") {
		t.Errorf("超大消息日志缺少类型:\n%s", out)
	}
	if n := strings.Count(out, "单条消息超过 10 KiB"); n != 1 {
		t.Errorf("只有一条超大消息，日志应出现 1 次，实际 %d 次:\n%s", n, out)
	}
	if strings.Contains(out, "small_push") {
		t.Errorf("小消息不应记超大日志:\n%s", out)
	}
}

func TestTraceLineHasByteCount(t *testing.T) {
	buf := &lockedBuffer{}
	tr := &tracer{w: buf}
	tr.log("→", 3, []byte(`{"type":"x"}`), 12)
	tr.log("←", 3, []byte("<超长的行>"), -1)
	out := buf.String()
	if !strings.Contains(out, "→ {\"type\":\"x\"} (12 字节)") {
		t.Errorf("协议日志应带字节数:\n%s", out)
	}
	if !strings.Contains(out, "(超过上限)") {
		t.Errorf("超长行应写“超过上限”:\n%s", out)
	}
}
