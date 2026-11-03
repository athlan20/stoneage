//! STAGE-276：社交页第三栏「设置」（`docs/ui.md`「社交页的开关」）：两行开关（标签、开/关、处理中）加底部说明。
//! 状态绿/红区分；`Enter` 或空格切换由 `app/social_settings.rs` 接管，这里只画。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Cell, Paragraph, Row, Table, Wrap};

use super::widgets::{select_mark, selected_style};
use crate::app::WorldState;
use crate::app::social_settings::{SocialField, selected_field};

const LABEL_STYLE: Style = Style::new().fg(Color::DarkGray);

pub fn render(frame: &mut Frame, world: &WorldState, area: Rect) {
    let block = Block::bordered().title("设置");
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let [rows_area, _gap, text] = Layout::vertical([
        Constraint::Length(SocialField::ALL.len() as u16),
        Constraint::Length(1),
        Constraint::Min(0),
    ])
    .areas(inner);
    let selected = selected_field(world);
    let rows: Vec<Row> = SocialField::ALL
        .iter()
        .map(|f| {
            let on = selected == Some(*f);
            let (status, style) = if world.social_busy == Some(*f) {
                ("处理中", LABEL_STYLE)
            } else if f.get(world.entry.social) {
                ("开", Style::default().fg(Color::Green))
            } else {
                ("关", Style::default().fg(Color::Red))
            };
            let row = Row::new(vec![
                Cell::from(select_mark(on)),
                Cell::from(f.label()),
                Cell::from(Span::styled(status, style)),
            ]);
            if on { row.style(selected_style(true)) } else { row }
        })
        .collect();
    let widths = [Constraint::Length(1), Constraint::Length(18), Constraint::Min(6)];
    frame.render_widget(Table::new(rows, widths), rows_area);
    let lines = vec![
        Line::from(
            "关掉「对附近玩家可见」就是隐身：别人的附近玩家列表里看不到你，你仍能看到别人，也能主动邀请和申请。",
        ),
        Line::default(),
        Line::from("关掉「接收组队邀请」后，别人的组队邀请会被直接拒绝；你自己的申请和主动邀请不受影响。"),
        Line::default(),
        Line::from("打完一场或重新登录后自动关闭；开着时同图的人可以直接向你发起决斗。"),
    ];
    frame.render_widget(Paragraph::new(lines).wrap(Wrap { trim: true }).style(LABEL_STYLE), text);
}
