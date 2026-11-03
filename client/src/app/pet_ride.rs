//! STAGE-243：宠物动作菜单里的「骑乘 / 下骑」项。
//!
//! 菜单里本地拦下客户端能判断的情况（不发请求），顺序与服务端 `pet_ride`（`docs/protocol.md` 5.52）一致；
//! 外形与种族只有服务端知道，这里不判断，照常发请求由服务端拒绝。
//! `pet ride <编号>` 命令目前不做这些本地判断，仍由服务端检查（命令行为保持不变）。

use crate::proto::Pet;

use super::world::WorldState;

/// 判断骑乘用到的信息：正在骑的栏位，以及人物的骑乘等级。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct RideContext {
    /// 正在骑的宠物栏位（同时最多 1 只）。
    pub riding: Option<u8>,
    /// 人物的骑乘等级（0 表示一次都没学过）。
    pub ride_level: u32,
}

impl WorldState {
    /// 从当前世界取骑乘判断用的信息。
    pub fn ride_context(&self) -> RideContext {
        RideContext {
            riding: self.pets.pets.iter().find(|p| is_riding(p)).map(|p| p.slot),
            ride_level: self.entry.status.ride_level,
        }
    }
}

/// 这只宠物正在骑（服务端状态 `ride`）。
pub fn is_riding(pet: &Pet) -> bool {
    pet.state == "ride"
}

/// 骑乘动作灰掉的原因；None 表示可以选（骑着这只时是「下骑」，也可选）。
pub fn ride_reason(pet: &Pet, ctx: RideContext) -> Option<String> {
    if is_riding(pet) {
        return None;
    }
    if let Some(slot) = ctx.riding {
        return Some(format!("已经在骑 {slot} 号栏的宠物，先下骑"));
    }
    // 服务端只认休息状态（PET_NOT_REST），出战和待命都要先设为休息。
    if matches!(pet.state.as_str(), "battle" | "standby") {
        return Some("出战或待命的宠物不能骑，先设为休息".to_string());
    }
    if ctx.ride_level == 0 {
        return Some("你还没有学过骑乘，先去庄园的骑乘训练师学习".to_string());
    }
    if pet.level > ctx.ride_level {
        return Some(format!("你目前只能骑乘等级小于等于 {} 级的宠物", ctx.ride_level));
    }
    if pet.loyalty < 100 {
        return Some("该骑宠的忠诚小于 100".to_string());
    }
    if pet.transmigration > 2 {
        return Some("转生次数超过 2 次的宠物不能骑".to_string());
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    fn pet(state: &str, level: u32) -> Pet {
        Pet {
            slot: 1,
            name: "配套虎".into(),
            state: state.into(),
            hp: 30,
            max_hp: 30,
            level,
            loyalty: 100,
            ..Pet::default()
        }
    }

    fn ctx(riding: Option<u8>, ride_level: u32) -> RideContext {
        RideContext { riding, ride_level }
    }

    #[test]
    fn rest_pet_can_be_ridden() {
        assert_eq!(ride_reason(&pet("rest", 5), ctx(None, 40)), None);
    }

    #[test]
    fn riding_pet_stays_enabled_to_unride() {
        // 骑着的这只：标签是「下骑」，动作可选（即使其它条件不满足）。
        let mut p = pet("ride", 50);
        p.loyalty = 0;
        assert_eq!(ride_reason(&p, ctx(Some(1), 0)), None);
    }

    #[test]
    fn already_riding_another_pet_is_explained() {
        assert_eq!(
            ride_reason(&pet("rest", 5), ctx(Some(0), 40)).as_deref(),
            Some("已经在骑 0 号栏的宠物，先下骑")
        );
    }

    #[test]
    fn battle_and_standby_pets_must_rest_first() {
        let msg = Some("出战或待命的宠物不能骑，先设为休息".to_string());
        assert_eq!(ride_reason(&pet("battle", 5), ctx(None, 40)), msg);
        assert_eq!(ride_reason(&pet("standby", 5), ctx(None, 40)), msg);
    }

    #[test]
    fn zero_ride_level_points_to_the_trainer() {
        assert_eq!(
            ride_reason(&pet("rest", 1), ctx(None, 0)).as_deref(),
            Some("你还没有学过骑乘，先去庄园的骑乘训练师学习")
        );
    }

    #[test]
    fn pet_above_ride_level_shows_the_cap() {
        assert_eq!(
            ride_reason(&pet("rest", 41), ctx(None, 40)).as_deref(),
            Some("你目前只能骑乘等级小于等于 40 级的宠物")
        );
        assert_eq!(ride_reason(&pet("rest", 40), ctx(None, 40)), None, "等级等于上限可以骑");
    }

    #[test]
    fn low_loyalty_is_explained() {
        let mut p = pet("rest", 5);
        p.loyalty = 99;
        assert_eq!(ride_reason(&p, ctx(None, 40)).as_deref(), Some("该骑宠的忠诚小于 100"));
    }

    #[test]
    fn too_many_transmigrations_is_explained() {
        let mut p = pet("rest", 5);
        p.transmigration = 3;
        assert_eq!(
            ride_reason(&p, ctx(None, 40)).as_deref(),
            Some("转生次数超过 2 次的宠物不能骑")
        );
        p.transmigration = 2;
        assert_eq!(ride_reason(&p, ctx(None, 40)), None, "转生 2 次还能骑");
    }
}
