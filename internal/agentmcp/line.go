package agentmcp

// 底层传输（STAGE-300）：TCP 一行一条 JSON，WebSocket 一帧一条（docs/protocol.md 第 1 节）。
// 两种入口的会话逻辑完全相同，差别只在这一层。

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// maxWireLine 是读入一行（线上字节数，不含换行）的上限。服务端单行上限是 64 KiB，这里放宽，防止异常数据占满内存。
const maxWireLine = 1 << 20

// writeTimeout 是没有 ctx 截止时间时单次写入的超时。
const writeTimeout = 15 * time.Second

// lineConn 是一条底层连接。ReadLine 返回的内容不含换行，WriteLine 会自己补上换行（TCP）。
type lineConn interface {
	ReadLine() ([]byte, error)
	WriteLine(ctx context.Context, line []byte) error
	Close() error
}

// isWebSocket 判断服务端地址是不是 ws:// 或 wss://。其他写法（host:port）一律走 TCP。
func isWebSocket(server string) bool {
	return strings.HasPrefix(server, "ws://") || strings.HasPrefix(server, "wss://")
}

// dialLine 按地址连上服务端，只做底层连接，不握手。
func dialLine(ctx context.Context, server string) (lineConn, error) {
	if isWebSocket(server) {
		c, _, err := websocket.Dial(ctx, server, nil)
		if err != nil {
			return nil, err
		}
		c.SetReadLimit(maxWireLine)
		return &wsLine{c: c}, nil
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", server)
	if err != nil {
		return nil, err
	}
	return &tcpLine{conn: conn, r: bufio.NewReaderSize(conn, 64*1024)}, nil
}

// wsLine 是 WebSocket 连接：一个文本帧就是一行。
type wsLine struct{ c *websocket.Conn }

func (l *wsLine) ReadLine() ([]byte, error) {
	_, data, err := l.c.Read(context.Background())
	return data, err
}

func (l *wsLine) WriteLine(ctx context.Context, line []byte) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, writeTimeout)
		defer cancel()
	}
	return l.c.Write(ctx, websocket.MessageText, line)
}

func (l *wsLine) Close() error { return l.c.CloseNow() }

// tcpLine 是裸 TCP 连接：每行一条 JSON，以 \n 结尾。
type tcpLine struct {
	conn net.Conn
	r    *bufio.Reader
}

func (l *tcpLine) ReadLine() ([]byte, error) {
	line, err := l.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) > maxWireLine {
		return nil, fmt.Errorf("单行超过 %d 字节", maxWireLine)
	}
	return bytes.TrimRight(line, "\r\n"), nil
}

func (l *tcpLine) WriteLine(ctx context.Context, line []byte) error {
	deadline := time.Now().Add(writeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := l.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	_, err := l.conn.Write(append(append([]byte{}, line...), '\n'))
	return err
}

func (l *tcpLine) Close() error { return l.conn.Close() }
