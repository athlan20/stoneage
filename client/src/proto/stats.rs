//! 角色页用到的能力、衍生数值和属性（STAGE-23，`docs/protocol.md` 5.8 的 `status` 对象）。
//! 衍生数值由服务端算好，客户端只显示，这里不做任何计算。

use serde::Deserialize;

use super::message::Status;

/// 推送 `status_changed`（`docs/protocol.md` 10.10）：角色数值变了，`status` 是推送这一刻的完整数值。
/// 加点（STAGE-24）和管理命令发点数都走这条推送。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct StatusChanged {
    #[serde(default)]
    pub status: Status,
}

/// 四项能力点（体力、腕力、耐久力、速度），键名与 `create_character` 的 `stats` 相同。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct AbilityPoints {
    #[serde(default)]
    pub vital: u32,
    #[serde(default)]
    pub strength: u32,
    #[serde(default)]
    pub toughness: u32,
    #[serde(default)]
    pub dexterity: u32,
}

/// 衍生数值：攻击、防御、敏捷、魅力、运气。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct DerivedStats {
    #[serde(default)]
    pub attack: u32,
    #[serde(default)]
    pub defense: u32,
    #[serde(default)]
    pub agility: u32,
    #[serde(default)]
    pub charm: u32,
    #[serde(default)]
    pub luck: u32,
}

/// 四项属性点（地、水、火、风），键名与 `create_character` 的 `elements` 相同。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct ElementPoints {
    #[serde(default)]
    pub earth: u32,
    #[serde(default)]
    pub water: u32,
    #[serde(default)]
    pub fire: u32,
    #[serde(default)]
    pub wind: u32,
}
