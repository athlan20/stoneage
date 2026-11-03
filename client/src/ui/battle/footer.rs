//! STAGE-167：下方指令与战斗日志。窄屏保留最新一条日志，不在中场重复播报。
//! STAGE-248：日志区不再读战斗记录，改从消息日志取最近几条，战斗文字、系统、错误、命令、队伍、耐久按时间混排；
//! 日志高度按终端高度分档（`height`）。
use super::bar;
use crate::app::{App, battle::BattleState};
use crate::app::Kind;
use crate::ui::widgets::{chat_items, message_items, message_line};
use crate::ui::world::BATTLE_BOTTOM_ROWS;
use ratatui::{
    Frame,
    layout::{Constraint, Layout, Rect},
    style::{Color, Style},
    text::{Line, Span},
    widgets::{Block, List, Paragraph},
};

/// 窄屏（左右分栏放不下）的日志区：3 行指令 + 1 行日志，总高 4 行。
const NARROW_HEIGHT: u16 = 4;
/// 宽屏日志区的高度范围（含边框）：`(内容区高度 / 4).clamp(4, 12)`。
const LOG_MIN: u16 = 4;
const LOG_MAX: u16 = 12;
/// 战场（战斗块，含顶栏和边框）至少保留的行数；日志不够让位时先压日志，压到 `LOG_MIN` 为止。
const FIELD_MIN: u16 = 12;
/// 宽屏（指令在左、日志在右）需要的内宽：战斗块左右边框各占一列，终端 100 列起分栏。
const WIDE_MIN_WIDTH: u16 = 98;

/// 日志区总高（含边框）。只由终端尺寸决定，输入 / 回放 / 结算切换时不跳（STAGE-248）。
/// `width` 是战斗块的内宽；`content_height` 是顶栏以下的高度（终端高度减去顶栏 3 行）。
/// 日志最多占内容区的 1/3，战场至少 `FIELD_MIN` 行（超出就压日志，压到 `LOG_MIN` 为止）。
pub(super) fn height(width: u16, content_height: u16) -> u16 {
    if width < WIDE_MIN_WIDTH {
        return NARROW_HEIGHT;
    }
    let base = (content_height / 4).clamp(LOG_MIN, LOG_MAX);
    let room = content_height.saturating_sub(BATTLE_BOTTOM_ROWS + FIELD_MIN);
    base.min(content_height / 3).min(room).max(LOG_MIN)
}

pub(super) fn render(frame: &mut Frame, app: &App, b: &BattleState, area: Rect) {
    if area.width >= WIDE_MIN_WIDTH && area.height >= 5 {
        let left = (area.width * 3 / 5).max(70);
        let [commands, logs] =
            Layout::horizontal([Constraint::Length(left), Constraint::Min(0)]).areas(area);
        let block = Block::bordered().title("指令");
        let inner = block.inner(commands);
        frame.render_widget(block, commands);
        bar::render(frame, app, b, inner, true);
        // 玩家正在聊天：日志区只给聊天，战斗文字不把它冲走（`docs/battle-ui.md`「聊天时的日志区」）。
        let chatting = app.chat_focus();
        let block = Block::bordered().title(if chatting { "聊天" } else { "战斗日志" });
        let inner = block.inner(logs);
        frame.render_widget(block, logs);
        let rows = usize::from(inner.height);
        let items = if chatting { chat_items(app, rows) } else { message_items(app, rows) };
        frame.render_widget(List::new(items), inner);
    } else {
        let [commands, log] =
            Layout::vertical([Constraint::Length(3), Constraint::Length(1)]).areas(area);
        bar::render(frame, app, b, commands, true);
        let chatting = app.chat_focus();
        let title = if chatting { "聊天 · " } else { "战斗日志 · " };
        let mut spans = vec![Span::styled(title, Style::default().fg(Color::DarkGray))];
        let latest = if chatting {
            app.log.all().iter().rev().find(|m| matches!(m.kind, Kind::Chat(_) | Kind::Agent))
        } else {
            app.log.recent(1).next()
        };
        match latest {
            Some(m) => spans.extend(message_line(m.kind, &m.text, false).spans),
            None => spans.push(Span::raw(if chatting { "暂无聊天" } else { "暂无战斗记录" })),
        }
        frame.render_widget(Paragraph::new(Line::from(spans)), log);
    }
}

#[cfg(test)]
mod tests {
    use super::height;

    /// 四档尺寸（STAGE-248 表）：200×60、120×40、100×30 是宽屏，内容区高度是终端高度减去顶栏 3 行。
    #[test]
    fn log_height_follows_the_terminal_height() {
        assert_eq!(height(198, 57), 12, "200×60：10 条");
        assert_eq!(height(118, 37), 9, "120×40：7 条");
        assert_eq!(height(98, 27), 6, "100×30：4 条");
    }

    /// 80×24 是窄屏：3 行指令 + 1 行日志，不管高度。
    #[test]
    fn narrow_terminal_keeps_one_log_line() {
        assert_eq!(height(78, 21), 4);
        assert_eq!(height(78, 60), 4);
    }

    /// 战场至少 12 行：内容区矮时先压日志，压到 4 行为止。
    #[test]
    fn field_keeps_twelve_rows_before_the_log_grows() {
        // 内容区 20：战场块 = 20 - 4 = 16，日志最多 4 行，战场 12 行。
        assert_eq!(height(118, 20), 4);
        // 内容区 21：日志 5 行，战场 21 - 4 - 5 = 12 行。
        assert_eq!(height(118, 21), 5);
        // 内容区很矮（不足以留出 12 行战场）：仍是下限 4 行。
        assert_eq!(height(118, 10), 4);
    }
}
