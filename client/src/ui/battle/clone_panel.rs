//! 分身面板（STAGE-186，`docs/battle-ui.md`「分身队」）：列出这回合能指定攻击目标的分身和分身的宠物，
//! 每行一个号（1–4 分身，5–8 分身的宠物）、名字和现在的目标；没指定的写服务端默认会打的敌人
//! （STAGE-312 起是合击顺序的第一个，“默认→ 敌N”）。从战场右侧浮出，选完或按 Esc 收起。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::Line;
use ratatui::widgets::{Block, Clear, Paragraph};
use unicode_width::UnicodeWidthChar;

use crate::app::battle::{BattleState, slots};
use crate::ui::widgets::{select_mark, selected_style};

const WIDTH: u16 = 40;
const NAME_WIDTH: usize = 16;

pub fn render_float(frame: &mut Frame, b: &BattleState, area: Rect) {
    let width = WIDTH.min(area.width);
    let rect = Rect {
        x: area.right() - width,
        width,
        ..area
    };
    frame.render_widget(Clear, rect);
    let block = Block::bordered().title(Line::styled(
        " 分身 · ↑↓ · Enter · Esc ",
        Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
    ));
    let inner = block.inner(rect);
    frame.render_widget(block, rect);
    let rows = b.clone_rows();
    let focus = b.clone_focus.min(rows.len().saturating_sub(1));
    let visible = usize::from(inner.height).max(1);
    let skip = (focus + 1).saturating_sub(visible);
    let lines: Vec<Line> = rows
        .iter()
        .enumerate()
        .skip(skip)
        .take(visible)
        .map(|(i, row)| {
            let target = match (row.target, row.default_target) {
                (Some(t), _) => format!("→ {}", slots::label(b.me_side(), t)),
                (None, Some(t)) => format!("默认→ {}", slots::label(b.me_side(), t)),
                (None, None) => "—".to_string(),
            };
            let text = format!(
                "{}{:>2} {}{}",
                select_mark(i == focus),
                row.key,
                fit(&row.name, NAME_WIDTH),
                target
            );
            let style = if i == focus {
                selected_style(true)
            } else if row.target.is_none() {
                Style::default().fg(Color::DarkGray)
            } else {
                Style::default()
            };
            Line::styled(text, style)
        })
        .collect();
    frame.render_widget(Paragraph::new(lines), inner);
}

/// 按显示宽度截断或补空格到 `width` 列（汉字占两列）。
fn fit(text: &str, width: usize) -> String {
    let mut out = String::new();
    let mut used = 0;
    for c in text.chars() {
        let w = c.width().unwrap_or(0);
        if used + w > width {
            break;
        }
        out.push(c);
        used += w;
    }
    out.push_str(&" ".repeat(width - used));
    out
}
