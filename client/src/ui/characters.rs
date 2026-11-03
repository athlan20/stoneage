//! 角色选择页：栏位表格 + 删除确认弹窗。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Cell, Clear, Paragraph, Row, Table, TableState};

use super::widgets::{centered, render_messages};
use crate::app::App;

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    if app.reconnect.is_some(){frame.render_widget(Paragraph::new("正在恢复连接与刷新资料 · Esc 返回只读页面"),Rect::new(area.x,area.y,area.width,1));}
    let col = centered(area, 76, 8 + 1 + 6);
    let [table_area, help, msgs] =
        Layout::vertical([Constraint::Length(8), Constraint::Length(1), Constraint::Length(6)]).areas(col);

    let header = Row::new(["栏位", "名字", "Lv", "所在地"]).style(Style::default().fg(Color::Yellow));
    let rows: Vec<Row> = (0..app.chars.slots())
        .map(|slot| match app.chars.character_at(slot) {
            Some(c) => Row::new(vec![
                Cell::from(format!("{}", slot + 1)),
                Cell::from(c.name.clone()),
                Cell::from(c.level.to_string()),
                Cell::from(c.location.map_name.clone()),
            ]),
            None => Row::new(vec![
                Cell::from(format!("{}", slot + 1)),
                Cell::from(Span::styled("(空)", Style::default().fg(Color::DarkGray))),
                Cell::from(""),
                Cell::from(""),
            ]),
        })
        .collect();
    // 名字最长 16 个字符，汉字占两格，所以名字列要留 32 格以上（STAGE-9 联调时发现原来只有 20 格，长名字被截断）。
    let widths = [Constraint::Length(6), Constraint::Length(34), Constraint::Length(5), Constraint::Min(12)];
    let title = Span::styled("选择角色", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD));
    let table = Table::new(rows, widths)
        .header(header)
        .block(Block::bordered().title(title))
        .row_highlight_style(Style::default().add_modifier(Modifier::BOLD))
        .highlight_symbol("> ");
    let mut state = TableState::default().with_selected(Some(app.chars.selected));
    frame.render_stateful_widget(table, table_area, &mut state);
    frame.render_widget(Paragraph::new("  ↑↓ 选栏位   Enter 动作/创建   d 删除（快捷）   Esc 返回登录"), help);
    render_messages(frame, app, msgs);

    if let Some(index)=app.chars.actions {
        let pop=centered(area,48,7);
        frame.render_widget(Clear,pop);
        let lines=["进入游戏","删除角色","返回"].iter().enumerate().map(|(i,s)|Line::styled(format!(" {} {s}",if index==i{">"}else{" "}),if index==i{Style::default().fg(Color::Yellow)}else{Style::default()})).collect::<Vec<_>>();
        frame.render_widget(Paragraph::new(lines).block(Block::bordered().title("角色动作 · ↑↓ 选择 · Enter 确认")),pop);
    }

    if let Some(c) = &app.chars.confirm {
        let pop = centered(area, 60, 9);
        frame.render_widget(Clear, pop);
        let block = Block::bordered().title(Span::styled("删除角色", Style::default().fg(Color::Red).add_modifier(Modifier::BOLD)));
        let inner = block.inner(pop);
        let lines = vec![
            Line::raw(" 请再输入一次完整角色名："),
            Line::raw(format!(" {}", c.name)),
            Line::raw(format!(" > {}", c.input.text())),
            Line::raw(""),
            Line::raw(if c.delete {" 取消    [删除]"}else{" [取消]    删除"}),
            Line::raw(" Tab 输入/按钮 · ←→ 选择 · Esc 取消"),
        ];
        frame.render_widget(Paragraph::new(lines).block(block), pop);
        if !c.buttons { frame.set_cursor_position((inner.x + 3 + c.input.cursor_col(None), inner.y + 2)); }
    }
}
