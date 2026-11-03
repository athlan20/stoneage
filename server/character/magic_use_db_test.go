package character

import (
	"testing"
	"time"
)

// STAGE-203：变身只在内存里。离开世界时清掉（定时器一起停），再进入时 status 里没有 metamo。
// 需要数据库（storetest），不设测试数据库时跳过。
func TestMetamoEndsWhenLeavingWorld(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	l := e.chars.LiveOf(e.session("alice"))
	if err := l.Operate(func(r *record) { l.p.startMetamo(r, "乌力", 100250, time.Now(), time.Hour) }); err != nil {
		t.Fatal(err)
	}
	st := c.ok("status", `{}`)["status"].(map[string]any)
	if _, ok := st["metamo"]; !ok {
		t.Fatal("变身中 status 应有 metamo")
	}
	c.ok("leave_world", `{}`)
	l.p.mu.Lock()
	left := l.p.rec.metamo == nil
	l.p.mu.Unlock()
	if !left {
		t.Fatal("离开世界后变身应该清掉")
	}
	st = c.ok("enter_world", `{"slot":0}`)["status"].(map[string]any)
	if _, ok := st["metamo"]; ok {
		t.Errorf("重新进入后不应有 metamo：%v", st["metamo"])
	}
}
