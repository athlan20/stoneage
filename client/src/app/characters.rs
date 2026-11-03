//! 角色选择页：进入、创建、删除。

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::json;

use super::session::Pending;
use super::{App, CreateState, Screen, TextInput, cycle};
use crate::proto::{Character, CharacterList};

/// 删除确认：玩家要再输入一次角色名。
#[derive(Debug)]
pub struct DeleteConfirm {
    pub slot: u8,
    pub name: String,
    pub input: TextInput,
    pub buttons: bool,
    pub delete: bool,
}

#[derive(Debug, Default)]
pub struct CharsState {
    pub list: CharacterList,
    /// 当前选中的栏位（0 起，对应协议里的 `slot`）。
    pub selected: usize,
    pub confirm: Option<DeleteConfirm>,
    pub actions: Option<usize>,
}

impl CharsState {
    pub fn slots(&self) -> usize {
        usize::from(self.list.max_slots)
    }

    pub fn character_at(&self, slot: usize) -> Option<&Character> {
        self.list.characters.iter().find(|c| usize::from(c.slot) == slot)
    }
}

impl App {
    pub(super) fn on_characters(&mut self, list: CharacterList) {
        let keep = self.chars.selected;
        self.chars.list = list;
        self.chars.confirm = None;
        self.chars.actions = None;
        self.chars.selected = keep.min(self.chars.slots().saturating_sub(1));
        self.screen = Screen::Characters;
        if self.reconnect_characters(){return;}
        if let Some(wanted) = self.take_auto_character() {
            self.auto_enter(&wanted);
        }
    }

    /// `--character` 指定的角色：先按名字找，再按显示栏位（从 1 起）找。
    fn auto_enter(&mut self, wanted: &str) {
        let by_name = self.chars.list.characters.iter().find(|c| c.name == wanted).map(|c| c.slot);
        let by_slot = wanted
            .parse::<usize>()
            .ok()
            .and_then(|n| n.checked_sub(1))
            .and_then(|slot| self.chars.character_at(slot))
            .map(|c| c.slot);
        match by_name.or(by_slot) {
            Some(slot) => {
                self.chars.selected = usize::from(slot);
                self.enter_selected();
            }
            None => self.log.error(format!("找不到角色“{wanted}”")),
        }
    }

    pub(super) fn characters_key(&mut self, key: KeyEvent) {
        if key.code==KeyCode::Esc && self.cancel_reconnect(){return;}
        if self.chars.confirm.is_some() {
            self.confirm_key(key);
            return;
        }
        if self.chars.actions.is_some() { self.character_actions_key(key); return; }
        let n = self.chars.slots().max(1);
        match key.code {
            KeyCode::Esc => {
                self.drop_link();
                self.screen = Screen::Login;
                self.chars = CharsState::default();
                self.create = CreateState::default();
                self.login.password.clear();
                self.log.system("已返回登录页");
            }
            KeyCode::Up => self.chars.selected = cycle(self.chars.selected, n, false),
            KeyCode::Down | KeyCode::Tab => self.chars.selected = cycle(self.chars.selected, n, true),
            KeyCode::Enter => {
                if self.session.busy() { return; }
                if self.chars.character_at(self.chars.selected).is_some() { self.chars.actions=Some(0); }
                else { self.enter_selected(); }
            }
            KeyCode::Char('d' | 'D') => self.ask_delete(),
            _ => {}
        }
    }

    fn enter_selected(&mut self) {
        if self.session.busy() {
            return;
        }
        let slot = self.chars.selected;
        if self.chars.character_at(slot).is_some() {
            self.send("enter_world", json!({"slot": slot}), Pending::Enter);
        } else {
            self.create = CreateState::new(slot as u8);
            self.screen = Screen::Create;
        }
    }

    fn ask_delete(&mut self) {
        let Some(c) = self.chars.character_at(self.chars.selected) else {
            self.log.error("这个栏位没有角色");
            return;
        };
        self.chars.confirm = Some(DeleteConfirm {
            slot: c.slot,
            name: c.name.clone(),
            input: TextInput::new(32),
            buttons: false,
            delete: false,
        });
    }

    fn confirm_key(&mut self, key: KeyEvent) {
        let Some(confirm) = self.chars.confirm.as_mut() else {
            return;
        };
        match key.code {
            KeyCode::Esc => self.chars.confirm = None,
            KeyCode::Tab | KeyCode::BackTab => confirm.buttons=!confirm.buttons,
            KeyCode::Left | KeyCode::Right if confirm.buttons => confirm.delete=!confirm.delete,
            KeyCode::Enter => {
                if !confirm.buttons {
                    if confirm.input.text()!=confirm.name { self.log.error("输入的名字与角色名不一致，没有删除");return; }
                    confirm.buttons=true;confirm.delete=false;return;
                }
                if !confirm.delete { self.chars.confirm=None;return; }
                if confirm.input.text() != confirm.name {
                    self.log.error("输入的名字与角色名不一致，没有删除");
                    return;
                }
                let (slot, name) = (confirm.slot, confirm.name.clone());
                if !self.session.busy() {
                    self.send("delete_character", json!({"slot": slot, "name": name}), Pending::Delete);
                }
            }
            _ if !confirm.buttons => {
                confirm.input.handle_key(&key);
            }
            _ => {}
        }
    }

    fn character_actions_key(&mut self,key:KeyEvent) {
        let Some(index)=self.chars.actions.as_mut() else{return;};
        match key.code {
            KeyCode::Esc=>self.chars.actions=None,
            KeyCode::Up=>*index=index.saturating_sub(1),
            KeyCode::Down=>*index=(*index+1).min(2),
            KeyCode::Enter=>match *index {
                0=>self.enter_selected(),
                1=>self.ask_delete(),
                _=>self.chars.actions=None,
            },
            _=>{}
        }
    }
}
