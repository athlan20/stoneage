//! 战后吃肉（STAGE-260）：肉的判定、选法，以及每一步该吃谁。纯函数，收发在 `auto_post.rs`。
//! 判定规则写在 `docs/auto-battle-settings.md`「战后吃肉与丢弃」，改规则两处一起改。

use std::collections::BTreeSet;

use super::auto_heal::heal_power;
use crate::proto::{InventorySlot, TargetType};

/// 单场战后最多吃几份（人物和战宠加起来）。
pub const MAX_EAT: u32 = 10;

/// 一份能吃的肉：背包格、名字、回复量（HP）。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Meat {
    pub slot: u8,
    pub name: String,
    pub heal: u32,
}

/// 吃回血的肉：名字含「肉」、效果文字能解析出 HP 回复量，魅力肉粽不算。
/// 气力肉、治状态的肉解析不出 HP 回复量，自然不在里面。
pub fn heal_meat_power(name: &str, effect: &str) -> Option<u32> {
    if !name.contains('肉') || name.contains('魅') || effect.contains('魅') {
        return None;
    }
    heal_power(effect)
}

/// 丢弃的肉：名字含「肉」就丢（HP 肉、气力肉、治状态肉都丢），魅力肉粽不丢。
pub fn is_meat_to_drop(name: &str, effect: &str) -> bool {
    name.contains('肉') && !name.contains('魅') && !effect.contains('魅')
}

/// 血量要补：倒下（0）不补，血量 ≤ 门槛（百分比）才补。
pub fn needs_heal(hp: u32, max: u32, pct: u8) -> bool {
    hp > 0 && max > 0 && u64::from(hp) * 100 <= u64::from(max) * u64::from(pct)
}

/// 血量百分比（向下取整）。
pub fn percent(hp: u32, max: u32) -> u32 {
    if max == 0 { 0 } else { hp * 100 / max }
}

/// 从背包挑下一份肉：回复量小的优先，同样多取格子靠前的；只看背包格 5–19，跳过本场拉黑的格子。
/// 人物用能对自己用的（自己 / 己方一个 / 己方全体）；喂宠物只认「他人」类（己方一个 / 全体）。
pub fn pick_meat(slots: &[InventorySlot], bad: &BTreeSet<u8>, for_pet: bool) -> Option<Meat> {
    slots
        .iter()
        .filter(|s| (5..=19).contains(&s.slot) && !bad.contains(&s.slot))
        .filter(|s| match s.target_type {
            TargetType::Ally | TargetType::AllyAll => true,
            TargetType::SelfOnly => !for_pet,
            _ => false,
        })
        .filter_map(|s| {
            heal_meat_power(&s.name, &s.effect).map(|heal| Meat {
                slot: s.slot,
                name: s.name.clone(),
                heal,
            })
        })
        .min_by_key(|m| (m.heal, m.slot))
}

/// 一个单位的血量（人物或宠物）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Body {
    pub hp: u32,
    pub max: u32,
}

/// 决定下一步吃谁要用到的全部输入。
pub struct EatView<'a> {
    pub pct: u8,
    pub player: Body,
    /// 出战宠：目标编号（1–20，即宠物栏位 + 1，STAGE-346）和血量；没有出战宠为 None。
    pub pet: Option<(u8, Body)>,
    pub slots: &'a [InventorySlot],
    pub bad: &'a BTreeSet<u8>,
    /// 本场已经吃了几份。
    pub eaten: u32,
}

/// 下一步。`Short` / `Capped` 都是这一场吃肉的终点，之后只看丢弃。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EatStep {
    /// 给人物吃（不传 target）。
    Person(Meat),
    /// 喂出战宠（target 为 1–20）。
    Pet { meat: Meat, target: u8 },
    /// 该补的没肉可补。
    Short,
    /// 该补，但已经到单场份数上限。
    Capped,
    /// 不用补了（人物达标，宠物没出战或达标）。
    Done,
}

