package character

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/wordfilter"
)

// STAGE-278：公开聊天的限流、重复、屏蔽词和禁言到期（纯函数，不碰数据库）。

func TestChatWaitRateLimit(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	sent := map[string]chatSent{}

	// 没说过话：不限。
	if w := chatWait(sent, chatChannelWorld, t0, 30*time.Second); w != 0 {
		t.Fatalf("没说过话时等待 %v，应为 0", w)
	}
	sent[chatChannelWorld] = chatSent{at: t0, text: "甲"}
	// 世界冷却 30 秒内：还要等剩下的时间。
	if w := chatWait(sent, chatChannelWorld, t0.Add(10*time.Second), 30*time.Second); w != 20*time.Second {
		t.Fatalf("冷却中等待 %v，应为 20s", w)
	}
	if got := chatRetrySeconds(20*time.Second + 300*time.Millisecond); got != 21 {
		t.Fatalf("剩余 20.3 秒应向上取整为 21，得到 %d", got)
	}
	// 冷却到期后可以再说。
	if w := chatWait(sent, chatChannelWorld, t0.Add(30*time.Second), 30*time.Second); w != 0 {
		t.Fatalf("冷却到期后等待 %v，应为 0", w)
	}
	// 两个频道各自计时：本图的记录不影响世界。
	if w := chatWait(sent, chatChannelMap, t0.Add(time.Millisecond), time.Second); w != 0 {
		t.Fatalf("本图不应受世界冷却影响，等待 %v", w)
	}
	// 冷却为 0 时不限制。
	if w := chatWait(sent, chatChannelWorld, t0, 0); w != 0 {
		t.Fatalf("冷却 0 时等待 %v，应为 0", w)
	}
	if got := chatRetrySeconds(time.Millisecond); got != 1 {
		t.Fatalf("不足 1 秒也至少报 1 秒，得到 %d", got)
	}
}

func TestChatDuplicateWithinTenSeconds(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	sent := map[string]chatSent{chatChannelMap: {at: t0, text: "有人去加加村吗"}}

	if !chatIsDuplicate(sent, chatChannelMap, "有人去加加村吗", t0.Add(9*time.Second)) {
		t.Fatal("9 秒内同一句应视为重复")
	}
	if chatIsDuplicate(sent, chatChannelMap, "有人去加加村吗", t0.Add(10*time.Second)) {
		t.Fatal("满 10 秒后同一句不再是重复")
	}
	if chatIsDuplicate(sent, chatChannelMap, "别的话", t0.Add(time.Second)) {
		t.Fatal("不同的文字不是重复")
	}
	if chatIsDuplicate(sent, chatChannelWorld, "有人去加加村吗", t0.Add(time.Second)) {
		t.Fatal("重复只看同一频道")
	}
}

func TestChatUsesWordFilter(t *testing.T) {
	// 匹配细节在 server/wordfilter 测；这里只确认没配外部词表时内置词表能兜底。
	const bad = "你个傻逼啊"
	if got := wordfilter.Current().Mask(bad); got == bad {
		t.Fatalf("内置屏蔽词没有被替换：%q", got)
	}
}

func TestChatMuteExpiry(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	if _, muted := chatMuteActive(nil, now); muted {
		t.Fatal("没有禁言记录时不应禁言")
	}
	until := now.Add(2 * time.Second)
	if got, muted := chatMuteActive(&until, now.Add(time.Second)); !muted || !got.Equal(until) {
		t.Fatalf("到期前应禁言，解除时间 %v，得到 %v %v", until, got, muted)
	}
	if _, muted := chatMuteActive(&until, until); muted {
		t.Fatal("到期那一刻禁言已解除")
	}
	if _, muted := chatMuteActive(&until, until.Add(time.Minute)); muted {
		t.Fatal("过了解除时间应恢复发言")
	}
}

func TestChatCleanText(t *testing.T) {
	if got, err := chatCleanText("  有人去加加村吗  "); err != nil || got != "有人去加加村吗" {
		t.Fatalf("首尾空白应去掉，得到 %q %v", got, err)
	}
	if _, err := chatCleanText("   "); err == nil {
		t.Fatal("纯空白应拒绝")
	}
	long := make([]rune, chatTextMax)
	for i := range long {
		long[i] = '一'
	}
	if _, err := chatCleanText(string(long)); err != nil {
		t.Fatalf("70 个字符应允许：%v", err)
	}
	if _, err := chatCleanText(string(append(long, '一'))); err == nil {
		t.Fatal("71 个字符应拒绝")
	}
	if _, err := chatCleanText("a\tb"); err == nil {
		t.Fatal("含制表符应拒绝")
	}
}

// 公开聊天的三项设置：整数、范围和名字都要检查（stoneage-admin set 用）。
func TestParseChatSettingValue(t *testing.T) {
	if v, err := ParseSettingValue("chat.world_min_level", "5"); err != nil || v != 5 {
		t.Fatalf("world_min_level 5 应通过，得到 %g %v", v, err)
	}
	for _, c := range [][2]string{{"chat.world_min_level", "5.5"}, {"chat.world_min_level", "161"}, {"chat.map_interval_ms", "-1"}, {"chat.unknown", "1"}} {
		if v, err := ParseSettingValue(c[0], c[1]); err == nil {
			t.Errorf("ParseSettingValue(%q, %q) = %g，应拒绝", c[0], c[1], v)
		}
	}
}
