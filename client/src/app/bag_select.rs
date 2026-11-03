//! 「背包」页的选择栏（STAGE-114，`docs/ui.md` 背包页「页面选择」）：物品栏（5–19）在前，装备栏（0–4）在后，
//! `Tab` 换栏。STAGE-132 起 `Enter` 打开对象动作菜单（装备 / 卸下、使用、移动、丢弃、说明），
//! 动作由 `bag_action.rs` 执行；这里只声明两栏和每格能不能选。

use super::select::{self, Column, Item, OnEnter};
use super::world::{BAG_LAST_SLOT, BAG_MENU, WorldState};

/// 装备格的上限（0–4）。
pub const MAX_EQUIP_SLOT: usize = 4;
/// 背包格的第一格（装备格之后）。
pub const FIRST_BAG_SLOT: usize = MAX_EQUIP_SLOT + 1;

/// 物品类型能不能穿（有能放的部位）。规则与 `server/gamedata/equip.go` 的 `EquipPlaces` 一致：
/// 拳套、斧、棍、枪、弓、投掷类，头盔，铠甲，饰品（手镯到护身符）。盾牌一期不支持。
pub fn can_equip(kind: u32) -> bool {
    matches!(kind, 0..=4 | 6 | 7 | 8..=15 | 17..=19)
}

/// 「背包」页的两栏：物品（5–19）、装备（0–4）。空格子不能选。
pub fn bag_columns(world: &WorldState) -> Vec<Column> {
    let slots = &world.inventory.slots;
    let find = |slot: usize| slots.iter().find(|s| usize::from(s.slot) == slot);
    let bag = (FIRST_BAG_SLOT..=BAG_LAST_SLOT)
        .map(|slot| {
            let it = find(slot);
            Item { key: slot as i64, usable: it.is_some(), enter: it.map(|_| OnEnter::Open) }
        })
        .collect();
    let equip = (0..=MAX_EQUIP_SLOT)
        .map(|slot| {
            let it = find(slot);
            Item { key: slot as i64, usable: it.is_some(), enter: it.map(|_| OnEnter::Open) }
        })
        .collect();
    vec![
        Column {
            name: "物品",
            items: bag,
            shown: true,
            detail: false,
            default_key: None,
        },
        Column {
            name: "装备",
            items: equip,
            shown: true,
            detail: false,
            default_key: None,
        },
    ]
}

impl WorldState {
    /// 「背包」页的焦点栏编号和每栏的选中格（界面画选中行用）。
    pub fn bag_marks(&self) -> (usize, Vec<Option<i64>>) {
        select::marks(&bag_columns(self), &self.select[BAG_MENU])
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn equip_rule_matches_the_server_table() {
        assert!(can_equip(0), "拳套");
        assert!(can_equip(1), "斧");
        assert!(can_equip(6), "头盔");
        assert!(can_equip(7), "铠甲");
        assert!(can_equip(8) && can_equip(15), "饰品两端");
        assert!(!can_equip(5), "5 没有对应部位");
        assert!(!can_equip(16), "16 没有对应部位");
        assert!(can_equip(19));
        assert!(!can_equip(20));
    }
}

