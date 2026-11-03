// Package app 把服务端的各个功能（账号、角色……）装到网络层上，
// 服务端主程序和协议用例的回归测试用同一套装配，避免两边不一致。
package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/auth"
	"github.com/athlan20/stoneage/server/character"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
)

// Options 是装配参数。
type Options struct {
	RegisterEnabled bool
	Hash            auth.HashParams // 零值取默认；测试用低成本参数
	NotifyChannel   string          // 账号禁用通知的频道，零值取默认；并行的测试各用各的
	ItemNotify      string          // 角色物品通知的频道，零值取默认；并行的测试各用各的
	PetNotify       string          // 角色宠物通知的频道，零值取默认；并行的测试各用各的
	PointNotify     string          // 角色未分配点数通知的频道，零值取默认；并行的测试各用各的
	MagicNotify     string          // 角色精灵通知的频道，零值取默认；并行的测试各用各的
	SettingsNotify  string          // 全服设置通知的频道，零值取默认；并行的测试各用各的
	QuestNotify     string          // 角色任务进度通知的频道，零值取默认；并行的测试各用各的
	RideNotify      string          // 角色骑乘等级通知的频道，零值取默认；并行的测试各用各的
	// ChatFilterNotify 是屏蔽词表重载通知的频道（STAGE-287），零值取默认；ChatFilterFile 是词表文件，空表示不用外部词表。
	ChatFilterNotify string
	ChatFilterFile   string
	// RideLevelGap 是骑宠的等级差上限（STAGE-123），0 取默认 5。
	RideLevelGap int
	// EncounterIntervalMs 是遇敌开关两场战斗之间的间隔（毫秒，STAGE-40）；0 取默认 500。
	EncounterIntervalMs int
	// WildRnd 是野外遇敌战斗的随机源（STAGE-40）：协议用例按用例名固定种子；nil 用全局随机。
	WildRnd func(n int) int
	// LuckRnd 是每日掷运气的随机源（STAGE-61）：协议用例按用例名固定种子；nil 用全局随机。
	LuckRnd func(n int) int
	// CreateRnd 是新建角色出生宠物资质的随机源（STAGE-62）：协议用例按用例名固定种子；nil 用全局随机。
	CreateRnd func(n int) int
	// NpcTalkRnd 是 NPC 对话选句的随机源（STAGE-69）：协议用例按用例名固定种子；nil 用全局随机。
	NpcTalkRnd func(n int) int
	// MergeRnd 是加工、料理合成的随机源（STAGE-207）：协议用例按用例名固定种子；nil 用全局随机。
	MergeRnd func(n int) int
	// MagicUseRnd 是战斗外精灵回复量的随机源（STAGE-230）：协议用例按用例名固定种子；nil 用全局随机。
	MagicUseRnd func(n int) int
	// AgentMaxOnline 是 agent 连续在线的上限（STAGE-118），0 不限制。
	AgentMaxOnline time.Duration
	// Autosave 是在线角色定时存盘的间隔（STAGE-290）：0 取默认 3 分钟，负数关闭。
	Autosave time.Duration
	// RegisterMax 是每个 IP 每小时最多注册几次，0 取默认（auth.DefaultLimits）；协议用例用它把上限收紧到 5。
	RegisterMax int
	// NewPlayerGifts 是新角色出生礼包（gamedata.NewPlayerGifts）；零值不送，生产入口传默认礼包。
	NewPlayerGifts []gamedata.NewPlayerGift
}

// Services 是装好的各项功能。
type Services struct {
	Auth       *auth.Service
	Characters *character.Service
}

// authLimits 在默认限制上套用 Options 里调过的项；没调的返回零值，auth.New 会取默认。
func authLimits(opts Options) auth.Limits {
	if opts.RegisterMax <= 0 {
		return auth.Limits{}
	}
	l := auth.DefaultLimits
	l.RegisterMax = opts.RegisterMax
	return l
}

// Install 创建各项功能并注册到 srv。
func Install(srv *gateway.Server, db *pgxpool.Pool, opts Options) (*Services, error) {
	chars := character.New(db, srv.Hub(), character.Options{ItemNotifyChannel: opts.ItemNotify, PetNotifyChannel: opts.PetNotify,
		PointNotifyChannel: opts.PointNotify, MagicNotifyChannel: opts.MagicNotify, SettingsNotifyChannel: opts.SettingsNotify,
		QuestNotifyChannel: opts.QuestNotify, RideNotifyChannel: opts.RideNotify, ChatFilterNotifyChannel: opts.ChatFilterNotify, ChatFilterFile: opts.ChatFilterFile, RideLevelGap: opts.RideLevelGap,
		EncounterInterval: time.Duration(opts.EncounterIntervalMs) * time.Millisecond, WildRnd: opts.WildRnd, LuckRnd: opts.LuckRnd, CreateRnd: opts.CreateRnd,
		NpcTalkRnd: opts.NpcTalkRnd, MergeRnd: opts.MergeRnd, MagicUseRnd: opts.MagicUseRnd, NewPlayerGifts: opts.NewPlayerGifts,
		AutosaveInterval: opts.Autosave})
	accounts, err := auth.New(db, srv.Hub(), auth.Options{
		RegisterEnabled: opts.RegisterEnabled,
		Hash:            opts.Hash,
		NotifyChannel:   opts.NotifyChannel,
		// 挤下线、账号被禁用时，先保存角色数据再断开。
		BeforeKick:     chars.BeforeKick,
		AgentMaxOnline: opts.AgentMaxOnline,
		Limits:         authLimits(opts),
	})
	if err != nil {
		return nil, err
	}
	accounts.Install(srv)
	chars.Install(srv)
	return &Services{Auth: accounts, Characters: chars}, nil
}

// Start 启动后台任务（账号禁用通知、角色物品通知的监听）。
func (s *Services) Start(ctx context.Context) error {
	if err := s.Auth.Start(ctx); err != nil {
		s.Characters.Stop()
		return err
	}
	if err := s.Characters.Start(ctx); err != nil {
		s.Auth.Stop()
		return err
	}
	return nil
}

// Stop 停止后台任务。
func (s *Services) Stop() {
	s.Auth.Stop()
	s.Characters.Stop()
}
