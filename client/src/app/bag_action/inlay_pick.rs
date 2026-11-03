//! 动作「精工」的材料选择步骤（STAGE-299，`docs/inlay.md` 2.8）：列出背包里的材料，`Space` 勾选或取消，
//! `Enter` 进入确认（确认窗口默认取消，见 `inventory_inlay.rs`），`Esc` 回动作列表。

use crossterm::event::{KeyCode, KeyEvent};

use super::super::App;
use super::super::inventory_inlay::{gear_free, is_material, max_pick};
use super::super::world::WorldState;
use super::BagStep;
use crate::proto::InventorySlot;

impl WorldState {
    /// 背包里的材料格（`role = material`），按格号从小到大。
    pub fn inlay_materials(&self) -> Vec<InventorySlot> {
        let mut list: Vec<InventorySlot> = self
            .inventory
            .slots
            .iter()
            .filter(|it| is_material(it))
            .cloned()
            .collect();
        list.sort_by_key(|it| it.slot);
        list
    }
}

impl App {
    /// 动作「精工」：背包里没有材料就只提示，不进入选择。
    pub(super) fn bag_start_inlay(&mut self, slot: u8) {
        let has_materials = self.world.as_ref().is_some_and(|w| !w.inlay_materials().is_empty());
        if !has_materials {
            return self.log.system("背包里没有可以用来精工的材料");
        }
        if let Some(world) = self.world.as_mut() {
            world.bag_step = Some(BagStep::InlayPick {
                slot,
                index: 0,
                picked: Vec::new(),
            });
        }
    }

    /// 材料选择的按键：`↑↓` 选，`Space` 勾选，`Enter` 进入确认，`Esc` 回动作列表。
    pub(super) fn bag_inlay_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.bag_step_back(),
            KeyCode::Up => self.bag_move_index(false),
            KeyCode::Down => self.bag_move_index(true),
            KeyCode::Char(' ') => self.bag_inlay_toggle(),
            KeyCode::Enter => self.bag_inlay_submit(),
            _ => true,
        }
    }

    /// `Space`：勾选或取消当前材料。已经勾满（最多剩余空格数、且不超过 3 件）时不再勾。
    fn bag_inlay_toggle(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let (target, index, mut picked) = match world.bag_step.as_ref() {
            Some(BagStep::InlayPick { slot, index, picked }) => (*slot, *index, picked.clone()),
            _ => return true,
        };
        let materials = world.inlay_materials();
        let Some(mat) = materials.get(index).map(|it| it.slot) else {
            return true;
        };
        let free = world.item_at(target).and_then(|it| gear_free(&it)).unwrap_or(0);
        let refused = if let Some(pos) = picked.iter().position(|&s| s == mat) {
            picked.remove(pos);
            None
        } else if picked.len() >= max_pick(free) {
            Some(format!("最多勾 {} 件（剩余 {free} 格）", max_pick(free)))
        } else {
            picked.push(mat);
            None
        };
        if let Some(text) = refused {
            self.log.error(text);
            return true;
        }
        if let Some(BagStep::InlayPick { picked: p, .. }) = self.world.as_mut().and_then(|w| w.bag_step.as_mut()) {
            *p = picked;
        }
        true
    }

    /// `Enter`：没勾材料时提示；否则进入确认（默认取消），动作流程随之收掉。
    fn bag_inlay_submit(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let (target, picked) = match world.bag_step.as_ref() {
            Some(BagStep::InlayPick { slot, picked, .. }) => (*slot, picked.clone()),
            _ => return true,
        };
        if picked.is_empty() {
            self.log.system("先用 Space 勾选材料");
            return true;
        }
        self.request_inlay(target, picked, false);
        self.close_bag_step();
        true
    }
}
