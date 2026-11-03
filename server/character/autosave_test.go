package character

// STAGE-290：定时存盘与战后存盘。需要 STONEAGE_TEST_DATABASE_URL。

import (
	"context"
	"testing"
)

// 在线角色的等级、经验、金币改了以后，定时存盘一轮就落库，不用等离开世界（服务端被强杀也不回档）。
func TestAutosaveWritesOnlineCharacter(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	p := e.onlinePresence(t, "勇者小明")
	p.mu.Lock()
	p.rec.Level, p.rec.Exp, p.rec.Gold = 12, 345, 6789
	p.mu.Unlock()

	e.chars.autosaveAll()

	var level int
	var exp, gold int64
	err := e.chars.db.QueryRow(context.Background(),
		`SELECT level, exp, gold FROM characters WHERE name = $1`, "勇者小明").Scan(&level, &exp, &gold)
	if err != nil {
		t.Fatal(err)
	}
	if level != 12 || exp != 345 || gold != 6789 {
		t.Fatalf("定时存盘后库里应是 等级12 经验345 金币6789，实际 %d/%d/%d", level, exp, gold)
	}
}

// 已经在离开的角色不再被定时存盘写库（离开的存盘是最终值，旧值不能盖上去）。
func TestAutosaveSkipsLeavingCharacter(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	p := e.onlinePresence(t, "勇者小明")
	p.mu.Lock()
	p.rec.Level = 30
	p.leaving = true
	p.mu.Unlock()

	e.chars.autosaveAll()

	var level int
	if err := e.chars.db.QueryRow(context.Background(),
		`SELECT level FROM characters WHERE name = $1`, "勇者小明").Scan(&level); err != nil {
		t.Fatal(err)
	}
	if level == 30 {
		t.Fatalf("正在离开的角色不应被定时存盘写库，库里等级变成了 %d", level)
	}
	p.mu.Lock()
	p.leaving = false // 还原，让清理时的离开存盘正常走
	p.mu.Unlock()
}

// onlinePresence 取世界中某个角色的 Presence。
func (e *env) onlinePresence(t *testing.T, name string) *Presence {
	t.Helper()
	e.chars.mu.Lock()
	all := make([]*Presence, 0, len(e.chars.presences))
	for _, p := range e.chars.presences {
		all = append(all, p)
	}
	e.chars.mu.Unlock() // 先放开服务锁再取角色锁（锁序：Presence.mu 在前）
	for _, p := range all {
		p.mu.Lock()
		n := p.rec.Name
		p.mu.Unlock()
		if n == name {
			return p
		}
	}
	t.Fatalf("世界中没有角色 %s", name)
	return nil
}
