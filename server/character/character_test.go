package character

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/auth"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/store/storetest"
)

var fastHash = auth.HashParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}

type env struct {
	t     *testing.T
	srv   *gateway.Server
	chars *Service
	acct  *auth.Service
	addr  string
	ips   atomic.Int32
}

type client struct {
	t       *testing.T
	conn    net.Conn
	r       *bufio.Reader
	next    int
	pending []map[string]any // 先读到但还没被消费的消息（battle_test 的 roundtrip 在等响应时存下先到的推送）
	agent   []map[string]any // agent 面板的推送（STAGE-119），recvNet 读到后先放这里，见 agentPushes
}

// newEnv 起一个装好账号和角色服务的服务端（独立的数据库 schema）。
func newEnv(t *testing.T, setup ...func(*Service)) *env {
	t.Helper()
	st := storetest.New(t)
	srv := gateway.New(gateway.Options{RegisterEnabled: true})
	chars := New(st.Pool, srv.Hub(), Options{})
	for _, f := range setup { // 测试钩子要在服务开始处理连接之前装好
		f(chars)
	}
	acct, err := auth.New(st.Pool, srv.Hub(), auth.Options{RegisterEnabled: true, Hash: fastHash, BeforeKick: chars.BeforeKick,
		NotifyChannel: fmt.Sprintf("account_disabled_%d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	acct.Install(srv)
	chars.Install(srv)
	if err := acct.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(acct.Stop)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return &env{t: t, srv: srv, chars: chars, acct: acct, addr: ln.Addr().String()}
}

func (e *env) dial(role string) *client {
	e.t.Helper()
	c, err := net.Dial("tcp", e.addr)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { c.Close() })
	cl := &client{t: e.t, conn: c, r: bufio.NewReader(c), next: 1}
	cl.ok("hello", fmt.Sprintf(`{"protocol":1,"role":%q}`, role))
	return cl
}

func (c *client) send(typ, data string) int {
	c.t.Helper()
	id := c.next
	c.next++
	if _, err := fmt.Fprintf(c.conn, `{"id":%d,"type":%q,"data":%s}`+"\n", id, typ, data); err != nil {
		c.t.Fatal(err)
	}
	return id
}

// recvNet 从网络上读一条消息（5 秒读不到就失败）。agent 面板的推送（STAGE-119）与各测试的断言无关，
// 先放进 agent 队列，需要时用 agentPushes 取，不挡住其他推送的断言。
func (c *client) recvNet() map[string]any {
	c.t.Helper()
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			c.t.Fatalf("读取失败: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(line, &m)
		if isAgentPanelPush(m) {
			c.agent = append(c.agent, m)
			continue
		}
		return m
	}
}

// isAgentPanelPush 判断是不是 agent 面板的推送（STAGE-119）。
func isAgentPanelPush(m map[string]any) bool {
	switch m["type"] {
	case "agent_status", "agent_plan", "agent_activity":
		return true
	}
	return false
}

// agentPushes 取出并清空已收到的 agent 面板推送（agent_status、agent_plan、agent_activity）。
func (c *client) agentPushes() []map[string]any {
	out := c.agent
	c.agent = nil
	return out
}

// recv 先消费待读队列（roundtrip 存下的先到推送），空了再从网络读。
func (c *client) recv() map[string]any {
	c.t.Helper()
	if len(c.pending) > 0 {
		m := c.pending[0]
		c.pending = c.pending[1:]
		return m
	}
	return c.recvNet()
}

func (c *client) call(typ, data string) map[string]any {
	c.t.Helper()
	c.send(typ, data)
	return c.recv()
}

func (c *client) ok(typ, data string) map[string]any {
	c.t.Helper()
	m := c.call(typ, data)
	if m["ok"] != true {
		c.t.Fatalf("%s 应成功: %v", typ, m)
	}
	return m["data"].(map[string]any)
}

func errCode(m map[string]any) string {
	if e, ok := m["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

func (c *client) closed() bool {
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err := c.r.ReadBytes('\n')
	return err != nil
}

func createJSON(slot int, name string, hometown int) string {
	// JSON Lines：一条请求只能占一行。
	return fmt.Sprintf(`{"slot":%d,"name":%q,"appearance":{"body":3,"color":2,"eye":1,"mouth":4},`+
		`"stats":{"vital":8,"strength":4,"toughness":4,"dexterity":4},`+
		`"elements":{"earth":6,"water":4,"fire":0,"wind":0},"hometown":%d}`, slot, name, hometown)
}

// loggedIn 注册（已注册则跳过）并登录，返回已登录的玩家连接。
func (e *env) loggedIn(account string) *client {
	e.t.Helper()
	// 每个账号用不同的来源 IP 注册，避开“每个 IP 每小时注册 50 次”的限制。
	ip := fmt.Sprintf("9.9.9.%d", e.ips.Add(1))
	if err := e.acct.Register(context.Background(), ip, account, "secret1"); err != nil {
		if pe, ok := err.(*protocol.Error); !ok || pe.Code != auth.CodeAccountExists {
			e.t.Fatal(err)
		}
	}
	c := e.dial("player")
	c.ok("login", fmt.Sprintf(`{"account":%q,"password":"secret1"}`, account))
	return c
}

// agentFor 让玩家连接生成一个令牌，并返回用它登录好的 agent 连接。
func (e *env) agentFor(player *client) *client {
	e.t.Helper()
	tok := player.ok("create_agent_token", `{"label":"测试"}`)["token"].(string)
	agent := e.dial("agent")
	agent.ok("agent_login", fmt.Sprintf(`{"token":%q}`, tok))
	return agent
}

func (e *env) session(account string) *gateway.Session {
	e.t.Helper()
	for _, s := range e.srv.Hub().ByAccount(account) {
		return s
	}
	e.t.Fatalf("没有 %s 的连接", account)
	return nil
}

// queryPos 直接从数据库读角色的位置和血量。
func (e *env) queryPos(name string) (floor, x, y, hp int) {
	e.t.Helper()
	err := e.chars.db.QueryRow(context.Background(), `SELECT floor, x, y, hp FROM characters WHERE name = $1`, name).Scan(&floor, &x, &y, &hp)
	if err != nil {
		e.t.Fatal(err)
	}
	return
}

func move(t *testing.T, l *Live, floor, x, y, hp int) {
	t.Helper()
	if err := l.With(func(r *record) { r.Floor, r.X, r.Y, r.HP = floor, x, y, hp }); err != nil {
		t.Fatal(err)
	}
}

func position(m map[string]any) (floor, x, y int) {
	p := m["position"].(map[string]any)
	return int(p["floor"].(float64)), int(p["x"].(float64)), int(p["y"].(float64))
}

// 主动离开世界：位置和血量保存，再次进入保持不变。
func TestLeaveSavesAndReenterKeepsPosition(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	move(t, e.chars.LiveOf(e.session("alice")), 2006, 33, 44, 17)
	c.ok("leave_world", `{}`)
	if f, x, y, hp := e.queryPos("勇者小明"); f != 2006 || x != 33 || y != 44 || hp != 17 {
		t.Fatalf("数据库里应是离开时的位置，实际 %d(%d,%d) hp=%d", f, x, y, hp)
	}
	m := c.ok("enter_world", `{"slot":0}`)
	if f, x, y := position(m); f != 2006 || x != 33 || y != 44 {
		t.Errorf("再次进入应在原位置，实际 %d(%d,%d)", f, x, y)
	}
	if hp := int(m["status"].(map[string]any)["hp"].(float64)); hp != 17 {
		t.Errorf("血量也应保留，实际 %d", hp)
	}
}

// 直接断开连接：同样保存；马上重连登录进入，读到的是保存后的位置，不是旧位置。
func TestDisconnectSavesAndImmediateReloginSeesIt(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	move(t, e.chars.LiveOf(e.session("alice")), 3006, 5, 6, 9)
	_ = c.conn.Close() // 不发 leave_world，直接断开

	c2 := e.loggedIn("alice")
	m := c2.ok("enter_world", `{"slot":0}`)
	if f, x, y := position(m); f != 3006 || x != 5 || y != 6 {
		t.Errorf("断开前的位置应被保存，实际 %d(%d,%d)", f, x, y)
	}
}

// 同账号同身份重复登录：旧连接先保存再被踢，新连接马上进入就能读到。
func TestDuplicateLoginSavesBeforeKick(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	old := e.chars.LiveOf(e.session("alice"))
	move(t, old, 4006, 7, 8, 5)

	c2 := e.loggedIn("alice")
	if m := c.recv(); m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonKickedDuplicateLogin {
		t.Fatalf("旧连接应被挤下线: %v", m)
	}
	m := c2.ok("enter_world", `{"slot":0}`)
	if f, x, y := position(m); f != 4006 || x != 7 || y != 8 {
		t.Errorf("新连接应读到旧连接保存的位置，实际 %d(%d,%d)", f, x, y)
	}
	// 旧连接已读入但还没处理的请求不能再改动角色。
	if err := old.With(func(r *record) { r.X = 99 }); err != ErrLeft {
		t.Errorf("旧连接的角色应已离开，With 返回 %v", err)
	}
}

// 账号被禁用：在世界中的连接先保存再断开。
func TestDisableSavesBeforeDisconnect(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	move(t, e.chars.LiveOf(e.session("alice")), 2006, 11, 12, 3)
	if err := e.acct.SetDisabled(context.Background(), "alice", true, "测试"); err != nil {
		t.Fatal(err)
	}
	if m := c.recv(); m["type"] != "disconnect" || m["data"].(map[string]any)["reason"] != protocol.ReasonAccountDisabled {
		t.Fatalf("应收到 ACCOUNT_DISABLED: %v", m)
	}
	if !c.closed() {
		t.Fatal("连接应被断开")
	}
	if f, x, y, hp := e.queryPos("勇者小明"); f != 2006 || x != 11 || y != 12 || hp != 3 {
		t.Errorf("断开前应已保存，实际 %d(%d,%d) hp=%d", f, x, y, hp)
	}
}

// 关服：所有在世界中的角色都被保存，Shutdown 等保存完才返回。
func TestShutdownSavesWorld(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	move(t, e.chars.LiveOf(e.session("alice")), 3006, 1, 2, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if f, x, y, hp := e.queryPos("勇者小明"); f != 3006 || x != 1 || y != 2 || hp != 4 {
		t.Errorf("关服后应已保存，实际 %d(%d,%d) hp=%d", f, x, y, hp)
	}
}

// 保存和请求处理并发：保存之后的修改一律被丢弃，数据库里的值就是保存那一刻的值。
func TestSaveAndRequestsAreSerialized(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	sess := e.session("alice")
	live := e.chars.LiveOf(sess)

	var wg sync.WaitGroup
	var mu sync.Mutex
	lastApplied := 0
	stop := make(chan struct{})
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 1; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				err := live.With(func(r *record) {
					r.Gold++
					mu.Lock()
					lastApplied = int(r.Gold)
					mu.Unlock()
				})
				if err == ErrLeft {
					return
				}
			}
		}()
	}
	time.Sleep(30 * time.Millisecond)
	e.chars.BeforeKick(sess) // 在别的 goroutine 里保存并让角色离开
	close(stop)
	wg.Wait()
	var gold int64
	if err := e.chars.db.QueryRow(context.Background(), `SELECT gold FROM characters WHERE name = '勇者小明'`).Scan(&gold); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gold != int64(lastApplied) {
		t.Errorf("数据库里的金钱 %d 应等于离开前最后一次修改 %d", gold, lastApplied)
	}
}

// 另一个角色已被同账号的 agent 连接带入世界：玩家进入另一个角色回复 CHARACTER_IN_USE，
// 进入同一个角色可以（以观察方）；该角色也不能被删除。
func TestCharacterInUse(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者甲", 0))
	c.ok("create_character", createJSON(1, "勇者乙", 1))
	agent := e.agentFor(c)
	agent.ok("enter_world", `{"slot":0}`)

	if m := c.call("enter_world", `{"slot":1}`); errCode(m) != protocol.CodeCharacterInUse {
		t.Errorf("进入另一个角色应回复 CHARACTER_IN_USE: %v", m)
	}
	if m := c.call("delete_character", `{"slot":0,"name":"勇者甲"}`); errCode(m) != protocol.CodeCharacterInUse {
		t.Errorf("正在游戏中的角色不能删除: %v", m)
	}
	m := c.ok("enter_world", `{"slot":0}`) // 同一个角色可以，agent 在操控，玩家是观察方
	if ctl := m["control"].(map[string]any); ctl["controller"] != "agent" || ctl["you"] != "observer" {
		t.Errorf("玩家应以观察方进入: %v", ctl)
	}
}

// 全服唯一：大小写、全角半角不同的同名也算重名；并发创建同名只有一个成功。
func TestNameUniqueness(t *testing.T) {
	e := newEnv(t)
	a, b := e.loggedIn("alice"), e.loggedIn("bob_x")
	a.ok("create_character", createJSON(0, "Hero", 0))
	for _, name := range []string{"hero", "HERO", "Ｈｅｒｏ", " Hero "} {
		if m := b.call("create_character", createJSON(0, name, 0)); errCode(m) != protocol.CodeNameTaken {
			t.Errorf("%q 应回复 NAME_TAKEN: %v", name, m)
		}
	}
	if m := b.call("characters", `{}`); len(m["data"].(map[string]any)["characters"].([]any)) != 0 {
		t.Errorf("重名失败后不应留下角色: %v", m)
	}

	var wg sync.WaitGroup
	results := make([]string, 6)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := e.loggedIn(fmt.Sprintf("racer%d", i))
			m := c.call("create_character", createJSON(0, "Racer", 0))
			if m["ok"] == true {
				results[i] = "ok"
			} else {
				results[i] = errCode(m)
			}
		}(i)
	}
	wg.Wait()
	okCount := 0
	for _, r := range results {
		switch r {
		case "ok":
			okCount++
		case protocol.CodeNameTaken:
		default:
			t.Errorf("意外的结果 %q", r)
		}
	}
	if okCount != 1 {
		t.Errorf("并发创建同名角色应只有 1 个成功，实际 %d：%v", okCount, results)
	}
}

