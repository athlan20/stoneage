//! Backspace 备用返回键（STAGE-172，`docs/interaction.md`「返回键」）：有的键盘（如 Mac 要按 fn+Esc）发不出单独的 Esc，
//! 这时 Backspace 和 Esc 一样逐层返回。只在世界里生效，并且只在能返回的层上替换成 Esc：
//! 有文字输入焦点（命令栏、表单、搜索框）时 Backspace 照旧删字；战斗、确认层、NPC 窗口的按键归它们自己；
//! 场景根页的 Esc 是打开游戏菜单，Backspace 在那里什么也不做。

use crossterm::event::{KeyCode, KeyEvent};

use super::App;
use super::system::SystemStep;
use super::world::WorldState;
use super::{BagStep, BrowseStep, CharStep, Layer, PetStep, SceneStep};

impl App {
    /// 世界里的按键先过这一层：Backspace 在能返回的层里换成 Esc，其余键和其他情况原样返回。
    pub(super) fn back_alias(&self, key: KeyEvent) -> KeyEvent {
        match self.world.as_ref() {
            Some(w) if key.code == KeyCode::Backspace && backspace_returns(w) => KeyEvent {
                code: KeyCode::Esc,
                ..key
            },
            _ => key,
        }
    }
}

/// Backspace 是否等同 Esc（见模块注释）。
fn backspace_returns(w: &WorldState) -> bool {
    // 战斗、战斗面板、确认层、NPC 窗口：这些层的按键归它们自己，不接管。
    if w.battle_front() || w.system.battle_open || w.confirm.is_some() || w.npc_window.is_some() {
        return false;
    }
    if has_text_focus(w) {
        return false;
    }
    // 场景根页：Esc 只有在没有打开的面板和详情时才是打开游戏菜单，这时 Backspace 不动。
    let scene_root = matches!(w.nav.top(), Layer::Scene) && w.scene.prompt.is_none() && !w.scene.details;
    !scene_root
}

/// 当前有没有文字输入焦点：这些层的 Backspace 只删字符。
/// 系统页的撤销令牌确认是默认取消的确认层，同样不接管 Backspace。
fn has_text_focus(w: &WorldState) -> bool {
    w.nav.is(&Layer::Command)
        || w.nav.is(&Layer::Chat)
        || w.travel.pets.typing
        || matches!(w.bag_step, Some(BagStep::DropCount { .. }))
        || matches!(w.pet_step, Some(PetStep::Rename { .. }))
        || matches!(w.char_step, Some(CharStep::AddPoint { .. }))
        || matches!(w.scene_step, Some(SceneStep::Catalog { .. }))
        || matches!(w.browse.step, Some(BrowseStep::Search { .. }))
        || matches!(
            w.system.step,
            Some(SystemStep::Chat(_) | SystemStep::Create { .. } | SystemStep::Revoke { .. })
        )
        // 自动战斗设置里的方案命名、名单输入、等级手输框：Backspace 删字，不关框。
        || w.auto_edit.as_ref().is_some_and(|e| e.naming.is_some() || e.names.is_some() || e.level.is_some())
}
