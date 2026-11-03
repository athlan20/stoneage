//! STAGE-274：真人组队的请求与推送（`docs/protocol.md` 5.67）。服务端没给的字段取默认值，多出来的字段忽略。

use serde::Deserialize;

/// 请求对象（邀请或申请）。`kind` 为 `invite`（邀请）或 `apply`（申请）；`expires_in` 固定 30 秒。
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct PartyRequest {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub from: String,
    #[serde(default)]
    pub to: String,
    /// 推送给对象时带的队伍摘要；没有队伍时为 null。
    #[serde(default)]
    pub party: Option<PartyBrief>,
    #[serde(default = "default_expires_in")]
    pub expires_in: u64,
}

fn default_expires_in() -> u64 {
    30
}

/// 请求里的队伍摘要：队长名、人数、上限。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyBrief {
    #[serde(default)]
    pub leader: String,
    #[serde(default)]
    pub size: u32,
    #[serde(default)]
    pub max: u32,
}

/// 推送 `party_request` 的 data，以及 `party_invite` / `party_join` 的回复，都是 `{"request": 请求对象}`。
#[derive(Debug, Clone, Deserialize)]
pub struct PartyRequestEnvelope {
    pub request: PartyRequest,
}

/// 推送 `party_request_result`：`status` 为 accepted / declined / expired / cancelled，取消时 `code` 写原因。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyRequestResult {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub from: String,
    #[serde(default)]
    pub to: String,
    #[serde(default)]
    pub code: String,
}
