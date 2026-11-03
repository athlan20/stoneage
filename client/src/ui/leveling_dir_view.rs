//! STAGE-279：快捷传送页「练级传送」的画面（`docs/ui.md`「根层的「练级传送」」）。
//! 内容区宽 ≥ 88 列时右侧放详情；窄屏（80×24）只有列表。长名字按显示宽度截断。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph};
use unicode_width::UnicodeWidthStr;

use super::widgets::{select_mark, selected_style};
use crate::app::leveling_dir::{ACTIONS, LevelingDir, LevelingLevel, LevelingStatus, SEGMENTS, segment_min};
use crate::app::scroll::follow_top;
use crate::proto::LevelingEntry;

const SIDE_BY_SIDE: u16 = 88;
const DIM: Style = Style::new().fg(Color::DarkGray);
const HEAD: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);

pub fn render(frame: &mut Frame, dir: &mut LevelingDir, area: Rect, _unicode: bool) {
    let block = Block::bordered().title(title(dir));
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let (left, right) = if inner.width >= SIDE_BY_SIDE {
        let cols = Layout::horizontal([Constraint::Percentage(60), Constraint::Percentage(40)]).split(inner);
        (cols[0], Some(cols[1]))
    } else {
        (inner, None)
    };
    let [head, list] = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).areas(left);
    frame.render_widget(Paragraph::new(Line::from(Span::styled(head_text(dir), HEAD))), head);
    dir.rows = list.height as usize;
    let lines = list_lines(dir, list.height as usize, list.width as usize);
    frame.render_widget(Paragraph::new(lines), list);
    if let Some(detail) = right {
        frame.render_widget(
            Paragraph::new(detail_lines(dir)).block(Block::bordered().title("详情")),
            detail,
        );
    }
}

/// 标题：L1 写段数，L2 写段和点数，L3 写练级点，L4 写地图名。
fn title(dir: &LevelingDir) -> Span<'static> {
    let text = match dir.level() {
        LevelingLevel::Segments => format!(" 练级传送 · {SEGMENTS} 段 "),
        LevelingLevel::Entries { seg } => {
            let min = segment_min(*seg);
            let n = dir.segment_summary(*seg).map_or(0, |s| s.entry_count);
            format!(" {}-{} 级 · {n} 个点 ", min, min + 9)
        }
        LevelingLevel::Spots { seg, entry } => {
            let min = segment_min(*seg);
            let n = dir.entries_of(*seg).get(*entry).map_or(0, |e| e.spots.len());
            format!(" {}-{} 级 · 练级点 · {n} 处 ", min, min + 9)
        }
        LevelingLevel::Actions { .. } => {
            let name = dir.current_spot().map_or(String::new(), |s| s.map_name);
            format!(" {name} ")
        }
    };
    Span::styled(text, Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD))
}

/// 列表表头（每层一行，黄色）。
fn head_text(dir: &LevelingDir) -> String {
    match dir.level() {
        LevelingLevel::Segments => "等级段                       条目 · 涉及地图".to_string(),
        LevelingLevel::Entries { .. } => "怪物（段内等级）                   落点".to_string(),
        LevelingLevel::Spots { .. } => "地图 (x,y)                    区域  同场  约几格一场".to_string(),
        LevelingLevel::Actions { .. } => "动作".to_string(),
    }
}

fn list_lines(dir: &mut LevelingDir, height: usize, width: usize) -> Vec<Line<'static>> {
    if height == 0 {
        return Vec::new();
    }
    match &dir.status {
        LevelingStatus::Idle | LevelingStatus::Loading => {
            return vec![Line::from(Span::styled("练级目录加载中…", DIM))];
        }
        LevelingStatus::Failed(reason) => {
            return vec![Line::from(Span::styled(
                format!("练级目录不可用：{reason}（Enter 重新取）"),
                DIM,
            ))];
        }
        LevelingStatus::Ready => {}
    }
    let rows = rows_for(dir);
    if rows.is_empty() {
        return vec![Line::from(Span::styled(empty_text(dir), DIM))];
    }
    let cursor = dir.cursor();
    let top = follow_top(dir.scroll(), cursor, height, rows.len());
    dir.set_scroll(top);
    rows.iter()
        .enumerate()
        .skip(top)
        .take(height)
        .map(|(i, (left, right))| {
            let selected = i == cursor;
            let right = right.clone();
            let room = width.saturating_sub(2 + right.width() + 1);
            let text = format!("{}{}", select_mark(selected), fit(left, room));
            let pad = width.saturating_sub(text.width() + right.width());
            let line = format!("{text}{}{right}", " ".repeat(pad));
            Line::from(Span::styled(line, selected_style(true)))
        })
        .collect()
}

