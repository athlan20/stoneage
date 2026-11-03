package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/store/storetest"
)

// 测试用的低成本 Argon2 参数（校验函数允许的最低内存是 8 MiB）。
var fastHash = HashParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// testLimits 是测试用的限制：注册上限收紧到每小时 5 次，让限流用例不用造几十个账号（线上默认见 DefaultLimits）。
func testLimits() Limits {
	l := DefaultLimits
	l.RegisterMax = 5
	return l
}

func newService(t *testing.T) (*Service, *clock) {
	t.Helper()
	st := storetest.New(t)
	clk := &clock{t: time.Now()}
	svc, err := New(st.Pool, gateway.New(gateway.Options{}).Hub(), Options{RegisterEnabled: true, Hash: fastHash, Now: clk.now, Limits: testLimits()})
	if err != nil {
		t.Fatal(err)
	}
	return svc, clk
}

func code(err error) string {
	if pe, ok := err.(*protocol.Error); ok {
		return pe.Code
	}
	if err == nil {
		return ""
	}
	return "非协议错误: " + err.Error()
}

func retryAfter(err error) int {
	if pe, ok := err.(*protocol.Error); ok {
		if m, ok := pe.Data.(map[string]any); ok {
			if n, ok := m["retry_after_seconds"].(int); ok {
				return n
			}
		}
	}
	return -1
}

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("secret1", fastHash)
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("%q %v", h, err)
	}
	if strings.Contains(h, "secret1") {
		t.Error("哈希里不能出现明文")
	}
	if !Verify(h, "secret1") || Verify(h, "secret2") || Verify("garbage", "secret1") || Verify("$argon2id$v=19$m=1,t=1,p=1$AAAA$AAAA", "x") {
		t.Error("校验结果不对")
	}
	h2, _ := Hash("secret1", fastHash)
	if h == h2 {
		t.Error("相同密码的两次哈希应因盐不同而不同")
	}
}

