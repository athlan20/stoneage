//! 创建角色：外形、名字与属性、出生地三步。

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::json;

use super::session::Pending;
use super::{App, Screen, TextInput, cycle};
use crate::proto::{
    BODY_COUNT, COLOR_COUNT, Elements, HOMETOWNS, NAME_MAX_CHARS, ProtoError, Stats, check_character_name,
};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CreateStep {
    Look,
    Stats,
    Home,
}

impl CreateStep {
    pub fn index(self) -> usize {
        match self {
            Self::Look => 0,
            Self::Stats => 1,
            Self::Home => 2,
        }
    }
}

/// 第一步的焦点行。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CreateFocus {
    Body,
    Color,
}

const LOOK_ORDER: [CreateFocus; 2] = [CreateFocus::Body, CreateFocus::Color];

/// 眼型、嘴型不让玩家选，一律用默认值（协议字段 `appearance.eye`、`appearance.mouth` 仍然要填）。
const DEFAULT_FACE: u8 = 0;

/// 第二步焦点行数：名字 1 行 + 能力点 4 行 + 属性点 4 行。
pub const STATS_ROWS: usize = 9;
pub const ROW_NAME: usize = 0;
pub const ROW_STATS: usize = 1;
pub const ROW_ELEMENTS: usize = 5;

#[derive(Debug)]
pub struct CreateState {
    pub slot: u8,
    pub step: CreateStep,
    pub look_focus: CreateFocus,
    pub body: u8,
    pub color: u8,
    pub name: TextInput,
    pub row: usize,
    pub stats: Stats,
    pub elements: Elements,
    /// 能力点没分完时，第一次按 Enter 只提醒；这里记录是否已提醒过。
    pub warned: bool,
    pub hometown: usize,
}

impl Default for CreateState {
    fn default() -> Self {
        Self::new(0)
    }
}

impl CreateState {
    pub fn new(slot: u8) -> Self {
        Self {
            slot,
            step: CreateStep::Look,
            look_focus: CreateFocus::Body,
            body: 0,
            color: 0,
            name: TextInput::new(NAME_MAX_CHARS),
            row: ROW_NAME,
            stats: Stats { vital: 5, strength: 5, toughness: 5, dexterity: 5 },
            elements: Elements { earth: 5, water: 5, fire: 0, wind: 0 },
            warned: false,
            hometown: 0,
        }
    }

    pub fn request(&self) -> serde_json::Value {
        json!({
            "slot": self.slot,
            "name": self.name.text().trim(),
            "appearance": {"body": self.body, "color": self.color, "eye": DEFAULT_FACE, "mouth": DEFAULT_FACE},
            "stats": self.stats.to_json(),
            "elements": self.elements.to_json(),
            "hometown": HOMETOWNS[self.hometown].id,
        })
    }
}

fn step_value(v: u8, count: u8, forward: bool) -> u8 {
    cycle(usize::from(v), usize::from(count), forward) as u8
}

impl App {
    pub(super) fn create_key(&mut self, key: KeyEvent) {
        match self.create.step {
            CreateStep::Look => self.look_key(key),
            CreateStep::Stats => self.stats_key(key),
            CreateStep::Home => self.home_key(key),
        }
    }

    fn look_key(&mut self, key: KeyEvent) {
        let c = &mut self.create;
        let idx = LOOK_ORDER.iter().position(|f| *f == c.look_focus).unwrap_or(0);
        match key.code {
            KeyCode::Esc => self.screen = Screen::Characters,
            KeyCode::Enter => c.step = CreateStep::Stats,
            KeyCode::Up | KeyCode::BackTab => c.look_focus = LOOK_ORDER[cycle(idx, LOOK_ORDER.len(), false)],
            KeyCode::Down | KeyCode::Tab => c.look_focus = LOOK_ORDER[cycle(idx, LOOK_ORDER.len(), true)],
            KeyCode::Left | KeyCode::Right => {
                let fwd = key.code == KeyCode::Right;
                match c.look_focus {
                    CreateFocus::Body => c.body = step_value(c.body, BODY_COUNT, fwd),
                    CreateFocus::Color => c.color = step_value(c.color, COLOR_COUNT, fwd),
                }
            }
            _ => {}
        }
    }

    fn stats_key(&mut self, key: KeyEvent) {
        let c = &mut self.create;
        match key.code {
            KeyCode::Esc => c.step = CreateStep::Look,
            KeyCode::Up | KeyCode::BackTab => c.row = cycle(c.row, STATS_ROWS, false),
            KeyCode::Down | KeyCode::Tab => c.row = cycle(c.row, STATS_ROWS, true),
            KeyCode::Enter => self.finish_stats(),
            KeyCode::Left | KeyCode::Right if c.row != ROW_NAME => {
                let up = key.code == KeyCode::Right;
                c.warned = false;
                if c.row < ROW_ELEMENTS {
                    let i = c.row - ROW_STATS;
                    if up {
                        if c.stats.remaining() > 0 {
                            *c.stats.get_mut(i) += 1;
                        }
                    } else if *c.stats.get_mut(i) > 0 {
                        *c.stats.get_mut(i) -= 1;
                    }
                } else {
                    let i = c.row - ROW_ELEMENTS;
                    if up { c.elements.increase(i) } else { c.elements.decrease(i) };
                }
            }
            _ if c.row == ROW_NAME => {
                c.name.handle_key(&key);
            }
            _ => {}
        }
    }

    fn finish_stats(&mut self) {
        if let Err(e) = check_character_name(self.create.name.text()) {
            self.create.row = ROW_NAME;
            self.log.error(e);
            return;
        }
        let left = self.create.stats.remaining();
        if left > 0 && !self.create.warned {
            self.create.warned = true;
            self.log.error(format!("还有 {left} 点能力点未分配，再按一次 Enter 确认继续"));
            return;
        }
        if self.create.elements.remaining() > 0 {
            self.log.error(format!("还有 {} 点属性点未分配", self.create.elements.remaining()));
            return;
        }
        self.create.step = CreateStep::Home;
    }

    fn home_key(&mut self, key: KeyEvent) {
        let c = &mut self.create;
        match key.code {
            KeyCode::Esc => c.step = CreateStep::Stats,
            KeyCode::Left | KeyCode::Up => c.hometown = cycle(c.hometown, HOMETOWNS.len(), false),
            KeyCode::Right | KeyCode::Down | KeyCode::Tab => c.hometown = cycle(c.hometown, HOMETOWNS.len(), true),
            KeyCode::Enter => {
                if self.session.busy() {
                    return;
                }
                self.log.system("正在创建角色…");
                let req = self.create.request();
                self.send("create_character", req, Pending::Create);
            }
            _ => {}
        }
    }

    /// 创建失败：回到出错字段所在的那一步（错误说明已由调用方写进消息栏）。
    pub(super) fn on_create_error(&mut self, err: &ProtoError) {
        match err.code.as_str() {
            "INVALID_APPEARANCE" => self.create.step = CreateStep::Look,
            "INVALID_NAME" | "NAME_TAKEN" => {
                self.create.step = CreateStep::Stats;
                self.create.row = ROW_NAME;
            }
            "INVALID_STATS" | "INVALID_ELEMENTS" => self.create.step = CreateStep::Stats,
            "INVALID_SLOT" | "SLOT_OCCUPIED" => {
                self.send("characters", json!({}), Pending::Characters);
            }
            _ => {}
        }
    }
}
