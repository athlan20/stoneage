package gateway

import (
	"log/slog"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-351：令牌桶、违规窗口、日志节流的单元测试。时间都从参数传入，不 sleep。

var t0 = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func TestTokenBucketRefillAndExhaust(t *testing.T) {
	b := tokenBucket{rate: 1, burst: 3}
	for i := 0; i < 3; i++ {
		if ok, _ := b.allow(t0); !ok {
			t.Fatalf("第 %d 个请求应当通过（容量 3）", i+1)
		}
	}
	ok, wait := b.allow(t0)
	if ok || wait != time.Second {
		t.Fatalf("耗尽后应拒绝，等待 1 秒：ok=%v wait=%v", ok, wait)
	}
	// 过了 500ms，补充半个令牌：还不够，还要等 500ms。
	ok, wait = b.allow(t0.Add(500 * time.Millisecond))
	if ok || wait != 500*time.Millisecond {
		t.Fatalf("半秒后仍应拒绝，还要等 500ms：ok=%v wait=%v", ok, wait)
	}
	// 再过 500ms 正好补满一个令牌。
	if ok, _ := b.allow(t0.Add(time.Second)); !ok {
		t.Fatal("满 1 秒后应补回一个令牌")
	}
}

func TestTokenBucketCapsAtBurst(t *testing.T) {
	b := tokenBucket{rate: 1, burst: 3}
	b.allow(t0) // 装满后取走一个
	// 空闲很久之后，令牌只能补到容量 3，不会攒成 100 个。
	later := t0.Add(100 * time.Second)
	n := 0
	for {
		ok, _ := b.allow(later)
		if !ok {
			break
		}
		n++
	}
	if n != 3 {
		t.Fatalf("空闲后最多连续通过 burst=3 个，实际 %d 个", n)
	}
}

func TestTokenBucketFractionalRate(t *testing.T) {
	// 每 2 秒补 1 个：取完之后 1 秒的等待是 1 秒，不是 2 秒。
	b := tokenBucket{rate: 0.5, burst: 1}
	b.allow(t0)
	if ok, wait := b.allow(t0); ok || wait != 2*time.Second {
		t.Fatalf("rate=0.5 时应等 2 秒：ok=%v wait=%v", ok, wait)
	}
	if ok, _ := b.allow(t0.Add(time.Second)); ok {
		t.Fatal("只过了 1 秒，不应有令牌")
	}
	if ok, _ := b.allow(t0.Add(2 * time.Second)); !ok {
		t.Fatal("过了 2 秒应有一个令牌")
	}
}

func TestRetrySecondsRoundsUpAndHasFloor(t *testing.T) {
	cases := map[time.Duration]int{
		0:                       1,
		time.Nanosecond:         1,
		50 * time.Millisecond:   1,
		time.Second:             1,
		1500 * time.Millisecond: 2,
		31 * time.Second:        31,
	}
	for d, want := range cases {
		if got := retrySeconds(d); got != want {
			t.Errorf("retrySeconds(%v) = %d，要 %d", d, got, want)
		}
	}
}

func TestViolationsDisconnectAt200InWindow(t *testing.T) {
	var v violations
	for i := 0; i < violationLimit-1; i++ {
		if v.hit(t0.Add(time.Duration(i) * time.Millisecond)) {
			t.Fatalf("第 %d 次拒绝不应断开", i+1)
		}
	}
	if !v.hit(t0.Add(time.Duration(violationLimit-1) * time.Millisecond)) {
		t.Fatal("窗口内第 200 次拒绝应当断开")
	}
}

func TestViolationsWindowSlides(t *testing.T) {
	var v violations
	for i := 0; i < violationLimit-1; i++ {
		v.hit(t0)
	}
	// 10 秒之后，前面的 199 次都滑出窗口，这一次只算 1 次。
	if v.hit(t0.Add(violationWindow)) {
		t.Fatal("旧的拒绝滑出 10 秒窗口后不应断开")
	}
	// 窗口里只有 1 次，再来 198 次仍然不到 200。
	for i := 0; i < violationLimit-2; i++ {
		if v.hit(t0.Add(violationWindow + time.Duration(i+1)*time.Millisecond)) {
			t.Fatalf("窗口内只有 %d 次，不应断开", i+2)
		}
	}
}

func TestLogThrottleOncePerSecondWithCount(t *testing.T) {
	var th logThrottle
	if n, emit := th.note(t0); !emit || n != 1 {
		t.Fatalf("第一次应当记日志，超出 1 次：n=%d emit=%v", n, emit)
	}
	if _, emit := th.note(t0.Add(100 * time.Millisecond)); emit {
		t.Fatal("不到 1 秒不应再记")
	}
	if _, emit := th.note(t0.Add(200 * time.Millisecond)); emit {
		t.Fatal("不到 1 秒不应再记")
	}
	// 1 秒时记一条，带上这期间的 3 次（100ms、200ms、1s 三次超出）。
	if n, emit := th.note(t0.Add(time.Second)); !emit || n != 3 {
		t.Fatalf("满 1 秒应记一条、带 3 次：n=%d emit=%v", n, emit)
	}
}

func mustRequest(t *testing.T, line string) *protocol.Request {
	t.Helper()
	req, perr, _ := protocol.ParseRequest([]byte(line))
	if perr != nil {
		t.Fatalf("解析 %s 失败: %v", line, perr)
	}
	return req
}

// newRateSession 造一个只带限速状态的会话，足够测 checkRate。
func newRateSession(mode string, burst int) *Session {
	opts := Options{RateLimitMode: mode, RateLimitRate: 0.5, RateLimitBurst: burst}
	opts.fill()
	return &Session{rate: newConnRate(opts), log: slog.Default()}
}

func TestCheckRateEnforceRejectsAndDropsAfterViolations(t *testing.T) {
	s := newRateSession(rateModeEnforce, 2)
	req := mustRequest(t, `{"id":1,"type":"login","data":{}}`)
	for i := 0; i < 2; i++ {
		if perr, _ := s.checkRate(req, t0); perr != nil {
			t.Fatalf("burst 内的请求应通过：%v", perr)
		}
	}
	perr, drop := s.checkRate(req, t0)
	if perr == nil || perr.Code != protocol.CodeRateLimited || drop {
		t.Fatalf("超限应回复 RATE_LIMITED 且暂不断开：perr=%v drop=%v", perr, drop)
	}
	data, _ := perr.Data.(map[string]any)
	if data["retry_after_seconds"] != 2 { // rate 0.5 时补满一个令牌要 2 秒
		t.Fatalf("retry_after_seconds = %v，要 2", data["retry_after_seconds"])
	}
	for i := 1; i < violationLimit; i++ {
		_, drop = s.checkRate(req, t0)
	}
	if !drop {
		t.Fatal("窗口内被拒绝满 200 次后应当断开")
	}
}

func TestCheckRateConnectionCategoryNotCounted(t *testing.T) {
	s := newRateSession(rateModeEnforce, 1)
	for i := 0; i < 50; i++ {
		if perr, _ := s.checkRate(mustRequest(t, `{"id":2,"type":"ping"}`), t0); perr != nil {
			t.Fatalf("心跳不应被限速：%v", perr)
		}
		if perr, _ := s.checkRate(mustRequest(t, `{"id":3,"type":"hello"}`), t0); perr != nil {
			t.Fatalf("hello 不应被限速：%v", perr)
		}
	}
	// 未知类型计入同一个桶：桶只有 1 个令牌，第一个未知类型取走它，第二个就被拒绝。
	if perr, _ := s.checkRate(mustRequest(t, `{"id":4,"type":"nope"}`), t0); perr != nil {
		t.Fatalf("第一个未知类型应通过：%v", perr)
	}
	if perr, _ := s.checkRate(mustRequest(t, `{"id":5,"type":"nope"}`), t0); perr == nil {
		t.Fatal("未知类型应计入同一个桶")
	}
}

func TestCheckRateLogAndOffNeverReject(t *testing.T) {
	for _, mode := range []string{rateModeLog, rateModeOff} {
		s := newRateSession(mode, 1)
		req := mustRequest(t, `{"id":1,"type":"login","data":{}}`)
		for i := 0; i < 100; i++ {
			if perr, drop := s.checkRate(req, t0); perr != nil || drop {
				t.Fatalf("模式 %s 不应拒绝或断开：perr=%v drop=%v", mode, perr, drop)
			}
		}
	}
}

func TestFillNormalizesRateLimitOptions(t *testing.T) {
	var o Options
	o.fill()
	if o.RateLimitMode != rateModeLog || o.RateLimitRate != 20 || o.RateLimitBurst != 40 {
		t.Fatalf("默认值不对：%+v", o)
	}
	o = Options{RateLimitMode: "ENFORCE", RateLimitRate: -1, RateLimitBurst: -3}
	o.fill()
	if o.RateLimitMode != rateModeLog || o.RateLimitRate != 20 || o.RateLimitBurst != 40 {
		t.Fatalf("不认识的模式应取 log，非正数取默认：%+v", o)
	}
}
