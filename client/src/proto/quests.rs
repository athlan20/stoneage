use serde::Deserialize;

/// 一个任务的进度（`docs/protocol.md` 5.44）。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct Quest {
    /// 任务编号（原版 jobdaily.txt 的编号）。
    #[serde(default)]
    pub no: u32,
    /// 任务名，服务端已转成 UTF-8。
    #[serde(default)]
    pub name: String,
    /// `doing`（进行中）或 `done`（已完成）。
    #[serde(default)]
    pub state: String,
}

/// `quests` 响应与 `quests_changed` 推送（`docs/protocol.md` 5.44、5.45）：`quests` 是全量列表，
/// `changed` 只在推送里有，列出这次变化的任务编号。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct Quests {
    #[serde(default)]
    pub quests: Vec<Quest>,
    #[serde(default)]
    pub changed: Vec<u32>,
}
