// Package character 实现角色列表、创建、删除、进入和离开世界（STAGE-4，docs/protocol.md 5.5–5.9）。
// 角色数据存在 PostgreSQL 的 characters 表；规则出处是原版服务端，见 server/gamedata。
package character

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// Options 是服务的参数，零值字段取默认值。
type Options struct {
	Now    func() time.Time // 测试用，默认 time.Now
	Logger *slog.Logger
	// ItemNotifyChannel 是“角色物品变了”的 PostgreSQL 通知频道（stoneage-admin give 用），
	// 零值取默认；并行的测试各用各的频道。
	ItemNotifyChannel string
	// PetNotifyChannel 是“角色宠物变了”的 PostgreSQL 通知频道（stoneage-admin givepet 用，STAGE-31），
	// 零值取默认；同样并行的测试各用各的。
	PetNotifyChannel string
	// PointNotifyChannel 是“角色未分配点数变了”的 PostgreSQL 通知频道（stoneage-admin givepoint 用，STAGE-24），
	// 零值取默认；同样并行的测试各用各的。
	PointNotifyChannel string
	// GoldNotifyChannel 是“角色石币变了”的 PostgreSQL 通知频道（stoneage-admin grant --kind gold 用，STAGE-310），
	// 零值取默认；并行的测试各用各的。
	GoldNotifyChannel string
	// MagicNotifyChannel 是“角色精灵变了”的 PostgreSQL 通知频道（stoneage-admin givemagic 用，STAGE-48）。
	MagicNotifyChannel string
	// SettingsNotifyChannel 是“全服设置变了”的 PostgreSQL 通知频道（stoneage-admin set 用，STAGE-59）。
	SettingsNotifyChannel string
	// QuestNotifyChannel 是“角色任务进度变了”的 PostgreSQL 通知频道（stoneage-admin quest 用，STAGE-86）。
	QuestNotifyChannel string
	// ChatFilterNotifyChannel 是“屏蔽词表要重载”的 PostgreSQL 通知频道（stoneage-admin chatfilter reload 用，STAGE-287）。
	ChatFilterNotifyChannel string
	// ChatFilterFile 是屏蔽词表文件（.env 的 STONEAGE_CHAT_FILTER_FILE）；收到重载通知时重读它，空表示不用外部词表。
	ChatFilterFile string
	// RideNotifyChannel 是“角色骑乘等级变了”的 PostgreSQL 通知频道（stoneage-admin ridelevel 用，STAGE-122）。
	RideNotifyChannel string
	// RideLevelGap 是骑宠时人物等级加上它不能低于宠物等级（STAGE-123，docs/ride.md §3）；0 取默认 5。
	RideLevelGap int
	// EncounterInterval 是遇敌开关两场战斗之间的间隔（STAGE-40），零值取默认 0.5 秒。
	EncounterInterval time.Duration
	// WildRnd 是野外遇敌战斗的随机源（STAGE-40）：协议用例按用例名固定种子，重放结果可复现；
	// 零值用全局随机。
	WildRnd func(n int) int
	// MergeRnd 是加工、料理合成的随机源（STAGE-207）：协议用例按用例名固定种子，重放的结果可复现；零值用全局随机。
	MergeRnd func(n int) int
	// MagicUseRnd 是战斗外精灵（治愈、滋润、恩惠）回复量的随机源（STAGE-230）：协议用例按用例名固定种子；零值用全局随机。
	MagicUseRnd func(n int) int
	// LuckRnd 是每日掷运气的随机源（0–99，STAGE-61）：协议用例按用例名固定种子，重放的运气可复现；零值用全局随机。
	LuckRnd func(n int) int
	// CreateRnd 是新建角色出生宠物资质的随机源（STAGE-62）：协议用例按用例名固定种子，重放的资质可复现；零值用全局随机。
	CreateRnd func(n int) int
	// NpcTalkRnd 是 NPC 对话选句的随机源（STAGE-69）：同上按用例名固定种子；nil 时取第一句（确定性），
	// 生产环境用全局随机。
	NpcTalkRnd func(n int) int
	// NpcEventEnabled 是任务脚本的放开判断（STAGE-92）：测试里换成自己的白名单，验证还没上线的脚本（比如暗号）；
	// 零值用 gamedata 的白名单（npcevent.go 的 npcEventEnabled）。
	NpcEventEnabled func(file string) bool
	// NpcEnemyEnabled 是战斗 NPC 的放开判断（STAGE-96）：测试里换成自己的白名单，验证还没放开的战斗（比如分批放开的守护兽）；
	// 零值用 gamedata 的白名单（npcenemy.go 的 npcEnemyEnabled）。
	NpcEnemyEnabled func(file string) bool
	// NewPlayerGifts 是新角色出生时放进背包的物品（gamedata.NewPlayerGifts）；零值不送，
	// 协议用例和大多数测试的背包因此保持是空的，由生产入口（cmd/stoneage-server）打开。
	NewPlayerGifts []gamedata.NewPlayerGift
	// AutosaveInterval 是在线角色定时存盘的间隔（STAGE-290）：0 取默认 3 分钟，负数关闭。
	AutosaveInterval time.Duration
}

