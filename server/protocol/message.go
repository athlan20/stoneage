// Package protocol 定义客户端与服务端之间的 JSON Lines 消息结构、错误码和请求元数据，
// 约定见 docs/protocol.md 第 1–4 节。
package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
)

// Version 是当前支持的协议版本。
const Version = 1

// MaxLineBytes 是单行消息的最大长度（64 KiB）。
const MaxLineBytes = 64 * 1024

// UnknownType 是无法确定请求类型时，响应里使用的 type。
const UnknownType = "error"

// Request 是客户端发来的请求。
type Request struct {
	ID   uint64
	Type string
	Data json.RawMessage
}

// Decode 把 data 解到 v 里。不认识的字段忽略；类型不对时返回 BAD_REQUEST。
func (r *Request) Decode(v any) *Error {
	if err := json.Unmarshal(r.Data, v); err != nil {
		return Errorf(CodeBadRequest, "请求的 data 字段不正确")
	}
	return nil
}

// Error 是失败响应里的 error 对象，同时也是处理函数返回的错误类型。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	// CloseAfter 为 true 时，响应发出后服务端断开连接（不出现在协议里）。
	CloseAfter bool `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf 构造一个错误；message 是给玩家看的中文说明。
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithData 附带 error.data（如 retry_after_seconds）。
func (e *Error) WithData(data any) *Error {
	e.Data = data
	return e
}

// Closing 标记发出响应后断开连接。
func (e *Error) Closing() *Error {
	e.CloseAfter = true
	return e
}

// ParseRequest 解析一行请求。
//
// 返回的 *Request 在出错时仍可能带有已解析出的 id 和 type，用于组织错误响应。
// fatal 为 true 表示整行都无法解析为 JSON，按协议应回复后断开连接。
func ParseRequest(line []byte) (req *Request, perr *Error, fatal bool) {
	line = bytes.TrimSpace(line)
	req = &Request{Type: UnknownType}
	if !json.Valid(line) {
		return req, Errorf(CodeBadRequest, "无法解析请求"), true
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(line, &env); err != nil || env == nil {
		return req, Errorf(CodeBadRequest, "请求必须是 JSON 对象"), false
	}
	var typ string
	if raw, ok := env["type"]; ok {
		if json.Unmarshal(raw, &typ) == nil && typ != "" {
			req.Type = typ
		} else {
			typ = ""
		}
	}
	id, idOK := parseID(env["id"])
	if idOK {
		req.ID = id
	}
	switch {
	case !idOK:
		return req, Errorf(CodeBadRequest, "缺少 id，或 id 不是正整数"), false
	case typ == "":
		return req, Errorf(CodeBadRequest, "缺少 type"), false
	}
	switch raw := bytes.TrimSpace(env["data"]); {
	case len(raw) == 0 || bytes.Equal(raw, []byte("null")):
		req.Data = json.RawMessage("{}")
	case raw[0] == '{':
		req.Data = raw
	default:
		return req, Errorf(CodeBadRequest, "data 必须是 JSON 对象"), false
	}
	return req, nil, false
}

func parseID(raw json.RawMessage) (uint64, bool) {
	if len(raw) == 0 || raw[0] == '"' { // 带引号的数字字符串不算整数
		return 0, false
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	f, err := n.Float64()
	if err != nil || f < 1 || f > math.MaxInt32*1024 || f != math.Trunc(f) {
		return 0, false
	}
	return uint64(f), true
}

type okResponse struct {
	ID   uint64 `json:"id"`
	Type string `json:"type"`
	OK   bool   `json:"ok"`
	Data any    `json:"data"`
}

type errResponse struct {
	ID    uint64 `json:"id"`
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
	Error *Error `json:"error"`
}

type pushMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// EncodeOK 编码成功响应（含结尾换行）。data 为 nil 时写成 {}。
func EncodeOK(id uint64, typ string, data any) []byte {
	if data == nil {
		data = struct{}{}
	}
	return mustLine(okResponse{ID: id, Type: typ, OK: true, Data: data})
}

// EncodeError 编码失败响应（含结尾换行）。
func EncodeError(id uint64, typ string, e *Error) []byte {
	return mustLine(errResponse{ID: id, Type: typ, OK: false, Error: e})
}

// EncodePush 编码服务端主动推送（没有 id）。
func EncodePush(typ string, data any) []byte {
	if data == nil {
		data = struct{}{}
	}
	return mustLine(pushMessage{Type: typ, Data: data})
}

func mustLine(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// 只可能是处理函数返回了无法编码的数据，属于程序错误。
		b, _ = json.Marshal(errResponse{Type: UnknownType, Error: Errorf(CodeInternal, "%s", MessageInternal)})
	}
	return append(b, '\n')
}
