//! 背包的对象动作（STAGE-132，`docs/cli-ui-redesign.md`「对象、动作、参数」）：
//! 背包页选中一格后 `Enter` 打开动作列表（装备 / 卸下、使用、移动、丢弃、说明），
//! 需要参数的动作再走一步——使用先选目标、移动先选目的格、丢弃先填数量，最后落到确认。
//! 动作调用 `items.rs` 里同一批请求函数，和 `item` 命令共用校验、Pending 与推送处理。
//!
//! 一次动作流程只在导航栈上压**一层** `Layer::Action`，子步骤靠 `BagStep` 自己前进 / 后退；
//! 退到底（`bag_back` 返回 false）才把那一层弹掉。动作层不是文字输入层：普通字符吞掉，`/` 除外。

use crossterm::event::{KeyCode, KeyEvent};

use crate::proto::{InventorySlot, TargetType};

use super::bag_select::{FIRST_BAG_SLOT, MAX_EQUIP_SLOT, can_equip};
use super::input::TextInput;
use super::world::{BAG_LAST_SLOT, WorldState};
use super::{App, Layer};

mod inlay_pick;
mod types;
pub use types::*;

mod targets;
pub use targets::{UseTargets, use_targets};

impl WorldState {
    /// 动作流程有没有开着。
    pub fn bag_busy(&self) -> bool {
        self.bag_step.is_some()
    }

    /// 动作列表、目标列表、目的格等要选中项的项数。
    pub fn bag_step_rows(&self) -> usize {
        match self.bag_step.as_ref() {
            Some(BagStep::Actions { slot, .. }) => self
                .item_at(*slot)
                .map_or(0, |it| actions(*slot, &it).len()),
            Some(BagStep::Target { slot, .. }) => {
                self.item_at(*slot).map_or(0, |it| match use_targets(&it) {
                    UseTargets::Pick { down_only } => 1 + self.bag_target_pets(down_only).len(),
                    _ => 0,
                })
            }
            Some(BagStep::MoveTo { .. }) => BAG_LAST_SLOT + 1 - FIRST_BAG_SLOT,
            Some(BagStep::DropCount { .. }) => 0,
            Some(BagStep::InlayPick { .. }) => self.inlay_materials().len(),
            None => 0,
        }
    }

    /// 可以选的宠物栏位（`down_only` 时只看倒下的）。
    pub fn bag_target_pets(&self, down_only: bool) -> Vec<u8> {
        self.pets
            .pets
            .iter()
            .filter(|p| !down_only || p.hp == 0)
            .map(|p| p.slot)
            .collect()
    }

    /// 宠物栏位的名字（目标列表渲染用）。
    pub fn pet_name(&self, slot: u8) -> String {
        self.pets
            .pets
            .iter()
            .find(|p| p.slot == slot)
            .map(|p| p.name.clone())
            .unwrap_or_default()
    }

    /// 背包里某一格的物品（没有物品为 None）。
    pub fn item_at(&self, slot: u8) -> Option<InventorySlot> {
        self.inventory
            .slots
            .iter()
            .find(|s| s.slot == slot)
            .cloned()
    }
}

impl App {
    /// 背包页 `Enter`：打开选中格的对象动作（STAGE-132）。没选中实体格子时返回 false。
    pub(super) fn bag_open_actions(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let (column, marks) = world.bag_marks();
        let Some(key) = marks.get(column).copied().flatten() else {
            return false;
        };
        let slot = key as u8;
        if world.item_at(slot).is_none() {
            return false;
        }
        if let Some(world) = self.world.as_mut() {
            world.bag_revision = world.inventory_epoch;
            world.bag_step = Some(BagStep::Actions { slot, index: 0 });
            world.nav.push(Layer::Action);
        }
        true
    }

