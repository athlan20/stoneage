//! 战后丢弃肉类（STAGE-260）：只挑背包 5–19 格里的肉，装备格 0–4 不动；判定见 `auto_meat::is_meat_to_drop`。

use std::collections::BTreeSet;

use super::auto_meat::is_meat_to_drop;
use crate::proto::InventorySlot;

/// 单场战后最多发几次丢弃请求（失败的也算）。
pub const MAX_DROP: u32 = 20;

/// 挑下一件要丢的肉：格子靠前的优先，跳过本场丢失败的格子。返回（格子、名字、数量）。
pub fn pick_drop(slots: &[InventorySlot], bad: &BTreeSet<u8>) -> Option<(u8, String, u32)> {
    slots
        .iter()
        .filter(|s| (5..=19).contains(&s.slot) && !bad.contains(&s.slot))
        .filter(|s| is_meat_to_drop(&s.name, &s.effect))
        .min_by_key(|s| s.slot)
        .map(|s| (s.slot, s.name.clone(), s.count))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::TargetType;

    fn item(slot: u8, name: &str, effect: &str) -> InventorySlot {
        InventorySlot {
            slot,
            name: name.into(),
            effect: effect.into(),
            target_type: TargetType::None,
            count: 3,
            ..Default::default()
        }
    }

    #[test]
    fn drops_meat_in_bag_order_and_skips_the_rest() {
        let slots = vec![
            item(4, "烤肉", "耐久力40前後回复"),
            item(9, "魅力肉粽", "耐久力30前後回复"),
            item(12, "治愈药", "耐久力40前後回复"),
            item(15, "毒肉", "毒状态回复"),
            item(8, "气力肉", "气力60前後回复"),
        ];
        assert_eq!(pick_drop(&slots, &BTreeSet::new()), Some((8, "气力肉".into(), 3)));
        let bad: BTreeSet<u8> = [8].into();
        assert_eq!(pick_drop(&slots, &bad), Some((15, "毒肉".into(), 3)));
        let bad: BTreeSet<u8> = [8, 15].into();
        assert_eq!(
            pick_drop(&slots, &bad),
            None,
            "剩下的只有魅力肉粽、药和装备格里的，丢到空"
        );
    }

    #[test]
    fn never_touches_equipment_slots() {
        let slots = vec![item(3, "烤肉", "耐久力40前後回复")];
        assert_eq!(pick_drop(&slots, &BTreeSet::new()), None);
    }
}
