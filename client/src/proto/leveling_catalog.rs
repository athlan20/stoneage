//! STAGE-279：练级传送目录（协议 5.70 `leveling_catalog`，`docs/protocol.md`）。
//! `{}` 只给 13 段摘要；带 `segment`（段下限）再给这一段的练级点。

use serde::Deserialize;

/// `leveling_catalog` 的回复。`segment` / `entries` 只在带 `segment` 请求时出现。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingCatalog {
    #[serde(default)]
    pub catalog_version: String,
    #[serde(default)]
    pub segments: Vec<LevelingSegment>,
    #[serde(default)]
    pub segment: Option<LevelingSegment>,
    #[serde(default)]
    pub entries: Option<Vec<LevelingEntry>>,
}

/// 一段（10 级）的摘要。`min` 是段下限（1、11、…、121）。
#[derive(Debug, Clone, Copy, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingSegment {
    pub min: i64,
    pub max: i64,
    #[serde(default)]
    pub entry_count: i64,
    #[serde(default)]
    pub map_count: i64,
}

/// 一个练级点：同段内怪物组合相同的区域合在一起。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingEntry {
    #[serde(default)]
    pub monsters: Vec<LevelingMonster>,
    #[serde(default)]
    pub spots: Vec<LevelingSpot>,
}

/// 练级点里的一种怪物（段内裁切后的等级）。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingMonster {
    pub name: String,
    #[serde(default)]
    pub level_min: i64,
    #[serde(default)]
    pub level_max: i64,
}

/// 练级点的一个落点（一个遇敌区域）。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingSpot {
    pub area_id: String,
    #[serde(default)]
    pub number: i64,
    pub floor: u32,
    #[serde(default)]
    pub map_name: String,
    #[serde(default)]
    pub anchor: LevelingPoint,
    #[serde(default)]
    pub rect: LevelingRect,
    #[serde(default)]
    pub max_enemies: i64,
    #[serde(default)]
    pub steps: i64,
}

#[derive(Debug, Clone, Copy, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingPoint {
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
}

#[derive(Debug, Clone, Copy, Default, Deserialize, PartialEq, Eq)]
pub struct LevelingRect {
    #[serde(default)]
    pub x1: i32,
    #[serde(default)]
    pub y1: i32,
    #[serde(default)]
    pub x2: i32,
    #[serde(default)]
    pub y2: i32,
}

impl LevelingRect {
    /// 闭区间矩形是否包含这个格子（同服务端 Contains）。
    pub fn contains(&self, x: i32, y: i32) -> bool {
        x >= self.x1 && x <= self.x2 && y >= self.y1 && y <= self.y2
    }
}