// Service 是角色服务。
type Service struct {
	db   *pgxpool.Pool
	hub  *gateway.Hub
	opts Options

	// mu 保护下面四个字段，只做内存操作，不在里面访问数据库，也不在持有它时去等 Presence.mu（STAGE-12：
	// Presence.mu 会在保存角色时被占几秒，全服共用的这把锁不能跟着被占住）。
	// 两把锁需要同时持有时，先取 Presence.mu，再取这把。
	mu             sync.Mutex
	lives          map[uint64]*Live    // 连接编号 → 在世界中的句柄
	presences      map[int64]*Presence // 账号 id → 在世界中的角色
	parties        *partyHub           // 真人组队的全服状态（STAGE-274）
	partyPlayersOn atomic.Bool         // party.players_enabled：真人组队开关（STAGE-274，settings.go）
	tradeOn        atomic.Bool         // trade.enabled：交易开关（STAGE-294，settings.go）
	duelOn         atomic.Bool         // duel.enabled：决斗开关（STAGE-306，settings.go）
	duelRate       duelRates           // 决斗发起的频率记录（STAGE-306，duel.go）
	trades         *tradeHub           // 交易的全服状态（STAGE-294，trade.go）
	players        *playerIndex        // 附近玩家索引（STAGE-273）
	quizzes        *quizHub            // 猜谜人物的座位表（STAGE-330，npc_quiz_seats.go）
	gone           map[uint64]bool     // 已被踢或已关闭的连接，不能再进入世界
	deleting       map[int64]bool      // 正在删除的角色 id：期间不能进入世界（STAGE-16）

	// boards 是每个账号的 agent 计划和实际活动（STAGE-119，agentboard.go），只在内存里，自己有锁。
	boards agentBoards

	notifyStop context.CancelFunc // 数据库通知监听的停止函数（Start 之后非空）
	notifyDone chan struct{}      // 数据库通知监听退出信号

	autosaveStop context.CancelFunc // 定时存盘的停止函数（STAGE-290，开着时 Start 之后非空）
	autosaveDone chan struct{}      // 定时存盘协程退出信号

	battleSeq int // 已开过的战斗数（battle_id 从 b_1 开始编号）

	// npcs 是全服共用的 NPC 当前位置（STAGE-93）：任务脚本的 NpcWarp 会移动 NPC，查询和对话都读它。
	// 它自己有锁，可以在持有 Presence.mu 时读；移动只在角色锁之外做（npcwarp.go）。
	npcs *gamedata.NPCBoard

	// expRate 是内存里的全服经验倍数（game_settings 的 exp_rate，STAGE-59）：
	// Start 时和收到设置通知后重读，战斗结算时读。
	expRate atomic.Value

	// chat 是公开聊天的全服设置（STAGE-278，chat_settings.go）：ReloadGameSettings 时更新，发言时读。
	chat atomic.Value

	hooks testHooks
}

