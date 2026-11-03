//! 宠物合成的两块面板（STAGE-208）：选材料（背包 5–19 的清单，空格勾选）和结果页。
//! 画在宠物页下方的流程面板里（见 `petpage.rs`），按键见 `app/pet_merge.rs`。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem, Paragraph};

use super::petpage::LABEL_STYLE;
use super::widgets::selected_style;
use crate::app::pet_merge::{MAX_MERGE_ITEMS, MIN_MERGE_ITEMS, mixes_food_and_tools};
use crate::app::{MergeKind, MergeReport, WorldState};

/// 结果页的高度（含边框）：结果行加一行操作提示。
pub(super) fn result_panel_height(report: &MergeReport) -> u16 {
    (report.lines.len() as u16 + 3).max(5)
}

/// 选材料：第一行写已选件数和提醒，下面是材料清单，`[1]` 这样的数字是勾选顺序。
pub(super) fn render_merge(
    frame: &mut Frame,
    world: &WorldState,
    title: String,
    cursor: usize,
    picked: &[u8],
    area: Rect,
) {
    let block = Block::bordered().title(title);
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let materials = world.merge_materials();
    let chosen = world.merge_picked(picked);
    let mixed = mixes_food_and_tools(chosen.iter().map(|s| &s.kind));
    let mut status = vec![Span::raw(format!(
        "已选 {} 件（要 {MIN_MERGE_ITEMS}–{MAX_MERGE_ITEMS} 件）",
        chosen.len()
    ))];
    if mixed {
        status.push(Span::styled(
            "  食物和道具不能混着合成",
            Style::default().fg(Color::Red),
        ));
    } else {
        status.push(Span::styled("  材料不论成败都会用掉", LABEL_STYLE));
    }
    let [head, body] = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).areas(inner);
    frame.render_widget(Paragraph::new(Line::from(status)), head);
    // 光标要看得见：列表比面板长时，让光标行落在窗口里。
    let visible = usize::from(body.height).max(1);
    let start = cursor
        .saturating_sub(visible - 1)
        .min(materials.len().saturating_sub(visible));
    let rows: Vec<ListItem> = materials
        .iter()
        .enumerate()
        .skip(start)
        .take(visible)
        .map(|(i, it)| {
            let mark = match picked.iter().position(|n| *n == it.slot) {
                Some(order) => format!("[{}]", order + 1),
                None => "[ ]".to_string(),
            };
            let mut spans = vec![Span::raw(format!("{mark} {:>2} {}", it.slot, it.name))];
            if it.count > 1 {
                spans.push(Span::styled(format!(" ×{}", it.count), LABEL_STYLE));
            }
            if !it.effect.is_empty() {
                spans.push(Span::styled(format!("  {}", it.effect), LABEL_STYLE));
            }
            let item = ListItem::new(Line::from(spans));
            if i == cursor {
                item.style(selected_style(true))
            } else {
                item
            }
        })
        .collect();
    frame.render_widget(List::new(rows), body);
}

/// 结果页：成功绿、还料和冷却黄、混合红，最后一行写用掉了什么。
pub(super) fn render_result(frame: &mut Frame, title: String, report: &MergeReport, area: Rect) {
    let color = match report.kind {
        MergeKind::Created => Color::Green,
        MergeKind::Fallback | MergeKind::Cooldown => Color::Yellow,
        MergeKind::Illegal => Color::Red,
    };
    let last = report.lines.len().saturating_sub(1);
    let mut lines: Vec<Line> = report
        .lines
        .iter()
        .enumerate()
        .map(|(i, text)| match i {
            0 => Line::from(Span::styled(text.clone(), Style::default().fg(color))),
            i if i == last => Line::from(Span::styled(text.clone(), LABEL_STYLE)),
            _ => Line::from(text.clone()),
        })
        .collect();
    lines.push(Line::from(Span::styled("Enter 再合成 · Esc 返回技能", LABEL_STYLE)));
    frame.render_widget(Paragraph::new(lines).block(Block::bordered().title(title)), area);
}
