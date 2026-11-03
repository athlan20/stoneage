//! 主界面第 8 页「百科」（STAGE-83，`docs/ui.md` 百科页）：左边是分类目录，中间是当前分类
//! （或搜索结果）的词条列表，打开词条后右边显示详情。窄屏（< 110 列）时详情替换列表，
//! `wiki back` 回到列表。操作全靠底部的 `wiki` 命令，PgUp/PgDn 翻页或滚动详情。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph, Row, Table};

use crate::app::{App, SIDE_BY_SIDE, page_cursor, wrap_text};
use crate::wiki::{DetailLine, LineStyle, wiki};

use super::widgets::{select_mark, selected_style};

const CATEGORY_WIDTH: u16 = 26;
const HEADER_STYLE: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_mut() else {
        return;
    };
    let category_cursor = page_cursor(world, "分类");
    let list_cursor = page_cursor(world, "词条");
    let w = &mut world.wiki;
    let [cats, rest] = Layout::horizontal([Constraint::Length(CATEGORY_WIDTH), Constraint::Min(0)]).areas(area);
    render_categories(frame, w.category, category_cursor, cats);
    let (list, detail) = match (&w.open, area.width >= SIDE_BY_SIDE) {
        (None, _) => (Some(rest), None),
        (Some(_), true) => {
            let [l, d] = Layout::horizontal([Constraint::Percentage(40), Constraint::Min(0)]).areas(rest);
            (Some(l), Some(d))
        }
        (Some(_), false) => (None, Some(rest)),
    };
    if let Some(area) = list {
        render_list(frame, w, list_cursor, area);
    }
    if let Some(area) = detail {
        render_detail(frame, w, area);
    }
}

/// 分类目录。当前分类黄色加粗；选中项（`▶`）按焦点栏反色（`selected_style`）。
/// 最小窗口里放不下全部分类：滚动到选中项（没有就是当前分类）可见，并在底边提示总数。
fn render_categories(frame: &mut Frame, current: Option<usize>, cursor: Option<(usize, bool)>, area: Rect) {
    let cats = wiki().categories();
    let rows_fit = usize::from(area.height.saturating_sub(2)).max(1);
    let anchor = cursor.map(|(sel, _)| sel).or(current);
    let skip = anchor.map_or(0, |c| (c + 1).saturating_sub(rows_fit));
    let mut rows: Vec<Row> = cats
        .iter()
        .enumerate()
        .map(|(i, c)| {
            let selected = matches!(cursor, Some((sel, _)) if sel == i);
            let focused = matches!(cursor, Some((sel, true)) if sel == i);
            let mut style = if current == Some(i) {
                Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
            } else {
                Style::default()
            };
            if selected {
                style = style.patch(selected_style(focused));
            }
            Row::new(vec![format!("{}{}", select_mark(selected), i + 1), c.name.clone(), c.count.to_string()])
                .style(style)
        })
        .collect();
    let search_selected = matches!(cursor, Some((i, _)) if i == cats.len());
    rows.push(Row::new(vec![">".to_string(), "搜索百科…".to_string(), "".to_string()]).style(if search_selected { selected_style(true) } else { Style::default() }));
    let rows: Vec<Row> = rows.into_iter().skip(skip).take(rows_fit).collect();
    let widths = [Constraint::Length(2), Constraint::Min(8), Constraint::Length(5)];
    let mut block = Block::bordered().title("分类");
    if cats.len() > rows_fit {
        block = block.title_bottom(format!(" 共 {} 类 ", cats.len()));
    }
    frame.render_widget(Table::new(rows, widths).block(block).column_spacing(1), area);
}

/// 一行的摘要：等级、HP、位置依次拼起来；都没有就用说明。
fn summary(r: &crate::wiki::Row) -> String {
    let mut parts = vec![];
    if !r.level.is_empty() {
        parts.push(format!("Lv{}", r.level));
    }
    if !r.hp.is_empty() {
        parts.push(format!("HP{}", r.hp));
    }
    if !r.location.is_empty() {
        parts.push(r.location.clone());
    }
    if parts.is_empty() { r.description.clone() } else { parts.join(" ") }
}

