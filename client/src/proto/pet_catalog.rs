//! STAGE-251：1 级宠物目录（协议 5.62 `pet_encounter_catalog`，`docs/protocol.md`）。
//! 服务端一次给全量，客户端缓存后在本地筛选；`pets` 已按 `sort_key` 排好，这里不改序。

use serde::Deserialize;

/// `pet_encounter_catalog` 的回复。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct PetCatalog {
    #[serde(default)]
    pub catalog_version: String,
    #[serde(default)]
    pub pets: Vec<PetEntry>,
    #[serde(default)]
    pub spots: Vec<PetSpot>,
}

/// 目录里的一只宠物（一个模板）。`aliases` 与 `series` 可能为空，显示时“有就显示”。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct PetEntry {
    pub pet_id: i64,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub aliases: Vec<String>,
    #[serde(default)]
    pub series: String,
    #[serde(default)]
    pub atoms: PetAtoms,
    #[serde(default)]
    pub size: i64,
    #[serde(default)]
    pub catch_difficulty: i64,
    #[serde(default)]
    pub spot_count: i64,
}

/// 地水火风四元素（只用于详情）。
#[derive(Debug, Clone, Copy, Default, Deserialize, PartialEq, Eq)]
pub struct PetAtoms {
    #[serde(default)]
    pub earth: i64,
    #[serde(default)]
    pub water: i64,
    #[serde(default)]
    pub fire: i64,
    #[serde(default)]
    pub wind: i64,
}

/// 一个可遇点：宠物在某个遇敌区域能遇到，`anchor` 是开战时的落点。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct PetSpot {
    pub pet_id: i64,
    pub area_id: String,
    #[serde(default)]
    pub number: i64,
    pub floor: u32,
    #[serde(default)]
    pub map_name: String,
    #[serde(default)]
    pub anchor: PetPoint,
    #[serde(default)]
    pub rect: PetRect,
    #[serde(default)]
    pub max_enemies: i64,
    #[serde(default)]
    pub steps: i64,
    #[serde(default)]
    pub also_level1: Vec<String>,
}

#[derive(Debug, Clone, Copy, Default, Deserialize, PartialEq, Eq)]
pub struct PetPoint {
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
}

#[derive(Debug, Clone, Copy, Default, Deserialize, PartialEq, Eq)]
pub struct PetRect {
    #[serde(default)]
    pub x1: i32,
    #[serde(default)]
    pub y1: i32,
    #[serde(default)]
    pub x2: i32,
    #[serde(default)]
    pub y2: i32,
}

impl PetRect {
    /// 闭区间矩形是否包含这个格子（同服务端 Contains）。
    pub fn contains(&self, x: i32, y: i32) -> bool {
        x >= self.x1 && x <= self.x2 && y >= self.y1 && y <= self.y2
    }
}
