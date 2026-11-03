//! 游戏菜单（STAGE-131，`docs/cli-ui-redesign.md`「场景与传送」）：场景根页按 Esc 打开，
//! 列出角色、宠物、背包、任务、快捷传送、社交、消息、百科、系统。Enter 进入功能页，Esc 回场景。
//! 已开放的功能都能从这里到达（STAGE-131 验收标准 1）；STAGE-162 起「社交」也开放了（队伍一栏）。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem};

use crate::app::{App, GAME_MENU};

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let index = world.menu_index.min(GAME_MENU.len() - 1);
    let items: Vec<ListItem> = GAME_MENU
        .iter()
        .enumerate()
        .map(|(i, (label, _menu))| {
            let item = ListItem::new(Line::from(Span::raw(*label)));
            if i == index {
                item.style(Style::default().fg(Color::Black).bg(Color::Yellow).add_modifier(Modifier::BOLD))
            } else {
                item
            }
        })
        .collect();
    frame.render_widget(List::new(items).block(Block::bordered().title("游戏菜单")), area);
}
