package character

import (
	"encoding/json"
	"testing"
)

// STAGE-328：gold_drop 的参数判断（纯函数，不需要数据库）。
func TestParseGoldAmount(t *testing.T) {
	ok := map[string]int64{"1": 1, "1000": 1000, "9999950": 9999950}
	for raw, want := range ok {
		got, err := parseGoldAmount(json.RawMessage(raw))
		if err != nil || got != want {
			t.Fatalf("parseGoldAmount(%s) = %d, %v; want %d", raw, got, err, want)
		}
	}
	bad := []string{"", "null", "0", "-5", "1.5", "1.0", "1e3", `"100"`, `true`, `[1]`, `99999999999999999999`}
	for _, raw := range bad {
		if _, err := parseGoldAmount(json.RawMessage(raw)); err == nil {
			t.Fatalf("parseGoldAmount(%q) 应该失败", raw)
		}
	}
}

func TestGoldConfirmed(t *testing.T) {
	if !goldConfirmed(json.RawMessage(`"yes"`)) {
		t.Fatal(`"yes" 应该通过`)
	}
	for _, raw := range []string{"", "null", "true", `"YES"`, `"y"`, `""`, `1`} {
		if goldConfirmed(json.RawMessage(raw)) {
			t.Fatalf("goldConfirmed(%q) 应该是 false", raw)
		}
	}
}