fn render_list(frame: &mut Frame, w: &mut crate::app::WikiState, cursor: Option<(usize, bool)>, area: Rect) {
    let cats = wiki().categories();
    let title = match (w.category, w.query.is_empty()) {
        (None, true) => "百科".to_string(),
        (None, false) => format!("搜索“{}”· 全部分类 · 共 {} 条", w.query, w.hits.len()),
        (Some(c), true) => format!("{} · 共 {} 条", cats[c].name, w.hits.len()),
        (Some(c), false) => format!("{} · 搜索“{}”· 共 {} 条", cats[c].name, w.query, w.hits.len()),
    };
    let block = Block::bordered().title(title).title_bottom(" Tab 搜索 · Enter 查看 · Esc 返回 ");
    let inner = block.inner(area);
    if w.category.is_none() && w.query.is_empty() {
        let help = vec![
            Line::raw(" 石器百科：宠物、装备、物品、地图、任务、NPC 等游戏资料。"),
            Line::raw(""),
            Line::raw(" wiki 分类        按左边的编号或名字进入分类，如 wiki 2、wiki 宠物"),
            Line::raw(" wiki find 关键词  搜索（在分类里就搜这个分类，否则搜全部）"),
            Line::raw(" wiki open 编号    看列表里该编号的词条详情"),
            Line::raw(" wiki next / prev  列表翻页（也可以用 PgDn / PgUp）"),
            Line::raw(" wiki back         退回上一层"),
        ];
        frame.render_widget(Paragraph::new(help).block(block), area);
        return;
    }
    // 表头占 1 行；每页条数 = 可见行数，翻页按它算。选中项移到哪一页，就显示哪一页（PgUp/PgDn 和 Home/End 靠它翻页）。
    w.list_rows = usize::from(inner.height.saturating_sub(1)).max(1);
    if let Some((sel, _)) = cursor {
        w.page = sel / w.list_rows;
    }
    let start = w.page_start();
    let show_cat = w.category.is_none();
    let open_hit = w.open.as_ref().map(|o| o.hit);
    let narrow = area.width < 60;
    let rows: Vec<Row> = w.hits.iter().enumerate().skip(start).take(w.list_rows).map(|(i, h)| {
        let r = wiki().row(*h);
        let selected = matches!(cursor, Some((sel, _)) if sel == i);
        let focused = matches!(cursor, Some((sel, true)) if sel == i);
        let mut cells = vec![format!("{}{}", select_mark(selected), i + 1)];
        if show_cat {
            cells.push(cats[h.category].name.clone());
        }
        cells.push(r.name.clone());
        if !narrow {
            cells.push(summary(r));
        }
        let mut style = if open_hit == Some(*h) { Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD) } else { Style::default() };
        if selected {
            style = style.patch(selected_style(focused));
        }
        Row::new(cells).style(style)
    }).collect();
    let mut header = vec!["编号"];
    let mut widths = vec![Constraint::Length(5)];
    if show_cat {
        header.push("分类");
        widths.push(Constraint::Length(12));
    }
    header.push("名称");
    widths.push(Constraint::Length(if narrow { 18 } else { 16 }));
    if !narrow {
        header.push("摘要");
        widths.push(Constraint::Min(8));
    }
    let table = Table::new(rows, widths).header(Row::new(header).style(HEADER_STYLE)).block(block.title_bottom(
        Line::from(format!(" 第 {}/{} 页 ", w.page.min(w.pages() - 1) + 1, w.pages())).right_aligned(),
    ));
    frame.render_widget(table, area);
    if w.hits.is_empty() {
        let hint = Rect::new(inner.x, inner.y + 2.min(inner.height.saturating_sub(1)), inner.width, 1);
        frame.render_widget(Paragraph::new("  没有符合的词条").style(Style::default().fg(Color::DarkGray)), hint);
    }
}

fn line_style(s: LineStyle) -> Style {
    match s {
        LineStyle::Title => Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
        LineStyle::Section => Style::default().fg(Color::Magenta).add_modifier(Modifier::BOLD),
        LineStyle::Header => HEADER_STYLE,
        LineStyle::Body => Style::default(),
        LineStyle::Dim => Style::default().fg(Color::DarkGray),
    }
}

fn render_detail(frame: &mut Frame, w: &mut crate::app::WikiState, area: Rect) {
    let Some(open) = w.open.as_mut() else { return };
    let title = format!("词条 · {}", wiki().row(open.hit).name);
    render_detail_panel(frame, area, title, &open.lines, &mut open.scroll, &mut open.view_rows);
}

/// 词条和任务攻略共用的详情面板（STAGE-89）：标题加青色粗体，正文按宽度换行，超出一屏时可以 PgUp/PgDn 滚动。
/// `scroll` 是顶部显示行，渲染时收口到最后一屏；`view_rows` 记下可见行数，翻页步进用。
pub fn render_detail_panel(frame: &mut Frame, area: Rect, title: String, lines: &[DetailLine], scroll: &mut usize, view_rows: &mut usize) {
    let block = Block::bordered().title(Span::styled(title, Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)));
    let inner = block.inner(area);
    let width = usize::from(inner.width).max(1);
    let rows = usize::from(inner.height).max(1);
    // 先按宽度换行成显示行，再按滚动位置截取；滚动位置收口到最后一屏。
    let wrapped: Vec<(LineStyle, &str)> = lines.iter().flat_map(|l| wrap_text(&l.text, width).into_iter().map(move |t| (l.style, t))).collect();
    *view_rows = rows;
    *scroll = (*scroll).min(wrapped.len().saturating_sub(rows));
    let shown: Vec<Line> = wrapped.iter().skip(*scroll).take(rows).map(|(s, t)| Line::styled(t.to_string(), line_style(*s))).collect();
    let mut block = block;
    if wrapped.len() > rows {
        let (a, b) = (*scroll + 1, (*scroll + rows).min(wrapped.len()));
        block = block.title_bottom(Line::from(format!(" PgUp/PgDn 滚动 · 第 {a}–{b}/{} 行 ", wrapped.len())).right_aligned());
    }
    frame.render_widget(Paragraph::new(shown).block(block), area);
}
