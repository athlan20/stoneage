package protocol

// State 是连接状态（docs/protocol.md 第 3 节）。
type State uint8

const (
	StateConnected  State = iota // 已连接，只能发 hello
	StateHandshaken              // 已握手
	StateLoggedIn                // 已登录
	StateInWorld                 // 世界中
)

func (s State) String() string {
	return [...]string{"已连接", "已握手", "已登录", "世界中"}[s]
}

// Role 是连接身份，由 hello 声明，之后不能再改。
type Role string

const (
	RolePlayer Role = "player"
	RoleAgent  Role = "agent"
)

// Category 是请求分类，服务端据此判断权限。
type Category uint8

const (
	CategoryConnection Category = iota // 连接本身的维护
	CategoryAccount                    // 账号与角色管理，只允许玩家连接
	CategoryView                       // 只读取数据
	CategoryChat                       // 发言
	CategoryOperation                  // 改变角色或世界
)

func (c Category) String() string {
	return [...]string{"连接", "账号", "查看", "聊天", "操作"}[c]
}

// Meta 描述一种请求：属于哪一类、允许在哪些状态下发、允许哪些身份发。
// 以后追加游戏请求时，必须在这里注明类别。
type Meta struct {
	Type     string
	Category Category
	States   []State
	Roles    []Role
}

// Allows 判断某个状态下是否允许这个请求。
func (m Meta) AllowsState(s State) bool {
	for _, x := range m.States {
		if x == s {
			return true
		}
	}
	return false
}

// AllowsRole 判断某种身份是否允许发这个请求。
func (m Meta) AllowsRole(r Role) bool {
	for _, x := range m.Roles {
		if x == r {
			return true
		}
	}
	return false
}

var (
	anyRole    = []Role{RolePlayer, RoleAgent}
	playerOnly = []Role{RolePlayer}
	agentOnly  = []Role{RoleAgent}

	afterHello = []State{StateHandshaken, StateLoggedIn, StateInWorld}
	loggedIn   = []State{StateLoggedIn, StateInWorld}
)