    /// 背包动作流程里的按键。返回 true 表示已处理。
    pub(super) fn bag_action_key(&mut self, key: KeyEvent) -> bool {
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.bag_step.is_some() && w.bag_revision != w.inventory_epoch)
        {
            self.close_bag_step();
            self.log.error("背包已变化，请重新选择物品");
            return true;
        }
        let Some(kind) = self
            .world
            .as_ref()
            .and_then(|w| w.bag_step.as_ref())
            .map(BagStep::kind)
        else {
            return false;
        };
        // 数量表单里的 `/` 是文字；其余步骤里 `/` 照常打开命令栏（STAGE-131 的入口）。
        if key.code == KeyCode::Char('/') && kind != StepKind::DropCount {
            if let Some(world) = self.world.as_mut() {
                world.nav.push(Layer::Command);
            }
            return true;
        }
        match kind {
            StepKind::Actions => self.bag_actions_key(key),
            StepKind::Target => self.bag_target_key(key),
            StepKind::MoveTo => self.bag_move_key(key),
            StepKind::DropCount => self.bag_count_key(key),
            StepKind::InlayPick => self.bag_inlay_key(key),
        }
    }

    /// 动作列表：`↑↓` 选，`Enter` 执行，`Esc` 退一步。
    fn bag_actions_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.bag_step_back(),
            KeyCode::Up => self.bag_move_action(false),
            KeyCode::Down => self.bag_move_action(true),
            KeyCode::Enter => self.bag_run_selected(),
            _ => true,
        }
    }

    /// 使用目标：`↑↓` 选自己或宠物，`Enter` 提交，`Esc` 回动作列表。
    fn bag_target_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.bag_step_back(),
            KeyCode::Up => self.bag_move_index(false),
            KeyCode::Down => self.bag_move_index(true),
            KeyCode::Enter => self.bag_use_selected_target(),
            _ => true,
        }
    }

    /// 移动目的格：`↑↓` 选 5–19，`Enter` 提交，`Esc` 回动作列表。当前格不可选。
    fn bag_move_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.bag_step_back(),
            KeyCode::Up => self.bag_move_index(false),
            KeyCode::Down => self.bag_move_index(true),
            KeyCode::Home => self.bag_set_index(0),
            KeyCode::End => self.bag_set_index(self.bag_rows_now().saturating_sub(1)),
            KeyCode::Enter => self.bag_move_selected(),
            _ => true,
        }
    }

    /// 丢弃数量表单：数字进输入框，`Enter` 进入确认，`Esc` 取消（不发请求）。
    fn bag_count_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.bag_step_back(),
            KeyCode::Enter => self.bag_count_submit(),
            _ => {
                if let Some(world) = self.world.as_mut() {
                    if let Some(BagStep::DropCount { text, .. }) = world.bag_step.as_mut() {
                        text.handle_key(&key);
                    }
                }
                true
            }
        }
    }

    /// `↑↓` 在动作列表里移动。
    fn bag_move_action(&mut self, down: bool) -> bool {
        self.bag_move_index(down)
    }

    /// `↑↓` 在当前这一步的列表里移动，到头停住。
    fn bag_move_index(&mut self, down: bool) -> bool {
        let rows = self.world.as_ref().map_or(0, WorldState::bag_step_rows);
        if rows == 0 {
            return true;
        }
        let Some(world) = self.world.as_mut() else {
            return true;
        };
        let index = step_index(&world.bag_step).unwrap_or(0).min(rows - 1);
        let next = if down {
            (index + 1).min(rows - 1)
        } else {
            index.saturating_sub(1)
        };
        set_step_index(&mut world.bag_step, next);
        true
    }

    fn bag_set_index(&mut self, index: usize) -> bool {
        let rows = self.world.as_ref().map_or(0, WorldState::bag_step_rows);
        if rows == 0 {
            return true;
        }
        if let Some(world) = self.world.as_mut() {
            set_step_index(&mut world.bag_step, index.min(rows - 1));
        }
        true
    }

    fn bag_rows_now(&self) -> usize {
        self.world.as_ref().map_or(0, WorldState::bag_step_rows)
    }

    /// `Esc`：动作流程退一步。退到底返回 false（调用方把 `Layer::Action` 弹掉）。
    pub(super) fn bag_step_back(&mut self) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        match world.bag_step.take() {
            Some(BagStep::Actions { .. }) | None => false,
            Some(BagStep::Target { slot, .. })
            | Some(BagStep::MoveTo { slot, .. })
            | Some(BagStep::DropCount { slot, .. })
            | Some(BagStep::InlayPick { slot, .. }) => {
                // 取消数量表单 / 目标 / 目的格：回到动作列表，不发任何请求。
                world.bag_step = Some(BagStep::Actions { slot, index: 0 });
                true
            }
        }
    }

    /// 关掉整个动作流程（动作完成或对象消失），并弹掉导航栈上那一层。
    pub(super) fn close_bag_step(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.bag_step = None;
            if world.nav.is(&Layer::Action) {
                world.nav.pop();
            }
        }
    }

    /// 执行动作列表里选中的那一项。
    fn bag_run_selected(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(BagStep::Actions { slot, index }) = world.bag_step.as_ref() else {
            return true;
        };
        let (slot, index) = (*slot, *index);
        // 对象消失（用掉、被换走）时动作作废，退回背包页（STAGE-132 验收标准 3）。
        let Some(it) = self.item_at(slot) else {
            self.close_bag_step();
            return true;
        };
        let acts = actions(slot, &it);
        let Some(&act) = acts.get(index) else {
            return true;
        };
        if act != Act::Describe && self.observer_blocks() {
            return true;
        }
        if let Some(reason) = disabled_reason(act, &it) {
            self.log.system(reason);
            return true;
        }
        match act {
            Act::Equip => {
                self.equip_slot(slot);
                self.close_bag_step();
            }
            Act::Unequip => {
                self.unequip_slot(slot);
                self.close_bag_step();
            }
            Act::Use => match use_targets(&it) {
                UseTargets::Direct(target) => {
                    self.use_item(slot, target);
                    self.close_bag_step();
                }
                UseTargets::Pick { .. } => {
                    if let Some(world) = self.world.as_mut() {
                        world.bag_step = Some(BagStep::Target { slot, index: 0 });
                    }
                }
                UseTargets::Blocked(reason) => self.log.system(reason),
            },
            Act::Move => {
                if let Some(world) = self.world.as_mut() {
                    world.bag_step = Some(BagStep::MoveTo {
                        slot,
                        index: first_move_index(slot),
                    });
                }
            }
            Act::Inlay => self.bag_start_inlay(slot),
            Act::Drop => {
                if let Some(world) = self.world.as_mut() {
                    world.bag_step = Some(BagStep::DropCount {
                        slot,
                        text: TextInput::new(5),
                    });
                }
            }
            // 说明：底部面板会画完整说明，这里把这一层留在原地即可（渲染时给足行数）。
            Act::Describe => self.log.system(format!("{}：{}", it.name, it.effect)),
        }
        true
    }

    /// 提交「使用」的目标。
    fn bag_use_selected_target(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(BagStep::Target { slot, index }) = world.bag_step.as_ref() else {
            return true;
        };
        let (slot, index) = (*slot, *index);
        let Some(it) = self.item_at(slot) else {
            self.close_bag_step();
            return true;
        };
        let target = match use_targets(&it) {
            UseTargets::Direct(t) => t,
            UseTargets::Pick { down_only } => {
                if index == 0 {
                    0
                } else {
                    match world.bag_target_pets(down_only).get(index - 1) {
                        // 协议目标编号：宠物栏位 0–19 → 1–20（STAGE-346）。
                        Some(slot) => slot + 1,
                        None => return true,
                    }
                }
            }
            UseTargets::Blocked(reason) => {
                self.log.system(reason);
                self.close_bag_step();
                return true;
            }
        };
        self.use_item(slot, target);
        self.close_bag_step();
        true
    }

    /// 提交「移动」的目的格。
    fn bag_move_selected(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(BagStep::MoveTo { slot, index }) = world.bag_step.as_ref() else {
            return true;
        };
        let (from, index) = (*slot, *index);
        let to = (FIRST_BAG_SLOT + index) as u8;
        if self.item_at(from).is_none() {
            self.close_bag_step();
            return true;
        }
        if to == from {
            self.log.system("选的是当前这一格，换一格试试");
            return true;
        }
        self.move_slot(from, to);
        self.close_bag_step();
        true
    }

    /// 提交「丢弃」的数量：先本地校验，再落到默认取消的确认。
    fn bag_count_submit(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(BagStep::DropCount { slot, text }) = world.bag_step.as_ref() else {
            return true;
        };
        let (slot, raw) = (*slot, text.text().to_string());
        let Some(it) = self.item_at(slot) else {
            self.close_bag_step();
            return true;
        };
        let max = it.count.max(1);
        let n = match raw.trim().parse::<u32>() {
            Ok(n) if (1..=max).contains(&n) => n,
            _ => {
                self.log.system(format!("数量要填 1–{max} 之间的整数"));
                return true;
            }
        };
        let what = if n >= max {
            it.name.clone()
        } else {
            format!("{} ×{n}", it.name)
        };
        let text = format!("丢弃后不能恢复：{what}（{slot} 号）");
        let count = if n >= max { None } else { Some(n) };
        self.ask_confirm(
            super::confirm::Confirm::ItemDrop {
                slot,
                name: it.name.clone(),
                count,
            },
            &text,
        );
        self.close_bag_step();
        true
    }
}

/// 移动目的格的初始选中项：跳过当前格（它不可选，STAGE-132 验收标准 3）。
fn first_move_index(from: u8) -> usize {
    if usize::from(from) == FIRST_BAG_SLOT {
        1
    } else {
        0
    }
}

/// 这一步的选中下标（数量表单没有列表，返回 None）。
fn step_index(step: &Option<BagStep>) -> Option<usize> {
    match step {
        Some(BagStep::Actions { index, .. })
        | Some(BagStep::Target { index, .. })
        | Some(BagStep::MoveTo { index, .. })
        | Some(BagStep::InlayPick { index, .. }) => Some(*index),
        _ => None,
    }
}

/// 把选中下标写回当前这一步。
fn set_step_index(step: &mut Option<BagStep>, index: usize) {
    match step {
        Some(BagStep::Actions { index: i, .. })
        | Some(BagStep::Target { index: i, .. })
        | Some(BagStep::MoveTo { index: i, .. })
        | Some(BagStep::InlayPick { index: i, .. }) => *i = index,
        _ => {}
    }
}

#[cfg(test)]
mod tests;
