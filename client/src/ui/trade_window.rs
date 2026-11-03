//! STAGE-295：交易窗口（`docs/trade.md` 5.2）：居中弹出（`Clear` + 边框），左右两块报价面板，底部状态、提示行和按钮。
//! 修改报价的物品、宠物列表和石币输入、「取消这笔交易？」确认都画在窗口里面。按键在 `app/trade_keys.rs`。

use ratatui::layout::{Alignment, Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Clear, List, ListItem, ListState, Paragraph};
use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};
use ratatui::Frame;

use super::widgets::{centered, pad, selected_style};
use crate::app::trade_text::last_error_text;
use crate::app::trade_keys::{candidate_label, item_candidates, pet_candidates};
use crate::app::trade_state::{Focus, Picker, TradeWindow, OFFER_ROWS};
use crate::app::{App, WorldState};
use crate::proto::TradeSide;

const WIDTH: u16 = 60;
const HEIGHT: u16 = 13;

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let Some(win) = world.trade.as_ref() else {
        return;
    };
    let rect = centered(area, WIDTH.min(area.width), HEIGHT.min(area.height));
    frame.render_widget(Clear, rect);
    let peer = win.trade.peer.name.clone();
    let secs = win.idle_left(std::time::Instant::now());
    let block = Block::bordered()
        .title(Line::from(Span::styled(
            format!(" 交易 · {peer} "),
            Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD),
        )))
        .title(Line::from(format!(" 剩余 {}:{:02} ", secs / 60, secs % 60)).alignment(Alignment::Right));
    let inner = block.inner(rect);
    frame.render_widget(block, rect);
    let [panels, _gap, status, notice, buttons, hint] = Layout::vertical([
        Constraint::Length(6),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
    ])
    .areas(inner);
    let [mine, theirs] = Layout::horizontal([Constraint::Percentage(50), Constraint::Percentage(50)]).areas(panels);
    let unicode = app.unicode;
    let focus_row = match win.focus {
        Focus::Row(r) => Some(r),
        _ => None,
    };
    render_side(
        frame,
        unicode,
        mine,
        "我的报价",
        Color::Cyan,
        &win.trade.me,
        &[],
        focus_row,
    );
    render_side(
        frame,
        unicode,
        theirs,
        "对方的报价",
        Color::Magenta,
        &win.trade.peer,
        &win.changed_rows,
        None,
    );
    frame.render_widget(
        Paragraph::new(status_line(win, unicode)).alignment(Alignment::Center),
        status,
    );
    frame.render_widget(
        Paragraph::new(notice_line(win, &peer)).alignment(Alignment::Center),
        notice,
    );
    frame.render_widget(Paragraph::new(button_row(win)).alignment(Alignment::Center), buttons);
    let arrows = if unicode {
        "↑↓ 选择 · Enter 修改/确认 · Esc 取消交易"
    } else {
        "Up/Down 选择 · Enter 修改/确认 · Esc 取消交易"
    };
    frame.render_widget(
        Paragraph::new(arrows)
            .style(Style::new().fg(Color::DarkGray))
            .alignment(Alignment::Center),
        hint,
    );
    if let Some(p) = win.picker.as_ref() {
        render_picker(frame, world, p, inner);
    } else if win.ask_cancel {
        render_ask_cancel(frame, win, inner);
    }
}