// Requests 是 docs/protocol.md 里定义的全部请求。服务端还没实现的请求也登记在这里，
// 这样权限和状态检查（如 agent 连接发 login 回复 WRONG_ROLE）从一开始就是对的。
var Requests = []Meta{
	{"hello", CategoryConnection, []State{StateConnected}, anyRole},
	{"ping", CategoryConnection, afterHello, anyRole},

	{"register", CategoryAccount, []State{StateHandshaken}, playerOnly},
	{"login", CategoryAccount, []State{StateHandshaken}, playerOnly},
	{"agent_login", CategoryAccount, []State{StateHandshaken}, agentOnly},

	{"characters", CategoryView, []State{StateLoggedIn}, anyRole},
	{"create_character", CategoryAccount, []State{StateLoggedIn}, playerOnly},
	{"delete_character", CategoryAccount, []State{StateLoggedIn}, playerOnly},
	{"enter_world", CategoryOperation, []State{StateLoggedIn}, anyRole},
	{"leave_world", CategoryOperation, []State{StateInWorld}, anyRole},
	{"status", CategoryView, []State{StateInWorld}, anyRole},
	{"turn", CategoryOperation, []State{StateInWorld}, anyRole},
	{"places", CategoryView, []State{StateInWorld}, anyRole},
	{"maps", CategoryView, []State{StateInWorld}, anyRole},
	{"npcs", CategoryView, []State{StateInWorld}, anyRole},            // 查看本地图 NPC（STAGE-68）
	{"npc_find", CategoryView, []State{StateInWorld}, anyRole},        // 按名字找 NPC（STAGE-84）
	{"players", CategoryView, []State{StateInWorld}, anyRole},         // 查看同图的其他玩家（STAGE-273）
	{"npc_list", CategoryView, []State{StateInWorld}, anyRole},        // NPC 目录：分类、搜索、分页（STAGE-134）
	{"talk", CategoryOperation, []State{StateInWorld}, anyRole},       // 和 NPC 对话（STAGE-69）：战斗中 IN_BATTLE
	{"npc_choose", CategoryOperation, []State{StateInWorld}, anyRole}, // 回应对话窗口（STAGE-69）
	{"npc_open", CategoryOperation, []State{StateInWorld}, anyRole},
	{"quest_npcs", CategoryView, []State{StateInWorld}, anyRole},
	{"npc_confirm", CategoryOperation, []State{StateInWorld}, anyRole},
	{"npc_back", CategoryOperation, []State{StateInWorld}, anyRole},
	{"npc_reply", CategoryOperation, []State{StateInWorld}, anyRole},  // 窗口的文字回复（STAGE-70）：数量等
	{"npc_close", CategoryConnection, []State{StateInWorld}, anyRole}, // 关闭对话窗口（STAGE-69）：确认类，观察方也可以发
	{"exits", CategoryView, []State{StateInWorld}, anyRole},
	{"use_exit", CategoryOperation, []State{StateInWorld}, anyRole},
	{"warp", CategoryOperation, []State{StateInWorld}, anyRole},
	{"inventory", CategoryView, []State{StateInWorld}, anyRole},
	{"pets", CategoryView, []State{StateInWorld}, anyRole},
	{"quests", CategoryView, []State{StateInWorld}, anyRole}, // 查看任务进度（STAGE-86）
	{"pet_state", CategoryOperation, []State{StateInWorld}, anyRole},
	{"pet_rename", CategoryOperation, []State{StateInWorld}, anyRole},
	{"pet_drop", CategoryOperation, []State{StateInWorld}, anyRole},
	{"pet_ride", CategoryOperation, []State{StateInWorld}, anyRole},
	{"pet_unride", CategoryOperation, []State{StateInWorld}, anyRole},
	{"pet_talk", CategoryOperation, []State{StateInWorld}, anyRole},        // 和宠物对话（STAGE-239）
	{"pet_skill", CategoryOperation, []State{StateInWorld}, anyRole},       // 战斗外使用宠物技能（STAGE-144）：战斗中回复 IN_BATTLE
	{"magic_use", CategoryOperation, []State{StateInWorld}, anyRole},       // 战斗外使用精灵（STAGE-203，目前只有变幻的精灵）：战斗中回复 IN_BATTLE
	{"party", CategoryView, []State{StateInWorld}, anyRole},                // 查看队伍（STAGE-162）
	{"party_clone", CategoryOperation, []State{StateInWorld}, anyRole},     // 组成分身队（STAGE-162）：战斗中 IN_BATTLE
	{"party_disband", CategoryOperation, []State{StateInWorld}, anyRole},   // 解散分身队（STAGE-162）：战斗中 IN_BATTLE
	{"party_invite", CategoryOperation, []State{StateInWorld}, anyRole},    // 邀请组真人队（STAGE-274）
	{"trade_invite", CategoryOperation, []State{StateInWorld}, anyRole},    // 邀请交易（STAGE-294，docs/protocol.md 5.72）
	{"trade_reply", CategoryOperation, []State{StateInWorld}, anyRole},     // 答复交易邀请（STAGE-294）
	{"trade_cancel", CategoryOperation, []State{StateInWorld}, anyRole},    // 撤回邀请或取消交易（STAGE-294）
	{"trade_offer", CategoryOperation, []State{StateInWorld}, anyRole},     // 修改自己的报价（STAGE-294）
	{"trade_lock", CategoryOperation, []State{StateInWorld}, anyRole},      // 锁定报价（STAGE-294，第三级：先问玩家）
	{"trade_unlock", CategoryOperation, []State{StateInWorld}, anyRole},    // 取消锁定（STAGE-294）
	{"trade_confirm", CategoryOperation, []State{StateInWorld}, anyRole},   // 确定交易（STAGE-294，第三级：先问玩家）
	{"party_join", CategoryOperation, []State{StateInWorld}, anyRole},      // 申请加入真人队（STAGE-274）
	{"party_reply", CategoryOperation, []State{StateInWorld}, anyRole},     // 答复邀请或申请（STAGE-274）
	{"party_cancel", CategoryOperation, []State{StateInWorld}, anyRole},    // 撤回自己发出的请求（STAGE-274）
	{"party_leave", CategoryOperation, []State{StateInWorld}, anyRole},     // 离开真人队（STAGE-274）
	{"party_kick", CategoryOperation, []State{StateInWorld}, anyRole},      // 请出真人队员（STAGE-274）
	{"party_leader", CategoryOperation, []State{StateInWorld}, anyRole},    // 转让真人队长（STAGE-277）
	{"social_settings", CategoryOperation, []State{StateInWorld}, anyRole}, // 隐身与拒绝邀请（STAGE-276，协议 5.69）
	{"duel_challenge", CategoryOperation, []State{StateInWorld}, anyRole},  // 发起决斗（STAGE-306，协议 5.74）
	{"duel_ranking", CategoryView, []State{StateInWorld}, anyRole},         // 决斗强者表（STAGE-308，协议 5.75）
	{"addpoint", CategoryOperation, []State{StateInWorld}, anyRole},
	{"item_equip", CategoryOperation, []State{StateInWorld}, anyRole},
	{"item_unequip", CategoryOperation, []State{StateInWorld}, anyRole},
	{"item_move", CategoryOperation, []State{StateInWorld}, anyRole},
	{"item_drop", CategoryOperation, []State{StateInWorld}, anyRole},
	{"gold_drop", CategoryOperation, []State{StateInWorld}, anyRole},  // 丢弃石币（STAGE-328，5.76）：战斗中回复 IN_BATTLE
	{"item_use", CategoryOperation, []State{StateInWorld}, anyRole},   // 使用消耗品（STAGE-30）：战斗中回复 IN_BATTLE
	{"item_inlay", CategoryOperation, []State{StateInWorld}, anyRole}, // 精工（STAGE-298，5.73）：战斗中回复 IN_BATTLE

	{"create_agent_token", CategoryAccount, loggedIn, playerOnly},
	{"agent_tokens", CategoryAccount, loggedIn, playerOnly},
	{"revoke_agent_token", CategoryAccount, loggedIn, playerOnly},
	{"take_control", CategoryAccount, []State{StateInWorld}, playerOnly},
	{"release_control", CategoryAccount, []State{StateInWorld}, playerOnly},
	{"kick_agent", CategoryAccount, []State{StateInWorld}, playerOnly},
	{"agent_say", CategoryChat, loggedIn, anyRole},
	{"agent_plan", CategoryChat, loggedIn, agentOnly},      // agent 报告计划（STAGE-119）：整份替换，不受控制权限制
	{"chat", CategoryChat, []State{StateInWorld}, anyRole}, // 公开聊天 map/world（STAGE-278，docs/protocol.md 5.68）：战斗中也能发，不受控制权限限制

	// 战斗（docs/protocol.md 第 10 章）。
	{"battle_command", CategoryOperation, []State{StateInWorld}, anyRole}, // 操作类：观察方被拒
	{"battle_ack", CategoryConnection, []State{StateInWorld}, anyRole},    // 确认类：观察方也可以发
	{"battle", CategoryView, []State{StateInWorld}, anyRole},              // 查看类

	// 遇敌（docs/protocol.md 5.29，STAGE-40）。
	{"encounter", CategoryOperation, []State{StateInWorld}, anyRole}, // 操作类：观察方被拒
	// 按区域遇敌（docs/protocol.md 5.50 / 5.51，STAGE-152）。
	{"encounter_areas", CategoryView, []State{StateInWorld}, anyRole},          // 查看类
	{"encounter_start", CategoryOperation, []State{StateInWorld}, anyRole},     // 操作类：观察方被拒
	{"pet_encounter_catalog", CategoryView, []State{StateInWorld}, anyRole},    // 查看类（STAGE-251）
	{"pet_encounter_start", CategoryOperation, []State{StateInWorld}, anyRole}, // 操作类：观察方被拒（STAGE-251）
	{"leveling_catalog", CategoryView, []State{StateInWorld}, anyRole},         // 查看类（STAGE-279）
	{"leveling_start", CategoryOperation, []State{StateInWorld}, anyRole},      // 操作类：观察方被拒（STAGE-279）
}

var byType = func() map[string]Meta {
	m := make(map[string]Meta, len(Requests))
	for _, r := range Requests {
		m[r.Type] = r
	}
	return m
}()

// Lookup 按 type 查请求元数据。
func Lookup(typ string) (Meta, bool) {
	m, ok := byType[typ]
	return m, ok
}
