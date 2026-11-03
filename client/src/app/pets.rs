//! 「宠物」页的数据与命令（STAGE-31，`docs/ui.md` 宠物页 A）：`pets` 响应和 `pets_changed` 推送写进世界状态，
//! `pet status <编号>` 把选中项移到那一只。详情页显示的就是选中项（STAGE-114，方向键和 `pet status` 共用一份选择）。

use serde_json::Value;

use super::select::{self, Column, Item, OnEnter};
use super::world::PETS_MENU;
use super::{App, WorldState};
use crate::proto::{Pet, Pets};

/// 宠物栏位编号的上限（0–19，`docs/protocol.md` 5.19；STAGE-346 起 20 格）。
pub(super) const MAX_PET_SLOT: u8 = 19;

/// 「宠物」页的一栏：20 个栏位（服务端 `max_slots`），空栏位不能选；Enter 打开对象动作菜单（STAGE-133）。
/// 还没有记过选中项时选出战的那只，没有出战的是第一只（`docs/ui.md` 宠物页）。
pub fn pet_columns(world: &WorldState) -> Vec<Column> {
    let items = (0..world.pets.max_slots)
        .map(|slot| {
            let pet = world.pets.pets.iter().find(|p| p.slot == slot);
            Item { key: i64::from(slot), usable: pet.is_some(), enter: pet.map(|_| OnEnter::Open) }
        })
        .collect();
    let default_key = default_pet(&world.pets.pets).map(|p| i64::from(p.slot));
    vec![Column { name: "宠物栏位", items, shown: true, detail: false, default_key }]
}

/// 默认的那只：出战的，没有就第一只。
fn default_pet(pets: &[Pet]) -> Option<&Pet> {
    pets.iter().find(|p| p.is_battle()).or_else(|| pets.first())
}

impl WorldState {
    /// 详情页显示的宠物：选中栏位上的那只。没有宠物时为 None。
    pub fn shown_pet(&self) -> Option<&Pet> {
        let key = self.selected_pet_slot()?;
        self.pets.pets.iter().find(|p| i64::from(p.slot) == key)
    }

    /// 宠物页选中的栏位编号（界面画选中行用）；没有宠物时为 None。
    pub fn selected_pet_slot(&self) -> Option<i64> {
        select::selections(&pet_columns(self), &self.select[PETS_MENU]).first().copied().flatten()
    }
}

impl App {
    /// `pet` 和 `pets_changed` 的内容：整张宠物列表，直接替换。选中项留在原栏位，栏位空了落到最近的一只。
    pub(super) fn on_pets(&mut self, data: Value) {
        let pets: Pets = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            w.pets_epoch+=1;w.pets = pets;
        }
    }

    /// `pet status <编号>`：把选中项移到那一只，详情页跟着显示。`pet` 的其他子命令在 `pets_manage`，
    /// 分发见 `world.rs` 的命令表（STAGE-26）。
    pub(super) fn pet_command(&mut self, rest: &str) {
        let args: Vec<&str> = rest.split_whitespace().collect();
        let ["status", num] = args.as_slice() else {
            self.log.error(format!("{}（编号 0–19）", super::commands::usage_of("pet")));
            return;
        };
        let slot = match num.parse::<u8>() {
            Ok(slot) if slot <= MAX_PET_SLOT => slot,
            _ => {
                self.log.error("宠物栏位要在 0–19 之间");
                return;
            }
        };
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let name = w.pets.pets.iter().find(|p| p.slot == slot).map(|p| p.name.clone());
        match name {
            Some(name) => {
                select::select_key(&pet_columns(w), &mut w.select[PETS_MENU], i64::from(slot));
                self.log.system(format!("宠物页显示 {slot} 号：{name}"));
            }
            None => self.log.error(format!("{slot} 号栏位没有宠物")),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::WorldEntry;

    fn pet(slot: u8, state: &str) -> Pet {
        Pet { slot, name: format!("宠{slot}"), state: state.into(), hp: 10, max_hp: 10, ..Pet::default() }
    }

    fn world(pets: Vec<Pet>) -> WorldState {
        let mut w = WorldState::new(WorldEntry::default());
        w.pets = Pets { max_slots: 5, pets };
        w
    }

    #[test]
    fn shows_battle_pet_by_default() {
        let w = world(vec![pet(0, "rest"), pet(2, "battle"), pet(4, "standby")]);
        assert_eq!(w.shown_pet().map(|p| p.slot), Some(2));
    }

    #[test]
    fn shows_first_pet_without_battle_pet() {
        let w = world(vec![pet(1, "rest"), pet(3, "standby")]);
        assert_eq!(w.shown_pet().map(|p| p.slot), Some(1));
    }

    #[test]
    fn arrows_move_over_pets_only_and_pet_status_moves_too() {
        let mut w = world(vec![pet(0, "rest"), pet(2, "battle"), pet(4, "standby")]);
        let cols = pet_columns(&w);
        assert!(select::navigate_page(&cols, &mut w.select[PETS_MENU], select::Nav::Down, crate::app::cursor::PAGE_STEP));
        assert_eq!(w.shown_pet().map(|p| p.slot), Some(4), "空栏位 3 跳过");
        assert!(select::select_key(&cols, &mut w.select[PETS_MENU], 0));
        assert_eq!(w.shown_pet().map(|p| p.slot), Some(0), "pet status 把选中项移过去");
        assert!(!select::select_key(&cols, &mut w.select[PETS_MENU], 3), "空栏位不能选");
    }

    #[test]
    fn selection_falls_back_to_nearest_when_pet_is_gone() {
        let mut w = world(vec![pet(0, "rest"), pet(2, "battle"), pet(4, "standby")]);
        select::select_key(&pet_columns(&w), &mut w.select[PETS_MENU], 2);
        w.pets.pets.retain(|p| p.slot != 2);
        assert_eq!(w.shown_pet().map(|p| p.slot), Some(4), "放生后落到原位置附近的一只");
    }

    #[test]
    fn enter_opens_the_action_menu_for_every_pet() {
        let mut fainted = pet(1, "rest");
        fainted.hp = 0;
        let w = world(vec![pet(0, "rest"), pet(2, "battle"), fainted]);
        let cols = pet_columns(&w);
        let enter = |slot: usize| cols[0].items[slot].enter.clone();
        for slot in [0, 1, 2] {
            assert_eq!(enter(slot), Some(OnEnter::Open), "{slot} 号宠物要打开动作菜单");
        }
        assert_eq!(enter(3), None, "空栏位没有动作");
    }

    #[test]
    fn no_pets_shows_nothing() {
        assert!(world(vec![]).shown_pet().is_none());
    }
}
