//! 宠物页用到的宠物列表（STAGE-31，`docs/protocol.md` 5.19、5.20）。
//! 攻击、防御、敏捷、忠诚和最大 HP 都由服务端按公式算好，客户端只显示，这里不做计算。

use serde::Deserialize;

use super::{ElementPoints, TargetType};

/// 宠物栏位数（`docs/protocol.md` 5.19），服务端没给时按这个数。
/// 服务的默认栏位数（STAGE-346：20 格）；正常情况下以服务端下发的 `max_slots` 为准。
const DEFAULT_SLOTS: u8 = 20;

fn default_slots() -> u8 {
    DEFAULT_SLOTS
}

/// 宠物的一项技能。`slot` 从 1 开始，是页面上的技能栏位编号。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PetSkill {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub id: i32,
    #[serde(default)]
    pub name: String,
    /// 战斗中的目标类型（STAGE-78）：攻击是 `enemy`，待机是 `none`。
    #[serde(default)]
    pub target_type: TargetType,
}

/// 一只宠物（`docs/protocol.md` 5.19）。资质和原始能力不在协议里，服务端不发。
#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
pub struct Pet {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub species_id: i32,
    /// 种族名。
    #[serde(default)]
    pub species: String,
    /// 宠物自己的名字。
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    /// 转生次数（STAGE-196）：旧服务端不带这个字段时为 0；超过 2 次的宠物不能骑。
    #[serde(default)]
    pub transmigration: u32,
    /// 累计经验。
    #[serde(default)]
    pub exp: i64,
    /// 升到下一级需要的累计经验；已是最高级时为 -1。
    #[serde(default)]
    pub next_exp: i64,
    #[serde(default)]
    pub hp: u32,
    #[serde(default)]
    pub max_hp: u32,
    #[serde(default)]
    pub attack: u32,
    #[serde(default)]
    pub defense: u32,
    #[serde(default)]
    pub agility: u32,
    /// 忠诚，0–100。
    #[serde(default)]
    pub loyalty: u32,
    /// 地、水、火、风（模板值，0–100）。
    #[serde(default)]
    pub elements: ElementPoints,
    /// 服务端给的状态：`rest` 休息、`standby` 待命、`battle` 出战、`ride` 骑乘（STAGE-123）。
    #[serde(default)]
    pub state: String,
    /// 技能列表，只含有技能的栏位，按 slot 从小到大。
    #[serde(default)]
    pub skills: Vec<PetSkill>,
    /// 成长率（STAGE-216）：旧服务端不带这个字段时为 None，界面按“无记录”处理。
    #[serde(default)]
    pub growth: Option<PetGrowth>,
    /// 隐藏素质（STAGE-216）：固定四项，旧服务端不带时为空，界面写 —。
    #[serde(default)]
    pub quality: Vec<PetQuality>,
}

/// 成长率（`docs/protocol.md` 5.19）。`basis` 是口径：`raised` 从 1 级起养，`caught` 捕获时已有等级，
/// `unknown` 是历史宠物（没有起始记录）。`from_level` 是起始等级，`unknown` 时为 None。
#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
pub struct PetGrowth {
    #[serde(default)]
    pub basis: String,
    #[serde(default)]
    pub from_level: Option<u32>,
    #[serde(default)]
    pub rates: PetRates,
}

/// 四项成长率，每升一级平均涨多少，两位小数。算不出时是 None（显示 —），不是 0。
#[derive(Debug, Clone, Copy, Default, PartialEq, Deserialize)]
pub struct PetRates {
    #[serde(default)]
    pub hp: Option<f64>,
    #[serde(default)]
    pub attack: Option<f64>,
    #[serde(default)]
    pub defense: Option<f64>,
    #[serde(default)]
    pub agility: Option<f64>,
}

/// 一项素质（`docs/protocol.md` 5.19）。`stat` 为 `vital`、`strength`、`toughness`、`dexterity`，
/// `points` 是生成时保存的素质点，`tier` 是档位：0 满，−1 到 −4 是距离满值的差（负数）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct PetQuality {
    #[serde(default)]
    pub stat: String,
    #[serde(default)]
    pub points: u32,
    #[serde(default)]
    pub tier: i32,
}

