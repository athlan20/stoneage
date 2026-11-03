//! STAGE-274：组队请求弹窗（`docs/ui.md`「请求弹窗」）：居中，默认焦点「拒绝」，倒计时，答复中显示「处理中」。
//! 战斗中、NPC 对话窗口里不画（由 `shown_popup` 决定）。

use std::time::Instant;

use ratatui::Frame;
use ratatui::layout::{Alignment, Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Clear, Paragraph, Wrap};

use crate::app::App;
use crate::app::party_requests::{PopupFocus, shown_popup};

const WIDTH: u16 = 34;
const HEIGHT: u16 = 8;

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let Some(p) = shown_popup(world) else {
        return;
    };
    let width = WIDTH.min(area.width);
    let height = HEIGHT.min(area.height);
    let rect = Rect {
        x: area.x + (area.width - width) / 2,
        y: area.y + (area.height - height) / 2,
        width,
        height,
    };
    frame.render_widget(Clear, rect);
    let apply = p.req.kind == "apply";
    let trade = p.req.kind == crate::app::trade::TRADE_KIND;
    let title = if trade {
        "交易邀请"
    } else if apply {
        "申请加入你的队伍"
    } else {
        "组队邀请"
    };
    let block = Block::bordered().title(Line::from(Span::styled(
        title,
        Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD),
    )));
    let inner = block.inner(rect);
    frame.render_widget(block, rect);

    let size = p
        .req
        .party
        .as_ref()
        .map(|b| format!("（{}/{}）", b.size, b.max))
        .unwrap_or_default();
    let what = if trade {
        let (lv, tr) = p.trade_from.unwrap_or_default();
        format!("{}（Lv{lv}，转生 {tr}）想和你交易", p.req.from)
    } else if apply {
        format!("{} 申请加入你的队伍{size}", p.req.from)
    } else {
        format!("{} 邀请你加入队伍{size}", p.req.from)
    };
    let [text, time, buttons] =
        Layout::vertical([Constraint::Min(2), Constraint::Length(1), Constraint::Length(1)]).areas(inner);
    frame.render_widget(
        Paragraph::new(what)
            .wrap(Wrap { trim: true })
            .alignment(Alignment::Center),
        text,
    );
    if p.answering {
        frame.render_widget(
            Paragraph::new("处理中…")
                .style(Style::new().fg(Color::DarkGray))
                .alignment(Alignment::Center),
            buttons,
        );
        return;
    }
    let remaining = p.remaining_secs(Instant::now());
    frame.render_widget(
        Paragraph::new(format!("剩余 {remaining} 秒"))
            .style(Style::new().fg(Color::DarkGray))
            .alignment(Alignment::Center),
        time,
    );
    let focus = world.party_requests.focus;
    let accept_label = if apply { "接受" } else { "加入" };
    let line = Line::from(vec![
        button("拒绝", focus == PopupFocus::Reject),
        Span::raw("    "),
        button(accept_label, focus == PopupFocus::Accept),
    ]);
    frame.render_widget(Paragraph::new(line).alignment(Alignment::Center), buttons);
}

fn button(label: &str, focused: bool) -> Span<'static> {
    let text = format!("[{label}]");
    if focused {
        Span::styled(
            text,
            Style::new()
                .fg(Color::Yellow)
                .add_modifier(Modifier::BOLD | Modifier::REVERSED),
        )
    } else {
        Span::raw(text)
    }
}
