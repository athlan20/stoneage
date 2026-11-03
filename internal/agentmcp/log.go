package agentmcp

// 协议日志与脱敏（STAGE-300）。stdout 是 MCP 通道，绝对不写日志；这里只写 --log 指定的文件。
// 令牌在日志和工具结果里都要替换成 ***：先由 protocol.Redact 隐去敏感字段，再把令牌本身替换掉。

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

type protoLog struct {
	mu    sync.Mutex
	w     io.Writer // nil 表示不记日志
	token string
}

// record 记一行协议内容，dir 为 "->"（发出）或 "<-"（收到）。line 应是解压后的原文。
func (l *protoLog) record(dir string, line []byte) {
	if l == nil || l.w == nil {
		return
	}
	l.write(dir + " " + protocol.Redact(line))
}

// note 记一条本地事件（不是协议内容）。
func (l *protoLog) note(text string) {
	if l == nil || l.w == nil {
		return
	}
	l.write("** " + text)
}

func (l *protoLog) write(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "%s %s\n", time.Now().Format("15:04:05.000"), l.scrub(text))
}

// scrub 把文本里的令牌替换掉。日志、工具结果、错误信息都要经过它。
func (l *protoLog) scrub(text string) string {
	if l == nil || l.token == "" || !strings.Contains(text, l.token) {
		return text
	}
	return strings.ReplaceAll(text, l.token, "***")
}
