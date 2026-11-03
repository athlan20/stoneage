use serde::Deserialize;

use super::{Inlay, TargetType};

/// `inventory` / `inventory_changed` 里有物品的一个格子（`docs/protocol.md` 5.17）。
/// 槽位 0–4 是装备位（头、身、手、饰品×2），5–19 是背包；编号就是以后命令的参数。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct InventorySlot {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub item_id: i32,
    #[serde(default)]
    pub name: String,
    /// 效果文字（如“攻 +9 防 -3 敏 -3”），服务端已转成 UTF-8。
    #[serde(default)]
    pub effect: String,
    /// 物品表里的类型，穿脱校验用它（STAGE-29）。
    #[serde(default, rename = "type")]
    pub kind: u32,
    /// 精工信息（STAGE-299，5.73）：只有类别码非空的物品才有；没有就是 None。
    #[serde(default)]
    pub inlay: Option<Inlay>,
    /// 数量；不可堆叠的物品是 1。
    #[serde(default)]
    pub count: u32,
    /// 当前耐久（STAGE-225，`docs/protocol.md` 5.17）：没有耐久的物品不下发，这里是 0。
    #[serde(default)]
    pub durability: u32,
    /// 最大耐久；为 0 表示这件物品没有耐久，界面不显示耐久行。
    #[serde(default)]
    pub max_durability: u32,
    /// 受损等级：0 完好、1 受损、2 毁坏、3 不堪使用；0 时服务端省略。
    #[serde(default)]
    pub crush_level: u8,
    /// 战斗中的目标类型（STAGE-78）：战斗中能用的消耗品才有目标，装备和材料是 `none`。
    #[serde(default)]
    pub target_type: TargetType,
    /// 世界中「使用」的目标类型（STAGE-326）：只在和 `target_type` 不同时才带（缺省同它）；背包的「使用」看它，战斗列表不看。
    #[serde(default)]
    pub field_target_type: Option<TargetType>,
}

/// 背包快照：`inventory` 响应与 `inventory_changed` 推送的内容相同。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct Inventory {
    #[serde(default)]
    pub gold: u64,
    /// 只含有物品的格子，按 slot 从小到大；空格子不出现。
    #[serde(default)]
    pub slots: Vec<InventorySlot>,
}
