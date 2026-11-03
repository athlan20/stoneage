//! 物品目标类型映射；与动作导航分开。
use crate::proto::{InventorySlot, TargetType};
/// 「使用」这一步的目标：直接提交、要玩家选、或者不能做。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum UseTargets {
    /// 不用选，目标编号直接确定（0 是自己）。
    Direct(u8),
    /// 要玩家选自己或宠物。
    Pick { down_only: bool },
    /// 不能使用，附原因。
    Blocked(&'static str),
}

pub fn use_targets(it: &InventorySlot) -> UseTargets {
    match it.field_target_type.unwrap_or(it.target_type) {
        TargetType::SelfOnly | TargetType::AllyAll => UseTargets::Direct(0),
        TargetType::Ally => UseTargets::Pick { down_only: false },
        TargetType::AllyDown => UseTargets::Pick { down_only: true },
        _ => UseTargets::Blocked("这个物品不能直接使用"),
    }
}
