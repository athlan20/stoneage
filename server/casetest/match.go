package casetest

import (
	"fmt"
	"sort"
	"strings"
)

// isWildcard 判断期望值是不是通配：`"*"` 或 `"*=样例"`。
func isWildcard(v any) bool {
	s, ok := v.(string)
	return ok && strings.HasPrefix(s, "*")
}

// Match 比对期望值和实际值，返回第一处差异；一致时返回 nil。
//
//   - 期望值里以 * 开头的字符串是通配：字段必须存在，但不比较具体值；
//   - 期望的对象里的每个字段都必须出现在实际值里，实际值多出来的字段忽略（协议约定收到不认识的字段一律忽略）；
//   - 数组必须长度相同、逐项一致。
func Match(expected, actual any, path string) error { return MatchCapture(expected, actual, path, nil) }

// MatchCapture 同 Match；vars 非 nil 时，期望值写成 `"*>名字"` 或 `"*>名字=样例"` 的通配会把实际值记到 vars[名字]，
// 供后面的 c2s 行用 `"$名字"` 引用（例如把 create_agent_token 返回的令牌交给 agent_login）。
func MatchCapture(expected, actual any, path string, vars map[string]any) error {
	if isWildcard(expected) {
		if actual == nil && !strings.HasPrefix(expected.(string), "*?=") {
			return fmt.Errorf("%s: 缺少字段", path)
		}
		if name, ok := captureName(expected); ok && vars != nil {
			vars[name] = actual
		}
		return nil
	}
	// 期望值写成 "$名字"：必须等于之前捕获的值（例如列表里的 token_id 就是刚生成的那个）。
	if name, ok := expected.(string); ok && strings.HasPrefix(name, "$") && len(name) > 1 && vars != nil {
		want, known := vars[name[1:]]
		if !known {
			return fmt.Errorf("%s: 引用了还没有捕获的变量 %s", path, name)
		}
		if want != actual {
			return fmt.Errorf("%s: 期望 %s（%s），实际 %s", path, name, show(want), show(actual))
		}
		return nil
	}
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: 期望对象，实际 %s", path, show(actual))
		}
		keys := make([]string, 0, len(e))
		for k := range e {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			av, present := a[k]
			if !present {
				return fmt.Errorf("%s.%s: 缺少字段", path, k)
			}
			if err := MatchCapture(e[k], av, path+"."+k, vars); err != nil {
				return err
			}
		}
		return nil
	case []any:
		a, ok := actual.([]any)
		if !ok || len(a) != len(e) {
			return fmt.Errorf("%s: 期望 %d 项的数组，实际 %s", path, len(e), show(actual))
		}
		for i := range e {
			if err := MatchCapture(e[i], a[i], fmt.Sprintf("%s[%d]", path, i), vars); err != nil {
				return err
			}
		}
		return nil
	}
	if expected != actual {
		return fmt.Errorf("%s: 期望 %s，实际 %s", path, show(expected), show(actual))
	}
	return nil
}

// captureName 取出 "*>名字" 或 "*>名字=样例" 里的名字。
func captureName(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, "*>") {
		return "", false
	}
	name, _, _ := strings.Cut(s[2:], "=")
	return name, name != ""
}

// Substitute 把 c2s 消息里值恰好是 "$名字" 的字符串换成之前捕获的值；引用了还没捕获的名字时返回错误。
func Substitute(v any, vars map[string]any) (any, error) {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "$") && len(x) > 1 {
			val, ok := vars[x[1:]]
			if !ok {
				return nil, fmt.Errorf("引用了还没有捕获的变量 %s", x)
			}
			return val, nil
		}
	case map[string]any:
		for k, val := range x {
			nv, err := Substitute(val, vars)
			if err != nil {
				return nil, err
			}
			x[k] = nv
		}
	case []any:
		for i := range x {
			nv, err := Substitute(x[i], vars)
			if err != nil {
				return nil, err
			}
			x[i] = nv
		}
	}
	return v, nil
}

func show(v any) string {
	s := fmt.Sprintf("%#v", v)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
