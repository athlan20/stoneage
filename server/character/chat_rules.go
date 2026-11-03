package character

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-278：公开聊天的规则（docs/chat.md 1.3、docs/protocol.md 5.68）。这里都是纯函数，单元测试直接调用。

const (
	chatChannelMap   = "map"
	chatChannelWorld = "world"
	chatChannelParty = "party"          // 队伍频道（STAGE-277）
	chatTextMax      = 70               // 文本上限，按 UTF-8 字符数（中英文都算 1 个）
	chatDupWindow    = 10 * time.Second // 同一频道内，与上一条成功推送相同的文字在这段时间内静默丢弃
)

// chatSent 是角色在一个频道上最近一次成功推送的记录（只在内存里，角色离开世界就清零）。
type chatSent struct {
	at   time.Time
	text string // 屏蔽词替换之后的文字，重复判断用它
}

// chatCleanText 去掉首尾空白，检查长度（1–70 个字符）和控制字符，返回要发出的文字。
func chatCleanText(raw string) (string, *protocol.Error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", protocol.Errorf(protocol.CodeBadRequest, "消息不能为空")
	}
	if n := utf8.RuneCountInString(text); n > chatTextMax {
		return "", protocol.Errorf(protocol.CodeBadRequest, "消息最长 %d 个字符，现在有 %d 个", chatTextMax, n)
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return "", protocol.Errorf(protocol.CodeBadRequest, "消息不能包含控制字符")
		}
	}
	return text, nil
}

// chatWait 返回还要等多久才能在这个频道再说话；0 表示现在可以。cooldown 不大于 0 时不限制。
// 计时只从成功推送的那条开始算，被拒绝或被静默丢弃的都不记（调用方负责）。
func chatWait(sent map[string]chatSent, channel string, now time.Time, cooldown time.Duration) time.Duration {
	last, ok := sent[channel]
	if !ok || cooldown <= 0 {
		return 0
	}
	if left := last.at.Add(cooldown).Sub(now); left > 0 {
		return left
	}
	return 0
}

// chatRetrySeconds 把剩余时间向上取整成秒，至少 1 秒（error.data.retry_after_seconds）。
func chatRetrySeconds(d time.Duration) int {
	n := int((d + time.Second - 1) / time.Second)
	if n < 1 {
		n = 1
	}
	return n
}

// chatIsDuplicate 判断 text（屏蔽词替换之后）是不是同一频道 10 秒内与上一条成功推送相同的文字。
func chatIsDuplicate(sent map[string]chatSent, channel, text string, now time.Time) bool {
	last, ok := sent[channel]
	return ok && last.text == text && now.Sub(last.at) < chatDupWindow
}

// chatMuteActive 判断禁言是否仍然有效（until 为空表示没有禁言），有效时返回解除时间。
func chatMuteActive(until *time.Time, now time.Time) (time.Time, bool) {
	if until == nil || !now.Before(*until) {
		return time.Time{}, false
	}
	return *until, true
}
