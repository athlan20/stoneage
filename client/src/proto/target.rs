//! 目标类型（`docs/protocol.md` 10.5 的“目标类型”，STAGE-78）：精灵、道具、宠物技能共用，
//! 决定要不要选目标、能选哪一方。服务端按它校验，客户端按它决定要不要让玩家选。

use serde::Deserialize;

/// 协议里的 `target_type`。服务端没给时按 `None`：不选目标，直接出招。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub enum TargetType {
    /// 不需要目标（装备、材料和战斗中不能用的物品也是这个取值）。
    #[default]
    #[serde(rename = "none")]
    None,
    /// 只能对自己用，不用选。
    #[serde(rename = "self")]
    SelfOnly,
    /// 己方一个活着的单位（自己或宠物），要选。
    #[serde(rename = "ally")]
    Ally,
    /// 己方全体，不用选。
    #[serde(rename = "ally_all")]
    AllyAll,
    /// 己方一个倒下的单位（人或宠物，复活精灵用），要选。
    #[serde(rename = "ally_down")]
    AllyDown,
    /// 敌方一个活着的单位，要选。
    #[serde(rename = "enemy")]
    Enemy,
    /// 敌方全体，不用选。
    #[serde(rename = "enemy_all")]
    EnemyAll,
    /// 己方或敌方任意一个活着的单位，要选（属性反转精灵，STAGE-200）。
    #[serde(rename = "any")]
    Any,
    /// 以后新增的取值：按“不选目标”处理，服务端照样校验。
    #[serde(other)]
    Unknown,
}

impl TargetType {
    /// 要让玩家在一个单位上确认（己方一个、己方倒下的一个、敌方一个）。
    pub fn picks_unit(self) -> bool {
        matches!(self, Self::Ally | Self::AllyDown | Self::Enemy | Self::Any)
    }

    /// 目标类型的大白话（`docs/protocol.md` 10.5 的“目标类型”）：战斗列表和角色页的精灵清单共用。
    pub fn text(self) -> &'static str {
        match self {
            Self::SelfOnly => "自己",
            Self::Ally => "己方一个",
            Self::AllyAll => "己方全体",
            Self::AllyDown => "倒下的己方",
            Self::Enemy => "敌方一个",
            Self::EnemyAll => "敌方全体",
            Self::Any => "己方或敌方一个",
            Self::None | Self::Unknown => "不用选",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::TargetType;

    #[derive(serde::Deserialize)]
    struct Holder {
        target_type: TargetType,
    }

    fn parse(value: &str) -> TargetType {
        serde_json::from_str::<Holder>(&format!("{{\"target_type\": \"{value}\"}}"))
            .unwrap()
            .target_type
    }

    #[test]
    fn every_protocol_value_has_a_variant() {
        assert_eq!(parse("none"), TargetType::None);
        assert_eq!(parse("self"), TargetType::SelfOnly);
        assert_eq!(parse("ally"), TargetType::Ally);
        assert_eq!(parse("ally_all"), TargetType::AllyAll);
        assert_eq!(parse("ally_down"), TargetType::AllyDown);
        assert_eq!(parse("enemy"), TargetType::Enemy);
        assert_eq!(parse("enemy_all"), TargetType::EnemyAll);
        assert_eq!(parse("any"), TargetType::Any);
    }

    #[test]
    fn only_single_unit_targets_are_picked() {
        assert!(TargetType::Ally.picks_unit());
        assert!(TargetType::AllyDown.picks_unit());
        assert!(TargetType::Enemy.picks_unit());
        assert!(TargetType::Any.picks_unit());
        for t in [
            TargetType::None,
            TargetType::SelfOnly,
            TargetType::AllyAll,
            TargetType::EnemyAll,
            TargetType::Unknown,
        ] {
            assert!(!t.picks_unit(), "{t:?} 不用选");
        }
    }

    #[test]
    fn future_values_do_not_break_the_message() {
        assert_eq!(parse("some_future_value"), TargetType::Unknown);
    }
}
