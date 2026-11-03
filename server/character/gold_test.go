package character

import "testing"

// STAGE-310：石币发放按持有上限封顶（纯函数，不碰数据库）。
func TestGrantGoldCapsAtMaxHave(t *testing.T) {
	const cap0 = 1000000 // 持有上限（函数只看传入的上限，这里用小数字）
	cases := []struct {
		name          string
		gold, amount  int64
		maxHave, want int64
	}{
		{"上限内全给", 0, 500000, cap0, 500000},
		{"刚好到上限", 500000, 500000, cap0, 500000},
		{"超过上限封顶", 600000, 500000, cap0, 400000},
		{"已在上限不再加", cap0, 10, cap0, 0},
		{"旧数据超过上限不加", cap0 + 5, 10, cap0, 0},
	}
	for _, c := range cases {
		if got := grantGold(c.gold, c.amount, c.maxHave); got != c.want {
			t.Errorf("%s: grantGold(%d,%d,%d) = %d，要 %d", c.name, c.gold, c.amount, c.maxHave, got, c.want)
		}
	}
}

func TestParseGoldPayload(t *testing.T) {
	id, granted, err := parseGoldPayload("42:500000")
	if err != nil || id != 42 || granted != 500000 {
		t.Fatalf("解析不对: %d %d %v", id, granted, err)
	}
	for _, bad := range []string{"", "42", "x:1", "42:y"} {
		if _, _, err := parseGoldPayload(bad); err == nil {
			t.Errorf("%q 应该解析失败", bad)
		}
	}
}
