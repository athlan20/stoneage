package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Redact 把一行协议消息里所有密码和令牌的值换成 "***"，用于协议日志。
// 无法解析的行不输出内容（可能包含明文密码），只写长度。
func Redact(line []byte) string {
	line = bytes.TrimSpace(line)
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return fmt.Sprintf("<无法解析的数据，%d 字节>", len(line))
	}
	out, err := json.Marshal(redactValue(v))
	if err != nil {
		return fmt.Sprintf("<无法输出，%d 字节>", len(line))
	}
	return string(out)
}

func redactValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if secretKey(k) {
				x[k] = "***"
			} else {
				x[k] = redactValue(val)
			}
		}
	case []any:
		for i := range x {
			x[i] = redactValue(x[i])
		}
	}
	return v
}

// secretKey 判断字段名是否属于密码或令牌（token_id 这类编号不算）。
func secretKey(k string) bool {
	k = strings.ToLower(k)
	return strings.Contains(k, "password") || k == "token"
}
