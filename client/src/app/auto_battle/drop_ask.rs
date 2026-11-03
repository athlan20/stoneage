//! 「战后自动丢弃肉类道具」从关切到开的确认框（STAGE-260）：默认「取消」，←→ 选到「开启」再 Enter 才写进草稿。
//! 丢弃不可恢复，所以不接受静默开启；Esc 只关掉确认框，不退出编辑。

use crossterm::event::KeyCode;

use super::edit::Editor;

/// 确认框开着时的按键：两个选项（0 取消、1 开启）。
pub(super) fn key(ed: &mut Editor, code: KeyCode) {
    match code {
        KeyCode::Left | KeyCode::Right | KeyCode::Tab | KeyCode::BackTab => {
            ed.ask_choice = (ed.ask_choice + 1) % 2;
        }
        KeyCode::Enter => {
            ed.ask = None;
            if ed.ask_choice == 1 {
                ed.draft.recover.post_drop_item = true;
                ed.dirty = true;
                ed.hint = Some("已开启战后丢弃肉类，记得「保存并应用」".into());
            } else {
                ed.hint = Some("已取消，战后丢弃保持关闭".into());
            }
        }
        KeyCode::Esc => {
            ed.ask = None;
            ed.hint = Some("已取消，战后丢弃保持关闭".into());
        }
        _ => {}
    }
}
