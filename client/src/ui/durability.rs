//! 装备耐久（STAGE-225，`docs/ui.md` 背包页、`docs/durability.md`）：背包页底边的耐久行。
//! 等级由服务端下发的 `crush_level` 决定，这里只负责文字和颜色；只用 16 色具名颜色。

use ratatui::style::{Color, Style};
use ratatui::text::Span;

use crate::proto::InventorySlot;

/// 受损等级的文字和颜色：完好绿、受损黄、毁坏和不堪使用红。
pub fn level_text(level: u8) -> (&'static str, Color) {
    match level {
        0 => ("完好", Color::Green),
        1 => ("受损", Color::Yellow),
        2 => ("毁坏", Color::Red),
        _ => ("不堪使用", Color::Red),
    }
}

/// 耐久行：`耐久 180/200（90%） 受损`。没有耐久（最大耐久为 0）的物品返回 None，界面不显示这一行。
pub fn durability_spans(it: &InventorySlot) -> Option<Vec<Span<'static>>> {
    if it.max_durability == 0 {
        return None;
    }
    let pct = (u64::from(it.durability) * 100 / u64::from(it.max_durability)).min(100);
    let (text, color) = level_text(it.crush_level);
    Some(vec![
        Span::raw(format!("耐久 {}/{}（{pct}%） ", it.durability, it.max_durability)),
        Span::styled(text, Style::default().fg(color)),
    ])
}
