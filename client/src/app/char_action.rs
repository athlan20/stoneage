//! 角色加点（STAGE-133，`docs/cli-ui-redesign.md`「对象、动作、参数」）：
//! 角色页的能力栏选中一项后 `Enter` 打开数量表单，填好点数回车提交。
//! 请求与 `addpoint` 命令共用一条路径（`points.rs`），数值由 `status_changed` 推送更新。
//!
//! 精灵栏的「使用」动作在 `char_magic.rs`（STAGE-231），这里只把按键和 `Enter` 分发过去。

use crossterm::event::{KeyCode, KeyEvent};

use serde_json::json;

use super::char_select::{ABILITIES, ABILITY_COLUMN, MAGIC_COLUMN, char_columns};
use super::input::TextInput;
use super::select;
use super::session::Pending;
use super::world::{CHAR_MENU, WorldState};
use super::{App, Layer};

/// 能力项（`ABILITIES` 的下标）对应的协议能力键，顺序与 `ABILITIES` 一致。
const ABILITY_KEYS: [&str; 4] = ["vital", "strength", "toughness", "dexterity"];

/// 加点数量最多填几位（未分配点数不会超过这个量级）。
const COUNT_MAX_CHARS: usize = 3;

/// 角色页的当前一步。
#[derive(Debug)]
pub enum CharStep {
    /// 加点数量表单：`stat` 是 `ABILITIES` 的下标。
    AddPoint { stat: usize, text: TextInput },
    /// 精灵动作列表（STAGE-231，只有「使用」）：`slot` 是精灵栏位。
    MagicAction { slot: u8 },
    /// 精灵的对象列表（STAGE-231）：`index` 0 是自己，`1..` 是第 index 只宠物。
    MagicTarget { slot: u8, index: usize },
}

impl CharStep {
    /// 这一步的标题（渲染用）。
    pub fn title(&self) -> String {
        match self {
            CharStep::AddPoint { stat, .. } => format!("把点数加到 {}{}", ABILITIES[*stat], "上"),
            CharStep::MagicAction { .. } => "精灵".to_string(),
            CharStep::MagicTarget { .. } => "对谁使用".to_string(),
        }
    }
}

impl WorldState {
    /// 角色页这一步有没有选中项的列表（加点表单没有，返回 0）。
    pub fn char_step_rows(&self) -> usize {
        0
    }
}

impl App {
    /// 角色页 `Enter`：焦点在能力栏时打开加点表单。返回 false 表示这一层没接手。
    pub(super) fn char_open_addpoint(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let columns = char_columns(world);
        let Some((column, index)) = select::locate(&columns, &world.select[CHAR_MENU]) else {
            return false;
        };
        // STAGE-231：精灵栏 Enter 打开精灵的动作列表（空栏位在 `char_open_magic` 里解释）。
        if columns[column].name == MAGIC_COLUMN {
            let slot = columns[column].items[index].key as u8;
            return self.char_open_magic(slot);
        }
        if columns[column].name != ABILITY_COLUMN {
            return false;
        }
        let left = world.entry.status.skill_points;
        if self.observer_blocks(){return true;}
        // 没有未分配点数：解释原因，不开表单（STAGE-133 验收标准 2）。
        if left == 0 {
            self.log.system("没有未分配点数，先升级或用管理命令发点数");
            return true;
        }
        if let Some(world) = self.world.as_mut() {
            world.char_step = Some(CharStep::AddPoint { stat: index, text: TextInput::new(COUNT_MAX_CHARS) });
            world.nav.push(Layer::Form);
        }
        true
    }

    /// 加点表单的按键。返回 true 表示已处理。
    pub(super) fn char_action_key(&mut self, key: KeyEvent) -> bool {
        if self.world.as_ref().is_some_and(|w| matches!(w.char_step, Some(CharStep::MagicAction { .. } | CharStep::MagicTarget { .. }))) {
            return self.char_magic_key(key);
        }
        match key.code {
            KeyCode::Esc => self.char_step_back(),
            KeyCode::Enter => self.addpoint_submit(),
            _ => {
                if let Some(world) = self.world.as_mut() {
                    if let Some(CharStep::AddPoint { text, .. }) = world.char_step.as_mut() {
                        text.handle_key(&key);
                    }
                }
                true
            }
        }
    }

    /// `Esc`：取消加点表单，不发请求（STAGE-133 验收标准 2）。
    fn char_step_back(&mut self) -> bool {
        self.close_char_step();
        true
    }

    /// 关掉加点表单，并弹掉导航栈上那一层。
    pub(super) fn close_char_step(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.char_step = None;
            if matches!(world.nav.top(), Layer::Form | Layer::Action) {
                world.nav.pop();
            }
        }
    }

    /// 提交加点：点数要是不大于未分配点数的正整数，错误留在表单里。
    fn addpoint_submit(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(CharStep::AddPoint { stat, text }) = world.char_step.as_ref() else {
            return true;
        };
        let (stat, raw) = (*stat, text.text().to_string());
        let left = world.entry.status.skill_points;
        if left == 0 {
            self.log.system("未分配点数已经用完了");
            self.close_char_step();
            return true;
        }
        let points = match raw.trim().parse::<u32>() {
            Ok(n) if (1..=left).contains(&n) => n,
            _ => {
                self.log.system(format!("要填 1–{left} 之间的整数"));
                return true;
            }
        };
        let label = ABILITIES[stat];
        let key = ABILITY_KEYS[stat];
        let pending = Pending::AddPoint { stat: label.to_string(), points };
        self.send("addpoint", json!({"stat": key, "points": points}), pending);
        self.close_char_step();
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{Status, WorldEntry};

    fn app_with_points(points: u32) -> App {
        use super::super::Options;
        use crate::app::Screen;
        let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
        let mut entry = WorldEntry::default();
        entry.status = Status { skill_points: points, ..Status::default() };
        let mut w = WorldState::new(entry);
        w.menu = CHAR_MENU;
        w.nav.push(Layer::Page(CHAR_MENU));
        a.world = Some(w);
        a.screen = Screen::World;
        a
    }

    fn key(a: &mut App, code: KeyCode) {
        a.on_key(KeyEvent::new(code, crossterm::event::KeyModifiers::NONE));
    }

    #[test]
    fn ability_keys_line_up_with_the_labels() {
        assert_eq!(ABILITY_KEYS.len(), ABILITIES.len());
    }

    #[test]
    fn no_points_explains_instead_of_opening_the_form() {
        let mut a = app_with_points(0);
        // 精灵栏是空的，焦点自动落在能力栏（不用 Tab）。
        key(&mut a, KeyCode::Enter);
        assert!(a.world.as_ref().unwrap().char_step.is_none(), "没有点数不开表单");
        assert!(a.log.all().iter().any(|m| m.text.contains("没有未分配点数")));
    }
}
