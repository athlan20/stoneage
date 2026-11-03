package character

import (
	"context"
	"testing"
)

// queryGold 直接从数据库读石币（不经过内存）。
func (e *env) queryGold(name string) int64 {
	e.t.Helper()
	var gold int64
	if err := e.chars.db.QueryRow(context.Background(), `SELECT gold FROM characters WHERE name = $1`, name).Scan(&gold); err != nil {
		e.t.Fatal(err)
	}
	return gold
}

// setGold 直接把石币写成 gold（测试准备用）。
func (e *env) setGold(name string, gold int64) {
	e.t.Helper()
	if _, err := e.chars.db.Exec(context.Background(), `UPDATE characters SET gold = $2 WHERE name = $1`, name, gold); err != nil {
		e.t.Fatal(err)
	}
}

// STAGE-310：管理命令发石币写库并通知在线角色，内存里的石币加上实发数后推送 status_changed。
func TestGiveGoldReachesOnlinePlayer(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	e.setGold("勇者小明", 0) // 新角色带的石币不是 0，先清零再进世界，免得内存和库不一致
	c.ok("enter_world", `{"slot":0}`)
	before := e.queryGold("勇者小明")
	res, err := GiveGold(context.Background(), e.chars.db, DefaultGoldNotifyChannel, "勇者小明", 500000)
	if err != nil || res.Granted != 500000 || res.Capped() {
		t.Fatalf("应全部发下：%+v %v", res, err)
	}
	if got := statusOf(t, c.recv())["gold"]; int64(got.(float64)) != before+500000 {
		t.Fatalf("在线角色应看到石币 %d，实际 %v", before+500000, got)
	}
	if got := e.queryGold("勇者小明"); got != before+500000 {
		t.Fatalf("数据库里的石币应是 %d，实际 %d", before+500000, got)
	}
}

// 统一 1000 万的上限：0 转角色也能发到 1000 万；超过的部分不发，已在上限的角色实发 0。
func TestGiveGoldCapsAtMaxHave(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	e.setGold("勇者小明", 0)
	c.ok("enter_world", `{"slot":0}`)
	res, err := GiveGold(context.Background(), e.chars.db, DefaultGoldNotifyChannel, "勇者小明", 10000000)
	if err != nil || res.Granted != 10000000 || res.Capped() || res.Cap != 10000000 {
		t.Fatalf("0 转角色应能发到 1000 万：%+v %v", res, err)
	}
	if got := e.queryGold("勇者小明"); got != 10000000 {
		t.Fatalf("数据库里的石币应是 10000000，实际 %d", got)
	}
	res, err = GiveGold(context.Background(), e.chars.db, DefaultGoldNotifyChannel, "勇者小明", 20000000)
	if err != nil || res.Granted != 0 || !res.Capped() {
		t.Fatalf("已在上限的角色实发应为 0：%+v %v", res, err)
	}
}
