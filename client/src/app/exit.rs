//! STAGE-137：所有屏幕的可见退出确认；离开世界等待真实响应。
use super::{App, session::Pending};
use crossterm::event::{KeyCode, KeyEvent};
use serde_json::json;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExitKind {
    Quit,
    Leave,
}

#[derive(Debug, Clone, Copy)]
pub struct ExitDialog {
    pub kind: ExitKind,
    pub confirm: bool,
}

impl App {
    pub(super) fn request_exit(&mut self, kind: ExitKind) {
        if kind == ExitKind::Leave && self.world.as_ref().is_some_and(|w| w.battle.is_some()) {
            self.log.error("战斗中不能离开世界");
            return;
        }
        self.exit_dialog = Some(ExitDialog {
            kind,
            confirm: false,
        });
    }

    pub(super) fn exit_key(&mut self, key: KeyEvent) {
        let Some(dialog) = self.exit_dialog.as_mut() else {
            return;
        };
        match key.code {
            KeyCode::Esc => self.exit_dialog = None,
            KeyCode::Left | KeyCode::Right | KeyCode::Tab | KeyCode::BackTab => {
                dialog.confirm = !dialog.confirm
            }
            KeyCode::Enter => {
                if !dialog.confirm {
                    self.exit_dialog = None;
                    return;
                }
                let kind = dialog.kind;
                if kind == ExitKind::Quit {
                    self.should_quit = true;
                    self.exit_dialog = None;
                } else if !self.session.busy() {
                    if self.world.as_ref().is_some_and(|w| w.battle.is_some()) {
                        self.log.error("战斗中不能离开世界");
                        self.exit_dialog = None;
                        return;
                    }
                    self.exit_dialog = None;
                    self.send("leave_world", json!({}), Pending::Leave);
                }
            }
            _ => {}
        }
    }
}