// testHooks 是测试用的插入点，用来在两步之间制造并发的另一个请求；生产环境全为 nil。
type testHooks struct {
	afterFind        func(rec *record) // handleEnter：读到角色之后、登记进世界之前
	afterDeleteCheck func(rec *record) // handleDelete：确认没在世界中并标记删除之后、真正删除之前
}

// New 创建角色服务。
func New(db *pgxpool.Pool, hub *gateway.Hub, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ItemNotifyChannel == "" {
		opts.ItemNotifyChannel = DefaultItemNotifyChannel
	}
	if opts.PetNotifyChannel == "" {
		opts.PetNotifyChannel = DefaultPetNotifyChannel
	}
	if opts.PointNotifyChannel == "" {
		opts.PointNotifyChannel = DefaultPointNotifyChannel
	}
	if opts.GoldNotifyChannel == "" {
		opts.GoldNotifyChannel = DefaultGoldNotifyChannel
	}
	if opts.MagicNotifyChannel == "" {
		opts.MagicNotifyChannel = DefaultMagicNotifyChannel
	}
	if opts.SettingsNotifyChannel == "" {
		opts.SettingsNotifyChannel = DefaultSettingsNotifyChannel
	}
	if opts.QuestNotifyChannel == "" {
		opts.QuestNotifyChannel = DefaultQuestNotifyChannel
	}
	if opts.ChatFilterNotifyChannel == "" {
		opts.ChatFilterNotifyChannel = DefaultChatFilterNotifyChannel
	}
	if opts.RideNotifyChannel == "" {
		opts.RideNotifyChannel = DefaultRideNotifyChannel
	}
	if opts.RideLevelGap == 0 {
		opts.RideLevelGap = DefaultRideLevelGap
	}
	s := &Service{
		db: db, hub: hub, opts: opts,
		lives: map[uint64]*Live{}, presences: map[int64]*Presence{}, players: newPlayerIndex(), quizzes: newQuizHub(), gone: map[uint64]bool{}, deleting: map[int64]bool{},
		npcs: gamedata.NewNPCBoard(),
	}
	s.parties = newPartyHub(s)
	s.trades = newTradeHub(s)
	return s
}

