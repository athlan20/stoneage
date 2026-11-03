//! 主界面第 5 页「任务」（STAGE-86，`docs/ui.md` 任务页）：一张带黄色表头的表，列是 编号 / 任务 / 状态；
//! 进行中黄色、已完成绿色，进行中的排在前面。标题写进行中和已完成的数量，没有任何任务时写灰色提示。
//! 高度不够时 PgUp/PgDn 翻页。显示范围由 `quest` 命令切换。
//! STAGE-89：`quest 编号` 打开攻略，宽度 ≥ 110 列时列表和攻略并排，更窄时攻略替换列表（同百科页），`quest back` 或 Esc 回列表。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::widgets::{Block, Cell, Paragraph, Row, Table};

use super::widgets::{select_mark, selected_style};
use super::wikipage::render_detail_panel;
use crate::app::{App, QuestFilter, SIDE_BY_SIDE, WorldState, page_cursor, scroll};

const HEADER_STYLE: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);
const HINT_STYLE: Style = Style::new().fg(Color::DarkGray);

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_mut() else {
        return;
    };
    if world.quest_guide.is_none() {
        render_list(frame, world, area);
    } else if area.width >= SIDE_BY_SIDE {
        let [list, guide] = Layout::horizontal([Constraint::Percentage(40), Constraint::Min(0)]).areas(area);
        render_list(frame, world, list);
        render_guide(frame, world, guide);
    } else {
        render_guide(frame, world, area);
    }
}

fn render_list(frame: &mut Frame, world: &mut WorldState, area: Rect) {
    let (doing, done) = world.quest_counts();
    let mut title = format!("任务 · 进行中 {doing} · 已完成 {done}");
    if let Some(label) = world.quest_filter.label() {
        title.push_str(" · ");
        title.push_str(label);
    }
    let block = Block::bordered().title(title);
    let inner = block.inner(area);
    frame.render_widget(block, area);

    if world.quests.quests.is_empty() {
        frame.render_widget(Paragraph::new("  还没有接到任务，去和村里的 NPC 聊聊吧").style(HINT_STYLE), inner);
        return;
    }
    let total = world.visible_quests().len();
    if total == 0 {
        let text = match world.quest_filter {
            QuestFilter::Doing => "  没有进行中的任务",
            QuestFilter::Done => "  没有已完成的任务",
            QuestFilter::All => "  没有任务",
        };
        frame.render_widget(Paragraph::new(text).style(HINT_STYLE), inner);
        return;
    }

    // 表头占一行，其余是数据行；窗口变小时滚动位置不越过最后一页。选中项移出可见范围时列表跟着滚。
    let data_rows = usize::from(inner.height.saturating_sub(1));
    world.quest_rows = data_rows;
    let cursor = page_cursor(world, "任务");
    let top = match cursor {
        Some((sel, _)) => scroll::follow_top(world.quest_scroll, sel, data_rows, total),
        None => world.quest_scroll.min(total.saturating_sub(data_rows)),
    };
    world.quest_scroll = top;

    let header = Row::new(["编号", "任务", "状态"]).style(HEADER_STYLE);
    let rows: Vec<Row> = world
        .visible_quests()
        .into_iter()
        .enumerate()
        .skip(top)
        .take(data_rows)
        .map(|(i, q)| {
            let (label, color) = if q.state == "done" { ("已完成", Color::Green) } else { ("进行中", Color::Yellow) };
            let selected = matches!(cursor, Some((sel, _)) if sel == i);
            let focused = matches!(cursor, Some((sel, true)) if sel == i);
            Row::new(vec![
                Cell::from(format!("{}{}", select_mark(selected), q.no)),
                Cell::from(q.name.clone()),
                Cell::from(label).style(Style::default().fg(color)),
            ])
            .style(if selected { selected_style(focused) } else { Style::default() })
        })
        .collect();
    let widths = [Constraint::Length(6), Constraint::Min(10), Constraint::Length(8)];
    frame.render_widget(Table::new(rows, widths).header(header), inner);
}

fn render_guide(frame: &mut Frame, world: &mut WorldState, area: Rect) {
    let Some(guide) = world.quest_guide.as_mut() else { return };
    let title = format!("攻略 · {}", guide.name);
    render_detail_panel(frame, area, title, &guide.lines, &mut guide.scroll, &mut guide.view_rows);
}
