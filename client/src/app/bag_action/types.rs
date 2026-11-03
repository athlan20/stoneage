//! 背包对象动作的类型与可用性规则。
use super::super::inventory_inlay::gear_free;
use super::{InventorySlot, MAX_EQUIP_SLOT, TargetType, TextInput, can_equip};
/// 背包动作流程当前停在哪一步。
#[derive(Debug)]
pub enum BagStep {
    /// 对象动作列表：`slot` 是对象所在格，`index` 是选中的动作。
    Actions { slot: u8, index: usize },
    /// 使用的目标：`index` 0 是自己，`1..` 是第 index 只宠物（协议目标编号 = 宠物 slot + 1）。
    Target { slot: u8, index: usize },
    /// 移动的目的格：`index` 从 0 数，对应 5–19 号背包格。
    MoveTo { slot: u8, index: usize },
    /// 丢弃数量表单。
    DropCount { slot: u8, text: TextInput },
    /// 精工的材料选择（STAGE-299）：`picked` 是已勾选的材料格，按勾选顺序。
    InlayPick { slot: u8, index: usize, picked: Vec<u8> },
}

/// 这一步是哪一类（借用来判断按键该交给谁）。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum StepKind {
    Actions,
    Target,
    MoveTo,
    DropCount,
    InlayPick,
}

impl BagStep {
    pub fn kind(&self) -> StepKind {
        match self {
            BagStep::Actions { .. } => StepKind::Actions,
            BagStep::Target { .. } => StepKind::Target,
            BagStep::MoveTo { .. } => StepKind::MoveTo,
            BagStep::DropCount { .. } => StepKind::DropCount,
            BagStep::InlayPick { .. } => StepKind::InlayPick,
        }
    }

    /// 这一步的标题（渲染用）。
    pub fn title(&self, name: &str) -> String {
        match self {
            BagStep::Actions { .. } => format!("动作 · {name}"),
            BagStep::Target { .. } => format!("对谁使用 {name}"),
            BagStep::MoveTo { .. } => format!("把 {name} 移到哪一格"),
            BagStep::DropCount { .. } => format!("丢弃 {name} · 输入数量"),
            BagStep::InlayPick { .. } => format!("精工 · {name} · 勾选材料"),
        }
    }
}

/// 背包对象能做的动作。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Act {
    Equip,
    Unequip,
    Use,
    Move,
    Drop,
    /// 精工（STAGE-299）：可镶装备（背包或穿着）。
    Inlay,
    Describe,
}

impl Act {
    pub fn label(self) -> &'static str {
        match self {
            Act::Equip => "装备",
            Act::Unequip => "卸下",
            Act::Use => "使用",
            Act::Move => "移动",
            Act::Drop => "丢弃",
            Act::Inlay => "精工",
            Act::Describe => "说明",
        }
    }
}

/// 这一格能做的动作（按显示顺序）。装备格只有卸下和说明；背包格按类型给装备 / 使用。
pub fn actions(slot: u8, it: &InventorySlot) -> Vec<Act> {
    if usize::from(slot) <= MAX_EQUIP_SLOT {
        let mut list = vec![Act::Unequip];
        if gear_free(it).is_some() {
            list.push(Act::Inlay);
        }
        list.push(Act::Describe);
        return list;
    }
    let mut list = Vec::new();
    if can_equip(it.kind) {
        list.push(Act::Equip);
    }
    if usable(it) {
        list.push(Act::Use);
    }
    if gear_free(it).is_some() {
        list.push(Act::Inlay);
    }
    list.push(Act::Move);
    list.push(Act::Drop);
    list.push(Act::Describe);
    list
}

/// 这个物品在非战斗时能不能用（有己方目标）。世界中的「使用」看 `field_target_type`（STAGE-326），
/// 缺省同 `target_type`；战斗列表不走这里。
pub fn usable(it: &InventorySlot) -> bool {
    matches!(
        it.field_target_type.unwrap_or(it.target_type),
        TargetType::SelfOnly | TargetType::Ally | TargetType::AllyAll | TargetType::AllyDown
    )
}

/// 动作不能做的原因（显示时保留这一项并解释，STAGE-132 验收标准 6）。
pub fn disabled_reason(act: Act, it: &InventorySlot) -> Option<&'static str> {
    match act {
        Act::Equip if it.count != 1 => Some("数量不是 1，先拆开再装备"),
        Act::Use if matches!(it.target_type, TargetType::Enemy | TargetType::EnemyAll) => {
            Some("只能对敌人用，战斗中才能用")
        }
        Act::Inlay if gear_free(it) == Some(0) => Some("已镶满，不能再精工"),
        _ => None,
    }
}

#[cfg(test)]
mod inlay_tests {
    use super::*;
    use crate::proto::Inlay;

    fn slot(slot: u8, inlay: Option<Inlay>) -> InventorySlot {
        InventorySlot { slot, name: "斧".into(), count: 1, kind: 1, inlay, ..Default::default() }
    }

    fn gear(free: u8) -> Option<Inlay> {
        Some(Inlay { role: "gear".into(), free, codes: vec![None; 3] })
    }

    #[test]
    fn inlay_entry_only_for_gear_and_greyed_when_full() {
        let with_space = slot(5, gear(2));
        assert!(actions(5, &with_space).contains(&Act::Inlay));
        assert_eq!(disabled_reason(Act::Inlay, &with_space), None);

        let full = slot(5, gear(0));
        assert!(actions(5, &full).contains(&Act::Inlay), "镶满的仍列出，灰掉并说明");
        assert_eq!(disabled_reason(Act::Inlay, &full), Some("已镶满，不能再精工"));

        let material = slot(6, Some(Inlay { role: "material".into(), ..Default::default() }));
        assert!(!actions(6, &material).contains(&Act::Inlay));
        assert!(!actions(7, &slot(7, None)).contains(&Act::Inlay), "没有 inlay 字段的物品没有入口");
    }

    #[test]
    fn worn_gear_offers_inlay_before_describe() {
        assert_eq!(actions(2, &slot(2, gear(1))), vec![Act::Unequip, Act::Inlay, Act::Describe]);
        assert_eq!(actions(2, &slot(2, None)), vec![Act::Unequip, Act::Describe]);
    }
}