/// 一块报价面板：四行（物品 1、物品 2、石币、宠物）。`changed` 是对方改过的行（`●`），`focus` 是自己的焦点行（`▶`）。
#[allow(clippy::too_many_arguments)]
fn render_side(
    frame: &mut Frame,
    unicode: bool,
    area: Rect,
    title: &str,
    color: Color,
    side: &TradeSide,
    changed: &[usize],
    focus: Option<usize>,
) {
    let block = Block::bordered().title(Line::from(Span::styled(
        title.to_string(),
        Style::new().fg(color).add_modifier(Modifier::BOLD),
    )));
    let inner = block.inner(area);
    frame.render_widget(block, area);
    // 物品名和宠物名按面板宽度截断，数量和等级永远保留（左边框、标记、标签各占一段）。
    let room = (inner.width as usize).saturating_sub(8);
    let lines: Vec<Line> = (0..OFFER_ROWS)
        .map(|r| {
            let mark = if changed.contains(&r) {
                if unicode {
                    "●"
                } else {
                    "*"
                }
            } else if focus == Some(r) {
                if unicode {
                    "▶"
                } else {
                    ">"
                }
            } else {
                " "
            };
            let (label, value) = row_text(side, r, room, unicode);
            let text = format!("{mark}{}{value}", pad(&label, 7));
            let style = if focus == Some(r) {
                selected_style(true)
            } else if changed.contains(&r) {
                Style::new().fg(Color::Yellow)
            } else {
                Style::default()
            };
            Line::from(Span::styled(text, style))
        })
        .collect();
    frame.render_widget(Paragraph::new(lines), inner);
}

fn row_text(side: &TradeSide, r: usize, room: usize, unicode: bool) -> (String, String) {
    match r {
        0 | 1 => {
            let value = match side.items.get(r) {
                Some(i) => fit_name(&i.name, &format!(" x{}", i.count), room, unicode),
                None => "（空）".to_string(),
            };
            (format!("物品 {}", r + 1), value)
        }
        2 => ("石币".into(), group_thousands(side.gold)),
        _ => {
            let value = match side.pet.as_ref() {
                Some(p) => fit_name(&p.name, &format!(" Lv{}", p.level), room, unicode),
                None => "（无）".to_string(),
            };
            ("宠物".into(), value)
        }
    }
}

/// 名字放不下时截成 `名字…` 并保留后缀（数量、等级）；非 Unicode 模式用 `..`。
pub fn fit_name(name: &str, suffix: &str, room: usize, unicode: bool) -> String {
    let budget = room.saturating_sub(UnicodeWidthStr::width(suffix));
    if UnicodeWidthStr::width(name) <= budget {
        return format!("{name}{suffix}");
    }
    let mark = if unicode { "…" } else { ".." };
    let keep = budget.saturating_sub(UnicodeWidthStr::width(mark));
    let mut out = String::new();
    let mut w = 0;
    for c in name.chars() {
        let cw = UnicodeWidthChar::width(c).unwrap_or(0);
        if w + cw > keep {
            break;
        }
        out.push(c);
        w += cw;
    }
    format!("{out}{mark}{suffix}")
}

/// 石币按千位加逗号：5000 写成 5,000。
pub fn group_thousands(n: u64) -> String {
    let s = n.to_string();
    let mut out = String::new();
    for (i, c) in s.chars().enumerate() {
        if i > 0 && (s.len() - i) % 3 == 0 {
            out.push(',');
        }
        out.push(c);
    }
    out
}

fn state_word(side: &TradeSide) -> (&'static str, Style) {
    if side.confirmed {
        ("已确定", Style::new().fg(Color::Green))
    } else if side.locked {
        ("已锁定", Style::new().fg(Color::Yellow))
    } else {
        ("编辑中", Style::new().fg(Color::White))
    }
}

fn status_line(win: &TradeWindow, unicode: bool) -> Line<'static> {
    let (me, me_style) = state_word(&win.trade.me);
    let (peer, peer_style) = state_word(&win.trade.peer);
    let tick = match (win.trade.peer.locked, unicode) {
        (true, true) => " ✓",
        (true, false) => " v",
        _ => "",
    };
    Line::from(vec![
        Span::raw("你："),
        Span::styled(me.to_string(), me_style),
        Span::raw("        对方："),
        Span::styled(format!("{peer}{tick}"), peer_style),
    ])
}

fn notice_line(win: &TradeWindow, peer: &str) -> Line<'static> {
    if win.peer_changed {
        return Line::from(Span::styled(
            "对方改动了报价，双方锁定已取消",
            Style::new().fg(Color::Yellow),
        ));
    }
    match win.trade.last_error.as_ref() {
        Some(e) => Line::from(Span::styled(last_error_text(e, peer), Style::new().fg(Color::Red))),
        None => Line::from(""),
    }
}

