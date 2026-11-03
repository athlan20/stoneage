package protocol

// 通用错误码（docs/protocol.md 第 4 节）和各消息的专用错误码。
const (
	CodePositionChanged = "POSITION_CHANGED"
	CodeExitNotFound    = "EXIT_NOT_FOUND"
	CodeExitUnavailable = "EXIT_UNAVAILABLE"
	CodeBadRequest      = "BAD_REQUEST"
	CodeUnknownType     = "UNKNOWN_TYPE"
	CodeWrongState      = "WRONG_STATE"
	CodeWrongRole       = "WRONG_ROLE"
	CodeRateLimited     = "RATE_LIMITED"
	CodeInternal        = "INTERNAL"
	CodeProtocolVersion = "PROTOCOL_VERSION"
	CodeClientTooOld    = "CLIENT_TOO_OLD" // STAGE-100：客户端版本低于最低版本，docs/protocol.md 5.1
	// STAGE-258：响应线上超过 64 KiB（CDN 限制），服务端不发原响应，改发这个错误，见 docs/protocol.md 第 1 节。
	CodeResponseTooLarge = "RESPONSE_TOO_LARGE"

	// 角色相关（docs/protocol.md 5.5–5.9）。
	CodeInvalidSlot       = "INVALID_SLOT"
	CodeSlotOccupied      = "SLOT_OCCUPIED"
	CodeInvalidName       = "INVALID_NAME"
	CodeNameTaken         = "NAME_TAKEN"
	CodeInvalidAppearance = "INVALID_APPEARANCE"
	CodeInvalidStats      = "INVALID_STATS"
	CodeInvalidElements   = "INVALID_ELEMENTS"
	CodeInvalidHometown   = "INVALID_HOMETOWN"
	CodeCharacterNotFound = "CHARACTER_NOT_FOUND"
	CodeNameMismatch      = "NAME_MISMATCH"
	CodeCharacterInUse    = "CHARACTER_IN_USE"
	CodePlaceNotFound     = "PLACE_NOT_FOUND"   // docs/protocol.md 5.16
	CodeNotEnoughPoints   = "NOT_ENOUGH_POINTS" // docs/protocol.md 5.21
	CodeCharacterDead     = "CHARACTER_DEAD"    // docs/protocol.md 5.21
	CodeNoPet             = "NO_PET"            // docs/protocol.md 5.22–5.24
	CodePetFainted        = "PET_FAINTED"       // docs/protocol.md 5.22
	CodeStandbyFull       = "STANDBY_FULL"      // docs/protocol.md 5.22
	// 骑乘（docs/protocol.md 5.47–5.48，docs/ride.md §3）。
	CodePetRiding      = "PET_RIDING"       // STAGE-123：骑乘中的宠物不能放生、寄放、卖、改状态
	CodeAlreadyRiding  = "ALREADY_RIDING"   // STAGE-123：已经在骑别的宠物
	CodePetNotRest     = "PET_NOT_REST"     // STAGE-123：出战或待命的宠物不能骑
	CodeRideNotLearned = "RIDE_NOT_LEARNED" // STAGE-123：骑乘等级不够宠物的等级
	CodeRideLoyaltyLow = "RIDE_LOYALTY_LOW" // STAGE-123：宠物忠诚不到 100
	CodeRideLevelGap   = "RIDE_LEVEL_GAP"   // STAGE-123：宠物比人物高太多级
	CodeRideTransLimit = "RIDE_TRANS_LIMIT" // STAGE-123：宠物转生超过 2 次（STAGE-196 起检查）
	CodeRideNotMatch   = "RIDE_NOT_MATCH"   // STAGE-123：人物外形和宠物种族不在骑乘表里
	// 分身队（docs/protocol.md 5.57，STAGE-162）。
	CodeAlreadyInParty = "ALREADY_IN_PARTY" // 已有队伍，不能再组
	CodeNotInParty     = "NOT_IN_PARTY"     // 没有队伍可解散
	// 真人组队（docs/protocol.md 5.67，STAGE-274）。
	CodePlayerNotFound  = "PLAYER_NOT_FOUND"  // 没有这个在线角色
	CodePlayerNotNearby = "PLAYER_NOT_NEARBY" // 不在同一张地图
	CodeTargetInBattle  = "TARGET_IN_BATTLE"  // 对方在战斗中
	CodeTargetDead      = "TARGET_DEAD"       // 决斗：对方（或对方队长）已倒下（STAGE-306，协议 5.74）
	CodeTargetInParty   = "TARGET_IN_PARTY"   // 对方已有队伍（只用于邀请）
	CodeTargetNotLeader = "TARGET_NOT_LEADER" // 申请的对象不是队长，或没有队伍
	CodeTargetBusy      = "TARGET_BUSY"       // 对方待处理的请求已满 5 条
	CodeTargetRefuses   = "TARGET_REFUSES"    // 对方不接收组队邀请（STAGE-276，协议 5.69）
	CodePartyFull       = "PARTY_FULL"        // 队伍已满 5 人
	CodeNotPartyLeader  = "NOT_PARTY_LEADER"  // 需要队长身份
	CodeNotPartyMember  = "NOT_PARTY_MEMBER"  // 目标不是队员
	CodePartyInBattle   = "PARTY_IN_BATTLE"   // 队伍战斗中不能变更
	CodePartyFollower   = "PARTY_FOLLOWER"    // 队员不能自己换图、遇敌

	// 交易（STAGE-294，docs/protocol.md 5.72）
	CodeInTrade         = "IN_TRADE"          // 交易中不能做这件事（或发起交易时已经在交易中）
	CodeTargetInTrade   = "TARGET_IN_TRADE"   // 对方已在交易中
	CodeTradeNotFound   = "TRADE_NOT_FOUND"   // 交易不存在、已结束，或不是你的
	CodeTradeStale      = "TRADE_STALE"       // rev 与当前不一致，对方刚改了报价
	CodeItemUntradable  = "ITEM_UNTRADABLE"   // 物品不能交易（VANISHATDROP）
	CodePetUntradable   = "PET_UNTRADABLE"    // 宠物不能交易（守护兽，家族做时启用）
	CodePetFull         = "PET_FULL"          // 宠物栏已满
	CodePetTooStrong    = "PET_TOO_STRONG"    // 宠物比接收方高太多级，收不了
	CodeRequestNotFound = "REQUEST_NOT_FOUND" // 请求不存在、已结束，或不是你的
	CodeRequestPending  = "REQUEST_PENDING"   // 已有一条待处理的同类请求
	// 背包（docs/protocol.md 5.25–5.28）。
	CodeNotEquipment      = "NOT_EQUIPMENT"
	CodeRequirementNotMet = "REQUIREMENT_NOT_MET"
	CodeEmptySlot         = "EMPTY_SLOT"
	CodeBagFull           = "BAG_FULL"
	// 精工（STAGE-298，docs/protocol.md 5.73）。
	CodeInlayTooMany     = "INLAY_TOO_MANY"
	CodeInlayNotSuitable = "INLAY_NOT_SUITABLE"
	CodeInlayMultiTarget = "INLAY_MULTI_TARGET"
	CodeInlayNoTarget    = "INLAY_NO_TARGET"
	CodeInlayFull        = "INLAY_FULL"
	// agent 协同（docs/protocol.md 第 9 节）。
	CodeTokenLimit         = "TOKEN_LIMIT"
	CodeTokenNotFound      = "TOKEN_NOT_FOUND"
	CodeInvalidToken       = "INVALID_TOKEN"
	CodeAgentOffline       = "AGENT_OFFLINE"
	CodePeerOffline        = "PEER_OFFLINE"
	CodeControlledByAgent  = "CONTROLLED_BY_AGENT"
	CodeControlledByPlayer = "CONTROLLED_BY_PLAYER"
	// 战斗（docs/protocol.md 10.5）。
	CodeInBattle         = "IN_BATTLE"
	CodeNotInBattle      = "NOT_IN_BATTLE"
	CodeWrongTurn        = "WRONG_TURN"
	CodeCommandNotNeeded = "COMMAND_NOT_NEEDED"
	CodeCommandOrder     = "COMMAND_ORDER"
	CodeCommandSubmitted = "COMMAND_SUBMITTED"
	CodeInvalidTarget    = "INVALID_TARGET"
	CodeActionNotAllowed = "ACTION_NOT_ALLOWED"
	CodeChatMuted        = "CHAT_MUTED"     // STAGE-278：被管理员禁言，error.data.until_ms
	CodeChatLevelLow     = "CHAT_LEVEL_LOW" // STAGE-278：世界频道等级不够，error.data.min_level
	CodeNotEnoughMP      = "NOT_ENOUGH_MP"
	CodeMagicUnusable    = "MAGIC_UNUSABLE"
	CodeItemUnusable     = "ITEM_UNUSABLE"
	CodeSkillUnusable    = "SKILL_UNUSABLE"
	CodePetUnavailable   = "PET_UNAVAILABLE"
	// 遇敌（docs/protocol.md 5.29）。
	CodeNoEnemyHere = "NO_ENEMY_HERE" // STAGE-40
	// 按区域遇敌（docs/protocol.md 5.51，docs/protocol/encounter-areas.md）。
	CodeEncounterAreaNotFound    = "ENCOUNTER_AREA_NOT_FOUND"   // STAGE-152：本图没有这个区域 ID
	CodeEncounterAreaUnavailable = "ENCOUNTER_AREA_UNAVAILABLE" // STAGE-152：区域存在但当前不可开战
	CodePetEncounterNotFound     = "PET_ENCOUNTER_NOT_FOUND"    // STAGE-251：这只宠物在这个区域没有可遇点
	CodeLevelingSpotNotFound     = "LEVELING_SPOT_NOT_FOUND"    // STAGE-279：这个区域不在练级传送目录里
	// NPC 对话（docs/protocol.md 5.33–5.35）。
	CodeNPCNotFound    = "NPC_NOT_FOUND"    // STAGE-69：没有这个编号的 NPC
	CodeNPCHere        = "NPC_NOT_HERE"     // STAGE-69：NPC 不在角色当前所在地图
	CodeNPCNotTalkable = "NPC_NOT_TALKABLE" // STAGE-69：这类 NPC 还没有对话
	CodeNPCBadChoice   = "NPC_BAD_CHOICE"   // STAGE-69：窗口没有选项，或编号不在选项里
	// 商店（docs/protocol.md 5.38）。
	CodeNotEnoughGold = "NOT_ENOUGH_GOLD" // STAGE-70：金币不够买
	CodeGoldFull      = "GOLD_FULL"       // STAGE-70：金币超过携带上限，不能再卖
)

