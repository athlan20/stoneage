//! STAGE-152 遇敌区域：`encounter_areas` 的响应模型与展示用的短标签。
use serde::{Deserialize, Deserializer};

/// 空列表允许服务端写成 null（`#[serde(default)]` 只管字段缺失，不管显式 null）。
fn null_as_default<'de, D, T>(d: D) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::<T>::deserialize(d)?.unwrap_or_default())
}

#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct AreaRect {
    pub x1: i32,
    pub y1: i32,
    pub x2: i32,
    pub y2: i32,
}

#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct AreaPoint {
    pub x: i32,
    pub y: i32,
}

#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct AreaEnemy {
    pub id: u32,
    pub name: String,
    pub level_min: i32,
    pub level_max: i32,
    pub create_max: i32,
    pub weight: i32,
    pub group_id: u32,
}

#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct EncounterArea {
    pub id: String,
    pub number: u32,
    pub rect: AreaRect,
    pub anchor: AreaPoint,
    pub available: bool,
    pub unavailable_reason: Option<String>,
    pub max_enemies: i32,
    pub level_min: i32,
    pub level_max: i32,
    #[serde(default, deserialize_with = "null_as_default")]
    pub enemies: Vec<AreaEnemy>,
    #[serde(default, deserialize_with = "null_as_default")]
    pub required_items: Vec<i32>,
    #[serde(default, deserialize_with = "null_as_default")]
    pub forbidden_items: Vec<i32>,
    /// STAGE-286：本区域能遇到的 1 级宠物名（抓宠点），没有时为空。
    #[serde(default, deserialize_with = "null_as_default")]
    pub level1_pets: Vec<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct AreaList {
    pub floor: u32,
    pub areas: Vec<EncounterArea>,
    pub server_time_ms: i64,
}

impl EncounterArea {
    pub fn label(&self) -> String {
        format!("区域 #{}", self.number)
    }
    pub fn level_label(&self) -> String {
        if self.level_min == self.level_max {
            format!("Lv{}", self.level_min)
        } else {
            format!("Lv{}-{}", self.level_min, self.level_max)
        }
    }
    /// 权重最高的那个敌人，列表行用；同权重时取先出现的。
    pub fn main_enemy(&self) -> Option<&AreaEnemy> {
        self.enemies
            .iter()
            .max_by_key(|e| e.weight)
    }
    pub fn enemies_label(&self) -> String {
        let Some(top) = self.main_enemy() else {
            return "没有可遇敌的敌人".into();
        };
        let lv = if top.level_min == top.level_max {
            format!("Lv{}", top.level_min)
        } else {
            format!("Lv{}-{}", top.level_min, top.level_max)
        };
        if self.enemies.len() > 1 {
            format!("{} {} 等", top.name, lv)
        } else {
            format!("{} {}", top.name, lv)
        }
    }
}
