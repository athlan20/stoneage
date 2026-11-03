//! 设置页里的两个手输框显示（名单 STAGE-173，等级手输 STAGE-264）：
//! 占名单行下面的两行，第一行是当前值，第二行是输入框；说明行放在底部提示的第二行。

use ratatui::{
    style::{Color, Modifier, Style},
    text::{Line, Span},
};

use crate::app::auto_battle::AutoProfile;
use crate::app::auto_battle::edit::Editor;
use crate::app::auto_battle::fields::Field;
use crate::app::auto_battle::level_input::LevelEdit;
use crate::app::auto_battle::names_input::NameEdit;

/// 输入框前面的“新名字：”占 9 列，后面的按键说明占 46 列，剩下的给输入内容。
const NAME_CHROME: usize = 9 + 46;
/// “ 手输：”占 7 列，后面的说明占 26 列。
const LEVEL_CHROME: usize = 7 + 26;

/// 名单编辑的两行：名单（选中项加【】）与输入框。
pub fn names_lines(ne: &NameEdit, draft: &AutoProfile, width: usize) -> Vec<Line<'static>> {
    let list: &[String] = match ne.field {
        Field::FleeNames => &draft.flee.names,
        Field::CaptureNames => &draft.capture.names,
        Field::MpItems => &draft.recover.mp_items,
        _ => &[],
    };
    let shown = if list.is_empty() {
        "（空，表示不限）".to_string()
    } else {
        let sel = ne.sel.min(list.len() - 1);
        list.iter()
            .enumerate()
            .map(|(i, n)| if i == sel { format!("【{n}】") } else { n.clone() })
            .collect::<Vec<_>>()
            .join("、")
    };
    let (visible, _) = ne.input.view(width.saturating_sub(NAME_CHROME).max(8));
    vec![
        Line::from(Span::styled(
            format!("{}：{shown}", ne.field.label()),
            Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
        )),
        Line::from(vec![
            Span::raw(" 新名字："),
            Span::styled(format!("{visible}_"), Style::default().fg(Color::Yellow)),
            Span::styled(
                "（Enter 加入 · ↑↓ 选择 · Del 删除 · Esc 返回）",
                Style::default().fg(Color::DarkGray),
            ),
        ]),
    ]
}

/// 等级手输的两行：当前值与输入框。
pub fn level_lines(le: &LevelEdit, draft: &AutoProfile, width: usize) -> Vec<Line<'static>> {
    let cur = match le.field {
        Field::CaptureLvMax => draft.capture.level_max,
        _ => draft.capture.level_min,
    };
    let cur_text = if cur == 0 {
        "不限".to_string()
    } else {
        format!("{cur} 级")
    };
    let (visible, _) = le.input.view(width.saturating_sub(LEVEL_CHROME).max(4));
    vec![
        Line::from(Span::styled(
            format!("{}：当前 {cur_text}", le.field.label()),
            Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
        )),
        Line::from(vec![
            Span::raw(" 手输："),
            Span::styled(format!("{visible}_"), Style::default().fg(Color::Yellow)),
            Span::styled(
                "（0–200 的整数 · Enter 确定 · Esc 取消）",
                Style::default().fg(Color::DarkGray),
            ),
        ]),
    ]
}

/// 手输框打开时，底部说明的第二行（第一行是上一步的提示）。
pub fn tip(ed: &Editor) -> Option<&'static str> {
    if ed.names.is_some() {
        Some("可用 | 一次输入多个名字（全角 ｜ 也行），如 乌力|凯比；重复的自动跳过")
    } else if ed.level.is_some() {
        Some("0＝不限；下限和上限都不是 0 时，下限不能大于上限")
    } else {
        None
    }
}
