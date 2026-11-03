package character

import (
	"strings"
	"time"
)

// STAGE-278：公开聊天的全服设置（docs/chat.md 1.1，docs/server.md）。三项都在 game_settings 里，
// 表里没有这一行时按默认值；ReloadGameSettings 时整体读进内存，发言时读内存，不查库。

// chatConfig 是公开聊天的限制参数（已经换算成 Go 的时长）。
type chatConfig struct {
	WorldMinLevel int           // chat.world_min_level：世界频道最低等级，默认 5
	WorldCooldown time.Duration // chat.world_cooldown_seconds：世界频道每人冷却，默认 30 秒
	MapInterval   time.Duration // chat.map_interval_ms：本图每人最短间隔，默认 1 秒
}

// defaultChatConfig 是表里没有设置、或设置的值超出范围时用的默认值。
var defaultChatConfig = chatConfig{WorldMinLevel: 5, WorldCooldown: 30 * time.Second, MapInterval: time.Second}

// chatConfig 返回当前的公开聊天参数；还没加载过（Start 之前）时用默认值。
func (s *Service) chatConfig() chatConfig {
	if c, ok := s.chat.Load().(chatConfig); ok {
		return c
	}
	return defaultChatConfig
}

// loadChatConfig 从整张 game_settings 表里取出 chat.* 项；值超出范围的记警告并用默认值。
func loadChatConfig(settings []GameSetting, warn func(msg string, args ...any)) chatConfig {
	c := defaultChatConfig
	for _, g := range settings {
		spec, ok := settingSpecs[g.Key]
		if !ok || !strings.HasPrefix(g.Key, "chat.") {
			continue
		}
		if g.Value < spec.min || g.Value > spec.max {
			warn("公开聊天设置的值超出范围，用默认值", "key", g.Key, "value", g.Value)
			continue
		}
		switch g.Key {
		case "chat.world_min_level":
			c.WorldMinLevel = int(g.Value)
		case "chat.world_cooldown_seconds":
			c.WorldCooldown = time.Duration(g.Value) * time.Second
		case "chat.map_interval_ms":
			c.MapInterval = time.Duration(g.Value) * time.Millisecond
		}
	}
	return c
}
