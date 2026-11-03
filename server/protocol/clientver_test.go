package protocol

import "testing"

func TestParseClientVer(t *testing.T) {
	for _, s := range []string{"0.1.0", "10.0.12"} {
		if _, ok := ParseClientVer(s); !ok {
			t.Errorf("%q 应能解析", s)
		}
	}
	for _, s := range []string{"", "0.1", "0.1.0.1", "v0.1.0", "0.1.x", "0.-1.0", "+0.1.0", "0.1.0-rc.1", "dev", "0..1"} {
		if _, ok := ParseClientVer(s); ok {
			t.Errorf("%q 不应解析", s)
		}
	}
	if v, _ := ParseClientVer("10.2.3"); v.String() != "10.2.3" {
		t.Errorf("String() = %s", v)
	}
}

func TestClientVersion(t *testing.T) {
	if v, ok := ClientVersion("stoneage-cli/0.1.0"); !ok || v.String() != "0.1.0" {
		t.Errorf("stoneage-cli/0.1.0 = %v %v", v, ok)
	}
	if _, ok := ClientVersion("stoneage-cli"); ok {
		t.Error("没有版本号时应取不到")
	}
	if _, ok := ClientVersion("case"); ok {
		t.Error("没有 / 时应取不到")
	}
}

func TestClientTooOld(t *testing.T) {
	cases := []struct {
		client, min string
		want        bool
	}{
		{"stoneage-cli/0.1.0", "0.2.0", true},
		{"stoneage-cli/0.1.9", "0.2.0", true},
		{"stoneage-cli/0.9.0", "0.10.0", true}, // 按数字比较，不按字符串
		{"stoneage-cli/0.2.0", "0.2.0", false}, // 等于最低版本放行
		{"stoneage-cli/0.3.0", "0.2.0", false},
		{"stoneage-cli/1.0.0", "0.2.0", false},
		{"stoneage-cli/0.1.0", "", false},    // 没配置最低版本
		{"stoneage-cli/0.1.0", "bad", false}, // 配置格式不对时放行
		{"dev", "0.2.0", false},              // 无法解析的客户端版本放行
		{"stoneage-cli/dev", "0.2.0", false}, // 同上
		{"", "0.2.0", false},
	}
	for _, c := range cases {
		if got := ClientTooOld(c.client, c.min); got != c.want {
			t.Errorf("ClientTooOld(%q, %q) = %v，想要 %v", c.client, c.min, got, c.want)
		}
	}
}
