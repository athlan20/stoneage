//! STAGE-148 ordinary exits; destinations remain server-authoritative.
use serde::Deserialize;
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct TriggerCell {
    pub x: i32,
    pub y: i32,
}
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct MapExit {
    pub id: String,
    pub to_floor: u32,
    pub to_name: String,
    pub trigger_cells: Vec<TriggerCell>,
    pub time_gate: String,
    pub available: bool,
    pub unavailable_reason: Option<String>,
    pub one_way: bool,
}
#[derive(Debug, Clone, Deserialize)]
pub struct ExitList {
    pub floor: u32,
    pub exits: Vec<MapExit>,
    pub server_time_ms: i64,
    pub next_change_ms: Option<i64>,
}
impl MapExit {
    pub fn label(&self) -> String {
        if self.to_name.is_empty() {
            format!("地图 #{}", self.to_floor)
        } else {
            self.to_name.clone()
        }
    }
    pub fn gate_label(&self) -> &str {
        match self.time_gate.as_str() {
            "morning" => "早晨开放",
            "noon" => "白天开放",
            "night" => "夜晚开放",
            _ => "全天",
        }
    }
}
