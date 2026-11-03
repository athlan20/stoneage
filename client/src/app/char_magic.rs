//! 角色页精灵清单的「使用」动作（STAGE-231，`docs/ui.md` 角色页 C）：`↑↓` 选精灵，`Enter` 打开动作列表（只有「使用」），
//! 滋润的精灵再选对象（自己或宠物），然后发 `magic_use`（`docs/protocol.md` 5.51）。
//! 变幻的精灵保持命令栏的写法：`Enter` 把 `magic <栏位> ` 填进输入行，宠物编号由玩家接着敲。
//! 其他精灵直接发，不能在战斗外用的由服务端回复原因。队友一期服务端一律无效，选项里不出现。
//! 动作流程在导航栈上压一层 `Layer::Action`，步骤记在 `CharStep` 里，和背包的对象动作一致。

use crossterm::event::{KeyCode, KeyEvent};

use crate::proto::MagicView;

use super::char_action::CharStep;
use super::magic_use::{MagicTo, MagicUse, magic_use_kind};
use super::world::WorldState;
use super::{App, Layer};

impl WorldState {
    /// 精灵动作流程当前这一步有几行可选（动作列表 1 行；对象列表是自己加每只宠物）。
    pub fn magic_step_rows(&self) -> usize {
        match self.char_step.as_ref() {
            Some(CharStep::MagicAction { .. }) => 1,
            Some(CharStep::MagicTarget { .. }) => 1 + self.pets.pets.len(),
            _ => 0,
        }
    }

    /// 栏位上精灵的名字（渲染和提示用）。
    pub fn magic_name(&self, slot: u8) -> String {
        self.entry
            .status
            .magic
            .iter()
            .find(|m| m.slot == slot)
            .map(|m| m.name.clone())
            .unwrap_or_default()
    }
}

/// 精灵的 MP 够不够（前端先挡一下，省得选完对象才被拒；服务端仍是最终判断）。
fn mp_short(magic: &MagicView, mp: u32) -> bool {
    mp < magic.mp
}

impl App {
    /// 角色页 `Enter` 落在精灵栏上：打开这只精灵的动作列表。空栏位没有动作。
    pub(super) fn char_open_magic(&mut self, slot: u8) -> bool {
        let has = self
            .world
            .as_ref()
            .is_some_and(|w| w.entry.status.magic.iter().any(|m| m.slot == slot));
        if !has {
            self.log.system("这个栏位没有精灵");
            return true;
        }
        if self.observer_blocks() {
            return true;
        }
        if let Some(world) = self.world.as_mut() {
            world.char_step = Some(CharStep::MagicAction { slot });
            world.nav.push(Layer::Action);
        }
        true
    }

    /// 精灵动作流程里的按键：`↑↓` `←→` 选对象，`Enter` 执行，`Esc` 退一步，`/` 打开命令栏，其他字符吞掉。
    pub(super) fn char_magic_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.char_magic_back(),
            KeyCode::Up | KeyCode::Left => self.char_magic_move(false),
            KeyCode::Down | KeyCode::Right => self.char_magic_move(true),
            KeyCode::Enter => self.char_magic_enter(),
            KeyCode::Char('/') => {
                if let Some(world) = self.world.as_mut() {
                    world.nav.push(Layer::Command);
                }
            }
            _ => {}
        }
        true
    }

    /// `Esc`：对象列表退回动作列表，动作列表退掉整个流程。
    fn char_magic_back(&mut self) {
        let back = match self.world.as_ref().and_then(|w| w.char_step.as_ref()) {
            Some(CharStep::MagicTarget { slot, .. }) => Some(*slot),
            _ => None,
        };
        match back {
            Some(slot) => {
                if let Some(world) = self.world.as_mut() {
                    world.char_step = Some(CharStep::MagicAction { slot });
                }
            }
            None => self.close_char_step(),
        }
    }

    /// `↑↓` 在对象列表里移动，到头停住；动作列表只有一项，不用动。
    fn char_magic_move(&mut self, down: bool) {
        let rows = self.world.as_ref().map_or(0, WorldState::magic_step_rows);
        if rows <= 1 {
            return;
        }
        if let Some(CharStep::MagicTarget { index, .. }) = self.world.as_mut().and_then(|w| w.char_step.as_mut()) {
            *index = if down {
                (*index + 1).min(rows - 1)
            } else {
                index.saturating_sub(1)
            };
        }
    }

    /// `Enter`：动作列表执行「使用」，对象列表提交选中的对象。
    fn char_magic_enter(&mut self) {
        let step = match self.world.as_ref().and_then(|w| w.char_step.as_ref()) {
            Some(CharStep::MagicAction { slot }) => Some((*slot, None)),
            Some(CharStep::MagicTarget { slot, index }) => Some((*slot, Some(*index))),
            _ => None,
        };
        match step {
            Some((slot, None)) => self.char_magic_use(slot),
            Some((slot, Some(index))) => self.char_magic_send_target(slot, index),
            None => {}
        }
    }

    /// 动作「使用」：变幻的精灵填命令，治愈、恩惠等直接发，滋润的精灵进入对象列表。
    fn char_magic_use(&mut self, slot: u8) {
        let Some(magic) = self
            .world
            .as_ref()
            .and_then(|w| w.entry.status.magic.iter().find(|m| m.slot == slot).cloned())
        else {
            self.close_char_step();
            self.log.system("这个栏位没有精灵");
            return;
        };
        match magic_use_kind(&magic) {
            MagicUse::Metamo => {
                self.close_char_step();
                if let Some(world) = self.world.as_mut() {
                    world.input.set(&format!("magic {slot} "));
                    world.history.reset();
                }
            }
            MagicUse::Pick => {
                let mp = self.world.as_ref().map_or(0, |w| w.entry.status.mp);
                if mp_short(&magic, mp) {
                    self.log
                        .error(format!("MP 不够，{}需要 MP {}（现有 {}）", magic.name, magic.mp, mp));
                    return;
                }
                if let Some(world) = self.world.as_mut() {
                    world.char_step = Some(CharStep::MagicTarget { slot, index: 0 });
                }
            }
            MagicUse::Direct => {
                self.close_char_step();
                self.send_magic_use(slot, None);
            }
        }
    }

    /// 对象列表提交：第 0 项是自己，之后依次是每只宠物。
    fn char_magic_send_target(&mut self, slot: u8, index: usize) {
        let to = if index == 0 {
            MagicTo::SelfOnly
        } else {
            let pet = self
                .world
                .as_ref()
                .and_then(|w| w.pets.pets.get(index - 1))
                .map(|p| p.slot);
            match pet {
                Some(pet) => MagicTo::Pet(pet),
                None => return,
            }
        };
        self.close_char_step();
        self.send_magic_use(slot, Some(to));
    }
}

#[cfg(test)]
mod tests;
