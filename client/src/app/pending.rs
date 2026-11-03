//! 发出去、还在等响应的请求（`Pending`），以及握手后要执行的动作（`Action`）。

use super::battle::auto_post::PostOp;
use super::items::ItemOp;
use super::pets_manage::PetOp;

/// 等连接就绪后要执行的动作。
pub enum Action {
    /// 只握手，用来得知服务端是否开放注册。
    Probe,
    Login {
        account: String,
        password: String,
    },
    Register {
        account: String,
        password: String,
    },
}

/// 已发出、还在等响应的请求。
#[derive(Debug, Clone)]
pub enum Pending {
    Hello,
    Login {
        account: String,
    },
    Register {
        account: String,
    },
    Characters,
    Create,
    Delete,
    Enter,
    Leave,
    Ping,
    Status,
    Turn,
    Places,
    /// 能传到的地图（进入世界取一次，STAGE-83）。
    Maps,
    Inventory,
    Pets,
    /// 任务进度（进入世界取一次，STAGE-86）。
    Quests,
    QuestNpcs { no: u32, state: String, query: u64 },
    /// 当前地图上的 NPC（进入世界取一次，换地图重取，STAGE-68）。
    Npcs(super::scene_state::SceneQuery),
    Exits(super::scene_state::SceneQuery),
    /// 本地图的其他在线玩家（STAGE-273，`docs/protocol.md` 5.66）：结果填进附近玩家列。
    Players(super::scene_state::SceneQuery),
    /// 本地图的遇敌区域（STAGE-152）。
    EncounterAreas(super::scene_state::SceneQuery),
    /// 按区域开战（STAGE-152）：成功后由推送更新坐标与战斗，客户端不提前改。
    EncounterStart,
    /// 停止遇敌（STAGE-152，`encounter off` 的复用）。
    EncounterStop,
    UseExit,
    /// 穿脱整理（穿、卸、移动、丢弃）：成功后写进消息栏（STAGE-29，`docs/protocol.md` 5.25–5.28）。
    ItemOp(ItemOp),
    /// 丢弃石币（STAGE-328，`docs/protocol.md` 5.76）：成功后写一句话，记下丢了多少。
    GoldDrop { amount: u64 },
    /// 精工（STAGE-299，`docs/protocol.md` 5.73）：成功写一句话，失败写服务端台词（黄色）。
    ItemInlay(super::inventory_inlay::InlayOp),
    /// 战后补血与丢弃（STAGE-260）：每一步一条请求，回包由 `auto_post` 接着判断下一步。
    AutoPost(PostOp),
    /// 传送的地点名：成功后写进消息栏（位置变化本身由 `position_changed` 推送告诉界面）。
    Warp(String),
    /// 决斗挑战（STAGE-307，5.74）：成功后由 `battle_start` 推送接管开战，响应只回 battle_id；失败写服务端的原因。
    DuelChallenge,
    /// 1 级宠物目录（STAGE-251，5.62）：结果填进目录页，失败写原因。
    PetCatalog,
    /// 按 1 级宠物开战或只传送（STAGE-251，5.63）：成功写消息、回场景；失败留在原层（原因由通用处理写进消息栏）。
    PetStart { map: String, mode: String },
    /// 练级传送目录（STAGE-279，5.70）：segment 为 None 时取摘要，Some(段下标) 时取这一段的练级点。
    LevelingCatalog { segment: Option<usize> },
    /// 按练级点开战或只传送（STAGE-279，5.71）：成功写消息、回场景；失败留在原层。
    LevelingStart { map: String, mode: String },
    /// `npc 名字` 的查找（STAGE-84）：结果在 `on_npc_find` 里决定对话、问是否传送，还是列出地图。
    NpcFind(String),
    /// NPC 目录（STAGE-134）：结果填进场景的目录页。
    NpcList {query:u64,offset:usize},
    /// 确认后去 NPC 所在的地图（STAGE-84）：成功后写一句话并直接和 NPC 对话。
    NpcWarp {
        npc: u32,
        map_name: String,
    },
    /// 和 NPC 对话（STAGE-69）：窗口本身由 `npc_window` 推送带来，响应没有内容。
    Talk,
    /// 回应对话窗口的选项（STAGE-69）：成功后窗口随 `npc_window_closed`(CHOSEN) 关闭。
    NpcChoose,
    /// 窗口的文字回复（STAGE-70）：商店报数量等，结果由推送带来。
    NpcReply,
    /// 关闭对话窗口（STAGE-69）：服务端幂等，客户端本地已经先清了窗口。
    NpcClose,
    /// 加点的能力（界面上的说法）和点数：成功后写进消息栏（STAGE-24，`docs/protocol.md` 5.21）。
    AddPoint {
        stat: String,
        points: u32,
    },
    /// 宠物管理（设状态、改名、放生）：成功后写进消息栏（STAGE-32，`docs/protocol.md` 5.22–5.24）。
    PetOp(PetOp),
    /// 战斗外使用精灵（STAGE-203，5.51）：变身提示由服务端推送（notice），响应本身没有内容。
    MagicUse,
    /// 分身队（STAGE-162）：组队、解散；结果由 `party_changed` 推送告诉界面。
    PartyOp(super::party::PartyOp),
    /// 真人组队（STAGE-274，5.67）：邀请、申请、答复、撤回、离队、踢人；结果由响应或推送告诉界面。
    PartyPlayers(super::party_players::PlayersOp),
    /// 交易（STAGE-295，5.72）：邀请、答复、撤回、报价、锁定、确定；结果由响应或推送告诉界面。
    Trade(super::trade::TradeOp),
    /// 社交开关（STAGE-276，5.69）：改哪个开关、改成什么；结果由响应告诉界面（推送也会改显示）。
    SocialSettings { field: super::social_settings::SocialField, value: bool },
    Take,
    Release,
    KickAgent,
    /// 发出的私聊文本：服务端不会把私聊推回给发送方，成功后由客户端自己把“你: 文本”写进消息栏（STAGE-17）。
    AgentSay(String),
    /// 公开聊天（STAGE-278）：发出的频道和文字，成功时清框，失败时保留文字并给原因。
    Chat { channel: crate::proto::ChatChannel, text: String },
    TokenCreate,
    TokenList,
    TokenRevoke,
    /// 战斗指令：回复里的 `next` 决定下一步；失败就留在原来的阶段（STAGE-38）。
    BattleCommand { turn: u32 },
    /// 给分身指定攻击目标（STAGE-186）：成功后记下目标，失败留在选目标。
    BattleClone { turn: u32, who: u8, target: u8 },
    BattleAck,
    /// 文字命令 `battle`：查看当前战斗。
    BattleQuery,
    /// 遇敌开关：Some(on/off) 是手动开关（成功后写进消息栏），None 是查询（STAGE-40）。
    Encounter {
        set: Option<bool>,
    },
}
