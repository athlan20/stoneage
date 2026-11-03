package gateway

import (
	"bytes"
	"slices"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-246：连接级的压缩。hello 里声明了 "compress":["gzip"] 的连接，发出超过 10 KiB 的消息时压缩；
// 其他连接永远原样发送。压缩格式见 protocol/compress.go。

// outMsg 是写循环里的一条待发消息。plain 是原文（协议日志和诊断日志用），wire 是线上真正发出的字节，
// 未压缩时两者是同一块内存。
type outMsg struct {
	plain []byte
	wire  []byte
}

// packOut 决定一条消息线上怎么发。只有连接已协商压缩、原文超过阈值、压缩后确实变小时才压缩。
// 在入队时决定（而不是写的时候），这样 hello 响应排在协商之前，就一定是原样的。
func (s *Session) packOut(line []byte) outMsg {
	m := outMsg{plain: line, wire: line}
	if !s.compress.Load() || lineBytes(line) <= protocol.CompressMinBytes {
		return m
	}
	z, err := protocol.EncodeGzip(bytes.TrimSuffix(line, []byte("\n")))
	if err != nil {
		s.log.Error("压缩消息失败，改发原文", "err", err)
		return m
	}
	if len(z) < len(line) {
		m.wire = z
	}
	return m
}

// negotiateCompress 处理 hello 里的 compress 字段：声明支持 gzip 时打开压缩，并返回要在 hello 响应里确认的值。
func (s *Session) negotiateCompress(offered []string) string {
	if !slices.Contains(offered, protocol.CompressGzip) {
		return ""
	}
	s.compress.Store(true)
	return protocol.CompressGzip
}
