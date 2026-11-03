package casetest

import (
	"encoding/json"
	"testing"
)

func parse(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name, want, got string
		ok              bool
	}{
		{"equal", `{"a":1,"b":"x"}`, `{"a":1,"b":"x"}`, true},
		{"wildcard", `{"a":"*","b":"*=样例"}`, `{"a":5,"b":"随便"}`, true},
		{"wildcard needs field", `{"a":"*"}`, `{}`, false},
		{"nullable wildcard", `{"a":"*?=原因"}`, `{"a":null}`, true},
		{"nullable needs field", `{"a":"*?=原因"}`, `{}`, false},
		{"nullable text", `{"a":"*?=原因"}`, `{"a":"关闭"}`, true},
		{"wildcard null", `{"a":"*"}`, `{"a":null}`, false},
		{"extra fields ignored", `{"a":1}`, `{"a":1,"new":2}`, true},
		{"value differs", `{"a":1}`, `{"a":2}`, false},
		{"type differs", `{"a":1}`, `{"a":"1"}`, false},
		{"nested", `{"e":{"code":"X","data":{"n":"*=3"}}}`, `{"e":{"code":"X","message":"m","data":{"n":9}}}`, true},
		{"nested differs", `{"e":{"code":"X"}}`, `{"e":{"code":"Y"}}`, false},
		{"array", `{"l":[1,"*"]}`, `{"l":[1,2]}`, true},
		{"array length", `{"l":[1]}`, `{"l":[1,2]}`, false},
		{"bool", `{"ok":true}`, `{"ok":false}`, false},
	}
	for _, tt := range tests {
		err := Match(parse(t, tt.want), parse(t, tt.got), "msg")
		if (err == nil) != tt.ok {
			t.Errorf("%s: err = %v，期望 ok=%v", tt.name, err, tt.ok)
		}
	}
}
