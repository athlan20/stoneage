package gateway

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// tracer 把收发的每一行写进协议日志。
type tracer struct {
	mu sync.Mutex
	w  io.Writer
}

// log 写一行协议日志。size 是这一行的原始字节数（不含换行，脱敏前），行末带上，方便统计大消息；
// size 为负数表示行超过了上限、读不到原文，只写“超过上限”。
func (t *tracer) log(dir string, sessionID uint64, line []byte, size int) {
	if t == nil {
		return
	}
	msg := protocol.Redact(line)
	sizeText := "超过上限"
	if size >= 0 {
		sizeText = fmt.Sprintf("%d 字节", size)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.w, "%s #%d %s %s (%s)\n", time.Now().Format("2006-01-02T15:04:05.000Z07:00"), sessionID, dir, msg, sizeText)
}
