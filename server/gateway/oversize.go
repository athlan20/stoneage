package gateway

import (
	"encoding/json"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-258：线上超限的处理。生产经 CDN 接入，CDN 限制单条帧的大小，超过 protocol.MaxLineBytes（64 KiB）的
// 线上字节数（压缩后的 wire，不含换行）不能发，否则连接会被 CDN 直接断开、客户端没有任何提示。
// 处理办法：请求的响应改发同 id、同 type 的 RESPONSE_TOO_LARGE 错误；推送直接丢弃；两者都记 Warn 日志，连接保持。

// oversizeMsg 处理一条线上超过 64 KiB 的消息：不发原消息，连接不断开。
func (s *Session) oversizeMsg(m outMsg) {
	id, typ := lineIDType(m.plain)
	attrs := s.msgAttrs(m)
	if id == 0 {
		s.log.Warn("推送超过 64 KiB，已丢弃", attrs...)
		return
	}
	s.log.Warn("响应超过 64 KiB，改发 RESPONSE_TOO_LARGE", append(attrs, "id", id)...)
	e := protocol.Errorf(protocol.CodeResponseTooLarge,
		"响应过大：请在 hello 里声明 compress gzip，或用 parts/keyword/offset/limit 缩小范围")
	em := s.packOut(protocol.EncodeError(id, typ, e))
	if lineBytes(em.wire) > protocol.MaxLineBytes {
		// 连类型名一起都超了（只可能是极长的未知类型名），就不发了，免得递归。
		s.log.Warn("RESPONSE_TOO_LARGE 错误本身也超过 64 KiB，已丢弃", s.msgAttrs(em)...)
		return
	}
	s.enqueue(em)
}

// lineIDType 从已编码的一行里取出 id 和 type。推送没有 id，返回 0。
func lineIDType(b []byte) (uint64, string) {
	var env struct {
		ID   uint64 `json:"id"`
		Type string `json:"type"`
	}
	if json.Unmarshal(b, &env) != nil {
		return 0, protocol.UnknownType
	}
	if env.Type == "" {
		env.Type = protocol.UnknownType
	}
	return env.ID, env.Type
}
