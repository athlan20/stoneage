//! STAGE-295：交易窗口的按键（`docs/trade.md` 5.2、`docs/interaction.md`）。窗口开着时占住按键，页面按键不接收。
//! 纯逻辑放在 `handle`：只改窗口状态，要发的请求和本地拒绝的提示放进 `Outcome`，由 `App::trade_key` 去发。

use crossterm::event::{KeyCode, KeyEvent};

use super::trade::TradeOp;
use super::trade_state::{Focus, Picker, TradeWindow};
use super::{App, WorldState};
use crate::proto::Trade;

/// 石币输入最多几位（防止溢出，足够表示上限）。
const GOLD_DIGITS: usize = 10;

/// 按键的结果：要发的请求，和本地拒绝时写进消息栏的一句。
#[derive(Debug, Default, PartialEq, Eq)]
pub struct Outcome {
    pub ops: Vec<TradeOp>,
    pub notice: Option<String>,
}

/// 物品行的候选：`None` 是「清空」（只在该行有东西时有），`Some(格号)` 是背包里能放的物品（格 5–19、有数量、没放在另一行）。
pub fn item_candidates(w: &WorldState, row: usize) -> Vec<Option<u32>> {
    let Some(t) = w.trade.as_ref() else {
        return vec![None];
    };
    let taken: Vec<u32> = t.trade.me.items.iter().filter_map(|i| i.slot).collect();
    let current = t.trade.me.items.get(row).and_then(|i| i.slot);
    let mut v = Vec::new();
    if current.is_some() {
        v.push(None);
    }
    for s in &w.inventory.slots {
        let slot = u32::from(s.slot);
        if !(5..=19).contains(&slot) || s.count == 0 {
            continue;
        }
        if taken.contains(&slot) && current != Some(slot) {
            continue;
        }
        v.push(Some(slot));
    }
    v
}

/// 宠物行的候选：`None` 是「清空」，`Some(栏位)` 是宠物栏里的宠物。
pub fn pet_candidates(w: &WorldState) -> Vec<Option<u32>> {
    let has_pet = w.trade.as_ref().is_some_and(|t| t.trade.me.pet.is_some());
    let mut v = Vec::new();
    if has_pet {
        v.push(None);
    }
    v.extend(w.pets.pets.iter().map(|p| Some(u32::from(p.slot))));
    v
}

/// 候选的显示文字（列表里一行）。
pub fn candidate_label(w: &WorldState, row: usize, cand: Option<u32>) -> String {
    let Some(slot) = cand else {
        return "清空".into();
    };
    if row == 3 {
        return w
            .pets
            .pets
            .iter()
            .find(|p| u32::from(p.slot) == slot)
            .map_or_else(|| format!("宠物 {slot}"), |p| format!("{} Lv{}", p.name, p.level));
    }
    w.inventory
        .slots
        .iter()
        .find(|s| u32::from(s.slot) == slot)
        .map_or_else(|| format!("格 {slot}"), |s| format!("{} x{}", s.name, s.count))
}

/// 当前报价的三项（物品格号、石币、宠物栏位），整体替换时用。
fn offer_of(t: &Trade) -> (Vec<u32>, u64, Option<u32>) {
    (
        t.me.items.iter().filter_map(|i| i.slot).collect(),
        t.me.gold,
        t.me.pet.as_ref().and_then(|p| p.slot),
    )
}

/// 把某一行改成 `value`（`None` 清空）。物品行按位置替换或追加，清空后后面的物品前移。
fn set_row(items: &mut Vec<u32>, row: usize, value: Option<u32>) {
    match value {
        None => {
            if row < items.len() {
                items.remove(row);
            }
        }
        Some(s) => {
            if row < items.len() {
                items[row] = s;
            } else {
                items.push(s);
            }
        }
    }
}

fn offer_op(win: &TradeWindow, items: Vec<u32>, gold: u64, pet: Option<u32>) -> TradeOp {
    TradeOp::Offer {
        id: win.trade.id.clone(),
        items,
        gold,
        pet,
    }
}

fn cancel_op(win: &TradeWindow) -> TradeOp {
    TradeOp::Cancel {
        id: win.trade.id.clone(),
    }
}

/// Esc 或「取消交易」按钮：报价为空且没人锁定就直接取消，否则弹「取消这笔交易？」。
fn cancel_press(win: &mut TradeWindow, out: &mut Outcome) {
    if win.offer_is_empty() && win.nobody_locked() {
        out.ops.push(cancel_op(win));
    } else {
        win.ask_cancel = true;
        win.cancel_selected = false;
    }
}

