package config

import (
	"fmt"
	"math"
	"strconv"
)

// STAGE-351：按连接限速的配置。模式和含义见 server/gateway/ratelimit.go，默认值和 docs/server.md 一致。
const (
	// EnvRateLimit 是按连接限速的模式：off（不检查）、log（超限只记日志，默认）、enforce（超限回复 RATE_LIMITED）。
	EnvRateLimit = "STONEAGE_RATE_LIMIT"
	// EnvRateLimitRate 是令牌桶每秒补充的令牌数（正数，默认 20）。
	EnvRateLimitRate = "STONEAGE_RATE_LIMIT_RATE"
	// EnvRateLimitBurst 是令牌桶的容量（正整数，默认 40）。
	EnvRateLimitBurst = "STONEAGE_RATE_LIMIT_BURST"

	DefaultRateLimitMode  = "log"
	DefaultRateLimitRate  = 20.0
	DefaultRateLimitBurst = 40
)

// parseRateLimit 读出限速的三项配置；get 的语义同 resolve 里的 get（空值取默认）。非法值报错。
func parseRateLimit(get func(key, def string) string) (mode string, rate float64, burst int, err error) {
	mode = get(EnvRateLimit, DefaultRateLimitMode)
	switch mode {
	case "off", "log", "enforce":
	default:
		return "", 0, 0, fmt.Errorf("%s 只能是 off、log 或 enforce，当前是 %q", EnvRateLimit, mode)
	}
	rate = DefaultRateLimitRate
	if raw := get(EnvRateLimitRate, ""); raw != "" {
		rate, err = strconv.ParseFloat(raw, 64)
		if err != nil || !(rate > 0) || math.IsInf(rate, 0) {
			return "", 0, 0, fmt.Errorf("%s 必须是大于 0 的数（每秒补充的令牌数），当前是 %q", EnvRateLimitRate, raw)
		}
	}
	burst = DefaultRateLimitBurst
	if raw := get(EnvRateLimitBurst, ""); raw != "" {
		burst, err = strconv.Atoi(raw)
		if err != nil || burst <= 0 {
			return "", 0, 0, fmt.Errorf("%s 必须是大于 0 的整数（令牌桶容量），当前是 %q", EnvRateLimitBurst, raw)
		}
	}
	return mode, rate, burst, nil
}