// 删除角色：删除后栏位空出来，可以重新创建；别的账号的角色不受影响。
func TestDeleteFreesSlotAndOnlyOwnCharacters(t *testing.T) {
	e := newEnv(t)
	a, b := e.loggedIn("alice"), e.loggedIn("bob_x")
	a.ok("create_character", createJSON(0, "勇者甲", 0))
	b.ok("create_character", createJSON(0, "勇者乙", 0))
	if m := a.call("delete_character", `{"slot":0,"name":"勇者乙"}`); errCode(m) != protocol.CodeNameMismatch {
		t.Errorf("用别人的角色名确认应失败: %v", m)
	}
	a.ok("delete_character", `{"slot":0,"name":"勇者甲"}`)
	a.ok("create_character", createJSON(0, "勇者甲", 2)) // 同名同栏位可以重建
	if n := len(b.ok("characters", `{}`)["characters"].([]any)); n != 1 {
		t.Errorf("别的账号的角色应还在，实际 %d 个", n)
	}
}

// 数据库里能直接看到常用字段，数值按原版服务端公式计算。
func TestStoredFieldsAndInitialStatus(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 3))
	var floor, x, y, level, bodyImg, faceImg, hp, mp int
	var gold int64
	err := e.chars.db.QueryRow(context.Background(),
		`SELECT floor, x, y, level, body_image, face_image, hp, mp, gold FROM characters WHERE name = '勇者小明'`).
		Scan(&floor, &x, &y, &level, &bodyImg, &faceImg, &hp, &mp, &gold)
	if err != nil {
		t.Fatal(err)
	}
	if floor != 4006 || x != 14 || y != 20 || level != 1 || bodyImg != 100070 || faceImg != 30359 || hp != 44 || mp != 100 {
		t.Errorf("存储的字段不对：%d(%d,%d) lv%d body=%d face=%d hp=%d mp=%d", floor, x, y, level, bodyImg, faceImg, hp, mp)
	}
	st := c.ok("enter_world", `{"slot":0}`)["status"].(map[string]any)
	if st["hp"].(float64) != 44 || st["max_hp"].(float64) != 44 || st["next_exp"].(float64) != 2 || st["max_mp"].(float64) != 100 {
		t.Errorf("进入世界的状态不对：%v", st)
	}
}

// 密码、令牌等不会出现在角色相关的任何表里（角色表本来就没有这类列），这里只确认表结构里没有意外的明文列。
func TestCharactersTableHasNoSecretColumns(t *testing.T) {
	e := newEnv(t)
	rows, err := e.chars.db.Query(context.Background(),
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'characters' AND table_schema = current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var col string
		_ = rows.Scan(&col)
		n++
		if col == "password" || col == "token" {
			t.Errorf("角色表不应有 %s 列", col)
		}
	}
	if n < 20 {
		t.Errorf("角色表列数太少：%d", n)
	}
}
