package protocol

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	tests := []struct{ in, mustNot, must string }{
		{`{"id":1,"type":"login","data":{"account":"a","password":"p4ss"}}`, "p4ss", `"password":"***"`},
		{`{"type":"x","data":{"nested":[{"new_password":"zzz"},{"Password":"yyy"}]}}`, "zzz", `"new_password":"***"`},
		{`{"data":{"token":"sat_abc","token_id":"t_1"}}`, "sat_abc", `"token_id":"t_1"`},
		{`{"id":18446744073709551615}`, "", `18446744073709551615`},
		{`{"password":"unterminated`, "unterminated", "无法解析"},
		{`{"a":1} {"password":"second"}`, "second", "无法解析"},
	}
	for _, tt := range tests {
		got := Redact([]byte(tt.in))
		if tt.mustNot != "" && strings.Contains(got, tt.mustNot) {
			t.Errorf("Redact(%s) = %s，不应包含 %q", tt.in, got, tt.mustNot)
		}
		if !strings.Contains(got, tt.must) {
			t.Errorf("Redact(%s) = %s，应包含 %q", tt.in, got, tt.must)
		}
	}
}