fn empty_text(dir: &LevelingDir) -> String {
    match dir.level() {
        LevelingLevel::Entries { seg } if dir.is_segment_loaded(*seg) => "本段没有野外遇敌点".to_string(),
        LevelingLevel::Entries { .. } => "练级点加载中…".to_string(),
        _ => "没有可以显示的内容".to_string(),
    }
}

/// 当前层每一行：左边的文字，右边的数字。
fn rows_for(dir: &LevelingDir) -> Vec<(String, String)> {
    match dir.level() {
        LevelingLevel::Segments => (0..SEGMENTS)
            .map(|seg| {
                let min = segment_min(seg);
                let left = format!("{}-{} 级", min, min + 9);
                let right = match dir.segment_summary(seg) {
                    Some(s) => format!("{} 个点 · {} 图", s.entry_count, s.map_count),
                    None => "—".to_string(),
                };
                (left, right)
            })
            .collect(),
        LevelingLevel::Entries { seg } => dir
            .entries_of(*seg)
            .iter()
            .map(|e| (monsters_text(e), format!("{} 处", e.spots.len())))
            .collect(),
        LevelingLevel::Spots { seg, entry } => dir
            .entries_of(*seg)
            .get(*entry)
            .map(|e| {
                e.spots
                    .iter()
                    .map(|s| {
                        let left = format!("{} ({},{}) 区域{}", s.map_name, s.anchor.x, s.anchor.y, s.number);
                        (left, format!("同场{} · 约{}格", s.max_enemies, s.steps))
                    })
                    .collect()
            })
            .unwrap_or_default(),
        LevelingLevel::Actions { .. } => ACTIONS.iter().map(|a| (a.to_string(), String::new())).collect(),
    }
}

/// 练级点的怪物：最多 3 种，多余写“等 N 种”。
fn monsters_text(e: &LevelingEntry) -> String {
    let mut parts: Vec<String> = e
        .monsters
        .iter()
        .take(3)
        .map(|m| format!("{} {}-{}", m.name, m.level_min, m.level_max))
        .collect();
    if e.monsters.len() > 3 {
        parts.push(format!("等 {} 种", e.monsters.len()));
    }
    parts.join("、")
}

/// 右侧详情：选中练级点的怪物构成和全部落点（前 10 个）。
fn detail_lines(dir: &LevelingDir) -> Vec<Line<'static>> {
    let Some(entry) = dir.selected_entry() else {
        let text = match dir.level() {
            LevelingLevel::Segments => "按 10 级一段列出全地图的野外遇敌点，选中段后看它的练级点。",
            _ => "没有可以显示的详情。",
        };
        return vec![Line::from(text)];
    };
    let mut out = vec![Line::from(Span::styled("怪物", HEAD))];
    for m in &entry.monsters {
        out.push(Line::from(format!("  {} {}-{} 级", m.name, m.level_min, m.level_max)));
    }
    out.push(Line::from(""));
    out.push(Line::from(Span::styled(
        format!("落点（{} 处）", entry.spots.len()),
        HEAD,
    )));
    for s in entry.spots.iter().take(10) {
        out.push(Line::from(format!(
            "  {} ({},{}) 区域{} 同场{} 约{}格一场",
            s.map_name, s.anchor.x, s.anchor.y, s.number, s.max_enemies, s.steps
        )));
    }
    if entry.spots.len() > 10 {
        out.push(Line::from(format!("  …还有 {} 处", entry.spots.len() - 10)));
    }
    out
}

/// 按显示宽度截断（中文占两列），超出写省略号。
fn fit(text: &str, width: usize) -> String {
    if text.width() <= width {
        return text.to_string();
    }
    let mut out = String::new();
    let mut w = 0;
    for c in text.chars() {
        let cw = unicode_width::UnicodeWidthChar::width(c).unwrap_or(0);
        if w + cw + 1 > width {
            break;
        }
        out.push(c);
        w += cw;
    }
    out.push('…');
    out
}
