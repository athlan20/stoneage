//! 抓宠丢弃记录页（STAGE-264 第 5 条）：设置页“捕捉”栏末尾进入，只读列表，Esc 返回。
//! 第一行是「清空记录」（要确认，默认取消），之后每条三行：时间与名字、四项资质、原因；最新在上。
//! 每行先按显示宽度截断，80×24 下不会错行；列表超出时跟着光标滚动。

use ratatui::{
    Frame,
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::Paragraph,
};
use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

use crate::app::auto_battle::capture_log::{self, CaptureRecord};
use crate::app::auto_battle::capture_log_view::LogView;

pub fn render(frame: &mut Frame, log: &LogView, area: Rect) {
    let [title_area, list_area, note_area, hint_area] = Layout::vertical([
        Constraint::Length(1),
        Constraint::Min(3),
        Constraint::Length(2),
        Constraint::Length(1),
    ])
    .areas(area);
    let width = area.width as usize;

    let title = format!(
        "抓宠丢弃记录 · {} · 最近 {} 条",
        log.character,
        capture_log::MAX_RECORDS
    );
    let title_style = Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD);
    frame.render_widget(
        Paragraph::new(Span::styled(fit(&title, width), title_style)),
        title_area,
    );

    // 清空行 1 行，每条记录 3 行；光标所在的行要留在视野里。
    let mut lines = vec![clear_line(log, width)];
    for row in 1..log.rows() {
        if let Some(rec) = log.record_at(row) {
            lines.extend(record_lines(rec, row == log.sel, width));
        }
    }
    let (start, len) = if log.sel == 0 {
        (0, 1)
    } else {
        (1 + (log.sel - 1) * 3, 3)
    };
    let h = list_area.height as usize;
    let scroll = (start + len).saturating_sub(h);
    frame.render_widget(Paragraph::new(lines).scroll((scroll as u16, 0)), list_area);

    let (note, tone) = if log.ask {
        let bar = if log.ask_choice == 0 {
            "[取消]  清空"
        } else {
            " 取消  [清空]"
        };
        (
            format!(
                "清空 {} 条记录？ {bar}（Enter 执行 · ←→ 选择 · Esc 取消）",
                log.records.len()
            ),
            Color::Yellow,
        )
    } else {
        (
            log.hint
                .clone()
                .unwrap_or_else(|| "记录只读；选第一行「清空记录」可清空（要确认）".into()),
            Color::Gray,
        )
    };
    let tip = "时间为 UTC；每个角色最多 50 条，超出丢最旧的";
    let note_lines = vec![
        Line::from(Span::styled(fit(&note, width), Style::default().fg(tone))),
        Line::from(Span::styled(fit(tip, width), Style::default().fg(Color::DarkGray))),
    ];
    frame.render_widget(Paragraph::new(note_lines), note_area);

    let keys = "↑↓ 选择 · Enter 执行 · Esc 返回设置页";
    frame.render_widget(
        Paragraph::new(fit(keys, width)).style(Style::default().fg(Color::DarkGray)),
        hint_area,
    );
}

fn clear_line(log: &LogView, width: usize) -> Line<'static> {
    let head = if log.sel == 0 { "› " } else { "  " };
    let text = if log.records.is_empty() {
        "清空记录（现在没有记录）"
    } else {
        "清空记录"
    };
    let style = if log.sel == 0 {
        Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
    } else if log.records.is_empty() {
        Style::default().fg(Color::DarkGray)
    } else {
        Style::default()
    };
    Line::from(Span::styled(fit(&format!("{head}{text}"), width), style))
}

/// 一条记录的三行：`› 10-21 04:12 乌力 Lv1`、`    体力 21(−1) …`、`    原因：体力 −1 < 0`。
fn record_lines(rec: &CaptureRecord, selected: bool, width: usize) -> Vec<Line<'static>> {
    let head = if selected { "› " } else { "  " };
    let when = rec.at.get(5..16).unwrap_or(&rec.at);
    let first = format!("{head}{when} {} Lv{}", rec.name, rec.level);
    let first_style = if selected {
        Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
    } else {
        Style::default()
    };
    let quality = format!("    {}", capture_log::quality_text(&rec.quality));
    let reason = format!("    原因：{}", rec.reason);
    vec![
        Line::from(Span::styled(fit(&first, width), first_style)),
        Line::from(Span::styled(fit(&quality, width), Style::default().fg(Color::Gray))),
        Line::from(Span::styled(fit(&reason, width), Style::default().fg(Color::Gray))),
    ]
}

/// 按显示宽度截到 `width` 列，超出的末尾换成 `…`（中文算两列）。
pub fn fit(text: &str, width: usize) -> String {
    if UnicodeWidthStr::width(text) <= width {
        return text.to_string();
    }
    let mut out = String::new();
    let mut used = 0;
    for c in text.chars() {
        let w = UnicodeWidthChar::width(c).unwrap_or(0);
        if used + w + 1 > width {
            break;
        }
        out.push(c);
        used += w;
    }
    out.push('…');
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn short_text_is_left_alone() {
        assert_eq!(fit("抓宠", 10), "抓宠");
    }

    #[test]
    fn long_text_is_cut_by_columns_not_chars() {
        let s = fit("体力 21(−1) 腕力 10(−4) 耐久力 15(−2) 速度 27(0) 原因很长很长", 20);
        assert!(UnicodeWidthStr::width(s.as_str()) <= 20, "{s}");
        assert!(s.ends_with('…'));
        assert_eq!(fit("abcdef", 4), "abc…");
    }
}
