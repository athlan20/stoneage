use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem};
use unicode_width::UnicodeWidthStr;

use crate::app::{App, Kind, LABEL_WIDTH, TextInput, wrap_text};

/// 在 `area` 里居中取一块 `w`×`h` 的区域（超出时收缩）。
pub fn centered(area: Rect, w: u16, h: u16) -> Rect {
    let w = w.min(area.width);
    let h = h.min(area.height);
    Rect::new(area.x + (area.width - w) / 2, area.y + (area.height - h) / 2, w, h)
}

/// 标签补空格到固定的列宽（中文占 2 列）。
pub fn pad(label: &str, width: usize) -> String {
    let w = label.width();
    format!("{label}{}", " ".repeat(width.saturating_sub(w)))
}

pub fn focus_style(focused: bool) -> Style {
    if focused { Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD) } else { Style::default() }
}

/// 列表里选中的一行（STAGE-113，`docs/interaction.md`「页面选择」）：焦点栏的选中行黄字加粗反色，
/// 其他栏的选中行只加粗，玩家靠反色认出 `↑↓` 现在动哪一栏。战斗列表面板和各页列表共用。
pub fn selected_style(focus_column: bool) -> Style {
    if focus_column {
        Style::default()
            .fg(Color::Yellow)
            .add_modifier(Modifier::BOLD | Modifier::REVERSED)
    } else {
        Style::default().add_modifier(Modifier::BOLD)
    }
}

/// 选中行的行首标记；没选中是一个空格，保证各行文字对齐。
pub fn select_mark(selected: bool) -> &'static str {
    if selected { "▶" } else { " " }
}

/// 一行按钮：焦点所在的按钮黄色加粗，不可用的灰色。
pub fn button_line(labels: &[&str], selected: usize, focused: bool, disabled: &[bool]) -> Line<'static> {
    let mut spans = vec![];
    for (i, label) in labels.iter().enumerate() {
        let style = if disabled.get(i).copied().unwrap_or(false) {
            Style::default().fg(Color::DarkGray)
        } else if focused && i == selected {
            Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD | Modifier::REVERSED)
        } else {
            Style::default().fg(Color::Green)
        };
        spans.push(Span::styled(format!("[ {label} ]"), style));
        spans.push(Span::raw("   "));
    }
    Line::from(spans).centered()
}

/// 输入框内容的显示字符串；密码框用圆点（关闭 unicode 时用 `*`）。
pub fn mask_char(unicode: bool) -> char {
    if unicode { '•' } else { '*' }
}

pub fn input_text(input: &TextInput, secret: bool, unicode: bool) -> String {
    input.display(secret.then(|| mask_char(unicode)))
}

/// 最近 `n` 条消息，每行是定宽的类型标签加正文，配色见 `docs/ui.md`。
/// 一行一条，过长的正文截断：世界页底部消息栏用它（`docs/ui.md`「消息」）。
pub fn message_items(app: &App, n: usize) -> Vec<ListItem<'static>> {
    app.log
        .recent(n)
        .map(|m| {
            ListItem::new(Line::from(vec![
                Span::styled(m.kind.label(), Style::default().fg(m.kind.color())),
                Span::raw(m.text.clone()),
            ]))
        })
        .collect()
}

/// 只含聊天类（公开聊天、agent 私聊）的最近 `n` 条消息；战斗中玩家正在聊天时，日志区用它。
pub fn chat_items(app: &App, n: usize) -> Vec<ListItem<'static>> {
    let chats: Vec<_> = app.log.all().iter().filter(|m| matches!(m.kind, Kind::Chat(_) | Kind::Agent)).collect();
    chats
        .into_iter()
        .rev()
        .take(n)
        .rev()
        .map(|m| {
            ListItem::new(Line::from(vec![
                Span::styled(m.kind.label(), Style::default().fg(m.kind.color())),
                Span::raw(m.text.clone()),
            ]))
        })
        .collect()
}

/// 一条消息的一行：第一行是彩色标签加正文，续行缩进到标签后（和消息页一致）。
pub fn message_line(kind: Kind, chunk: &str, continuation: bool) -> Line<'static> {
    if continuation {
        Line::from(vec![Span::raw(" ".repeat(LABEL_WIDTH)), Span::raw(chunk.to_string())])
    } else {
        Line::from(vec![
            Span::styled(kind.label(), Style::default().fg(kind.color())),
            Span::raw(chunk.to_string()),
        ])
    }
}

/// 最近 `n` 行消息：正文超过 `width` 时换行，续行缩进到标签后（STAGE-100）。
/// 登录、注册、角色选择这些表单页的消息面板用它，服务端的长提示（升级命令等）才看得全；
/// 世界页底部消息栏仍是一行一条的 `message_items`。
pub fn wrapped_message_items(app: &App, n: usize, width: usize) -> Vec<ListItem<'static>> {
    let mut lines: Vec<(Kind, &str, bool)> = Vec::new();
    for m in app.log.all().iter().rev() {
        for (i, chunk) in wrap_text(&m.text, width.max(1)).into_iter().enumerate().rev() {
            if lines.len() == n {
                break;
            }
            lines.push((m.kind, chunk, i > 0));
        }
        if lines.len() == n {
            break;
        }
    }
    lines.reverse();
    lines.into_iter().map(|(kind, chunk, continuation)| ListItem::new(message_line(kind, chunk, continuation))).collect()
}

pub fn render_messages(frame: &mut Frame, app: &App, area: Rect) {
    let rows = usize::from(area.height.saturating_sub(2));
    let width = usize::from(area.width.saturating_sub(2)).saturating_sub(LABEL_WIDTH);
    let list = List::new(wrapped_message_items(app, rows, width)).block(Block::bordered().title("消息"));
    frame.render_widget(list, area);
}

/// 上下两块：表单区在上，消息区在下，整体居中。
pub fn form_and_messages(area: Rect, form_h: u16, msg_h: u16, width: u16) -> [Rect; 2] {
    let col = centered(area, width, form_h + msg_h);
    let [a, b] = Layout::vertical([Constraint::Length(form_h), Constraint::Length(msg_h)]).areas(col);
    [a, b]
}
