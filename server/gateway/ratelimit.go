package gateway

import (
	"math"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-351：按连接限速。每条连接一个令牌桶，只在连接自己的处理循环里用，不加锁。
// 心跳、握手、退出（CategoryConnection）不计入；其它分类和未知类型计入同一个桶。

// 限速模式（Options.RateLimitMode，配置项 STONEAGE_RATE_LIMIT）。
const (
	rateModeOff     = "off"     // 不检查
	rateModeLog     = "log"     // 超限只记日志，照常分发
	rateModeEnforce = "enforce" // 超限回复 RATE_LIMITED，持续超限断开
)

const (
	// violationWindow 和 violationLimit 决定持续超限的断开：窗口内被拒绝达到 violationLimit 次就断开。
	violationWindow = 10 * time.Second
	violationLimit  = 200
	// logInterval 是 log 模式下同一连接记超限日志的最短间隔。
	logInterval = time.Second
)

// tokenBucket 是令牌桶：容量 burst，每秒补充 rate 个令牌，每个计入的请求取一个。
type tokenBucket struct {
	rate   float64 // 每秒补充的令牌数，要大于 0
	burst  float64 // 容量
	tokens float64
	last   time.Time // 上次补充的时间；零值表示还没用过，第一次调用时装满
}

// allow 取一个令牌。够用返回 true；不够返回 false 和要等多久才会有下一个令牌。
func (b *tokenBucket) allow(now time.Time) (bool, time.Duration) {
	switch {
	case b.last.IsZero():
		b.tokens = b.burst
		b.last = now
	case now.After(b.last):
		b.tokens = math.Min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	if b.rate <= 0 {
		return false, time.Hour // 不会发生：配置和 Options.fill 都保证 rate 大于 0
	}
	wait := (1 - b.tokens) / b.rate
	return false, time.Duration(wait * float64(time.Second))
}

// violations 记录滑动窗口内被拒绝的时间，判断是不是持续超限。
type violations struct {
	times []time.Time
}

// hit 记下一次被拒绝，返回窗口内的次数是否已经达到断开的门槛。
func (v *violations) hit(now time.Time) bool {
	cut := 0
	for cut < len(v.times) && now.Sub(v.times[cut]) >= violationWindow {
		cut++
	}
	v.times = append(v.times[cut:], now)
	return len(v.times) >= violationLimit
}

// logThrottle 让 log 模式的超限日志每秒最多一条，并带上两条之间超出的次数。
type logThrottle struct {
	last    time.Time
	pending int // 上次记日志以来超出的次数
}

// note 记一次超出。距上次记日志不到 logInterval 时返回 false；否则返回这期间超出的次数，要记一条日志。
func (t *logThrottle) note(now time.Time) (int, bool) {
	t.pending++
	if !t.last.IsZero() && now.Sub(t.last) < logInterval {
		return 0, false
	}
	t.last = now
	n := t.pending
	t.pending = 0
	return n, true
}

// connRate 是一条连接的限速状态。
type connRate struct {
	mode    string
	bucket  tokenBucket
	rejects violations
	warn    logThrottle
}

func newConnRate(opts Options) connRate {
	return connRate{
		mode:   opts.RateLimitMode,
		bucket: tokenBucket{rate: opts.RateLimitRate, burst: float64(opts.RateLimitBurst), tokens: float64(opts.RateLimitBurst)},
	}
}

// checkRate 按限速规则检查一个请求。返回 nil 表示照常分发；返回的错误要回复给客户端，
// drop 为 true 时还要断开连接（持续超限）。
func (s *Session) checkRate(req *protocol.Request, now time.Time) (perr *protocol.Error, drop bool) {
	r := &s.rate
	if r.mode == rateModeOff {
		return nil, false
	}
	if meta, ok := protocol.Lookup(req.Type); ok && meta.Category == protocol.CategoryConnection {
		return nil, false
	}
	ok, wait := r.bucket.allow(now)
	if ok {
		return nil, false
	}
	if r.mode == rateModeLog {
		if n, emit := r.warn.note(now); emit {
			s.log.Warn("请求超出按连接限速（只记录，未拒绝）", "account", s.Account(), "type", req.Type, "exceeded", n)
		}
		return nil, false
	}
	perr = protocol.Errorf(protocol.CodeRateLimited, "操作太快了，请稍后再试").
		WithData(map[string]any{"retry_after_seconds": retrySeconds(wait)})
	return perr, r.rejects.hit(now)
}

// retrySeconds 把等待时间向上取整到秒，最少 1 秒（和 server/auth 的 seconds 口径一致）。
func retrySeconds(d time.Duration) int {
	n := int((d + time.Second - 1) / time.Second)
	if n < 1 {
		n = 1
	}
	return n
}