func TestValidation(t *testing.T) {
	for _, ok := range []string{"abc", "alice", "a_1", "abcdefghijklmno"} {
		if !ValidAccount(ok) {
			t.Errorf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"", "ab", "1abc", "_abc", "ABC", "a-b", "a b", "abcdefghijklmnop", "账号账号账"} {
		if ValidAccount(bad) {
			t.Errorf("%q 应不合法", bad)
		}
	}
	for _, ok := range []string{"secret", "pass word", strings.Repeat("密", 64)} {
		if !ValidPassword(ok) {
			t.Errorf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"short", " secret", "secret ", strings.Repeat("x", 65)} {
		if ValidPassword(bad) {
			t.Errorf("%q 应不合法", bad)
		}
	}
	if got := auditName("A\x00B" + strings.Repeat("x", 100)); strings.ContainsRune(got, 0) || len([]rune(got)) > 64 {
		t.Errorf("auditName = %q", got)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	if err := svc.Register(ctx, "1.1.1.1", "Alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	if c := code(svc.Register(ctx, "1.1.1.1", "alice", "secret1")); c != CodeAccountExists {
		t.Errorf("重复注册 = %s", c)
	}
	if c := code(svc.Register(ctx, "1.1.1.1", "ALICE", "other-pw")); c != CodeAccountExists {
		t.Errorf("大小写不同也算重复 = %s", c)
	}
	if c := code(svc.Register(ctx, "1.1.1.1", "x", "secret1")); c != CodeInvalidAccount {
		t.Errorf("= %s", c)
	}
	if c := code(svc.Register(ctx, "1.1.1.1", "bobby", "123")); c != CodeInvalidPassword {
		t.Errorf("= %s", c)
	}
	id, name, err := svc.Login(ctx, "1.1.1.1", "ALICE", "secret1")
	if err != nil || name != "alice" || id == 0 {
		t.Fatalf("登录: %d %q %v", id, name, err)
	}
	// 密码错误和账号不存在：错误码和说明完全一致。
	_, _, e1 := svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1")
	_, _, e2 := svc.Login(ctx, "1.1.1.1", "nobody", "wrongpw1")
	if code(e1) != CodeWrongCredentials || e1.Error() != e2.Error() {
		t.Errorf("两种失败应一致: %v / %v", e1, e2)
	}
}

func TestRegisterDisabled(t *testing.T) {
	svc, _ := newService(t)
	svc.opts.RegisterEnabled = false
	if c := code(svc.Register(context.Background(), "1.1.1.1", "alice", "secret1")); c != CodeRegisterDisabled {
		t.Errorf("= %s", c)
	}
}

func TestRegisterRateLimit(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := svc.Register(ctx, "2.2.2.2", fmt.Sprintf("user%d", i), "secret1"); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Minute)
	}
	err := svc.Register(ctx, "2.2.2.2", "user9", "secret1")
	if code(err) != protocol.CodeRateLimited {
		t.Fatalf("第 6 次应被限制: %v", err)
	}
	// 最早一次在 5 分钟前，窗口 1 小时，所以还要等 55 分钟。
	if got := retryAfter(err); got != 55*60 {
		t.Errorf("retry_after_seconds = %d，期望 %d", got, 55*60)
	}
	if err := svc.Register(ctx, "3.3.3.3", "other1", "secret1"); err != nil {
		t.Errorf("其他 IP 不受影响: %v", err)
	}
	clk.advance(56 * time.Minute)
	if err := svc.Register(ctx, "2.2.2.2", "user9", "secret1"); err != nil {
		t.Errorf("窗口过去后应能再注册: %v", err)
	}
}

func TestLockAfterFiveFailures(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	if err := svc.Register(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1"); code(err) != CodeWrongCredentials {
			t.Fatalf("第 %d 次失败应是 WRONG_CREDENTIALS: %v", i+1, err)
		}
		clk.advance(10 * time.Second)
	}
	// 锁定期间，密码正确也不行，并告知剩余秒数。
	_, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1")
	if code(err) != CodeAccountLocked || retryAfter(err) < 15*60-60 || retryAfter(err) > 15*60 {
		t.Fatalf("应锁定约 15 分钟: %v (%d)", err, retryAfter(err))
	}
	clk.advance(14 * time.Minute)
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); code(err) != CodeAccountLocked {
		t.Fatalf("14 分钟后仍在锁定: %v", err)
	}
	clk.advance(2 * time.Minute)
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Fatalf("锁定到期后应能登录: %v", err)
	}
}

func TestLockCoversUnknownAccountsToo(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _, _ = svc.Login(ctx, "1.1.1.1", "ghost", "wrongpw1")
	}
	// 不存在的账号名同样被锁，外人无法借“是否被锁定”判断账号是否存在。
	if _, _, err := svc.Login(ctx, "1.1.1.1", "ghost", "wrongpw1"); code(err) != CodeAccountLocked {
		t.Errorf("= %v", err)
	}
}

func TestFailuresOutsideWindowDontCount(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	_ = svc.Register(ctx, "1.1.1.1", "alice", "secret1")
	for i := 0; i < 4; i++ {
		_, _, _ = svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1")
	}
	clk.advance(11 * time.Minute)
	_, _, _ = svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1") // 窗口里只剩这 1 次
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Errorf("窗口外的失败不应计入: %v", err)
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	_ = svc.Register(ctx, "1.1.1.1", "alice", "secret1")
	for round := 0; round < 3; round++ {
		for i := 0; i < 4; i++ {
			_, _, _ = svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1")
		}
		if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
			t.Fatalf("第 %d 轮：成功登录会清零失败次数，不应被锁: %v", round, err)
		}
	}
}

func TestIPRateLimit(t *testing.T) {
	svc, clk := newService(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		_, _, _ = svc.Login(ctx, "9.9.9.9", fmt.Sprintf("ghost%d", i), "wrongpw1")
		clk.advance(5 * time.Second)
	}
	_ = svc.Register(ctx, "1.1.1.1", "alice", "secret1")
	_, _, err := svc.Login(ctx, "9.9.9.9", "alice", "secret1")
	if code(err) != protocol.CodeRateLimited || retryAfter(err) < 1 {
		t.Fatalf("同一 IP 失败 20 次后应被限制: %v", err)
	}
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Errorf("其他 IP 不受影响: %v", err)
	}
	clk.advance(11 * time.Minute)
	if _, _, err := svc.Login(ctx, "9.9.9.9", "alice", "secret1"); err != nil {
		t.Errorf("窗口过去后应恢复: %v", err)
	}
}

