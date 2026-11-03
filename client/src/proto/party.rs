//! 分身队（STAGE-162，`docs/protocol.md` 5.57）：`party` 的队伍对象和推送 `party_changed`。
//! 服务端没给的字段取默认值；以后多出来的字段忽略（第 1 节）。

use serde::Deserialize;

use super::AbilityPoints;

/// 一支队伍最多的人数（队长加 4 个分身，`docs/protocol.md` 5.57）。
pub const MAX_PARTY_SIZE: usize = 5;

/// 一支队伍。目前只有分身队（`kind` 为 `clone`）：队长在第一位，后面是最多 4 个分身。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct Party {
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub leader: String,
    #[serde(default)]
    pub members: Vec<PartyMember>,
}

/// 队伍里的一个成员。`index` 是稳定编号（队长 0，分身 1–4），界面用它保持选中。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyMember {
    #[serde(default)]
    pub index: u8,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub leader: bool,
    #[serde(default)]
    pub clone: bool,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub hp: u32,
    #[serde(default)]
    pub max_hp: u32,
    #[serde(default)]
    pub mp: u32,
    #[serde(default)]
    pub max_mp: u32,
    /// 真人队成员是否在线（5.67：本期总是 true）。
    #[serde(default)]
    pub online: bool,
    /// 真人队成员是否战斗中（5.67 未列出，服务端给了才有）。
    #[serde(default)]
    pub in_battle: Option<bool>,
    /// 出战宠物；没有出战宠物时为空。
    #[serde(default)]
    pub pet: Option<PartyPet>,
    #[serde(default)]
    pub position: PartyPosition,
    #[serde(default)]
    pub stats: AbilityPoints,
    #[serde(default)]
    pub derived: PartyDerived,
    #[serde(default)]
    pub equipment: Vec<PartyEquipment>,
    #[serde(default)]
    pub magic: Vec<PartyMagic>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyPet {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub hp: u32,
    #[serde(default)]
    pub max_hp: u32,
    #[serde(default)]
    pub skills: Vec<String>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyPosition {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub map_name: String,
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
}

/// 衍生数值：攻击、防御、敏捷（服务端算好，客户端只显示）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct PartyDerived {
    #[serde(default)]
    pub attack: u32,
    #[serde(default)]
    pub defense: u32,
    #[serde(default)]
    pub agility: u32,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyEquipment {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub name: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyMagic {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub name: String,
}

/// 推送 `party_changed`（5.57）：`party` 是权威的最新队伍，`null` 表示没有队伍。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyChanged {
    /// `created`、`disbanded`、`moved`、`battle_end`。
    #[serde(default)]
    pub reason: String,
    /// 真人队的变化带上的人（加入、离队、被请出的人，或新队长）；分身队没有。
    #[serde(default)]
    pub who: String,
    #[serde(default)]
    pub party: Option<Party>,
    /// 战斗中的离队才带（STAGE-275，10.14）：`battle_escape`、`battle_timeout`、`battle_offline`。
    #[serde(default)]
    pub cause: Option<String>,
}

/// `party` 响应和 `party_clone` 响应：`{"party": 队伍或 null}`。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PartyReply {
    #[serde(default)]
    pub party: Option<Party>,
    /// `party_clone` 带了自动战斗方案时的结果：`applied`（按方案出招）或 `ignored`（不认，分身走默认）（STAGE-195）。
    /// 请求没带方案时响应里没有这个字段。
    #[serde(default)]
    pub auto_profile: Option<String>,
}

impl Party {
    /// 队伍人数（含队长）。
    pub fn size(&self) -> usize {
        self.members.len()
    }

    /// 按稳定编号找成员。
    pub fn member(&self, index: u8) -> Option<&PartyMember> {
        self.members.iter().find(|m| m.index == index)
    }

    /// 分身个数。
    pub fn clones(&self) -> usize {
        self.members.iter().filter(|m| m.clone).count()
    }
}
