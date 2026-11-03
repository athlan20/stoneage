//! STAGE-295：交易的请求、交易对象与推送（`docs/protocol.md` 5.72）。服务端没给的字段取默认值，多出来的字段忽略。

use serde::Deserialize;

/// 请求对象（`trade_invite` 的响应，`trade_request` 推送多 `from_level`、`from_transmigration`）。
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeRequest {
    pub id: String,
    pub from: String,
    pub to: String,
    pub expires_in: u64,
    pub from_level: u32,
    pub from_transmigration: u32,
}

impl Default for TradeRequest {
    fn default() -> Self {
        Self {
            id: String::new(),
            from: String::new(),
            to: String::new(),
            expires_in: 30,
            from_level: 0,
            from_transmigration: 0,
        }
    }
}

/// `trade_invite` 的响应与 `trade_request` 推送：`{"request": 请求对象}`。
#[derive(Debug, Clone, Deserialize)]
pub struct TradeRequestEnvelope {
    pub request: TradeRequest,
}

/// 推送 `trade_request_result`：`status` 为 accepted / declined / expired / cancelled，取消时 `code` 写原因。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeRequestResult {
    pub id: String,
    pub status: String,
    pub from: String,
    pub to: String,
    pub code: String,
}

/// 报价里的一件物品。`slot` 只有自己的报价有（对方的没有）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeItem {
    pub slot: Option<u32>,
    pub item_id: u32,
    pub name: String,
    pub effect: String,
    pub count: u32,
    pub durability: u32,
    pub max_durability: u32,
    pub crush_level: u32,
}

/// 报价里的一只宠物（只取界面要用的字段）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradePet {
    pub slot: Option<u32>,
    pub name: String,
    pub level: u32,
}

/// 一方的报价和状态。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeSide {
    pub name: String,
    pub items: Vec<TradeItem>,
    pub gold: u64,
    pub pet: Option<TradePet>,
    pub locked: bool,
    pub confirmed: bool,
}

/// 报价（结束摘要里的 `gave` / `got`）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeOffer {
    pub items: Vec<TradeItem>,
    pub gold: u64,
    pub pet: Option<TradePet>,
}

/// 回到编辑阶段时的失败原因：`who` 为 `me` 或 `peer`。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeLastError {
    pub code: String,
    pub who: String,
}

/// 交易对象。`me` 和 `peer` 已由服务端按接收者视角换好。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct Trade {
    pub id: String,
    pub rev: u64,
    pub idle_expires_in: u64,
    pub me: TradeSide,
    pub peer: TradeSide,
    pub last_error: Option<TradeLastError>,
}

/// `trade_reply` 接受、`trade_offer` 等的响应：`{"trade": 交易对象}`。
#[derive(Debug, Clone, Deserialize)]
pub struct TradeEnvelope {
    pub trade: Trade,
}

/// `trade_updated` 推送：`{"trade": 交易对象}`。
pub type TradeUpdated = TradeEnvelope;

/// 推送 `trade_ended`：`status` 为 completed 或 cancelled；`code` 取消原因；`summary` 只在 completed 时有。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeEnded {
    pub id: String,
    pub status: String,
    pub by: Option<String>,
    pub code: Option<String>,
    pub summary: Option<TradeSummary>,
}

/// 交换结果：`gave` 是自己付出的，`got` 是得到的。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct TradeSummary {
    pub gave: TradeOffer,
    pub got: TradeOffer,
}
