//! 「角色」页的选择栏（STAGE-116，`docs/ui.md`「页面选择」2 角色）：精灵清单（0–4）在前，能力（体力、腕力、耐久力、速度）在后。
//! 精灵 Enter 打开动作列表「使用」（STAGE-231，`char_magic.rs`）；能力项 Enter 打开加点表单（STAGE-133，没有未分配点数时解释原因）。
//! 能力栏矮到画不下表头和一行时没有画出来，`Tab` 跳过它（`ui/charpage.rs` 的 `ability_fits`）。

use crate::ui::charpage::{MAGIC_SLOTS, ability_fits};

use super::select::{Column, Item, OnEnter};
use super::world::WorldState;

/// 精灵栏的名字（Tab 提示和渲染用）。
pub const MAGIC_COLUMN: &str = "精灵";
/// 能力栏的名字（Tab 提示和渲染用）。
pub const ABILITY_COLUMN: &str = "能力";
/// 能力栏的四项，下标就是选中项的编号；名字是 `addpoint` 的参数。
pub const ABILITIES: [&str; 4] = ["体力", "腕力", "耐久力", "速度"];

/// 角色页的两栏：精灵（有精灵的栏位能选）、能力（四项，能否画出来看页面高度）。
pub fn char_columns(world: &WorldState) -> Vec<Column> {
    let status = &world.entry.status;
    // STAGE-231：有精灵的栏位 Enter 打开动作列表（「使用」），空栏位不能选。
    let magic = (0..MAGIC_SLOTS)
        .map(|slot| {
            let usable = status.magic.iter().any(|m| usize::from(m.slot) == slot);
            Item { key: slot as i64, usable, enter: usable.then_some(OnEnter::Open) }
        })
        .collect();
    // STAGE-133：能力项打开加点表单；能不能加（有没有未分配点数）由 `char_action` 当场解释。
    let abilities = ABILITIES
        .iter()
        .enumerate()
        .map(|(i, _)| Item { key: i as i64, usable: true, enter: Some(OnEnter::Open) })
        .collect();
    // 页面尺寸记的是边框外的大小，扣掉上下边框后才是中间区域的内容高度。
    let inner_height = world.page_height.saturating_sub(2);
    vec![
        Column { name: MAGIC_COLUMN, items: magic, shown: true, detail: false, default_key: None },
        Column {
            name: ABILITY_COLUMN,
            items: abilities,
            shown: ability_fits(inner_height),
            detail: false,
            default_key: None,
        },
    ]
}