// Install 把角色相关请求注册到网络层，并让连接结束时保存世界中的角色。
func (s *Service) Install(srv *gateway.Server) {
	srv.Handle("characters", s.handleCharacters)
	srv.Handle("create_character", s.handleCreate)
	srv.Handle("delete_character", s.handleDelete)
	srv.Handle("enter_world", s.handleEnter)
	srv.Handle("leave_world", s.handleLeave)
	srv.Handle("status", s.handleStatus)
	srv.Handle("turn", s.handleTurn)
	srv.Handle("places", s.handlePlaces)
	srv.Handle("maps", s.handleMaps)
	srv.Handle("npcs", s.handleNpcs)
	srv.Handle("players", s.handlePlayers)          // STAGE-273
	srv.Handle("party_invite", s.handlePartyInvite) // STAGE-274：真人组队
	srv.Handle("trade_invite", s.handleTradeInvite) // STAGE-294：交易（docs/protocol.md 5.72）
	srv.Handle("trade_reply", s.handleTradeReply)
	srv.Handle("trade_cancel", s.handleTradeCancel)
	srv.Handle("trade_offer", s.handleTradeOffer)
	srv.Handle("trade_lock", s.handleTradeLock)
	srv.Handle("trade_unlock", s.handleTradeUnlock)
	srv.Handle("trade_confirm", s.handleTradeConfirm)
	srv.Handle("party_join", s.handlePartyJoin)
	srv.Handle("party_reply", s.handlePartyReply)
	srv.Handle("party_cancel", s.handlePartyCancel)
	srv.Handle("party_leave", s.handlePartyLeave)
	srv.Handle("party_kick", s.handlePartyKick)
	srv.Handle("party_leader", s.handlePartyLeader)
	srv.Handle("social_settings", s.handleSocialSettings) // STAGE-276：隐身与拒绝邀请（docs/protocol.md 5.69）
	srv.Handle("duel_challenge", s.handleDuelChallenge)   // STAGE-306：发起决斗（docs/protocol.md 5.74）
	srv.Handle("duel_ranking", s.handleDuelRanking)       // STAGE-308：决斗强者表（docs/protocol.md 5.75）
	srv.Handle("npc_find", s.handleNpcFind)
	srv.Handle("npc_list", s.handleNpcList)
	srv.Handle("talk", s.handleTalk)
	srv.Handle("npc_open", s.handleTalk)
	srv.Handle("npc_confirm", s.handleNpcConfirm)
	srv.Handle("npc_back", s.handleNpcBack)
	srv.Handle("npc_choose", s.handleNpcChoose)
	srv.Handle("npc_reply", s.handleNpcReply)
	srv.Handle("npc_close", s.handleNpcClose)
	srv.Handle("warp", s.handleWarp)
	srv.Handle("exits", s.handleExits)
	srv.Handle("use_exit", s.handleUseExit)
	srv.Handle("inventory", s.handleInventory)
	srv.Handle("pets", s.handlePets)
	srv.Handle("party", s.handleParty)
	srv.Handle("party_clone", s.handlePartyClone)
	srv.Handle("party_disband", s.handlePartyDisband)
	srv.Handle("quests", s.handleQuests)
	srv.Handle("quest_npcs", s.handleQuestNPCs)
	srv.Handle("pet_state", s.handlePetState)
	srv.Handle("pet_rename", s.handlePetRename)
	srv.Handle("pet_drop", s.handlePetDrop)
	srv.Handle("pet_ride", s.handlePetRide)
	srv.Handle("pet_talk", s.handlePetTalk) // STAGE-239
	srv.Handle("pet_unride", s.handlePetUnride)
	srv.Handle("pet_skill", s.handlePetSkill)
	srv.Handle("magic_use", s.handleMagicUse)
	srv.Handle("addpoint", s.handleAddPoint)
	srv.Handle("item_equip", s.handleItemEquip)
	srv.Handle("item_unequip", s.handleItemUnequip)
	srv.Handle("item_move", s.handleItemMove)
	srv.Handle("item_drop", s.handleItemDrop)
	srv.Handle("gold_drop", s.handleGoldDrop) // STAGE-328：丢弃石币（协议 5.76）
	srv.Handle("item_use", s.handleItemUse)
	srv.Handle("item_inlay", s.handleItemInlay)
	srv.Handle("take_control", s.handleTakeControl)
	srv.Handle("release_control", s.handleReleaseControl)
	srv.Handle("kick_agent", s.handleKickAgent)
	srv.Handle("agent_say", s.handleAgentSay)
	srv.Handle("agent_plan", s.handleAgentPlan)
	srv.Handle("chat", s.handleChat) // STAGE-278：公开聊天（docs/protocol.md 5.68）
	srv.Handle("battle_command", s.handleBattleCommand)
	srv.Handle("battle_ack", s.handleBattleAck)
	srv.Handle("battle", s.handleBattle)
	srv.Handle("encounter", s.handleEncounter)
	srv.Handle("encounter_areas", s.handleEncounterAreas)
	srv.Handle("encounter_start", s.handleEncounterStart)
	srv.Handle("pet_encounter_catalog", s.handlePetEncounterCatalog) // STAGE-251
	srv.Handle("pet_encounter_start", s.handlePetEncounterStart)     // STAGE-251
	srv.Handle("leveling_catalog", s.handleLevelingCatalog)          // STAGE-279
	srv.Handle("leveling_start", s.handleLevelingStart)              // STAGE-279
	srv.SetGuard(s.guard)
	srv.OnDone(s.auditRequest)
	srv.OnDone(s.observeDone)
	srv.OnClose(s.OnClose)
	srv.OnClose(s.observeClose)
}

