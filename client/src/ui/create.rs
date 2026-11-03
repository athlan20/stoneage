//! 创建角色：外形、名字与属性、出生地三步。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::symbols;
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, LineGauge, List, ListItem, ListState, Paragraph, Tabs};

use super::widgets::{focus_style, pad, render_messages};
use super::worldmap;
use crate::app::{App, CreateFocus, CreateState, CreateStep};
use crate::proto::{BODY_NAMES, Elements, HOMETOWNS, STAT_POINTS, body_name, color_name};

const STEP_TITLES: [&str; 3] = ["1外形", "2名字与属性", "3出生地"];

fn bold_title(text: &str) -> Span<'static> {
    Span::styled(text.to_string(), Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD))
}

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let [tabs, body, help, msgs] = Layout::vertical([
        Constraint::Length(3),
        Constraint::Min(10),
        Constraint::Length(1),
        Constraint::Length(6),
    ])
    .areas(area);
    let titles: Vec<Line> = STEP_TITLES.iter().map(|t| Line::from(Span::styled(*t, Style::default().fg(Color::Green)))).collect();
    let tabs_w = Tabs::new(titles)
        .block(Block::bordered().title(bold_title(&format!("创建角色 · 栏位 {}", app.create.slot + 1))))
        .highlight_style(Style::default().fg(Color::Yellow))
        .select(app.create.step.index());
    frame.render_widget(tabs_w, tabs);
    let hint = match app.create.step {
        CreateStep::Look => "  ↑↓ 选择项目   ←→ 修改   Enter 下一步   Esc 返回",
        CreateStep::Stats => "  ↑↓ 选择项目   ←→ 减少/增加   Enter 下一步   Esc 上一步",
        CreateStep::Home => "  ←→ 选择出生地   Enter 创建角色   Esc 上一步",
    };
    frame.render_widget(Paragraph::new(hint), help);
    match app.create.step {
        CreateStep::Look => render_look(frame, &app.create, body),
        CreateStep::Stats => render_stats(frame, app, body),
        CreateStep::Home => render_home(frame, app, body),
    }
    render_messages(frame, app, msgs);
}

fn selector(label: &str, value: String, focused: bool) -> Line<'static> {
    let style = focus_style(focused);
    Line::from(vec![
        Span::styled(if focused { "> " } else { "  " }, style),
        Span::styled(pad(label, 8), style),
        Span::styled(format!("◂ {value} ▸"), style),
    ])
}

fn render_look(frame: &mut Frame, c: &CreateState, area: Rect) {
    let [left, right] = Layout::horizontal([Constraint::Length(22), Constraint::Min(30)]).areas(area);
    let items: Vec<ListItem> = BODY_NAMES.iter().enumerate().map(|(i, n)| ListItem::new(format!("{:>2} {n}", i + 1))).collect();
    // 左侧外形列不画箭头，选中的外形只用颜色高亮。
    let list = List::new(items)
        .block(Block::bordered().title("外形"))
        .highlight_style(Style::default().add_modifier(Modifier::BOLD).fg(Color::Yellow));
    frame.render_stateful_widget(list, left, &mut ListState::default().with_selected(Some(usize::from(c.body))));

    let f = c.look_focus;
    let lines = vec![
        Line::raw(""),
        selector("外形", body_name(c.body).to_string(), f == CreateFocus::Body),
        selector("颜色", color_name(c.body, c.color).to_string(), f == CreateFocus::Color),
    ];
    frame.render_widget(Paragraph::new(lines).block(Block::bordered().title("外形与颜色")), right);
}

/// 一行进度：标签、LineGauge、数值、备注。
fn gauge_row(frame: &mut Frame, area: Rect, unicode: bool, label: &str, value: u8, max: u8, focused: bool, note: &str) {
    let [l, g, v, n] = Layout::horizontal([
        Constraint::Length(9),
        Constraint::Min(8),
        Constraint::Length(5),
        Constraint::Length(14),
    ])
    .areas(area);
    let style = focus_style(focused);
    frame.render_widget(Paragraph::new(Span::styled(format!("{}{}", if focused { "> " } else { "  " }, pad(label, 6)), style)), l);
    let sym = if unicode { symbols::line::THICK_HORIZONTAL } else { symbols::line::HORIZONTAL };
    let color = if note.is_empty() { Color::Magenta } else { Color::DarkGray };
    let gauge = LineGauge::default()
        .filled_style(Style::default().fg(color))
        .unfilled_style(Style::default().fg(Color::DarkGray))
        .filled_symbol(sym)
        .unfilled_symbol(sym)
        .label("")
        .ratio(f64::from(value) / f64::from(max));
    frame.render_widget(gauge, g);
    frame.render_widget(Paragraph::new(format!(" {value:>2}")), v);
    frame.render_widget(Paragraph::new(Span::styled(note.to_string(), Style::default().fg(Color::DarkGray))), n);
}

fn render_stats(frame: &mut Frame, app: &mut App, area: Rect) {
    let c = &app.create;
    let block = Block::bordered().title("名字与属性");
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let rows = Layout::vertical([
        Constraint::Length(1), // 名字
        Constraint::Length(1),
        Constraint::Length(1), // 剩余点数标题
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
    ])
    .split(inner);
    let name_style = focus_style(c.row == 0);
    let name_line = Line::from(vec![
        Span::styled(if c.row == 0 { "> " } else { "  " }, name_style),
        Span::styled(pad("名字", 8), name_style),
        Span::raw(c.name.text().to_string()),
    ]);
    frame.render_widget(Paragraph::new(name_line), rows[0]);
    if c.row == 0 {
        frame.set_cursor_position((rows[0].x + 10 + c.name.cursor_col(None), rows[0].y));
    }

    let halves = |r: Rect| Layout::horizontal([Constraint::Percentage(50), Constraint::Percentage(50)]).areas::<2>(r);
    let [ls, rs] = halves(rows[2]);
    let sleft = c.stats.remaining();
    let eleft = c.elements.remaining();
    frame.render_widget(Paragraph::new(Span::styled(format!("  能力点 剩余 {sleft}"), Style::default().fg(if sleft > 0 { Color::Yellow } else { Color::Green }))), ls);
    frame.render_widget(Paragraph::new(Span::styled(format!("  属性点 剩余 {eleft}"), Style::default().fg(if eleft > 0 { Color::Yellow } else { Color::Green }))), rs);

    let stat_names = ["体力", "腕力", "耐久力", "速度"];
    let stat_vals = [c.stats.vital, c.stats.strength, c.stats.toughness, c.stats.dexterity];
    let elem_names = ["地", "水", "火", "风"];
    let elem_vals = c.elements.as_array();
    for i in 0..4 {
        let [l, r] = halves(rows[3 + i]);
        gauge_row(frame, l, app.unicode, stat_names[i], stat_vals[i], STAT_POINTS, c.row == 1 + i, "");
        let note = if c.elements.blocked(i) {
            format!("(与{}互斥)", elem_names[Elements::opposite(i)])
        } else {
            String::new()
        };
        gauge_row(frame, r, app.unicode, elem_names[i], elem_vals[i], 10, c.row == 5 + i, &note);
    }
}

fn render_home(frame: &mut Frame, app: &mut App, area: Rect) {
    let t = &HOMETOWNS[app.create.hometown];
    let block = Block::bordered().title(bold_title(&format!("出生地 · {}", t.name)));
    let inner = block.inner(area);
    frame.render_widget(block, area);
    worldmap::draw(frame, inner, app.unicode, &worldmap::hometown_marks(Some(app.create.hometown)), None);
}
