//! STAGE-278：聊天输入框的绘制（`docs/chat.md` 第三节）。盖在界面底部、消息栏之上：
//! 频道行（Tab 切换）、输入行（`> 文字_`）、计数与提示行（Enter 发送 · Esc 收起，或错误原因）。
use crate::app::{App, CHAT_MAX, Layer};
use crate::proto::ChatChannel;
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Clear, Paragraph},
};

/// 面板高 5 行：上下边框、频道行、输入行、计数提示行。宽度不超过 56 列。
const HEIGHT: u16 = 5;
const MAX_WIDTH: u16 = 56;

pub fn draw(frame: &mut Frame, app: &App, area: Rect) {
    let Some(w) = app.world.as_ref() else {
        return;
    };
    if !w.nav.is(&Layer::Chat) || area.height < HEIGHT {
        return;
    }
    let width = area.width.min(MAX_WIDTH);
    let rect = Rect::new(area.x + (area.width - width) / 2, area.bottom() - HEIGHT, width, HEIGHT);
    frame.render_widget(Clear, rect);
    let title = Span::styled(" 聊天 ", Style::new().fg(Color::Cyan).add_modifier(Modifier::BOLD));
    let block = Block::bordered().title(title);
    let inner = block.inner(rect);
    frame.render_widget(block, rect);

    let c = &w.chat;
    let party_ok = crate::app::party_rules::party_chat_ok(w);
    let dim = Style::new().fg(Color::DarkGray);
    let channel = |ch: ChatChannel| {
        if c.channel == ch {
            Span::styled(
                format!("[{}]", ch.name()),
                Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD),
            )
        } else {
            Span::styled(format!(" {} ", ch.name()), dim)
        }
    };
    let mut spans = vec![
        Span::raw("频道: "),
        channel(ChatChannel::Map),
        Span::raw(" "),
        channel(ChatChannel::World),
    ];
    // 不在真人队里时队伍频道不出现（Tab 也会跳过它）；正选着它时保留显示。
    if party_ok || c.channel == ChatChannel::Party {
        spans.push(Span::raw(" "));
        spans.push(channel(ChatChannel::Party));
    }
    spans.push(Span::styled("  Tab 切换", dim));
    let channel_line = Line::from(spans);

    let text = c.input.text();
    let count = text.chars().count();
    let input_line = Line::from(vec![Span::raw("> "), Span::raw(text.to_string()), Span::raw("_")]);

    let count_style = if count >= CHAT_MAX {
        Style::new().fg(Color::Red)
    } else {
        dim
    };
    let (hint, hint_style) = match (&c.hint, app.chat_busy()) {
        (Some(reason), _) => (reason.clone(), Style::new().fg(Color::Red)),
        (None, true) => ("发送中…".to_string(), dim),
        (None, false) => ("Enter 发送 · Esc 收起".to_string(), dim),
    };
    let hint_line = Line::from(vec![
        Span::styled(format!("{count}/{CHAT_MAX} "), count_style),
        Span::styled(hint, hint_style),
    ]);

    frame.render_widget(Paragraph::new(vec![channel_line, input_line, hint_line]), inner);
}
