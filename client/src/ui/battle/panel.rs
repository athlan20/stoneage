//! 列表面板（STAGE-107，`docs/battle-ui.md`「列表面板」）：道具、精灵、换宠、宠物技能的完整列表，每行一个号、
//! 名字、附加信息和目标。宽屏时整栏换成列表（`render`），窄屏时从战场右侧浮出（`render_float`）。
//! 高亮的行在说明栏里写效果；一页放不下时按页显示，PgUp、PgDn 翻页。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::Line;
use ratatui::widgets::{Block, Borders, Clear, Paragraph};
use unicode_width::UnicodeWidthChar;

use crate::app::App;
use crate::app::battle::{BattleState, Pick, list, list_focus};
use crate::ui::widgets::{select_mark, selected_style};

/// 窄屏浮层的宽度。
const FLOAT_WIDTH: u16 = 40;
/// 名字和附加信息占的显示宽度（超出的截掉）。
const NAME_WIDTH: usize = 12;
const EXTRA_WIDTH: usize = 10;

/// 窄屏：盖在战场右边（`area` 是战场），选完或按 Esc 收起由调用方决定。
pub fn render_float(frame: &mut Frame, app: &App, b: &BattleState, area: Rect) {
    let width = FLOAT_WIDTH.min(area.width);
    let rect = Rect {
        x: area.right() - width,
        width,
        ..area
    };
    frame.render_widget(Clear, rect);
    draw(frame, app, b, rect);
}

fn draw(frame: &mut Frame, app: &App, b: &BattleState, area: Rect) {
    let Some(w) = app.world.as_ref() else {
        return;
    };
    let rows = list::rows(w, b.pick);
    let focus = list_focus::clamp(&rows, b.list_focus);
    let inner_h = area.height.saturating_sub(2);
    let desc_h = desc_height(inner_h);
    let visible = usize::from(inner_h.saturating_sub(desc_h)).max(1);
    let page = focus / visible;
    let pages = rows.len().div_ceil(visible).max(1);
    let mut block = Block::bordered().title(Line::styled(
        format!(" {} · ↑↓ · Enter · Esc ", title(b.pick)),
        Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
    ));
    if pages > 1 {
        block = block.title_bottom(format!(" 第 {}/{} 页 · PgUp PgDn ", page + 1, pages));
    }
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let [list_area, desc_area] = Layout::vertical([Constraint::Min(0), Constraint::Length(desc_h)]).areas(inner);
    if rows.is_empty() {
        frame.render_widget(Paragraph::new(Line::styled(" 这里没有可以选的", dim())), list_area);
        return;
    }
    let lines: Vec<Line> = rows
        .iter()
        .enumerate()
        .skip(page * visible)
        .take(visible)
        .map(|(i, row)| row_line(row, i == focus))
        .collect();
    frame.render_widget(Paragraph::new(lines), list_area);
    if desc_h > 0 {
        let [rule, text] = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).areas(desc_area);
        frame.render_widget(Block::new().borders(Borders::TOP).border_style(dim()), rule);
        let lines: Vec<Line> = rows[focus]
            .desc
            .lines()
            .take(usize::from(desc_h) - 1)
            .map(|l| Line::raw(format!(" {l}")))
            .collect();
        frame.render_widget(Paragraph::new(lines), text);
    }
}

/// 说明栏的高度（含顶部分隔线）：区域高时两行，矮时一行，太矮时不画。
fn desc_height(inner: u16) -> u16 {
    if inner >= 9 {
        4
    } else if inner >= 5 {
        2
    } else {
        0
    }
}

fn title(pick: Pick) -> &'static str {
    match pick {
        Pick::Item => "道具",
        Pick::Magic => "精灵",
        Pick::Pet => "换宠",
        Pick::Skill => "宠物技能",
        Pick::Action | Pick::Target { .. } | Pick::Clones | Pick::CloneAim { .. } => "",
    }
}

/// 一行：`▶ 7 回复药      ×3        己方一个`。高亮的行反色黄字；不能选的灰色。
fn row_line(row: &list::Row, focused: bool) -> Line<'static> {
    let mark = select_mark(focused);
    let number = row.number.map_or_else(|| "-".to_string(), |n| n.to_string());
    let text = format!(
        "{mark}{number:>2} {}{}{}",
        fit(&row.name, NAME_WIDTH),
        fit(&row.extra, EXTRA_WIDTH),
        row.target
    );
    let style = if focused {
        selected_style(true)
    } else if row.blocked.is_some() {
        dim()
    } else {
        Style::default()
    };
    Line::styled(text, style)
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

fn dim() -> Style {
    Style::default().fg(Color::DarkGray)
}

#[cfg(test)]
mod tests {
    use super::fit;

    #[test]
    fn fit_counts_wide_characters_as_two_columns() {
        assert_eq!(fit("回复药", 8), "回复药  ");
        assert_eq!(fit("回复药水很长", 7), "回复药 ", "超出的截掉，不拆开汉字");
    }
}
