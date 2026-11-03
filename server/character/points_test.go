package character

import (
	"context"
	"strings"
	"testing"
)

// newPointsEnv 同 newEnv，另外启动角色服务的数据库通知监听：管理命令发点数要靠它推给在线的角色
// （newEnv 只启动账号服务，正式服务和用例回放由 app.Services.Start 启动）。
func newPointsEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	if err := e.chars.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.chars.Stop)
	return e
}

// queryPoints 直接从数据库读四项能力和未分配点数（不经过内存）。
func (e *env) queryPoints(name string) (vital, strength, toughness, dexterity, points int) {
	e.t.Helper()
	err := e.chars.db.QueryRow(context.Background(),
		`SELECT vital, strength, toughness, dexterity, skill_points FROM characters WHERE name = $1`, name).
		Scan(&vital, &strength, &toughness, &dexterity, &points)
	if err != nil {
		e.t.Fatal(err)
	}
	return
}

// givePoints 走管理命令的同一条路径发未分配点数（写库并 NOTIFY）。
func (e *env) givePoints(name string, points int) {
	e.t.Helper()
	if err := GivePoints(context.Background(), e.chars.db, DefaultPointNotifyChannel, name, points); err != nil {
		e.t.Fatal(err)
	}
}

// statusOf 取 status_changed 推送或 status 响应里的 status 对象。
func statusOf(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	if d, ok := m["data"].(map[string]any); ok {
		if st, ok := d["status"].(map[string]any); ok {
			return st
		}
	}
	t.Fatalf("消息里没有 status：%v", m)
	return nil
}

func intOf(m map[string]any, key string) int {
	return int(m[key].(float64))
}

func statsOf(st map[string]any) map[string]any {
	return st["stats"].(map[string]any)
}

// 管理命令发的点数，在线的角色立刻收到 status_changed；数据库里也是新的值。
func TestGivePointsReachesOnlinePlayer(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 3)
	if st := statusOf(t, c.recv()); intOf(st, "skill_points") != 3 {
		t.Fatalf("在线的角色应收到未分配点数 3，实际 %v", st["skill_points"])
	}
	if _, _, _, _, p := e.queryPoints("勇者小明"); p != 3 {
		t.Fatalf("数据库里的未分配点数应是 3，实际 %d", p)
	}
}

// 离线时发的点数只写库，下次进入世界时可见。
func TestGivePointsOfflineShowsOnEnter(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	e.givePoints("勇者小明", 4)
	st := c.ok("enter_world", `{"slot":0}`)["status"].(map[string]any)
	if intOf(st, "skill_points") != 4 {
		t.Errorf("离线发的点数进入世界后应可见，实际 %v", st["skill_points"])
	}
}

// 加点：扣未分配点数，能力 +点数，最大 HP 按体力 ×4 重算；当前 HP 不变；推送先于响应。
func TestAddPointSpendsPointsAndRaisesMaxHP(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 3)
	c.recv() // 管理命令发的点数

	resp, pushes := c.callp("addpoint", `{"stat":"vital","points":2}`)
	if resp["ok"] != true {
		t.Fatalf("加点应成功：%v", resp)
	}
	if d, _ := resp["data"].(map[string]any); len(d) != 0 {
		t.Errorf("响应的 data 应为空，实际 %v", resp["data"])
	}
	wantPushes(t, pushes, "status_changed")
	st := statusOf(t, pushes[0])
	if intOf(st, "skill_points") != 1 || intOf(st, "hp") != 44 || intOf(st, "max_hp") != 52 {
		t.Errorf("体力 +2 后应剩 1 点、血量 44/52，实际 %v", st)
	}
	if intOf(statsOf(st), "vital") != 10 {
		t.Errorf("体力应为 10，实际 %v", statsOf(st))
	}
	// 攻击 = 腕力 + (耐久×10 + 体力×10 + 速度×5)/100 = 4 + 1
	if d := st["derived"].(map[string]any); intOf(d, "attack") != 5 || intOf(d, "defense") != 5 || intOf(d, "agility") != 4 {
		t.Errorf("衍生数值不对：%v", d)
	}
	if v, s, tg, dx, p := e.queryPoints("勇者小明"); v != 10 || s != 4 || tg != 4 || dx != 4 || p != 1 {
		t.Errorf("数据库里应是 10/4/4/4、剩 1 点，实际 %d/%d/%d/%d 点数 %d", v, s, tg, dx, p)
	}
}

