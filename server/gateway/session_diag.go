package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"syscall"

	"github.com/coder/websocket"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-246：诊断日志。写失败、超大消息的记录都在这里，写循环只调用这些函数。

// bigMessageBytes 是单条消息记“超过 10 KiB”日志的阈值，按不含换行的字节数算。
const bigMessageBytes = 10 * 1024

// lineBytes 返回一行的字节数，不含结尾换行。协议日志和诊断日志都用这个口径。
func lineBytes(b []byte) int {
	return len(bytes.TrimSuffix(b, []byte("\n")))
}

// lineType 从已编码的一行里取出 type。只在写失败或消息超大时调用，正常发送不解析 JSON。
func lineType(b []byte) string {
	var env struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(b, &env) != nil || env.Type == "" {
		return protocol.UnknownType
	}
	return env.Type
}

// writeFailReason 把写失败的原因归成三类：超时、对端关闭、其他。
func writeFailReason(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "超时"
	case errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed), errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ECONNRESET), websocket.CloseStatus(err) != -1:
		return "对端关闭"
	default:
		return "其他"
	}
}

// msgAttrs 返回日志里描述一条消息的字段：类型、原文字节数，有账号时还有账号；
// 压缩过的消息另外带上线上字节数 wire_bytes。
func (s *Session) msgAttrs(m outMsg) []any {
	attrs := []any{"type", lineType(m.plain), "bytes", lineBytes(m.plain)}
	if len(m.wire) != len(m.plain) {
		attrs = append(attrs, "wire_bytes", lineBytes(m.wire))
	}
	if acct := s.Account(); acct != "" {
		attrs = append(attrs, "account", acct)
	}
	return attrs
}

// logWriteFailure 记录写失败的会话、账号、消息类型、字节数和原因。随后连接会被关闭。
func (s *Session) logWriteFailure(m outMsg, err error) {
	attrs := append(s.msgAttrs(m), "reason", writeFailReason(err), "err", err.Error())
	s.log.Warn("写消息失败，断开连接", attrs...)
}

// logBigMessage 记录一条超过 10 KiB 的已发出消息，用来统计哪类消息最大。
func (s *Session) logBigMessage(m outMsg) {
	s.log.Info("单条消息超过 10 KiB", s.msgAttrs(m)...)
}
