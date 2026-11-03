// 全服设置（STAGE-59）的单元测试：值的校验、默认 1 倍、写库通知后重读、坏值兜底。

package character

import (
	"context"
	"testing"
	"time"
)

func TestParseSettingValue(t *testing.T) {
	ok := map[string]float64{"2": 2, "1.5": 1.5, "0.1": 0.1, "100": 100, "1.05": 1.05}
	for in, want := range ok {
		got, err := ParseSettingValue("exp_rate", in)
		if err != nil || got != want {
			t.Errorf("ParseSettingValue(%q) = %g, %v，应 %g", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "-1", "0.01", "100.01", "101", "abc", "1.", ".5", "1.555", "1e2", "", " 2", "+1"} {
		if got, err := ParseSettingValue("exp_rate", in); err == nil {
			t.Errorf("ParseSettingValue(%q) = %g，应拒绝", in, got)
		}
	}
}

// 没启动监听（Start 之前）或没读过表时按 1 倍。
func TestExpRateBeforeStart(t *testing.T) {
	if got := New(nil, nil, Options{}).ExpRate(); got != 1 {
		t.Fatalf("未加载时的经验倍数 = %g，应 1", got)
	}
}

// 写库并 NOTIFY 后，监听中的服务端重读；非法值和未知设置名被拒绝、数据库不变。
func TestGameSettingsNotify(t *testing.T) {
	e := newPointsEnv(t)
	ctx := context.Background()
	if got := e.chars.ExpRate(); got != 1 {
		t.Fatalf("初始经验倍数 = %g，应 1", got)
	}
	if _, err := e.chars.SetGameSetting(ctx, "exp_rate", "2", "test"); err != nil {
		t.Fatal(err)
	}
	if got := e.chars.ExpRate(); got != 2 {
		t.Fatalf("改完后的经验倍数 = %g，应 2", got)
	}
	// 非法值被拒绝，数据库里的值不变。
	if _, err := e.chars.SetGameSetting(ctx, "exp_rate", "0", "test"); err == nil {
		t.Fatal("0 应被拒绝")
	}
	list, err := ListGameSettings(ctx, e.chars.db)
	if err != nil || len(list) != 1 || list[0].Key != "exp_rate" || list[0].Value != 2 || list[0].UpdatedBy != "test" {
		t.Fatalf("改完的设置 = %+v, %v", list, err)
	}
	// 未知设置名拒绝。
	if err := WriteGameSetting(ctx, e.chars.db, DefaultSettingsNotifyChannel, "gold_rate", 2, "test"); err == nil {
		t.Fatal("未知设置名应被拒绝")
	}
	// 绕过校验直接写库 + NOTIFY，监听方也要重读（stoneage-admin set 的路径）。
	if err := WriteGameSetting(ctx, e.chars.db, DefaultSettingsNotifyChannel, "exp_rate", 1.5, "admin"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for e.chars.ExpRate() != 1.5 {
		if time.Now().After(deadline) {
			t.Fatalf("通知后经验倍数 = %g，应 1.5", e.chars.ExpRate())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 数据库里的值坏了（超出范围、行没了）：重读时按 1 倍兜底。
	if _, err := e.chars.db.Exec(ctx, `UPDATE game_settings SET value = 999 WHERE key = 'exp_rate'`); err != nil {
		t.Fatal(err)
	}
	if err := e.chars.ReloadGameSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.chars.ExpRate(); got != 1 {
		t.Fatalf("坏值的经验倍数 = %g，应按 1 倍兜底", got)
	}
	if _, err := e.chars.db.Exec(ctx, `DELETE FROM game_settings`); err != nil {
		t.Fatal(err)
	}
	if err := e.chars.ReloadGameSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.chars.ExpRate(); got != 1 {
		t.Fatalf("缺行的经验倍数 = %g，应按 1 倍兜底", got)
	}
}
