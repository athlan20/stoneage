//! 自动战斗设置页（STAGE-173）：五栏用 Tab 切换，一次显示一栏，80×24 下也能完整操作。
//! 当前栏名字在顶部加色，焦点行黄色加粗；值右边括注来源或不可用的原因，底部是提示与按键说明。

use ratatui::{
    Frame,
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::Paragraph,
};

use super::autosettings_input;
use crate::app::auto_battle::edit::Ask;
use crate::app::auto_battle::fields;
use crate::app::App;

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(ed) = app.world.as_ref().and_then(|w| w.auto_edit.as_ref()) else {
        frame.render_widget(Paragraph::new("设置页已关闭").style(Style::default().fg(Color::Red)), area);
        return;
    };
    // 抓宠丢弃记录页（STAGE-264）整页盖住设置行。
    if let Some(log) = ed.log.as_ref() {
        super::capture_log_page::render(frame, log, area);
        return;
    }
    let ctx = app.auto_context();
    // 名单编辑或等级手输打开时多占两行（当前值 + 输入框）。
    let extra = if ed.names.is_some() || ed.level.is_some() { 2 } else { 0 };
    let [tabs_area, list_area, names_area, note_area, hint_area] = Layout::vertical([
        Constraint::Length(1),
        Constraint::Min(3),
        Constraint::Length(extra),
        Constraint::Length(2),
        Constraint::Length(1),
    ])
    .areas(area);

    // 顶部：五个栏名，当前栏反色。
    let mut tabs = Vec::new();
    for (i, (name, _)) in fields::columns().iter().enumerate() {
        let style = if i == ed.column {
            Style::default().fg(Color::Black).bg(Color::Cyan).add_modifier(Modifier::BOLD)
        } else {
            Style::default().fg(Color::Gray)
        };
        tabs.push(Span::styled(format!(" {name} "), style));
    }
    frame.render_widget(Paragraph::new(Line::from(tabs)), tabs_area);

    // 中间：当前栏的行。
    let rows = fields::column_fields(ed.column);
    let mut lines = Vec::new();
    for (i, f) in rows.iter().enumerate() {
        let (val, sub) = f.value(&ed.draft, &ctx);
        let blocked = f.blocked(&ed.draft, &ctx);
        let note = match (blocked, sub.is_empty()) {
            (Some(why), _) => format!("（{why}）"),
            (None, false) => format!("（{sub}）"),
            (None, true) => String::new(),
        };
        let tag = if f.read_only() {
            " · 只读"
        } else if f.is_name_list() {
            " · 名单"
        } else {
            ""
        };
        let head = if i == ed.row { "› " } else { "  " };
        let text = format!("{head}{}：{val}{note}{tag}", f.label());
        let style = if blocked.is_some() {
            Style::default().fg(Color::DarkGray)
        } else if i == ed.row {
            Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
        } else {
            Style::default()
        };
        lines.push(Line::from(Span::styled(text, style)));
    }
    // 一栏的行可能比区域高（80×24 下恢复栏有 15 行）：窗口跟着焦点行走，焦点行一直可见（STAGE-260）。
    let height = list_area.height as usize;
    let top = (ed.row + 1).saturating_sub(height);
    let visible: Vec<Line> = lines.into_iter().skip(top).take(height).collect();
    frame.render_widget(Paragraph::new(visible), list_area);

    // 名单编辑（M07 补气物品 / M10 逃跑敌人 / M13 捕捉目标）与等级手输（STAGE-264）：占两行。
    let width = area.width as usize;
    if let Some(ne) = ed.names.as_ref() {
        frame.render_widget(Paragraph::new(autosettings_input::names_lines(ne, &ed.draft, width)), names_area);
    } else if let Some(le) = ed.level.as_ref() {
        frame.render_widget(Paragraph::new(autosettings_input::level_lines(le, &ed.draft, width)), names_area);
    }

    // 提示：确认框优先，其次是上一步的结果，最后是默认说明。
    let (note, style) = match ed.ask {
        Some(ask) => {
            let (head, items): (&str, &[&str]) = match ask {
                Ask::Exit => ("有未保存的改动", &["继续编辑", "保存并应用", "放弃改动"]),
                Ask::Delete => ("删除方案", &["取消", "删除"]),
                Ask::DropOn => ("开启战后丢弃肉类（丢弃不可恢复）", &["取消", "开启"]),
            };
            let bar = items
                .iter()
                .enumerate()
                .map(|(i, s)| if i == ed.ask_choice { format!("[{s}]") } else { format!(" {s} ") })
                .collect::<Vec<_>>()
                .join("  ");
            let tone = if ask == Ask::Exit { Color::Yellow } else { Color::Red };
            let name = if ask == Ask::DropOn { String::new() } else { format!("「{}」", ed.draft.name) };
            (format!("{head}{name}：{bar}（Enter 执行 · ←→ 选择 · Esc 返回）"), Style::default().fg(tone))
        }
        None => (
            ed.hint.clone().unwrap_or_else(|| format!("方案「{}」· 改完要「保存并应用」才生效", ed.draft.name)),
            Style::default().fg(Color::Gray),
        ),
    };
    if let Some(input) = ed.naming.as_ref() {
        frame.render_widget(
            Paragraph::new(format!("新方案名：{}_（Enter 确定 · Esc 取消）", input.text()))
                .style(Style::default().fg(Color::Yellow)),
            note_area,
        );
    } else if let Some(tip) = autosettings_input::tip(ed) {
        // 名单 / 手输打开时说明占第二行，例如「可用 | 一次输入多个」。
        let lines = vec![
            Line::from(Span::styled(note, style)),
            Line::from(Span::styled(tip, Style::default().fg(Color::DarkGray))),
        ];
        frame.render_widget(Paragraph::new(lines), note_area);
    } else {
        frame.render_widget(Paragraph::new(note).style(style), note_area);
    }
    let dirty = if ed.dirty { " · 未保存" } else { "" };
    frame.render_widget(
        Paragraph::new(format!("Tab 换栏 · ↑↓ 选择 · ←→ 调整 · Enter 执行 · Esc 返回{dirty}"))
            .style(Style::default().fg(Color::DarkGray)),
        hint_area,
    );
}
