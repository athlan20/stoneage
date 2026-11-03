use crate::app::{App, ExitKind};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
    widgets::{Block, Clear, Paragraph},
};
pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(d) = app.exit_dialog else {
        return;
    };
    let pop = super::widgets::centered(area, 56, 6);
    frame.render_widget(Clear, pop);
    let title = if d.kind == ExitKind::Quit {
        "退出游戏"
    } else {
        "离开世界"
    };
    let buttons = if d.confirm {
        "取消      [确认]"
    } else {
        "[取消]      确认"
    };
    frame.render_widget(
        Paragraph::new(format!("\n {buttons}\n ←→ 选择 · Enter 执行 · Esc 取消")).block(
            Block::bordered()
                .title(title)
                .border_style(Style::default().fg(Color::Yellow)),
        ),
        pop,
    );
}