/// 页面上显示的宠物状态。已倒下不是服务端给的状态：HP 为 0 时由客户端判断。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PetStatus {
    Battle,
    Standby,
    Rest,
    Ride,
    Fainted,
}

impl Pet {
    pub fn status(&self) -> PetStatus {
        if self.hp == 0 {
            return PetStatus::Fainted;
        }
        match self.state.as_str() {
            "battle" => PetStatus::Battle,
            "standby" => PetStatus::Standby,
            "ride" => PetStatus::Ride,
            _ => PetStatus::Rest,
        }
    }

    /// 出战的宠物（`state` 为 `battle`，即使已倒下也算）：默认显示它。
    pub fn is_battle(&self) -> bool {
        self.state == "battle"
    }
}

/// 宠物列表：`pets` 响应与 `pets_changed` 推送的内容相同。
#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct Pets {
    #[serde(default = "default_slots")]
    pub max_slots: u8,
    /// 只含有宠物的栏位，按 slot 从小到大；空栏位不出现，由界面按 max_slots 补上。
    #[serde(default)]
    pub pets: Vec<Pet>,
}

impl Default for Pets {
    fn default() -> Self {
        Self { max_slots: DEFAULT_SLOTS, pets: vec![] }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// STAGE-217：`docs/protocol.md` 5.19 的示例，成长率和素质都有。
    #[test]
    fn pet_parses_growth_and_quality() {
        let v = serde_json::json!({
            "slot": 0, "name": "乌力", "level": 5,
            "growth": {"basis": "raised", "from_level": 1,
                       "rates": {"hp": 7.25, "attack": 1.1, "defense": 0.85, "agility": 0.92}},
            "quality": [{"stat": "vital", "points": 22, "tier": -1}, {"stat": "dexterity", "points": 24, "tier": 0}]
        });
        let p: Pet = serde_json::from_value(v).unwrap();
        let g = p.growth.expect("有 growth");
        assert_eq!(g.basis, "raised");
        assert_eq!(g.from_level, Some(1));
        assert_eq!(g.rates.hp, Some(7.25));
        assert_eq!(g.rates.agility, Some(0.92));
        assert_eq!(p.quality.len(), 2);
        assert_eq!(p.quality[0], PetQuality { stat: "vital".into(), points: 22, tier: -1 });
        assert_eq!(p.quality[1].tier, 0);
    }

    /// 旧服务端或用例没有 growth、quality 字段时按“无”处理，不报错。
    #[test]
    fn pet_without_growth_and_quality_parses_as_none() {
        let p: Pet = serde_json::from_str(r#"{"slot":0,"name":"乌力","level":5}"#).unwrap();
        assert_eq!(p.growth, None);
        assert!(p.quality.is_empty());
    }

    /// 算不出的成长率是 null，解析成 None；unknown 口径的 from_level 也是 null。
    #[test]
    fn growth_rates_null_parse_as_none() {
        let v = serde_json::json!({
            "growth": {"basis": "unknown", "from_level": null,
                       "rates": {"hp": null, "attack": null, "defense": null, "agility": null}}
        });
        let p: Pet = serde_json::from_value(v).unwrap();
        let g = p.growth.expect("growth 对象本身存在");
        assert_eq!(g.basis, "unknown");
        assert_eq!(g.from_level, None);
        assert_eq!(g.rates, PetRates::default());
        assert_eq!(g.rates.hp, None);
    }

    /// rates 整个缺失时四项都是 None。
    #[test]
    fn growth_without_rates_has_no_rates() {
        let v = serde_json::json!({"growth": {"basis": "caught", "from_level": 5}});
        let p: Pet = serde_json::from_value(v).unwrap();
        let g = p.growth.unwrap();
        assert_eq!(g.from_level, Some(5));
        assert_eq!(g.rates, PetRates::default());
    }
}