// MessageInternal 是 INTERNAL 错误统一给玩家看的说明。
const MessageInternal = "服务器内部错误，请稍后再试"

// 推送 disconnect 的原因（docs/protocol.md 5.10）。
const (
	ReasonKickedDuplicateLogin = "KICKED_DUPLICATE_LOGIN"
	ReasonIdleTimeout          = "IDLE_TIMEOUT"
	ReasonServerShutdown       = "SERVER_SHUTDOWN"
	ReasonProtocolError        = "PROTOCOL_ERROR"
	ReasonAgentTokenRevoked    = "AGENT_TOKEN_REVOKED"
	ReasonKickedByPlayer       = "KICKED_BY_PLAYER"
	ReasonAccountDisabled      = "ACCOUNT_DISABLED"
	ReasonRateLimited          = "RATE_LIMITED" // STAGE-351：持续超过按连接限速，断开
)

// DisconnectMessage 返回断开原因对应的中文说明。
func DisconnectMessage(reason string) string {
	switch reason {
	case ReasonKickedDuplicateLogin:
		return "账号在别处登录，你已下线"
	case ReasonIdleTimeout:
		return "长时间没有响应，连接已断开"
	case ReasonServerShutdown:
		return "服务器维护中，连接已断开"
	case ReasonProtocolError:
		return "收到无法处理的数据，连接已断开"
	case ReasonAgentTokenRevoked:
		return "令牌已被吊销或已过期，连接已断开"
	case ReasonKickedByPlayer:
		return "玩家已让你下线"
	case ReasonAccountDisabled:
		return "账号已被禁用，连接已断开"
	case ReasonRateLimited:
		return "请求过于频繁，连接已断开"
	}
	return "连接已断开"
}

// Disconnect 是推送 disconnect 的 data。
type Disconnect struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}
