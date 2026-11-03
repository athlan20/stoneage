package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
)

// STAGE-246：应用层压缩。TCP 与 WebSocket 共用这一套格式：压缩后仍是一行 JSON，形如
//
//	{"z":"gzip","d":"<base64 of gzip(原文)>"}
//
// 只有 hello 里声明了 "compress":["gzip"] 的连接，服务端才会对超过 10 KiB 的消息压缩；
// 没有声明的连接永远收到原样 JSON。客户端发来的压缩行服务端也认（对称实现）。

const (
	// CompressGzip 是目前唯一支持的压缩方式，hello 的 compress 数组里写它。
	CompressGzip = "gzip"
	// CompressMinBytes 是服务端压缩的阈值：原文（不含换行）超过这个字节数才压缩。
	CompressMinBytes = 10 * 1024
	// MaxInflatedBytes 是压缩行解压后的原文上限，防止解压炸弹。
	MaxInflatedBytes = 1 << 20
)

// zEnvelope 是压缩行的外壳，用来解码。带 type 的行是普通消息，不当作压缩行。
type zEnvelope struct {
	Z    *string         `json:"z"`
	D    *string         `json:"d"`
	Type json.RawMessage `json:"type"`
}

// zOut 是编码用的外壳。
type zOut struct {
	Z string `json:"z"`
	D string `json:"d"`
}

// EncodeGzip 把一行原文（不含换行）压缩成外壳行，结尾带换行。
func EncodeGzip(line []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(line); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(zOut{Z: CompressGzip, D: base64.StdEncoding.EncodeToString(buf.Bytes())})
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// DecodeLine 还原收到的一行：压缩行解压成原文，普通行原样返回（不做任何检查，行内容由 ParseRequest 处理）。
// 压缩行出错时返回 BAD_REQUEST：不支持的压缩方式、d 不是 base64、gzip 损坏、解压后超过 1 MiB。
func DecodeLine(line []byte) ([]byte, *Error) {
	if !bytes.Contains(line, []byte(`"z"`)) { // 大多数请求没有 z，先用字节检查省掉一次解析
		return line, nil
	}
	var env zEnvelope
	if json.Unmarshal(line, &env) != nil || env.Z == nil || len(env.Type) != 0 {
		return line, nil
	}
	if *env.Z != CompressGzip {
		return nil, Errorf(CodeBadRequest, "不支持的压缩方式：%s", *env.Z)
	}
	if env.D == nil {
		return nil, Errorf(CodeBadRequest, "压缩消息缺少 d 字段")
	}
	raw, err := base64.StdEncoding.DecodeString(*env.D)
	if err != nil {
		return nil, Errorf(CodeBadRequest, "压缩消息的 d 不是合法的 base64")
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, Errorf(CodeBadRequest, "压缩消息无法解压")
	}
	out, err := io.ReadAll(io.LimitReader(zr, MaxInflatedBytes+1))
	if err != nil {
		return nil, Errorf(CodeBadRequest, "压缩消息无法解压")
	}
	if len(out) > MaxInflatedBytes {
		return nil, Errorf(CodeBadRequest, "压缩消息解压后超过 1 MiB")
	}
	return out, nil
}
