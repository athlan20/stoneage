//! NPC 对话窗口的推送（`docs/protocol.md` 5.34，STAGE-69）：服务端驱动的窗口，
//! 玩家和 agent 看到同一个；关闭推送带原因。

use serde::Deserialize;

/// 窗口里的一个选项（本期对话没有选项，商店等以后接入）。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct NpcOption {
    #[serde(default)]
    pub no: u32,
    #[serde(default)]
    pub label: String,
}

/// 推送 `npc_window`：一个打开的对话窗口。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcWindow {
    #[serde(default)]
    pub window_id: String,
    #[serde(default)]
    pub stage: String,
    #[serde(default)]
    pub input: Option<NpcInput>,
    #[serde(default)]
    pub trade: Option<NpcTrade>,
    #[serde(default)]
    pub npc: u32,
    #[serde(default)]
    pub name: String,
    #[serde(rename = "type", default)]
    pub type_: String,
    #[serde(default)]
    pub text: String,
    #[serde(default)]
    pub options: Vec<NpcOption>,
    /// 窗口接受文字回复（`reply 文本`，docs/protocol.md 5.34）：任务 NPC 的暗号窗口带，其余没有。
    #[serde(default)]
    pub accepts_text: bool,
    /// 决斗强者表的排行页（docs/protocol.md 5.75，STAGE-308）：只有排行窗口带，其余为 None。
    #[serde(default)]
    pub ranking: Option<NpcRanking>,
}

/// 强者表的一页排行（5.75 的 `ranking` 字段）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcRanking {
    #[serde(default)]
    pub top: Vec<NpcRankRow>,
    #[serde(default)]
    pub page: u32,
    #[serde(default)]
    pub pages: u32,
    #[serde(default)]
    pub me: NpcRankMe,
}

/// 排行里的一个人；`is_self` 对应 JSON 的 `self`（请求者自己）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcRankRow {
    #[serde(default)]
    pub rank: u32,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub duel_point: i64,
    #[serde(rename = "self", default)]
    pub is_self: bool,
}

/// 请求者自己的名次和决斗点数。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcRankMe {
    #[serde(default)]
    pub rank: u32,
    #[serde(default)]
    pub duel_point: i64,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcInput {
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub min: u32,
    #[serde(default)]
    pub max: u32,
    #[serde(default)]
    pub max_chars: usize,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcTrade {
    #[serde(default)]
    pub direction: String,
    #[serde(default)]
    pub item_id: u32,
    #[serde(default)]
    pub slot: i32,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub unit_price: u64,
    #[serde(default)]
    pub count: u32,
    #[serde(default)]
    pub total: u64,
}

/// 推送 `npc_window_closed` 的 `reason`（docs/protocol.md 5.34）。
pub const WINDOW_REASON_CHOSEN: &str = "CHOSEN";
pub const WINDOW_REASON_CLOSED: &str = "CLOSED";
pub const WINDOW_REASON_MOVED: &str = "MOVED";
pub const WINDOW_REASON_BATTLE: &str = "BATTLE";

#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcWindowClosed {
    #[serde(default)]
    pub reason: String,
}
