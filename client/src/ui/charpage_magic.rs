//! 角色页精灵动作的底部一行（STAGE-231，`docs/ui.md` 角色页 C）：动作列表只有「使用」；对象列表是一行横排（自己、各只宠物），
//! 选中的那一项反色。三块压缩后仍是 3 行（边框加一行内容），和加点表单同一个高度。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph};

use crate::app::{CharStep, WorldState};

use super::widgets::selected_style;

/// 画精灵动作的那一行。调用方只在 `char_step` 是 `MagicAction` 或 `MagicTarget` 时调用。
pub fn render_step(frame: &mut Frame, world: &WorldState, area: Rect) {
    let dim = Style::default().fg(Color::DarkGray);
    let (title, spans) = match world.char_step.as_ref() {
        Some(CharStep::MagicAction { slot }) => {
            let name = world.magic_name(*slot);
            let spans = vec![
                Span::styled(" 使用 ", selected_style(true)),
                Span::styled(format!("   {name} · Enter 执行 · Esc 取消"), dim),
            ];
            (format!("精灵 · {name}"), spans)
        }
        Some(CharStep::MagicTarget { slot, index }) => {
            let name = world.magic_name(*slot);
            let mut spans = vec![Span::raw(" 对谁用：")];
            for (i, label) in target_labels(world).into_iter().enumerate() {
                spans.push(Span::styled(format!(" {label} "), selected_style(i == *index)));
                spans.push(Span::raw(" "));
            }
            spans.push(Span::styled("· ←→ 选 · Enter 确认 · Esc 返回", dim));
            (format!("使用 · {name}"), spans)
        }
        _ => return,
    };
    frame.render_widget(
        Paragraph::new(Line::from(spans)).block(Block::bordered().title(title)),
        area,
    );
}

/// 对象列表的各项文字：第 0 项是自己，之后是每只宠物（栏位号加名字，倒下的加「倒」）。队友一期无效，不列。
fn target_labels(world: &WorldState) -> Vec<String> {
    let mut labels = vec!["自己".to_string()];
    labels.extend(world.pets.pets.iter().map(|p| {
        let down = if p.hp == 0 { "·倒" } else { "" };
        format!("{} {}{down}", p.slot, p.name)
    }));
    labels
}
