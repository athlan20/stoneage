package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrecedence(t *testing.T) {
	path := writeEnv(t, "# 注释\nSTONEAGE_LISTEN=0.0.0.0:7777\nexport STONEAGE_DATABASE_URL=\"postgres://u:p@127.0.0.1:5433/db\"\n")
	t.Setenv(EnvListen, "")
	t.Setenv(EnvDatabaseURL, "")
	c, err := Load(path)
	if err != nil || c.Listen != "0.0.0.0:7777" || c.DatabaseURL != "postgres://u:p@127.0.0.1:5433/db" {
		t.Fatalf("文件配置: %+v %v", c, err)
	}
	t.Setenv(EnvListen, "127.0.0.1:9000")
	c, err = Load(path)
	if err != nil || c.Listen != "127.0.0.1:9000" {
		t.Fatalf("环境变量应覆盖文件: %+v %v", c, err)
	}
	if c.DatabaseURL != "postgres://u:p@127.0.0.1:5433/db" {
		t.Errorf("未设置的项仍取文件: %+v", c)
	}
}

func TestDefaultsAndMissingFile(t *testing.T) {
	t.Setenv(EnvListen, "")
	t.Setenv(EnvDatabaseURL, "")
	c, err := Load(filepath.Join(t.TempDir(), "none.env"))
	if err != nil || c.Listen != DefaultListen || c.DatabaseURL != "" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestInvalid(t *testing.T) {
	t.Setenv(EnvListen, "not-an-address")
	if _, err := Load(""); err == nil {
		t.Error("监听地址格式不对应当报错")
	}
	if _, err := ParseEnvFile(writeEnv(t, "oops\n")); err == nil {
		t.Error(".env 行格式不对应当报错")
	}
}

func TestParseDatabaseURL(t *testing.T) {
	p, err := ParseDatabaseURL("postgres://stoneage:s3cret@127.0.0.1:5433/stoneage?sslmode=disable")
	if err != nil || p != (DatabaseParts{"stoneage", "s3cret", "stoneage", "127.0.0.1", "5433"}) {
		t.Fatalf("%+v %v", p, err)
	}
	if p, _ := ParseDatabaseURL("postgres://u:p@localhost/db"); p.Port != "5432" {
		t.Errorf("端口缺省应为 5432: %+v", p)
	}
	for _, bad := range []string{"mysql://u:p@h/db", "postgres://h/db", "postgres://u:p@h/", "::"} {
		if _, err := ParseDatabaseURL(bad); err == nil {
			t.Errorf("%q 应当报错", bad)
		}
	}
}

func TestIsLocalDatabase(t *testing.T) {
	cases := map[string]bool{
		"postgres://u:p@127.0.0.1:5432/db":                true,
		"postgresql://u:p@localhost/db":                   true,
		"postgres://u:p@[::1]:5432/db":                    true,
		"postgres://u@/db?host=/var/run/postgresql":       true,
		"postgres://u:p@10.0.0.5:5432/db":                 false,
		"postgres://u:p@db.example.com/db":                false,
		"postgres://u:p@127.0.0.1/db?host=db.example.com": false,
		"不是连接串":                                           false,
	}
	for raw, want := range cases {
		if got := IsLocalDatabase(raw); got != want {
			t.Errorf("IsLocalDatabase(%q) = %v，期望 %v", raw, got, want)
		}
	}
}

func TestWebSocketAndTrustedProxies(t *testing.T) {
	path := writeEnv(t, "STONEAGE_WS_LISTEN=0.0.0.0:7067\nSTONEAGE_TRUSTED_PROXIES=173.245.48.0/20, 2400:cb00::/32 ,10.1.2.3\n")
	t.Setenv(EnvWSListen, "")
	t.Setenv(EnvTrustedProxies, "")
	c, err := Load(path)
	if err != nil || c.WSListen != "0.0.0.0:7067" || len(c.TrustedProxies) != 3 {
		t.Fatalf("文件配置: %+v %v", c, err)
	}
	if got := c.TrustedProxies[0].String(); got != "173.245.48.0/20" {
		t.Errorf("CIDR 应原样保留: %s", got)
	}
	if got := c.TrustedProxies[2].String(); got != "10.1.2.3/32" {
		t.Errorf("单个 IP 应当成 /32: %s", got)
	}

	// 默认：不开 WebSocket，不信任任何代理。
	c, err = Load(filepath.Join(t.TempDir(), "none.env"))
	if err != nil || c.WSListen != "" || c.TrustedProxies != nil {
		t.Fatalf("默认应不开 WebSocket、不信任代理: %+v %v", c, err)
	}
}

func TestWebSocketInvalid(t *testing.T) {
	t.Setenv(EnvDatabaseURL, "")
	t.Setenv(EnvWSListen, "7067")
	if _, err := Load(""); err == nil {
		t.Error("WebSocket 监听地址缺少端口应当报错")
	}
	t.Setenv(EnvWSListen, "")
	t.Setenv(EnvTrustedProxies, "not-an-ip")
	if _, err := Load(""); err == nil {
		t.Error("可信代理格式不对应当报错")
	}
}

// STAGE-100：客户端版本要求必须是 x.y.z，空表示不检查。
func TestClientVersionConfig(t *testing.T) {
	t.Setenv(EnvMinClientVersion, "")
	t.Setenv(EnvLatestClientVersion, "")
	c, err := Load(filepath.Join(t.TempDir(), "none.env"))
	if err != nil || c.MinClientVersion != "" || c.LatestClientVersion != "" {
		t.Fatalf("默认应为空: %+v %v", c, err)
	}
	t.Setenv(EnvMinClientVersion, "0.2.0")
	t.Setenv(EnvLatestClientVersion, "0.3.1")
	c, err = Load(filepath.Join(t.TempDir(), "none.env"))
	if err != nil || c.MinClientVersion != "0.2.0" || c.LatestClientVersion != "0.3.1" {
		t.Fatalf("环境变量应生效: %+v %v", c, err)
	}
	for _, bad := range []string{"0.2", "v0.2.0", "0.2.0-rc.1"} {
		t.Setenv(EnvMinClientVersion, bad)
		if _, err := Load(filepath.Join(t.TempDir(), "none.env")); err == nil {
			t.Errorf("%s 应被拒绝", bad)
		}
	}
}

// STAGE-118：agent 在线时长上限，0 不限制，负数和非整数报错。
func TestAgentMaxOnlineMinutes(t *testing.T) {
	t.Setenv(EnvAgentMaxOnlineMinutes, "")
	path := writeEnv(t, "STONEAGE_AGENT_MAX_ONLINE_MINUTES=90\n")
	c, err := Load(path)
	if err != nil || c.AgentMaxOnlineMinutes != 90 {
		t.Fatalf("文件配置: %+v %v", c, err)
	}
	if c, err = Load(writeEnv(t, "")); err != nil || c.AgentMaxOnlineMinutes != 0 {
		t.Fatalf("不写时应不限制: %+v %v", c, err)
	}
	for _, bad := range []string{"-1", "abc", "1.5"} {
		t.Setenv(EnvAgentMaxOnlineMinutes, bad)
		if _, err := Load(writeEnv(t, "")); err == nil {
			t.Errorf("%q 应当报错", bad)
		}
	}
}

// STAGE-290：定时存盘间隔，不写取 180 秒，0 关闭，负数和非整数报错。
func TestAutosaveSeconds(t *testing.T) {
	t.Setenv(EnvAutosaveSeconds, "")
	if c, err := Load(writeEnv(t, "")); err != nil || c.AutosaveSeconds != 180 {
		t.Fatalf("不写时应是 180: %+v %v", c, err)
	}
	if c, err := Load(writeEnv(t, "STONEAGE_AUTOSAVE_SECONDS=60\n")); err != nil || c.AutosaveSeconds != 60 {
		t.Fatalf("文件配置: %+v %v", c, err)
	}
	if c, err := Load(writeEnv(t, "STONEAGE_AUTOSAVE_SECONDS=0\n")); err != nil || c.AutosaveSeconds != 0 {
		t.Fatalf("0 应关闭: %+v %v", c, err)
	}
	for _, bad := range []string{"-1", "abc", "1.5"} {
		t.Setenv(EnvAutosaveSeconds, bad)
		if _, err := Load(writeEnv(t, "")); err == nil {
			t.Errorf("%q 应当报错", bad)
		}
	}
}

// STAGE-214：任务脚本开关原样保留（格式在 gamedata 里解析），不设为空；.env 文件和环境变量都能写。
func TestNpcEventOpenConfig(t *testing.T) {
	t.Setenv(EnvNpcEventOpen, "")
	if c, err := Load(writeEnv(t, "")); err != nil || c.NpcEventOpen != "" {
		t.Fatalf("不设时应为空: %+v %v", c, err)
	}
	const spec = " king/event69_1, jaruga/event/ruri05 "
	t.Setenv(EnvNpcEventOpen, spec)
	if c, err := Load(writeEnv(t, "")); err != nil || c.NpcEventOpen != spec {
		t.Fatalf("环境变量应原样带出: %q %v", c.NpcEventOpen, err)
	}
	t.Setenv(EnvNpcEventOpen, "")
	if c, err := Load(writeEnv(t, "STONEAGE_NPC_EVENT_OPEN=all\n")); err != nil || c.NpcEventOpen != "all" {
		t.Fatalf(".env 文件里的值应生效: %+v %v", c, err)
	}
}

// STAGE-287：聊天屏蔽词表文件路径，没配是空串（只用内置词表）。
func TestChatFilterFileConfig(t *testing.T) {
	t.Setenv(EnvChatFilterFile, "")
	if c, err := Load(writeEnv(t, "")); err != nil || c.ChatFilterFile != "" {
		t.Fatalf("没配应是空串: %q %v", c.ChatFilterFile, err)
	}
	if c, err := Load(writeEnv(t, "STONEAGE_CHAT_FILTER_FILE=/data/words.txt\n")); err != nil || c.ChatFilterFile != "/data/words.txt" {
		t.Fatalf(".env 里的路径应带出: %q %v", c.ChatFilterFile, err)
	}
	t.Setenv(EnvChatFilterFile, "/etc/words.txt")
	if c, err := Load(writeEnv(t, "STONEAGE_CHAT_FILTER_FILE=/data/words.txt\n")); err != nil || c.ChatFilterFile != "/etc/words.txt" {
		t.Fatalf("环境变量应优先: %q %v", c.ChatFilterFile, err)
	}
}

// STAGE-297：精工覆盖表路径（STONEAGE_INLAY_OVERLAY_FILE），规则同屏蔽词表：环境变量优先，没配是空串。
func TestInlayOverlayFileConfig(t *testing.T) {
	t.Setenv(EnvInlayOverlayFile, "")
	if c, err := Load(writeEnv(t, "")); err != nil || c.InlayOverlayFile != "" {
		t.Fatalf("没配应是空串: %q %v", c.InlayOverlayFile, err)
	}
	if c, err := Load(writeEnv(t, "STONEAGE_INLAY_OVERLAY_FILE=/data/inlay.tsv\n")); err != nil || c.InlayOverlayFile != "/data/inlay.tsv" {
		t.Fatalf(".env 里的路径应带出: %q %v", c.InlayOverlayFile, err)
	}
}

// STAGE-351：按连接限速的三项配置。模式只能是 off/log/enforce，速率和容量必须是正数；不写取 log、20、40。
func TestRateLimitConfig(t *testing.T) {
	for _, env := range []string{EnvRateLimit, EnvRateLimitRate, EnvRateLimitBurst} {
		t.Setenv(env, "")
	}
	c, err := Load(writeEnv(t, ""))
	if err != nil || c.RateLimitMode != "log" || c.RateLimitRate != 20 || c.RateLimitBurst != 40 {
		t.Fatalf("不写时应是 log、20、40: %+v %v", c, err)
	}
	c, err = Load(writeEnv(t, "STONEAGE_RATE_LIMIT=enforce\nSTONEAGE_RATE_LIMIT_RATE=5.5\nSTONEAGE_RATE_LIMIT_BURST=10\n"))
	if err != nil || c.RateLimitMode != "enforce" || c.RateLimitRate != 5.5 || c.RateLimitBurst != 10 {
		t.Fatalf(".env 文件配置: %+v %v", c, err)
	}
	t.Setenv(EnvRateLimit, "off")
	if c, err = Load(writeEnv(t, "STONEAGE_RATE_LIMIT=enforce\n")); err != nil || c.RateLimitMode != "off" {
		t.Fatalf("环境变量应覆盖 .env: %+v %v", c, err)
	}
	for _, bad := range []string{"ENFORCE", "on", "1"} {
		t.Setenv(EnvRateLimit, bad)
		if _, err := Load(writeEnv(t, "")); err == nil {
			t.Errorf("模式 %q 应当报错", bad)
		}
	}
	t.Setenv(EnvRateLimit, "")
	for _, bad := range []string{"0", "-1", "abc", "NaN", "Inf"} {
		t.Setenv(EnvRateLimitRate, bad)
		if _, err := Load(writeEnv(t, "")); err == nil {
			t.Errorf("速率 %q 应当报错", bad)
		}
	}
	t.Setenv(EnvRateLimitRate, "")
	for _, bad := range []string{"0", "-3", "2.5", "abc"} {
		t.Setenv(EnvRateLimitBurst, bad)
		if _, err := Load(writeEnv(t, "")); err == nil {
			t.Errorf("容量 %q 应当报错", bad)
		}
	}
}
