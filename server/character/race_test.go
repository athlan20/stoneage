package character

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-12：一个角色保存得慢（数据库行被锁住），只能让同账号的请求等，别的账号不受影响。
func TestSlowSaveDoesNotBlockOtherAccounts(t *testing.T) {
	e := newEnv(t)
	alice := e.loggedIn("alice")
	alice.ok("create_character", createJSON(0, "勇者甲", 0))
	alice.ok("enter_world", `{"slot":0}`)
	move(t, e.chars.LiveOf(e.session("alice")), 3006, 5, 6, 9) // 先于 agent 登录：此时 alice 只有玩家一条连接
	agent := e.agentFor(alice)
	bob := e.loggedIn("bob")
	bob.ok("create_character", createJSON(0, "勇者乙", 1))
	bob.ok("enter_world", `{"slot":0}`)

	// 另一个数据库会话锁住 alice 的角色行，模拟保存变慢。
	ctx := context.Background()
	tx, err := e.chars.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = tx.Rollback(ctx)
		}
	}
	defer release()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM characters WHERE name = '勇者甲' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}

	_ = alice.conn.Close() // 断线：保存卡在行锁上
	time.Sleep(200 * time.Millisecond)
	agent.send("enter_world", `{"slot":0}`) // alice 的 agent 进入：要等保存完成
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	if m := bob.call("status", `{}`); m["ok"] != true {
		t.Fatalf("bob 的 status 应成功: %v", m)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("alice 保存卡住期间，bob 的 status 用了 %v，不该被拖慢", d)
	}
	agent.noMessage() // agent 仍在等保存完成

	release()
	m := agent.recv()
	if m["ok"] != true {
		t.Fatalf("保存完成后 agent 应进入成功: %v", m)
	}
	if f, x, y := position(m["data"].(map[string]any)); f != 3006 || x != 5 || y != 6 {
		t.Errorf("agent 应读到保存后的位置，实际 %d(%d,%d)", f, x, y)
	}
}

// hookGate 让钩子停在某一步，等测试里的另一个请求做完再继续。
type hookGate struct{ reached, proceed chan struct{} }

func newGate() *hookGate {
	return &hookGate{reached: make(chan struct{}), proceed: make(chan struct{})}
}

func (g *hookGate) hook(*record) {
	g.reached <- struct{}{}
	<-g.proceed
}

func (g *hookGate) wait(t *testing.T) {
	t.Helper()
	select {
	case <-g.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("钩子没有被触发")
	}
}

// STAGE-16：删除角色的“检查是否在世界中”之后、真正删除之前，agent 来进入这个角色：
// 进入被拒绝，删除成功；不会出现角色被删了还有人在玩。
func TestEnterDuringDeleteIsRejected(t *testing.T) {
	g := newGate()
	e := newEnv(t, func(s *Service) { s.hooks.afterDeleteCheck = g.hook })
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者甲", 0))
	agent := e.agentFor(player)

	id := player.send("delete_character", `{"slot":0,"name":"勇者甲"}`)
	g.wait(t)
	if m := agent.call("enter_world", `{"slot":0}`); errCode(m) != protocol.CodeCharacterNotFound {
		t.Errorf("正在删除的角色不能进入，应回复 CHARACTER_NOT_FOUND: %v", m)
	}
	g.proceed <- struct{}{}
	if m := player.recv(); int(m["id"].(float64)) != id || m["ok"] != true {
		t.Fatalf("删除应成功: %v", m)
	}
	if len(e.chars.presences) != 0 {
		t.Errorf("世界里不该留下角色: %v", e.chars.presences)
	}
	if list := player.ok("characters", `{}`)["characters"].([]any); len(list) != 0 {
		t.Errorf("角色应已删除: %v", list)
	}
}

// STAGE-16：agent 已经读到角色、还没登记进世界时，玩家把角色删了：进入要失败，世界里不能留下已删除的角色。
func TestEnterAfterConcurrentDeleteIsUndone(t *testing.T) {
	g := newGate()
	e := newEnv(t, func(s *Service) { s.hooks.afterFind = g.hook })
	player := e.loggedIn("alice")
	player.ok("create_character", createJSON(0, "勇者甲", 0))
	agent := e.agentFor(player)

	agent.send("enter_world", `{"slot":0}`)
	g.wait(t) // agent 已读到角色，停在登记之前
	player.ok("delete_character", `{"slot":0,"name":"勇者甲"}`)
	g.proceed <- struct{}{}
	if m := agent.recv(); errCode(m) != protocol.CodeCharacterNotFound {
		t.Errorf("角色已被删除，agent 进入应回复 CHARACTER_NOT_FOUND: %v", m)
	}
	if e.chars.LiveOf(e.session("alice")) != nil || len(e.chars.presences) != 0 {
		t.Error("撤销进入之后世界里不该有角色")
	}
	// 连接状态没有变成“在世界中”：可以继续正常使用。
	if m := agent.call("characters", `{}`); m["ok"] != true {
		t.Errorf("进入失败后连接仍应可用: %v", m)
	}
}

// STAGE-16：保存时角色已不在数据库里（更新 0 行）要报错，不能静默丢掉；离开世界不受影响。
func TestSaveOfDeletedCharacterIsReported(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者甲", 0))
	c.ok("enter_world", `{"slot":0}`)
	var rec record
	if err := e.chars.LiveOf(e.session("alice")).With(func(r *record) { rec = *r }); err != nil {
		t.Fatal(err)
	}
	if _, err := e.chars.db.Exec(context.Background(), `DELETE FROM characters WHERE id = $1`, rec.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.chars.save(context.Background(), &rec, time.Now()); !errors.Is(err, errCharacterGone) {
		t.Errorf("保存已删除的角色应返回 errCharacterGone，实际 %v", err)
	}
	if m := c.call("leave_world", `{}`); m["ok"] != true {
		t.Errorf("角色已被删除时仍应能离开世界: %v", m)
	}
}

// STAGE-13：重复登录挤掉旧连接时，旧连接要先保存；保存卡住期间，别的账号的登录不受影响。
func TestSlowKickDoesNotBlockOtherLogins(t *testing.T) {
	e := newEnv(t)
	alice := e.loggedIn("alice")
	alice.ok("create_character", createJSON(0, "勇者甲", 0))
	alice.ok("enter_world", `{"slot":0}`)
	move(t, e.chars.LiveOf(e.session("alice")), 3006, 5, 6, 9)

	ctx := context.Background()
	tx, err := e.chars.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = tx.Rollback(ctx)
		}
	}
	defer release()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM characters WHERE name = '勇者甲' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}

	second := e.dial("player") // alice 在别处重新登录：旧连接保存卡在行锁上
	second.send("login", `{"account":"alice","password":"secret1"}`)
	time.Sleep(500 * time.Millisecond)

	start := time.Now()
	carol := e.loggedIn("carol")
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("alice 的旧连接保存卡住期间，carol 的注册加登录用了 %v，不该被拖慢", d)
	}
	carol.ok("create_character", createJSON(0, "勇者丙", 1))

	release()
	if m := second.recv(); m["ok"] != true {
		t.Fatalf("保存完成后 alice 的新连接应登录成功: %v", m)
	}
	alice.disconnectReason(protocol.ReasonKickedDuplicateLogin)
	m := second.ok("enter_world", `{"slot":0}`)
	if f, x, y := position(m); f != 3006 || x != 5 || y != 6 {
		t.Errorf("新连接应读到旧连接保存的位置，实际 %d(%d,%d)", f, x, y)
	}
}
