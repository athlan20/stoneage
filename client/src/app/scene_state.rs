//! STAGE-148/152: independent columns, query generations and the encounter action prompt.
use super::players_state::PlayersState;
use crate::proto::{EncounterArea, MapExit};
use std::time::Instant;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum LoadState {
    #[default]
    Loading,
    Ready,
    Failed(String),
}
impl LoadState {
    pub fn placeholder(&self, empty: bool) -> Option<String> {
        match self {
            Self::Loading => Some("加载中…".into()),
            Self::Failed(e) => Some(format!("加载失败：{e} · F5 重试")),
            Self::Ready if empty => Some("确实为空".into()),
            _ => None,
        }
    }
    /// 遇敌列的空状态要说得更具体（“本地图没有可遇敌区域”）。
    pub fn area_placeholder(&self, empty: bool) -> Option<String> {
        match self {
            Self::Loading => Some("加载中…".into()),
            Self::Failed(e) => Some(format!("加载失败：{e} · F5 重试")),
            Self::Ready if empty => Some("本地图没有可遇敌区域".into()),
            _ => None,
        }
    }
}

/// 场景页的四列（STAGE-152 三列，STAGE-273 加「附近玩家」）：出口、NPC、附近玩家、遇敌。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum SceneColumn {
    #[default]
    Exit,
    Npc,
    Players,
    Encounter,
}
impl SceneColumn {
    pub fn left(self) -> Self {
        match self {
            Self::Exit => Self::Exit,
            Self::Npc => Self::Exit,
            Self::Players => Self::Npc,
            Self::Encounter => Self::Players,
        }
    }
    pub fn right(self) -> Self {
        match self {
            Self::Exit => Self::Npc,
            Self::Npc => Self::Players,
            Self::Players => Self::Encounter,
            Self::Encounter => Self::Encounter,
        }
    }
    pub fn is_encounter(self) -> bool {
        self == Self::Encounter
    }
}

/// 选中可用区域后弹出的动作面板（STAGE-152）；默认“遇敌 1 次”。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum EncounterChoice {
    #[default]
    Once,
    Infinite,
    Cancel,
}
impl EncounterChoice {
    pub fn label(self) -> &'static str {
        match self {
            Self::Once => "遇敌 1 次",
            Self::Infinite => "无限遇敌",
            Self::Cancel => "取消",
        }
    }
    pub fn mode(self) -> Option<&'static str> {
        match self {
            Self::Once => Some("once"),
            Self::Infinite => Some("infinite"),
            Self::Cancel => None,
        }
    }
    pub fn up(self) -> Self {
        match self {
            Self::Once => Self::Once,
            Self::Infinite => Self::Once,
            Self::Cancel => Self::Infinite,
        }
    }
    pub fn down(self) -> Self {
        match self {
            Self::Once => Self::Infinite,
            Self::Infinite => Self::Cancel,
            Self::Cancel => Self::Cancel,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct EncounterPrompt {
    pub choice: EncounterChoice,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SceneQuery {
    pub floor: u32,
    pub epoch: u64,
    pub query: u64,
}
#[derive(Debug, Default)]
pub struct SceneState {
    pub column: SceneColumn,
    pub left_touched: bool,
    pub npc_index: usize,
    pub npc_top: usize,
    pub area_index: usize,
    pub area_top: usize,
    pub rows: usize,
    /// 附近玩家列（STAGE-273）：同图其他在线玩家，`players` 整表加 `players_changed` 增量。
    pub players: PlayersState,
    pub exits: Vec<MapExit>,
    pub areas: Vec<EncounterArea>,
    pub exit_load: LoadState,
    pub npc_load: LoadState,
    pub area_load: LoadState,
    pub epoch: u64,
    pub exit_query: u64,
    pub npc_query: u64,
    pub area_query: u64,
    pub refresh_due: Option<Instant>,
    pub exit_requested_at: Option<Instant>,
    pub using_exit: bool,
    pub details: bool,
    pub detail_scroll: u16,
    pub prompt: Option<EncounterPrompt>,
    pub starting_encounter: bool,
}
impl SceneState {
    pub fn accepts(&self, context: SceneQuery, floor: u32, column: SceneColumn) -> bool {
        let query = match column {
            SceneColumn::Exit => self.exit_query,
            SceneColumn::Npc => self.npc_query,
            SceneColumn::Players => self.players.query,
            SceneColumn::Encounter => self.area_query,
        };
        context.floor == floor && context.epoch == self.epoch && context.query == query
    }
    pub fn invalidate(&mut self) {
        self.epoch += 1;
        self.left_touched = false;
        self.exit_query = 0;
        self.npc_query = 0;
        self.area_query = 0;
        self.players.invalidate();
        self.exits.clear();
        self.areas.clear();
        self.exit_load = LoadState::Loading;
        self.npc_load = LoadState::Loading;
        self.area_load = LoadState::Loading;
        self.refresh_due = None;
        self.npc_index = 0;
        self.npc_top = 0;
        self.area_index = 0;
        self.area_top = 0;
        self.details = false;
        self.prompt = None;
        // using_exit is cleared only by its response: position push arrives first.
    }
    /// 当前列的选中项数量。
    pub fn len(&self, npc_count: usize) -> usize {
        match self.column {
            SceneColumn::Exit => self.exits.len() + 3,
            SceneColumn::Npc => npc_count,
            // 末尾多一行固定的「聊天…」（STAGE-278），编号等于玩家数。
            SceneColumn::Players => self.players.len() + 1,
            // 遇敌列末尾固定一个「停止遇敌」，不占区域编号（STAGE-152）。
            SceneColumn::Encounter => self.areas.len() + 1,
        }
    }
    /// 遇敌列的选中项是不是末尾那个固定的「停止遇敌」。
    pub fn stop_selected(&self) -> bool {
        self.column.is_encounter() && self.area_index >= self.areas.len()
    }
    pub fn selected_area(&self) -> Option<&EncounterArea> {
        self.areas.get(self.area_index)
    }
}
