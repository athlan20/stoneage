package auth

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// run 同时发起 n 次调用，返回各错误码出现的次数（成功算 "ok"）。
func run(n int, f func(i int) error) map[string]int {
	var mu sync.Mutex
	counts := map[string]int{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			c := code(f(i))
			if c == "" {
				c = "ok"
			}
			mu.Lock()
			counts[c]++
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()
	return counts
}

// STAGE-7：同一账号同时发 60 次错误密码，最多 5 次真正校验（WRONG_CREDENTIALS），其余被锁定。
func TestConcurrentWrongPasswordsLockAccount(t *testing.T) {
	for name, ipOf := range map[string]func(int) string{
		"同一IP":   func(int) string { return "10.0.0.2" },
		"每次不同IP": func(i int) string { return fmt.Sprintf("10.1.0.%d", i) },
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := newService(t)
			ctx := context.Background()
			if err := svc.Register(ctx, "9.9.9.9", "victim", "secret1"); err != nil {
				t.Fatal(err)
			}
			counts := run(60, func(i int) error {
				_, _, err := svc.Login(ctx, ipOf(i), "victim", "wrongpw")
				return err
			})
			if counts[CodeWrongCredentials] > 5 || counts[CodeWrongCredentials]+counts[CodeAccountLocked] != 60 {
				t.Errorf("最多 5 次校验、其余应被锁定，实际 %v", counts)
			}
		})
	}
}

// STAGE-7：同一 IP 对不同账号同时发 60 次错误密码，最多 20 次真正校验，其余被限制。
func TestConcurrentWrongPasswordsLimitIP(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	counts := run(60, func(i int) error {
		_, _, err := svc.Login(ctx, "10.0.0.3", fmt.Sprintf("user%02d", i), "wrongpw")
		return err
	})
	if counts[CodeWrongCredentials] > 20 || counts[CodeWrongCredentials]+counts["RATE_LIMITED"] != 60 {
		t.Errorf("最多 20 次校验、其余应被限制，实际 %v", counts)
	}
}

// STAGE-7：同一 IP 同时发 20 次注册，最多成功 5 次。
func TestConcurrentRegisterLimitIP(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	counts := run(20, func(i int) error {
		return svc.Register(ctx, "10.0.0.4", fmt.Sprintf("user%02d", i), "secret1")
	})
	if counts["ok"] != 5 || counts["RATE_LIMITED"] != 15 {
		t.Errorf("应成功 5 次、其余被限制，实际 %v", counts)
	}
}

// 同一账号名同时登录正确密码不受影响：串行处理但都能成功。
func TestConcurrentCorrectLoginsAllSucceed(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	if err := svc.Register(ctx, "9.9.9.9", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	counts := run(10, func(i int) error {
		_, _, err := svc.Login(ctx, "10.0.0.5", "alice", "secret1")
		return err
	})
	if counts["ok"] != 10 {
		t.Errorf("%v", counts)
	}
}

// 等待排队时连接断开（ctx 取消）要立即放弃，不再校验密码。
func TestQueuedLoginGivesUpWhenCancelled(t *testing.T) {
	svc, _ := newService(t)
	bg := context.Background()
	if err := svc.Register(bg, "9.9.9.9", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	hold, err := svc.attempts.lockAll(bg, "login-ip:10.0.0.6", "login-account:alice") // 模拟前一个尝试还在处理
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	done := make(chan error, 1)
	go func() { _, _, err := svc.Login(ctx, "10.0.0.6", "alice", "wrongpw"); done <- err }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("应返回 context.Canceled，实际 %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后仍在等待")
	}
	hold()
	var n int
	if err := svc.db.QueryRow(bg, `SELECT count(*) FROM auth_audit WHERE kind = 'login'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("放弃的尝试不应写审计，实际 %d 条（%v）", n, err)
	}
}

// 校验完密码之后连接才断开，这次失败仍要记入统计（否则发完就断线可以白猜）。
func TestFailureRecordedEvenIfConnectionDropsAfterVerify(t *testing.T) {
	svc, _ := newService(t)
	bg := context.Background()
	if err := svc.Register(bg, "9.9.9.9", "alice", "secret1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	svc.opts.Hash = fastHash
	svc.afterVerify = cancel // 测试钩子：校验完立刻“断线”
	if _, _, err := svc.Login(ctx, "10.0.0.7", "alice", "wrongpw"); code(err) != CodeWrongCredentials {
		t.Fatalf("%v", err)
	}
	var n int
	if err := svc.db.QueryRow(bg, `SELECT count(*) FROM auth_audit WHERE kind = 'login' AND result = 'WRONG_CREDENTIALS'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("失败应写入审计，实际 %d 条（%v）", n, err)
	}
}

// STAGE-8：账号名里带空字符，按“密码错误”处理并写审计，不报内部错误。
func TestLoginWithNulInAccount(t *testing.T) {
	svc, _ := newService(t)
	bg := context.Background()
	_, _, err := svc.Login(bg, "10.0.0.8", "ab\x00c", "whatever")
	if code(err) != CodeWrongCredentials {
		t.Fatalf("应返回 WRONG_CREDENTIALS，实际 %q（%v）", code(err), err)
	}
	var n int
	if err := svc.db.QueryRow(bg, `SELECT count(*) FROM auth_audit WHERE kind = 'login' AND result = 'WRONG_CREDENTIALS'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("应有 1 条审计，实际 %d（%v）", n, err)
	}
	// 连续输错也能锁定，不报错。
	for i := 0; i < 5; i++ {
		_, _, err = svc.Login(bg, "10.0.0.9", "ab\x00c", "whatever")
	}
	if c := code(err); c != CodeAccountLocked && c != CodeWrongCredentials {
		t.Errorf("意外的错误 %q", c)
	}
}