// 点数不够、能力名不对、点数不是正整数：整条命令失败，能力和点数都不变，也没有推送。
func TestAddPointRejectsWithoutChange(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 1)
	c.recv()

	cases := []struct{ data, code string }{
		{`{"stat":"strength","points":5}`, "NOT_ENOUGH_POINTS"},
		{`{"stat":"luck","points":1}`, "BAD_REQUEST"},
		{`{"stat":"力量","points":1}`, "BAD_REQUEST"},
		{`{"points":1}`, "BAD_REQUEST"},
		{`{"stat":"vital","points":0}`, "BAD_REQUEST"},
		{`{"stat":"vital","points":-1}`, "BAD_REQUEST"},
		{`{"stat":"vital","points":"1"}`, "BAD_REQUEST"},
		{`{"stat":"vital","points":1.5}`, "BAD_REQUEST"},
	}
	for _, tc := range cases {
		if got := errCode(c.call("addpoint", tc.data)); got != tc.code {
			t.Errorf("addpoint %s：期望 %s，实际 %q", tc.data, tc.code, got)
		}
	}
	c.noMessage()
	st := statusOf(t, c.call("status", `{}`))
	if intOf(st, "skill_points") != 1 || intOf(statsOf(st), "strength") != 4 || intOf(st, "max_hp") != 44 {
		t.Errorf("失败的加点不应改变任何数值，实际 %v", st)
	}
}

// 省略点数时加 1 点。
func TestAddPointOmittedPointsIsOne(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 1)
	c.recv()
	_, pushes := c.callp("addpoint", `{"stat":"dexterity"}`)
	st := statusOf(t, pushes[0])
	if intOf(statsOf(st), "dexterity") != 5 || intOf(st, "skill_points") != 0 || intOf(st, "max_hp") != 45 {
		t.Errorf("省略点数应加 1 点到速度，实际 %v", st)
	}
}

// 死亡（血量为 0）的角色不能加点。
func TestAddPointRefusesDeadCharacter(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 1)
	c.recv()
	move(t, e.chars.LiveOf(e.session("alice")), 1006, 15, 22, 0)
	if got := errCode(c.call("addpoint", `{"stat":"vital"}`)); got != "CHARACTER_DEAD" {
		t.Fatalf("死亡的角色应回复 CHARACTER_DEAD，实际 %q", got)
	}
}

// 加点的结果离开世界再进入后还在（写库，不等离开时保存）。
func TestAddPointPersistsAcrossReenter(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 2)
	c.recv()
	c.okp("addpoint", `{"stat":"vital","points":1}`)
	c.okp("addpoint", `{"stat":"strength","points":1}`)
	c.ok("leave_world", `{}`)
	if v, s, tg, dx, p := e.queryPoints("勇者小明"); v != 9 || s != 5 || tg != 4 || dx != 4 || p != 0 {
		t.Fatalf("离开后数据库应是 9/5/4/4、剩 0 点，实际 %d/%d/%d/%d 点数 %d", v, s, tg, dx, p)
	}
	st := c.ok("enter_world", `{"slot":0}`)["status"].(map[string]any)
	if intOf(statsOf(st), "vital") != 9 || intOf(st, "max_hp") != 49 || intOf(st, "skill_points") != 0 {
		t.Errorf("再次进入应看到加点的结果，实际 %v", st)
	}
}

// 能力的上限是 smallint（32767）：到顶了再加回复 BAD_REQUEST，正好到顶则成功。
func TestAddPointStatLimit(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 40000)
	c.recv()
	if got := errCode(c.call("addpoint", `{"stat":"vital","points":32760}`)); got != "BAD_REQUEST" {
		t.Fatalf("超过上限应回复 BAD_REQUEST，实际 %q", got)
	}
	c.okp("addpoint", `{"stat":"vital","points":32759}`)
	if v, _, _, _, _ := e.queryPoints("勇者小明"); v != 32767 {
		t.Fatalf("体力应正好到顶 32767，实际 %d", v)
	}
}

// 管理命令的参数检查：角色不存在、点数不是正数。
func TestGivePointsRejectsBadInput(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	err := GivePoints(context.Background(), e.chars.db, DefaultPointNotifyChannel, "不存在", 1)
	if err == nil || !strings.Contains(err.Error(), "找不到角色") {
		t.Errorf("角色不存在时应报错，实际 %v", err)
	}
	err = GivePoints(context.Background(), e.chars.db, DefaultPointNotifyChannel, "勇者小明", 0)
	if err == nil || !strings.Contains(err.Error(), "至少是 1") {
		t.Errorf("点数为 0 时应报错，实际 %v", err)
	}
}
