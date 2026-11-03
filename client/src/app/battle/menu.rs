//! 指令条上哪些格子现在能选（`docs/battle-ui.md`「指令输入」）：没有精灵、没有道具、没有可换的宠物、
//! 这场战斗不允许捕捉或逃跑时，对应的动作变灰；挂机管理项（STAGE-153）一直可以按，关着也能停。

use super::BattleState;
use super::command::{Act, Actor, MenuSlot, PET_MENU, PLAYER_MENU};
use super::grid::{Grid, PET_COLS, PLAYER_COLS};
use crate::app::WorldState;

impl BattleState {
    /// 指令条上这个动作现在能不能选。不能选的变灰；文字命令仍可以发，由服务端判断。
    pub fn enabled(&self, world: &WorldState, actor: Actor, act: Act) -> bool {
        let battle_pet = world.pets.pets.iter().find(|p| p.state == "battle");
        match (actor, act) {
            // 精灵（魔法）：看角色有没有学会精灵（`status.magic`，STAGE-25）。
            (Actor::Player, Act::Magic) => !world.entry.status.magic.is_empty(),
            (Actor::Player, Act::Item) => world.inventory.slots.iter().any(|s| (5..=19).contains(&s.slot)),
            (Actor::Player, Act::Capture) => self.allow.capture,
            (Actor::Player, Act::Escape) => self.allow.escape,
            (Actor::Player, Act::SwapPet) => battle_pet.is_some() || world.pets.pets.iter().any(|p| p.state != "battle" && p.hp > 0),
            (Actor::Pet, Act::Skill) => battle_pet.is_some_and(|p| !p.skills.is_empty()),
            _ => act.allowed_for(actor),
        }
    }

    /// 指令条上这个格子现在能不能执行。管理项不参与出招的条件判断：灰着也能按，
    /// 由动作本身在消息栏说明原因（正在关闭遇敌时不允许重开等）。
    pub fn slot_enabled(&self, world: &WorldState, actor: Actor, slot: MenuSlot) -> bool {
        match slot {
            MenuSlot::Act(act) => self.enabled(world, actor, act),
            MenuSlot::AutoToggle => true,
        }
    }

    /// 指令条上当前阶段的格子表（人物或宠物）。
    pub fn menu(&self, actor: Actor) -> &'static [MenuSlot] {
        match actor {
            Actor::Player => &PLAYER_MENU,
            Actor::Pet => &PET_MENU,
        }
    }

    /// 当前阶段动作层的网格布局（STAGE-153）：绘制、高亮、方向键和数字键都按它算。
    pub fn grid(&self, actor: Actor) -> Grid {
        let cols = match actor {
            Actor::Player => PLAYER_COLS,
            Actor::Pet => PET_COLS,
        };
        Grid::new(cols, self.menu(actor).len())
    }
}