func TestDisabledAccount(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	_ = svc.Register(ctx, "1.1.1.1", "alice", "secret1")
	if err := svc.SetDisabled(ctx, "alice", true, "测试"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); code(err) != CodeAccountDisabled {
		t.Errorf("= %v", err)
	}
	// 密码不对时不暴露账号被禁用。
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1"); code(err) != CodeWrongCredentials {
		t.Errorf("= %v", err)
	}
	if err := svc.SetDisabled(ctx, "alice", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Errorf("启用后应能登录: %v", err)
	}
	if err := svc.SetDisabled(ctx, "nobody", true, ""); err == nil {
		t.Error("禁用不存在的账号应报错")
	}
}

func TestUnlock(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	_ = svc.Register(ctx, "1.1.1.1", "alice", "secret1")
	for i := 0; i < 5; i++ {
		_, _, _ = svc.Login(ctx, "1.1.1.1", "alice", "wrongpw1")
	}
	if err := svc.RunAdminAction(ctx, "unlock_account", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "1.1.1.1", "alice", "secret1"); err != nil {
		t.Errorf("解锁后应能登录: %v", err)
	}
	list, err := svc.ListAccounts(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "alice" || list[0].LastLoginAt == nil {
		t.Errorf("ListAccounts = %+v %v", list, err)
	}
}

// 验收项：数据库里找不到明文密码；每次注册和登录都有审计记录。
func TestNoPlaintextAndAudit(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	const secret = "PLAINTEXT-pw-123"
	_ = svc.Register(ctx, "1.1.1.1", "alice", secret)
	_, _, _ = svc.Login(ctx, "1.1.1.1", "alice", "wrong-PLAINTEXT-9")
	_, _, _ = svc.Login(ctx, "1.1.1.1", "alice", secret)
	_ = svc.Register(ctx, "1.1.1.1", "alice", "another-PLAINTEXT-2")

	// 把所有表的所有行转成文本，整体搜索明文。
	var dump string
	err := svc.db.QueryRow(ctx, `SELECT
		(SELECT coalesce(string_agg(a::text, E'\n'), '') FROM accounts a) || E'\n' ||
		(SELECT coalesce(string_agg(a::text, E'\n'), '') FROM auth_audit a) || E'\n' ||
		(SELECT coalesce(string_agg(l::text, E'\n'), '') FROM login_locks l)`).Scan(&dump)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, "PLAINTEXT") {
		t.Errorf("数据库里出现了明文密码:\n%s", dump)
	}
	if !strings.Contains(dump, "$argon2id$") {
		t.Error("账号表里应保存 Argon2id 哈希")
	}
	rows, err := svc.db.Query(ctx, `SELECT kind, account, ip, success, result FROM auth_audit ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var kind, account, ip, result string
		var ok bool
		_ = rows.Scan(&kind, &account, &ip, &ok, &result)
		got = append(got, fmt.Sprintf("%s/%s/%s/%v/%s", kind, account, ip, ok, result))
	}
	want := []string{
		"register/alice/1.1.1.1/true/ok",
		"login/alice/1.1.1.1/false/WRONG_CREDENTIALS",
		"login/alice/1.1.1.1/true/ok",
		"register/alice/1.1.1.1/false/ACCOUNT_EXISTS",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("审计记录:\n%s\n期望:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestConcurrentRegisterSameName(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- code(svc.Register(ctx, fmt.Sprintf("10.0.0.%d", i), "alice", "secret1"))
		}()
	}
	wg.Wait()
	close(results)
	ok, exists := 0, 0
	for r := range results {
		switch r {
		case "":
			ok++
		case CodeAccountExists:
			exists++
		default:
			t.Errorf("意外结果 %q", r)
		}
	}
	if ok != 1 || exists != 7 {
		t.Errorf("并发注册同名账号：成功 %d，已存在 %d", ok, exists)
	}
}
