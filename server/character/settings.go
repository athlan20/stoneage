// 全服游戏设置（STAGE-59）：game_settings 表的读写、变更通知和内存里的经验倍数。
// 现在只有战斗经验倍数 exp_rate（原版 setup.cf 的 battleexp，原私服配 300，这里默认 1 倍同原版节奏）；
// 以后金币倍数、掉落倍数也放这张表，只需要加行。管理命令在同一个事务里 UPDATE 并 NOTIFY，
// 运行中的服务端 LISTEN 收到后重读整张表（做法同背包/宠物的通知）。

package character

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultSettingsNotifyChannel 是“全服设置变了”的 PostgreSQL 通知频道（stoneage-admin set 用）。
const DefaultSettingsNotifyChannel = "game_settings_changed"

// 倍数类设置的取值范围：0.1–100，最多两位小数（写法如 1.5）。
const (
	settingRateMin float64 = 0.1
	settingRateMax float64 = 100
)

// settingSpec 是一个全服设置的取值规则（ParseSettingValue 用）。表里没有这一行时服务端按默认值处理，
// 所以 WriteGameSetting 写的是 upsert：chat.* 这类设置不需要迁移种子。
type settingSpec struct {
	min, max float64
	integer  bool   // 只收整数（写法如 5，不收 5.5）
	note     string // 新增行时写进 note 列
}

// settingSpecs 列出全部可以用 stoneage-admin set 修改的设置；不在这里的名字一律拒绝。
var settingSpecs = map[string]settingSpec{
	"exp_rate":                    {min: settingRateMin, max: settingRateMax, note: "战斗经验倍数"},
	"chat.world_min_level":        {min: 1, max: 160, integer: true, note: "公开聊天：世界频道最低等级（STAGE-278）"},
	"chat.world_cooldown_seconds": {min: 0, max: 3600, integer: true, note: "公开聊天：世界频道每人冷却秒数（STAGE-278）"},
	"chat.map_interval_ms":        {min: 0, max: 60000, integer: true, note: "公开聊天：本图每人最短间隔毫秒数（STAGE-278）"},
	"party.players_enabled":       {min: 0, max: 1, integer: true, note: "真人组队总开关：1 开 0 关（STAGE-274）"},
	"trade.enabled":               {min: 0, max: 1, integer: true, note: "交易总开关：1 开 0 关，默认关（STAGE-294，docs/trade.md 3.1）"},
	"duel.enabled":                {min: 0, max: 1, integer: true, note: "决斗总开关：1 开 0 关，没有这一行时开（STAGE-306，docs/pk.md 3.3）"},
}

// settingValueRe 限定值只能写成非负数字符串（最多两位小数），负号、科学计数法都不收。
var settingValueRe = regexp.MustCompile(`^\d+(\.\d{1,2})?$`)

// ParseSettingValue 解析并校验设置 key 的一个值：名字要认识、是数字、最多两位小数（整数设置不收小数）、在范围之内。
// 非法的值在这里拒绝，数据库不会被改动。
func ParseSettingValue(key, raw string) (float64, error) {
	spec, ok := settingSpecs[key]
	if !ok {
		return 0, fmt.Errorf("没有设置 %q", key)
	}
	if !settingValueRe.MatchString(raw) || (spec.integer && strings.Contains(raw, ".")) {
		if spec.integer {
			return 0, fmt.Errorf("值 %q 要写成整数", raw)
		}
		return 0, fmt.Errorf("值 %q 要写成数字，最多两位小数（如 1.5）", raw)
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("值 %q 不是合法数字", raw)
	}
	if v < spec.min || v > spec.max {
		return 0, fmt.Errorf("值 %s 超出范围，要在 %g 到 %g 之间", raw, spec.min, spec.max)
	}
	return v, nil
}

// GameSetting 是 game_settings 表的一行。
type GameSetting struct {
	Key       string
	Value     float64
	Note      string
	UpdatedAt time.Time
	UpdatedBy string
}

