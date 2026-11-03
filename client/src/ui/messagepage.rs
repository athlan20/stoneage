//! 主界面第 6 页「消息」（STAGE-27，`docs/ui.md` 消息页）：占满中间区域显示本次登录以来的
//! 全部消息，最新的在最下面；标签与配色复用底部消息栏的 `Kind`；长消息换行，续行缩进到标签后。
//! 滚动锚定、翻页步进的纯逻辑在 `app/msgview.rs`。

use ratatui::Frame;
use ratatui::layout::{Alignment, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem, Paragraph};

use crate::app::msgview;
use crate::app::{App, LABEL_WIDTH, wrap_text};

use super::widgets::message_line;

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_mut() else {
        return;
    };
    world.msg_view.rows = usize::from(area.height.saturating_sub(2)); // 扣掉上下边框
    world.msg_view.cols = usize::from(area.width.saturating_sub(2));
    let view = world.msg_view;
    let log = &app.log;

    let (title, right, items) = if log.is_empty() {
        ("消息 · 暂无".to_string(), None, vec![])
    } else {
        let Some((first, last)) = msgview::window(log, &view) else {
            return;
        };
        let latest = log.latest_seq();
        let title = format!("消息 · 第 {first}–{last} / {latest} 条");
        let right = match view.anchor {
            Some(_) => {
                let n = msgview::unseen_count(log, &view);
                (n > 0).then(|| {
                    Line::from(Span::styled(
                        format!(" 有 {n} 条新消息 ↓ "),
                        Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD),
                    ))
                })
            }
            None => None,
        };
        let body_width = view.cols.saturating_sub(LABEL_WIDTH);
        let mut items: Vec<ListItem<'static>> = vec![];
        for m in log.from_seq(first).take_while(|m| m.seq <= last) {
            for (i, chunk) in wrap_text(&m.text, body_width).into_iter().enumerate() {
                items.push(ListItem::new(message_line(m.kind, chunk, i > 0)));
            }
        }
        // 屏幕顶部那条消息滚过去的行不显示：逐行滚动和跟随时露出的半条（STAGE-116）。
        let skip = msgview::top(log, &view).map_or(0, |(_, skip)| skip);
        items.drain(..skip.min(items.len()));
        // 跟随最新时内容贴底（新消息出现在最下面）；锚定旧位置时从锚点往下读，顶部对齐。
        if view.anchor.is_none() {
            let blank = view.rows.saturating_sub(items.len());
            items.splice(0..0, std::iter::repeat(ListItem::new("")).take(blank));
        }
        (title, right, items)
    };

    let mut block = Block::bordered().title(title);
    if let Some(r) = right {
        block = block.title(r.right_aligned());
    }
    let inner = block.inner(area);
    frame.render_widget(block, area);
    if log.is_empty() {
        frame.render_widget(
            Paragraph::new("本次登录以来还没有消息").style(Style::default().fg(Color::DarkGray)).alignment(Alignment::Center),
            Rect::new(inner.x, inner.y + inner.height / 2, inner.width, 1),
        );
    } else {
        frame.render_widget(List::new(items), inner);
    }
}
