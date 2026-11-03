package gateway

import (
	"bytes"
	"context"
	"net"
	"time"

	"github.com/coder/websocket"
)

// wsFrameQueue 是后台读循环最多提前收多少帧。队列满说明客户端发得太快、服务端处理不过来，
// 这时断开这一条连接，不让它一直占着内存（TCP 入口靠流控挡住，WebSocket 没有这种流控）。
const wsFrameQueue = 256

// wsConn 把一条 WebSocket 连接包成 net.Conn，交给 Session 使用，会话逻辑与裸 TCP 完全一样。
// 约定：一行 JSON 对应一个文本帧。写出时去掉行尾换行；读入时如果帧没有换行就补上，
// 这样 Session 的按行读取不必区分两种入口。
//
// 读取由后台 goroutine 独占：它持续读帧（这样 WebSocket 的 ping/pong 才能被处理），
// Read 只在 channel 上等，超时只是返回超时错误、不取消底层读取，连接不会因为读超时被库关掉。
type wsConn struct {
	c            *websocket.Conn
	ctx          context.Context
	cancel       context.CancelFunc
	remote       net.Addr
	maxLine      int
	writeTimeout time.Duration

	frames  chan []byte // 后台读循环交来的一帧（已补换行）
	readErr error       // 读循环结束的原因，在 close(frames) 之前写入
	pending []byte      // 当前帧还没交给调用方的部分
	readDL  time.Time   // 只由 Session 的读循环访问
}

func newWSConn(c *websocket.Conn, remote net.Addr, maxLine int, writeTimeout time.Duration) *wsConn {
	ctx, cancel := context.WithCancel(context.Background())
	w := &wsConn{
		c: c, ctx: ctx, cancel: cancel, remote: remote,
		maxLine: maxLine, writeTimeout: writeTimeout,
		frames: make(chan []byte, wsFrameQueue),
	}
	// 单帧上限比一行上限略宽：超长的行仍交给 readLine，由它按协议回复 BAD_REQUEST。
	c.SetReadLimit(int64(4 * maxLine))
	go w.readLoop()
	return w
}

func (w *wsConn) readLoop() {
	defer close(w.frames)
	for {
		_, data, err := w.c.Read(w.ctx)
		if err != nil {
			w.readErr = err
			return
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			data = append(data, '\n')
		}
		select {
		case w.frames <- data:
		default:
			w.readErr = errWSQueueFull
			_ = w.c.CloseNow()
			return
		}
	}
}

// Read 按帧交付数据，每次读到的字节不会跨过当前帧。
func (w *wsConn) Read(b []byte) (int, error) {
	if len(w.pending) == 0 {
		var timeout <-chan time.Time
		if !w.readDL.IsZero() {
			t := time.NewTimer(time.Until(w.readDL))
			defer t.Stop()
			timeout = t.C
		}
		select {
		case f, ok := <-w.frames:
			if !ok {
				return 0, w.readErr
			}
			w.pending = f
		case <-timeout:
			return 0, wsTimeoutError{}
		}
	}
	n := copy(b, w.pending)
	w.pending = w.pending[n:]
	return n, nil
}

// Write 把一行写成一个文本帧。writeLoop 每次只写一整行，所以这里一次调用就是一帧。
func (w *wsConn) Write(b []byte) (int, error) {
	ctx, cancel := context.WithTimeout(w.ctx, w.writeTimeout)
	defer cancel()
	if err := w.c.Write(ctx, websocket.MessageText, bytes.TrimSuffix(b, []byte("\n"))); err != nil {
		return 0, err
	}
	return len(b), nil
}

// Close 发送关闭帧并等对方确认（库内部有超时），之后取消连接的 ctx。
func (w *wsConn) Close() error {
	err := w.c.Close(websocket.StatusNormalClosure, "")
	w.cancel()
	return err
}

// keepAlive 定时发 ping，让中间的代理（如 Cloudflare）看到连接上有流量，不会因为空闲被断开。
// ping 收不到 pong 说明对端已经没了，直接断开，Session 会随之结束。
func (w *wsConn) keepAlive(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-t.C:
		}
		pctx, cancel := context.WithTimeout(w.ctx, every)
		err := w.c.Ping(pctx)
		cancel()
		if err != nil {
			_ = w.c.CloseNow()
			return
		}
	}
}

func (w *wsConn) LocalAddr() net.Addr  { return wsAddr{} }
func (w *wsConn) RemoteAddr() net.Addr { return w.remote }

func (w *wsConn) SetDeadline(t time.Time) error {
	w.readDL = t
	return nil
}

// SetReadDeadline 只影响 Read 的等待；写入有自己的超时（Options.WriteTimeout）。
func (w *wsConn) SetReadDeadline(t time.Time) error {
	w.readDL = t
	return nil
}

func (w *wsConn) SetWriteDeadline(time.Time) error { return nil }

// wsAddr 是 WebSocket 连接的本地地址占位，网络层不用它。
type wsAddr struct{}

func (wsAddr) Network() string { return "websocket" }
func (wsAddr) String() string  { return "websocket" }

// wsTimeoutError 让 Read 的超时和 TCP 的超时一样，被 isTimeout 识别。
type wsTimeoutError struct{}

func (wsTimeoutError) Error() string   { return "i/o timeout" }
func (wsTimeoutError) Timeout() bool   { return true }
func (wsTimeoutError) Temporary() bool { return true }

type wsError string

func (e wsError) Error() string { return string(e) }

const errWSQueueFull = wsError("WebSocket 发送太快，接收队列已满")