/// 修改报价的子选择开着时的按键。`cands` 是这一行的候选（石币行为空）。
fn picker_key(w: &mut WorldState, picker: Picker, cands: &[Option<u32>], key: KeyEvent, out: &mut Outcome) {
    let gold_now = w.inventory.gold;
    let Some(win) = w.trade.as_mut() else {
        return;
    };
    let close = |win: &mut TradeWindow| win.picker = None;
    match key.code {
        KeyCode::Esc => close(win),
        KeyCode::Up if picker.row != 2 => {
            if let Some(p) = win.picker.as_mut() {
                p.cursor = p.cursor.saturating_sub(1);
            }
        }
        KeyCode::Down if picker.row != 2 => {
            if let Some(p) = win.picker.as_mut() {
                p.cursor = (p.cursor + 1).min(cands.len().saturating_sub(1));
            }
        }
        KeyCode::Enter if picker.row == 2 => {
            let gold = picker.gold.parse::<u64>().unwrap_or(0);
            if gold > gold_now {
                out.notice = Some("你身上的石币不够".into());
                return;
            }
            let (items, _, pet) = offer_of(&win.trade);
            out.ops.push(offer_op(win, items, gold, pet));
            close(win);
        }
        KeyCode::Enter => {
            let Some(value) = cands.get(picker.cursor).copied() else {
                return;
            };
            let (mut items, gold, pet) = offer_of(&win.trade);
            if picker.row == 3 {
                out.ops.push(offer_op(win, items, gold, value));
            } else {
                set_row(&mut items, picker.row, value);
                out.ops.push(offer_op(win, items, gold, pet));
            }
            close(win);
        }
        KeyCode::Backspace if picker.row == 2 => {
            if let Some(p) = win.picker.as_mut() {
                p.gold.pop();
            }
        }
        KeyCode::Char(c) if picker.row == 2 && c.is_ascii_digit() => {
            if let Some(p) = win.picker.as_mut() {
                if p.gold.len() < GOLD_DIGITS {
                    p.gold.push(c);
                }
            }
        }
        _ => {}
    }
}

/// 窗口开着时的全部按键。返回要发的请求；窗口不存在时什么也不做。
pub fn handle(w: &mut WorldState, key: KeyEvent) -> Outcome {
    let mut out = Outcome::default();
    let Some(picker) = w.trade.as_ref().and_then(|t| t.picker.clone()) else {
        handle_window(w, key, &mut out);
        return out;
    };
    let cands = match picker.row {
        3 => pet_candidates(w),
        2 => Vec::new(),
        r => item_candidates(w, r),
    };
    picker_key(w, picker, &cands, key, &mut out);
    out
}

fn handle_window(w: &mut WorldState, key: KeyEvent, out: &mut Outcome) {
    let Some(win) = w.trade.as_mut() else {
        return;
    };
    win.acknowledge();
    if win.ask_cancel {
        match key.code {
            KeyCode::Left | KeyCode::Right => win.cancel_selected = !win.cancel_selected,
            KeyCode::Esc => win.ask_cancel = false,
            KeyCode::Enter => {
                win.ask_cancel = false;
                if win.cancel_selected {
                    out.ops.push(cancel_op(win));
                }
            }
            _ => {}
        }
        return;
    }
    match key.code {
        KeyCode::Esc => cancel_press(win, out),
        KeyCode::Up => win.move_focus(-1),
        KeyCode::Down => win.move_focus(1),
        KeyCode::Left | KeyCode::Right => {
            // 底部按钮之间左右切换；报价行上不动。
            let bottom: Vec<Focus> = win
                .focus_order()
                .into_iter()
                .filter(|f| !matches!(f, Focus::Row(_)))
                .collect();
            if let Some(i) = bottom.iter().position(|f| *f == win.focus) {
                let next = if key.code == KeyCode::Left {
                    i.saturating_sub(1)
                } else {
                    i + 1
                };
                win.focus = bottom[next.min(bottom.len() - 1)];
            }
        }
        KeyCode::Enter => enter_key(win, out),
        _ => {}
    }
}

/// 当前焦点上按 Enter：报价行打开选择，按钮发请求。
fn enter_key(win: &mut TradeWindow, out: &mut Outcome) {
    if win.busy {
        return;
    }
    match win.focus {
        Focus::Row(r) => {
            win.picker = Some(Picker {
                row: r,
                cursor: 0,
                gold: String::new(),
            });
        }
        Focus::Lock => {
            if win.trade.me.locked {
                out.ops.push(TradeOp::Unlock {
                    id: win.trade.id.clone(),
                });
            } else if !win.trade.me.confirmed {
                out.ops.push(TradeOp::Lock {
                    id: win.trade.id.clone(),
                    rev: win.trade.rev,
                });
            }
        }
        Focus::Confirm => {
            if !win.trade.me.confirmed {
                out.ops.push(TradeOp::Confirm {
                    id: win.trade.id.clone(),
                    rev: win.trade.rev,
                });
            }
        }
        Focus::Cancel => cancel_press(win, out),
    }
}

impl App {
    /// 交易窗口开着时的按键入口：全部按键由窗口吃掉，返回 true。
    pub(super) fn trade_key(&mut self, key: KeyEvent) -> bool {
        let Some(w) = self.world.as_mut() else {
            return false;
        };
        if w.trade.is_none() {
            return false;
        }
        let out = handle(w, key);
        if let Some(n) = out.notice {
            self.log.error(n);
        }
        for op in out.ops {
            self.trade_request(op);
        }
        true
    }
}
