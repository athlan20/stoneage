use crate::app::{Layer, WorldState};
use ratatui::{
    Frame,
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::Line,
    widgets::{Block, Clear, List, ListItem, ListState, Paragraph, Wrap},
};

pub fn render(frame: &mut Frame, world: &WorldState, area: Rect) {
    let Some(w) = world.npc_window.as_ref() else {
        return;
    };
    let pop = super::widgets::centered(area, area.width.saturating_sub(2).min(86), area.height);
    frame.render_widget(Clear, pop);
    let block = Block::bordered()
        .title(format!("{} · {}", w.name, if w.trade.is_some(){"确认交易"}else if w.input.is_some(){"输入"}else{"对话"}))
        .border_style(Style::default().fg(Color::Yellow));
    let inner = block.inner(pop);
    frame.render_widget(block, pop);
    let body_h = if w.options.is_empty() {
        inner.height.saturating_sub(4)
    } else {
        options_body_height(&w.text, usize::from(inner.width), inner.height, w.options.len())
    };
    let [body, controls, hint] =
        Layout::vertical([Constraint::Length(body_h), Constraint::Min(2), Constraint::Length(1)]).areas(inner);
    frame.render_widget(
        Paragraph::new(w.text.clone())
            .wrap(Wrap { trim: false })
            .scroll((world.dialog.scroll.min(u16::MAX as usize) as u16, 0)),
        body,
    );
    let mut help = "↑↓ 选择 · Enter 确认 · PgUp/PgDn 翻页 · Esc 关闭";
    if let Some(t) = &w.trade {
        let selected = if world.dialog.confirm {
            "取消    [确认]"
        } else {
            "[取消]    确认"
        };
        frame.render_widget(
            Paragraph::new(vec![
                Line::raw(format!(
                    "{} {} ×{} = {} 金币",
                    if t.direction == "buy" {
                        "买入"
                    } else {
                        "卖出"
                    },
                    t.name,
                    t.count,
                    t.total
                )),
                Line::raw(selected),
            ]),
            controls,
        );
        help = "←→ 选择取消/确认 · Enter 执行 · Esc 取消";
    } else if let Some(input) = &w.input {
        let (value, cursor) = world
            .dialog
            .input
            .view(usize::from(controls.width.saturating_sub(2)));
        frame.render_widget(Paragraph::new(format!("> {value}")), controls);
        if !world.nav.is(&Layer::Command) {
            frame.set_cursor_position((controls.x + 2 + cursor, controls.y));
        }
        help = if input.kind == "integer" {
            "输入数量 · Enter 取得报价 · Esc 关闭"
        } else {
            "输入原文 · Enter 发送 · Esc 关闭"
        };
    } else {
        let rows = w
            .options
            .iter()
            .map(|o| ListItem::new(format!("{}. {}", o.no, o.label)))
            .collect::<Vec<_>>();
        let mut state = ListState::default().with_selected(if rows.is_empty() {
            None
        } else {
            Some(world.dialog.index.min(rows.len() - 1))
        });
        frame.render_stateful_widget(
            List::new(rows)
                .highlight_style(
                    Style::default()
                        .fg(Color::Yellow)
                        .add_modifier(Modifier::REVERSED),
                )
                .highlight_symbol("> "),
            controls,
            &mut state,
        );
        if w.options.is_empty() {
            frame.render_widget(Paragraph::new("[关闭]"), controls);
        }
    }
    frame.render_widget(
        Paragraph::new(help).style(Style::default().fg(Color::DarkGray)),
        hint,
    );
}

/// 选项列表最多占的行数（STAGE-347，长列表自己滚动）。
const MAX_OPTION_ROWS: u16 = 12;

/// 有选项时正文的高度（STAGE-281）：按实际折行后的行数取，至少 3 行、至多留出
/// 选项行和底部提示行；选项很多放不下时正文让位，选项列表自己滚动。
fn options_body_height(text: &str, width: usize, inner_h: u16, options: usize) -> u16 {
    let lines = text
        .split('\n')
        .map(|raw| crate::app::msgview::wrap_text(raw, width).len())
        .sum::<usize>();
    let lines = u16::try_from(lines).unwrap_or(u16::MAX);
    // STAGE-347：道具仓库 50 格这类长列表只按可见的 12 行让位，剩下的自己滚动，正文不至于被挤没。
    let opt_rows = u16::try_from(options).unwrap_or(u16::MAX).min(MAX_OPTION_ROWS);
    let max = inner_h.saturating_sub(opt_rows.saturating_add(1));
    lines.max(3).min(max)
}
