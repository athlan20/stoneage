//! 强者表的排行窗口（STAGE-308，`docs/protocol.md` 5.75，`docs/pk.md` 5.3）：和普通对话同样是居中带边框的窗口，
//! 正文是一张带黄色表头的 `Table`（名次、决斗点数、等级、名字），请求者自己的那行名字前加 ★；
//! 表下面是页码和自己的名次，再下面是选项与提示。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Cell, Clear, Paragraph, Row, Table};

use super::widgets::centered;
use crate::proto::{NpcRanking, NpcWindow};

/// 窗口宽度（放不下时贴着区域宽度收窄）。
const WINDOW_WIDTH: u16 = 60;
/// 表头样式，同 `wikipage` 的黄色表头。
const HEADER_STYLE: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);

/// `selected` 是菜单式窗口（带 window_id）当前高亮的选项下标；旧式数字选择的窗口传 `None`。
pub fn render(frame: &mut Frame, area: Rect, w: &NpcWindow, rk: &NpcRanking, selected: Option<usize>) {
    let has_options = !w.options.is_empty();
    let rows: Vec<Row> = rk
        .top
        .iter()
        .map(|r| {
            let name = if r.is_self { format!("★{}", r.name) } else { format!(" {}", r.name) };
            Row::new(vec![
                Cell::from(format!("{:>4}", r.rank)),
                Cell::from(format!("{:>8}", r.duel_point)),
                Cell::from(format!("{:>4}", r.level)),
                Cell::from(name),
            ])
        })
        .collect();
    let table_rows = rows.len().max(1) as u16;
    // 内容高度：页码行、空行、表头 + 表体、空行、（选项 + 空行）、提示行；加上下边框。
    let option_rows = if has_options { w.options.len() as u16 + 1 } else { 0 };
    let height = 1 + 1 + 1 + table_rows + 1 + option_rows + 1 + 2;
    let pop = centered(area, WINDOW_WIDTH.min(area.width.saturating_sub(2)), height.min(area.height));
    frame.render_widget(Clear, pop);

    let title = if w.name.is_empty() { w.type_.clone() } else { w.name.clone() };
    let block = Block::bordered()
        .title(Span::styled(format!(" {title} "), Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)));
    let inner = block.inner(pop);
    frame.render_widget(block, pop);

    let [caption, _, table_area, _, options_area, hint] = Layout::vertical([
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(table_rows + 1),
        Constraint::Length(1),
        Constraint::Length(option_rows),
        Constraint::Length(1),
    ])
    .areas(inner);

    let page_line =
        format!(" 第 {}/{} 页 · 我的名次 {}，决斗点数 {}", rk.page, rk.pages.max(1), rk.me.rank, rk.me.duel_point);
    frame.render_widget(Paragraph::new(Line::raw(page_line)), caption);

    let header = Row::new(vec!["名次", "决斗点数", "等级", "名字"]).style(HEADER_STYLE);
    let widths = [Constraint::Length(6), Constraint::Length(10), Constraint::Length(6), Constraint::Min(8)];
    frame.render_widget(Table::new(rows, widths).header(header).column_spacing(1), table_area);

    if has_options {
        let mut lines: Vec<Line> = vec![];
        for (i, o) in w.options.iter().enumerate() {
            if selected == Some(i) {
                lines.push(Line::styled(
                    format!("> {}. {}", o.no, o.label),
                    Style::default().fg(Color::Yellow).add_modifier(Modifier::REVERSED),
                ));
            } else {
                lines.push(Line::from(vec![
                    Span::styled(format!("  {}. ", o.no), Style::default().fg(Color::Yellow)),
                    Span::raw(o.label.clone()),
                ]));
            }
        }
        frame.render_widget(Paragraph::new(lines), options_area);
    }
    let hint_text = match (has_options, selected.is_some()) {
        (true, true) => "↑↓ 选择 · Enter 确认 · Esc 关闭",
        (true, false) => "数字选择 · Esc 关闭",
        (false, _) => "按 Esc 关闭",
    };
    frame.render_widget(
        Paragraph::new(Line::styled(format!(" {hint_text}"), Style::default().fg(Color::DarkGray))),
        hint,
    );
}