// ListGameSettings 列出全部全服设置，按名字排序。
func ListGameSettings(ctx context.Context, db *pgxpool.Pool) ([]GameSetting, error) {
	rows, err := db.Query(ctx, `SELECT key, value::text, note, updated_at, updated_by
		FROM game_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GameSetting
	for rows.Next() {
		var g GameSetting
		var raw string
		if err := rows.Scan(&g.Key, &raw, &g.Note, &g.UpdatedAt, &g.UpdatedBy); err != nil {
			return nil, err
		}
		if g.Value, err = strconv.ParseFloat(raw, 64); err != nil {
			return nil, fmt.Errorf("设置 %s 的值 %q 不是数字", g.Key, raw)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// WriteGameSetting 改一个全服设置（没有这一行就新增），并在同一个事务里 NOTIFY（运行中的服务端收到后重读）。
// 值的校验在调用方做（ParseSettingValue）；名字不在 settingSpecs 里时返回错误，数据库不变。
func WriteGameSetting(ctx context.Context, db *pgxpool.Pool, channel, key string, value float64, by string) error {
	spec, ok := settingSpecs[key]
	if !ok {
		return fmt.Errorf("没有设置 %q", key)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO game_settings (key, value, note, updated_at, updated_by) VALUES ($1, $2, $3, now(), $4)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		key, value, spec.note, by); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetGameSetting 校验、写入一个全服设置并立刻把内存里的值重读一遍（本地管理口 set_setting 用，
// stoneage-admin 走 WriteGameSetting 就行：服务端靠通知重读）。返回新值供调用方回显。
func (s *Service) SetGameSetting(ctx context.Context, key, raw, by string) (float64, error) {
	v, err := ParseSettingValue(key, raw)
	if err != nil {
		return 0, err
	}
	if err := WriteGameSetting(ctx, s.db, s.opts.SettingsNotifyChannel, key, v, by); err != nil {
		return 0, err
	}
	if err := s.ReloadGameSettings(ctx); err != nil {
		return 0, err
	}
	return v, nil
}

// ExpRate 返回此刻的全服经验倍数；还没加载过（Start 之前）或值坏掉时按 1 倍。
// 战斗结算回调在结算那一刻调用它，改倍数对进行中的战斗立即生效。
func (s *Service) ExpRate() float64 {
	if v, ok := s.expRate.Load().(float64); ok && v > 0 {
		return v
	}
	return 1
}

// ReloadGameSettings 把 game_settings 整张表读进内存。
// 值缺失或超出范围时按 1 倍处理并记警告日志（需求：数据库里的值坏了不能拦住战斗结算）。
func (s *Service) ReloadGameSettings(ctx context.Context) error {
	settings, err := ListGameSettings(ctx, s.db)
	if err != nil {
		if ctx.Err() == nil { // 服务正在停时的通知重读失败不用叫
			s.opts.Logger.Warn("读取全服设置失败，经验按 1 倍处理", "error", err)
		}
		return err
	}
	rate := 1.0
	found := false
	for _, g := range settings {
		if g.Key == "exp_rate" {
			found = true
			if g.Value >= settingRateMin && g.Value <= settingRateMax {
				rate = g.Value
			} else {
				s.opts.Logger.Warn("全服设置 exp_rate 的值超出范围，经验按 1 倍处理", "value", g.Value)
			}
		}
	}
	if !found {
		s.opts.Logger.Warn("全服设置里没有 exp_rate，经验按 1 倍处理")
	}
	partyOn := false // party.players_enabled（STAGE-274）：没有这一行时关闭
	for _, g := range settings {
		if g.Key == partySettingKey {
			partyOn = g.Value == 1
		}
	}
	s.partyPlayersOn.Store(partyOn)
	tradeOn := false // trade.enabled（STAGE-294）：没有这一行时关闭
	for _, g := range settings {
		if g.Key == tradeSettingKey {
			tradeOn = g.Value == 1
		}
	}
	s.tradeOn.Store(tradeOn)
	duelOn := true // duel.enabled（STAGE-306）：默认开，只有明确写 0 才关
	for _, g := range settings {
		if g.Key == duelSettingKey {
			duelOn = g.Value == 1
		}
	}
	s.duelOn.Store(duelOn)
	s.expRate.Store(rate)
	s.chat.Store(loadChatConfig(settings, s.opts.Logger.Warn))
	return nil
}