/// 先人物，人物达标后才轮到出战宠。人物倒下（血量 0）时不吃，也不会轮到宠物。
pub fn next_eat(v: &EatView) -> EatStep {
    if v.player.hp == 0 {
        return EatStep::Done;
    }
    let (target, meat) = if needs_heal(v.player.hp, v.player.max, v.pct) {
        (None, pick_meat(v.slots, v.bad, false))
    } else {
        match v.pet {
            Some((t, b)) if needs_heal(b.hp, b.max, v.pct) => (Some(t), pick_meat(v.slots, v.bad, true)),
            _ => return EatStep::Done,
        }
    };
    match (meat, target) {
        (None, _) => EatStep::Short,
        (Some(_), _) if v.eaten >= MAX_EAT => EatStep::Capped,
        (Some(m), None) => EatStep::Person(m),
        (Some(m), Some(target)) => EatStep::Pet { meat: m, target },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn slot(slot: u8, name: &str, effect: &str, target: TargetType) -> InventorySlot {
        InventorySlot {
            slot,
            name: name.into(),
            effect: effect.into(),
            target_type: target,
            count: 1,
            ..Default::default()
        }
    }

    #[test]
    fn heal_meat_is_a_meat_with_an_hp_wording() {
        assert_eq!(heal_meat_power("烤肉", "耐久力40前後回复"), Some(40));
        assert_eq!(heal_meat_power("生肉", "耐久力10前後回复"), Some(10));
        // 气力肉：有「肉」，但效果是气力，解析不出 HP 回复量。
        assert_eq!(heal_meat_power("烤肉", "气力60前後回复"), None);
        // 治状态的肉：没有数字，不是回血肉。
        assert_eq!(heal_meat_power("毒肉", "毒状态回复"), None);
        // 魅力肉粽：名字或效果带「魅」都不吃。
        assert_eq!(heal_meat_power("魅力肉粽", "耐久力30前後回复"), None);
        assert_eq!(heal_meat_power("肉粽", "魅力回复"), None);
        // 非肉的回血道具不算。
        assert_eq!(heal_meat_power("治愈药", "耐久力40前後回复"), None);
    }

    #[test]
    fn every_meat_is_dropped_except_the_charm_dumpling() {
        assert!(is_meat_to_drop("烤肉", "耐久力40前後回复"));
        assert!(is_meat_to_drop("烤肉", "气力60前後回复"));
        assert!(is_meat_to_drop("毒肉", "毒状态回复"));
        assert!(!is_meat_to_drop("魅力肉粽", "耐久力30前後回复"));
        assert!(!is_meat_to_drop("治愈药", "耐久力40前後回复"));
    }

    #[test]
    fn heal_threshold_is_inclusive_and_skips_the_dead() {
        assert!(needs_heal(90, 100, 90));
        assert!(!needs_heal(91, 100, 90));
        assert!(!needs_heal(0, 100, 90));
        assert!(!needs_heal(1, 0, 90));
        assert_eq!(percent(42, 100), 42);
    }

    #[test]
    fn smallest_heal_goes_first_then_lower_slot() {
        let slots = vec![
            slot(7, "烤肉", "耐久力40前後回复", TargetType::Ally),
            slot(5, "生肉", "耐久力10前後回复", TargetType::SelfOnly),
            slot(6, "生肉", "耐久力10前後回复", TargetType::Ally),
            slot(3, "生肉", "耐久力1前後回复", TargetType::Ally),
        ];
        let m = pick_meat(&slots, &BTreeSet::new(), false).unwrap();
        assert_eq!(
            (m.slot, m.heal),
            (5, 10),
            "回复量相同取靠前的；格子 3 不在 5–19 里，回复 1 也不选"
        );
        let bad: BTreeSet<u8> = [5].into();
        let m = pick_meat(&slots[..3], &bad, false).unwrap();
        assert_eq!(m.slot, 6, "回复量相同时取靠前的格子，5 号已拉黑");
    }

    #[test]
    fn pet_only_takes_meat_aimed_at_others() {
        let slots = vec![slot(5, "烤肉", "耐久力40前後回复", TargetType::SelfOnly)];
        assert!(pick_meat(&slots, &BTreeSet::new(), true).is_none());
        assert!(pick_meat(&slots, &BTreeSet::new(), false).is_some());
    }

    fn view<'a>(
        slots: &'a [InventorySlot],
        bad: &'a BTreeSet<u8>,
        player: (u32, u32),
        pet: Option<(u8, u32, u32)>,
        eaten: u32,
    ) -> EatView<'a> {
        EatView {
            pct: 90,
            player: Body {
                hp: player.0,
                max: player.1,
            },
            pet: pet.map(|(t, hp, max)| (t, Body { hp, max })),
            slots,
            bad,
            eaten,
        }
    }

    #[test]
    fn person_is_fed_before_the_pet() {
        let slots = vec![slot(5, "烤肉", "耐久力40前後回复", TargetType::SelfOnly)];
        let bad = BTreeSet::new();
        let step = next_eat(&view(&slots, &bad, (30, 100), Some((1, 40, 100)), 0));
        assert!(matches!(step, EatStep::Person(ref m) if m.slot == 5));
    }

    #[test]
    fn full_person_only_feeds_the_pet() {
        let slots = vec![slot(5, "烤肉", "耐久力40前後回复", TargetType::Ally)];
        let bad = BTreeSet::new();
        let step = next_eat(&view(&slots, &bad, (100, 100), Some((2, 50, 100)), 0));
        assert!(matches!(step, EatStep::Pet { target: 2, .. }));
    }

    #[test]
    fn pet_that_is_fine_or_down_is_skipped() {
        let slots = vec![slot(5, "烤肉", "耐久力40前後回复", TargetType::Ally)];
        let bad = BTreeSet::new();
        assert_eq!(
            next_eat(&view(&slots, &bad, (100, 100), Some((1, 95, 100)), 0)),
            EatStep::Done
        );
        assert_eq!(
            next_eat(&view(&slots, &bad, (100, 100), Some((1, 0, 100)), 0)),
            EatStep::Done
        );
        assert_eq!(next_eat(&view(&slots, &bad, (100, 100), None, 0)), EatStep::Done);
    }

    #[test]
    fn dead_person_is_not_fed_and_the_pet_waits_too() {
        let slots = vec![slot(5, "烤肉", "耐久力40前後回复", TargetType::Ally)];
        let bad = BTreeSet::new();
        assert_eq!(
            next_eat(&view(&slots, &bad, (0, 100), Some((1, 10, 100)), 0)),
            EatStep::Done
        );
    }

    #[test]
    fn short_of_meat_stops_at_the_person_and_never_feeds_the_pet() {
        let bad = BTreeSet::new();
        let step = next_eat(&view(&[], &bad, (42, 100), Some((1, 10, 100)), 0));
        assert_eq!(step, EatStep::Short);
    }

    #[test]
    fn reaching_the_cap_stops_eating() {
        let slots = vec![slot(5, "烤肉", "耐久力40前後回复", TargetType::SelfOnly)];
        let bad = BTreeSet::new();
        assert_eq!(next_eat(&view(&slots, &bad, (30, 100), None, MAX_EAT)), EatStep::Capped);
        assert!(matches!(
            next_eat(&view(&slots, &bad, (30, 100), None, MAX_EAT - 1)),
            EatStep::Person(_)
        ));
    }
}