func (s *Service) handleCharacters(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	list, err := s.listByAccount(ctx, sess.AccountID())
	if err != nil {
		return nil, err
	}
	views := make([]characterView, 0, len(list))
	for _, r := range list {
		views = append(views, r.view())
	}
	return map[string]any{"max_slots": MaxSlots, "characters": views}, nil
}

func (s *Service) handleCreate(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in createRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if !in.complete() {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 slot、name、appearance、stats、elements、hometown 字段")
	}
	if *in.Slot < 0 || *in.Slot >= MaxSlots {
		return nil, protocol.Errorf(protocol.CodeInvalidSlot, "栏位要在 0–%d 之间", MaxSlots-1)
	}
	existing, err := s.findBySlot(ctx, sess.AccountID(), *in.Slot)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, protocol.Errorf(protocol.CodeSlotOccupied, "这个栏位已经有角色了")
	}
	c, perr := validateCreate(&in)
	if perr != nil {
		return nil, perr
	}
	rec, err := s.insert(ctx, sess.AccountID(), c, s.createRnd())
	if err != nil {
		return nil, err
	}
	return map[string]any{"character": rec.view()}, nil
}

// createRnd 返回出生宠物掷资质的随机源：测试里按用例名固定种子，生产环境用全局随机（同 luckRnd）。
func (s *Service) createRnd() func(n int) int {
	if s.opts.CreateRnd != nil {
		return s.opts.CreateRnd
	}
	return rand.IntN
}

type deleteRequest struct {
	Slot *int    `json:"slot"`
	Name *string `json:"name"`
}

func (s *Service) handleDelete(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in deleteRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Slot == nil || in.Name == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 slot 和 name 字段")
	}
	if *in.Slot < 0 || *in.Slot >= MaxSlots {
		return nil, protocol.Errorf(protocol.CodeInvalidSlot, "栏位要在 0–%d 之间", MaxSlots-1)
	}
	// 只在这个账号自己的角色里找，所以删不到别人的角色。
	rec, err := s.findBySlot(ctx, sess.AccountID(), *in.Slot)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, protocol.Errorf(protocol.CodeCharacterNotFound, "这个栏位没有角色")
	}
	if strings.TrimSpace(*in.Name) != rec.Name {
		return nil, protocol.Errorf(protocol.CodeNameMismatch, "输入的名字和角色名不一致，没有删除")
	}
	// 检查“是否在世界中”和删除之间不能让别人进入这个角色（STAGE-16）：先标记“正在删除”，进入世界看到标记就拒绝。
	if perr := s.beginDelete(rec.ID); perr != nil {
		return nil, perr
	}
	defer s.endDelete(rec.ID)
	if h := s.hooks.afterDeleteCheck; h != nil {
		h(rec)
	}
	if _, err := s.deleteByID(ctx, sess.AccountID(), rec.ID); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

// beginDelete 标记角色正在删除。角色正在世界中（任意连接）回复 CHARACTER_IN_USE；
// 已经有另一个删除请求在处理同一个角色，这个栏位马上就没有角色了，回复 CHARACTER_NOT_FOUND。
func (s *Service) beginDelete(id int64) *protocol.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.presences {
		if p.charID == id {
			return protocol.Errorf(protocol.CodeCharacterInUse, "这个角色正在游戏中，不能删除")
		}
	}
	if s.deleting[id] {
		return protocol.Errorf(protocol.CodeCharacterNotFound, "这个栏位没有角色")
	}
	s.deleting[id] = true
	return nil
}

func (s *Service) endDelete(id int64) {
	s.mu.Lock()
	delete(s.deleting, id)
	s.mu.Unlock()
}
