//! 战斗结束（`docs/protocol.md` 10.9）：结算的奖励、失败的损失和 `battle_end` 本身。
//! 只读结构体，显示在结果窗口里；规则在服务端。

use serde::Deserialize;

use super::{BattleNpc, Position};

/// 一项经验（人物或宠物各一项，10.9）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct ExpGain {
    /// `player` 或 `pet`。
    #[serde(default)]
    pub who: String,
    #[serde(default)]
    pub slot: Option<u8>,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub gain: u64,
    #[serde(default)]
    pub level_from: u32,
    #[serde(default)]
    pub level_to: u32,
    /// 人物升级时获得的能力点。
    #[serde(default)]
    pub skill_points: u32,
}

/// 掉落的物品（10.9）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct ItemGain {
    #[serde(default)]
    pub item_id: i64,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub count: u32,
}

/// 捕获到的宠物（10.9）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct CapturedPet {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
}

/// 胜利奖励（10.9 的 `rewards`）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct Rewards {
    #[serde(default)]
    pub exp: Vec<ExpGain>,
    #[serde(default)]
    pub gold: u64,
    #[serde(default)]
    pub items: Vec<ItemGain>,
    #[serde(default)]
    pub pets: Vec<CapturedPet>,
}

/// 失败时的损失（10.9 的 `penalty`）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct Penalty {
    /// 失败后人物所在的地点，结构同 `position`：被打飞是送回的地点，被打死是原地不动的当前位置。
    #[serde(default)]
    pub respawn: Option<Position>,
    /// 被打死原地以 1 血复活（STAGE-194）；被打飞时服务端省略，即 false。
    #[serde(default)]
    pub in_place: bool,
    #[serde(default)]
    pub gold_lost: u64,
    #[serde(default)]
    pub exp_lost: u64,
}

/// 决斗点数的变化（STAGE-307，`battle_end.duel_point`，`docs/pk.md` 2.2）：`change` 是写回后的实际变化，`total` 是写回后的值。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct DuelPoint {
    #[serde(default)]
    pub change: i64,
    #[serde(default)]
    pub total: i64,
}

/// `null` 也当作没有（决斗的 `rewards` 为 null，10.9）。
fn nullable<'de, D, T>(d: D) -> Result<T, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::<T>::deserialize(d)?.unwrap_or_default())
}

/// 推送 `battle_end`（10.9）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct BattleEnd {
    #[serde(default)]
    pub battle_id: String,
    /// `win`、`lose`、`escaped`、`draw`。
    #[serde(default)]
    pub result: String,
    #[serde(default)]
    pub turn: u32,
    #[serde(default, deserialize_with = "nullable")]
    pub rewards: Rewards,
    #[serde(default)]
    pub penalty: Option<Penalty>,
    /// 决斗结束才有（STAGE-307）：决斗点数的变化与现值；有它时结果窗口不写经验和掉落。
    #[serde(default)]
    pub duel_point: Option<DuelPoint>,
    /// NPC 发起的战斗赢了且 NPC 有结束台词时才有（STAGE-96）：`say` 写进战斗记录。
    #[serde(default)]
    pub npc: Option<BattleNpc>,
}
