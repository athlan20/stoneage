//! 背包动作「精工」的材料选择面板（STAGE-299，`docs/inlay.md` 2.8）：列出背包里的材料，`[x]` 是已勾选，
//! 标题写已选件数和上限；没有材料时写一句话。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::Style;
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem};

use super::inventpage::row_style;
use crate::app::WorldState;
use crate::app::inventory_inlay::max_pick;

/// 材料选择面板：`free` 是目标还剩的空格，`index` 是选中的材料行，`picked` 是已勾选的格号。
pub(super) fn render_inlay_pick(
    frame: &mut Frame,
    world: &WorldState,
    area: Rect,
    title: String,
    free: u8,
    index: usize,
    picked: &[u8],
) {
    let materials = world.inlay_materials();
    let title = format!("{title} · 已选 {}/{}", picked.len(), max_pick(free));
    let rows: Vec<ListItem> = if materials.is_empty() {
        vec![ListItem::new(Line::from(Span::styled(
            "背包里没有精工材料",
            Style::default(),
        )))]
    } else {
        materials
            .iter()
            .enumerate()
            .map(|(i, it)| {
                let mark = if picked.contains(&it.slot) { "[x]" } else { "[ ]" };
                let text = format!("{mark} {:>2} 号 {}  ×{}", it.slot, it.name, it.count);
                ListItem::new(Line::from(Span::raw(text))).style(row_style(i == index, false))
            })
            .collect()
    };
    frame.render_widget(List::new(rows).block(Block::bordered().title(title)), area);
}