/// 底部按钮：锁定（或取消锁定）、确定（双方都锁定后出现）、取消交易。焦点所在的按钮加粗反色。
fn button_row(win: &TradeWindow) -> Line<'static> {
    let me = &win.trade.me;
    let mut spans = Vec::new();
    let mut push = |label: &str, focused: bool, style: Style| {
        if !spans.is_empty() {
            spans.push(Span::raw("   "));
        }
        let s = if focused { selected_style(true) } else { style };
        spans.push(Span::styled(format!("[{label}]"), s));
    };
    let wait = Style::new().fg(Color::DarkGray);
    for f in win.focus_order() {
        match f {
            Focus::Row(_) => {}
            Focus::Lock => {
                let label = if me.confirmed {
                    "等待对方确定…"
                } else if me.locked {
                    "取消锁定"
                } else {
                    "锁定"
                };
                let style = if me.confirmed { wait } else { Style::default() };
                push(label, win.focus == f, style);
            }
            Focus::Confirm => {
                let (label, style) = if me.confirmed {
                    ("等待对方确定…", wait)
                } else {
                    ("确定", Style::new().fg(Color::Green).add_modifier(Modifier::BOLD))
                };
                push(label, win.focus == f, style);
            }
            Focus::Cancel => push("取消交易", win.focus == f, Style::default()),
        }
    }
    Line::from(spans)
}

fn render_picker(frame: &mut Frame, world: &WorldState, p: &Picker, area: Rect) {
    let title = match p.row {
        0 | 1 => format!("选择物品（物品 {}）", p.row + 1),
        2 => "石币".to_string(),
        _ => "选择宠物".to_string(),
    };
    let w = 34.min(area.width);
    if p.row == 2 {
        let rect = centered(area, w, 5.min(area.height));
        frame.render_widget(Clear, rect);
        let have = world.inventory.gold;
        let block = Block::bordered().title(title);
        let inner = block.inner(rect);
        frame.render_widget(block, rect);
        let lines = vec![
            Line::from(format!("要放入 {}_", p.gold)),
            Line::from(format!("身上有 {}", group_thousands(have))).style(Style::new().fg(Color::DarkGray)),
            Line::from("数字输入，Enter 确定，Esc 取消").style(Style::new().fg(Color::DarkGray)),
        ];
        frame.render_widget(Paragraph::new(lines), inner);
        return;
    }
    let cands = if p.row == 3 {
        pet_candidates(world)
    } else {
        item_candidates(world, p.row)
    };
    let h = (cands.len() as u16 + 2).clamp(3, area.height.max(3));
    let rect = centered(area, w, h);
    frame.render_widget(Clear, rect);
    let items: Vec<ListItem> = cands
        .iter()
        .enumerate()
        .map(|(i, c)| {
            let text = format!(
                "{} {}",
                if i == p.cursor { "▶" } else { " " },
                candidate_label(world, p.row, *c)
            );
            let style = if i == p.cursor {
                selected_style(true)
            } else {
                Style::default()
            };
            ListItem::new(Span::styled(text, style))
        })
        .collect();
    // STAGE-346：宠物栏 20 格，候选比选择框高时跟着光标滚动。
    let mut state = ListState::default().with_selected(Some(p.cursor));
    frame.render_stateful_widget(
        List::new(items).block(Block::bordered().title(title)),
        rect,
        &mut state,
    );
}

fn render_ask_cancel(frame: &mut Frame, win: &TradeWindow, area: Rect) {
    let rect = centered(area, 30.min(area.width), 5.min(area.height));
    frame.render_widget(Clear, rect);
    let block = Block::bordered().title("取消这笔交易？");
    let inner = block.inner(rect);
    frame.render_widget(block, rect);
    let cont = if win.cancel_selected {
        Style::default()
    } else {
        selected_style(true)
    };
    let stop = if win.cancel_selected {
        selected_style(true)
    } else {
        Style::default()
    };
    let line = Line::from(vec![
        Span::styled("[继续交易]", cont),
        Span::raw("   "),
        Span::styled("[取消交易]", stop),
    ]);
    frame.render_widget(Paragraph::new(line).alignment(Alignment::Center), inner);
}
