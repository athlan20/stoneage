//! 主界面第 2 页「角色」（`docs/ui.md` 角色页）：上面三栏——左边基本信息，中间能力与衍生数值，
//! 右边地水火风；底部是精灵（魔法）清单（STAGE-25）。中间区域高度不足时三栏压缩，
//! 精灵清单保持完整。

use std::time::{SystemTime, UNIX_EPOCH};

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::symbols;
use ratatui::text::{Line, Span};
use ratatui::widgets::{Bar, BarChart, Block, LineGauge, Paragraph, Row, Table};

use crate::app::char_select::{ABILITIES, ABILITY_COLUMN, MAGIC_COLUMN};
use crate::app::{App, CharStep, WorldState, page_cursor};
use crate::proto::{ElementPoints, Status};

use super::widgets::{select_mark, selected_style};

const INFO_WIDTH: u16 = 30;
const ABILITY_WIDTH: u16 = 28;
/// 属性点最高 10，柱状图的满格就是 10。
const ELEMENT_MAX: u64 = 10;
/// 精灵栏位数（`docs/protocol.md` 5.8 的 `status.magic`，栏位 0–4）。
pub const MAGIC_SLOTS: usize = 5;
/// 精灵清单的高度：5 个栏位加边框。
const MAGIC_HEIGHT: u16 = 7;
/// 三栏至少要有 3 行（上下边框加标题行）才画得出标题。
const PANELS_MIN_HEIGHT: u16 = 3;
/// 能力表至少要有表头和一行数据，加上边框是 4 行；矮于这个高度时能力栏不能选（STAGE-116）。
const ABILITY_MIN_HEIGHT: u16 = 4;

/// 能力表画得出来吗（中间区域的内容高度）：精灵清单固定占 `MAGIC_HEIGHT` 行，剩下的三栏要放得下表头和一行。
/// 80×24 时三栏只剩边框，能力栏不能选（`docs/ui.md`「角色页」的页面选择）。
pub fn ability_fits(inner_height: u16) -> bool {
    inner_height.saturating_sub(MAGIC_HEIGHT) >= ABILITY_MIN_HEIGHT
}

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    // 加点表单开着时，底部留出面板（STAGE-133）。
    let panel = match app.world.as_ref().and_then(|w| w.char_step.as_ref()) {
        Some(_) => 3,
        None => 0,
    };
    let [body, panel_area] = Layout::vertical([Constraint::Min(0), Constraint::Length(panel)]).areas(area);
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let [panels, magic] =
        Layout::vertical([Constraint::Min(PANELS_MIN_HEIGHT), Constraint::Length(MAGIC_HEIGHT)]).areas(body);
    let [info, ability, elements] =
        Layout::horizontal([Constraint::Length(INFO_WIDTH), Constraint::Length(ABILITY_WIDTH), Constraint::Min(0)]).areas(panels);
    render_info(frame, app.unicode, &world.entry.character.name, &world.entry.status, world.entry.duel_point, info);
    render_ability(frame, &world.entry.status, ability, page_cursor(world, ABILITY_COLUMN));
    render_elements(frame, app.unicode, &world.entry.status, elements);
    render_magic(frame, &world.entry.status, magic, page_cursor(world, MAGIC_COLUMN));
    // 加点表单（STAGE-133）和精灵动作（STAGE-231）共用底部那一行。
    match world.char_step.as_ref() {
        Some(CharStep::AddPoint { .. }) => render_addpoint(frame, world, panel_area),
        Some(_) => super::charpage_magic::render_step(frame, world, panel_area),
        None => {}
    }
}

/// 加点表单（STAGE-133）：显示能力名、已填点数和还没分配的剩余点数。
fn render_addpoint(frame: &mut Frame, world: &WorldState, area: Rect) {
    let Some(CharStep::AddPoint { stat, text }) = world.char_step.as_ref() else {
        return;
    };
    let left = world.entry.status.skill_points;
    let label = Style::default().fg(Color::DarkGray);
    let line = Line::from(vec![
        Span::raw(format!("{}  ", ABILITIES[*stat])),
        Span::styled(format!("{}_", text.text()), Style::default().fg(Color::Yellow)),
        Span::styled(format!("    1–{left} · Enter 确认 · Esc 取消"), label),
    ]);
    frame.render_widget(Paragraph::new(line).block(Block::bordered().title("加点")), area);
}

/// 基本信息：名字、称号、等级、转生、石币、决斗点数（STAGE-307），再加 HP、MP、经验三条进度。变身中在称号下面多一行（STAGE-203）。
fn render_info(frame: &mut Frame, unicode: bool, name: &str, s: &Status, duel_point: i64, area: Rect) {
    let block = Block::bordered().title("基本信息");
    let inner = block.inner(area);
    frame.render_widget(block, area);
    // 没有变身时不占行，下面的行号不变；变身时三条进度和等级、石币都往下挪一行。
    let off = usize::from(s.metamo.is_some());
    let rows = Layout::vertical(vec![Constraint::Length(1); 8 + off]).split(inner);

    let label = Style::default().fg(Color::DarkGray);
    let row = |k: &str, v: String| Line::from(vec![Span::styled(format!(" {k}  "), label), Span::raw(v)]);
    let title = if s.title.is_empty() { "无".to_string() } else { s.title.clone() };
    frame.render_widget(Paragraph::new(row("名字", name.to_string())), rows[0]);
    frame.render_widget(Paragraph::new(row("称号", title)), rows[1]);
    if let Some(m) = &s.metamo {
        let left = metamo_left_secs(m.until, now_ms());
        frame.render_widget(Paragraph::new(format!(" 变身：{}（剩 {} 秒）", m.pet, left)), rows[2]);
    }
    // 内宽 28 列：前缀 7 列，数值按骑乘 200 的宽度留位，整行正好放下（STAGE-154）。
    frame.render_widget(Paragraph::new(row("等级", format!("{:<4}转生 {:<4}骑乘 {}", s.level, s.transmigration, s.ride_level))), rows[2 + off]);
    frame.render_widget(Paragraph::new(row("石币", s.gold.to_string())), rows[3 + off]);
    frame.render_widget(Paragraph::new(row("决斗点数", duel_point.to_string())), rows[4 + off]);

    let exp_ratio = if s.next_exp == 0 { 0.0 } else { s.exp as f64 / s.next_exp as f64 };
    gauge_row(frame, rows[5 + off], unicode, "HP", ratio(s.hp, s.max_hp), &format!("{}/{}", s.hp, s.max_hp), Color::Green);
    gauge_row(frame, rows[6 + off], unicode, "MP", ratio(s.mp, s.max_mp), &format!("{}/{}", s.mp, s.max_mp), Color::Blue);
    gauge_row(frame, rows[7 + off], unicode, "经验", exp_ratio, &format!("{}/{}", s.exp, s.next_exp), Color::Yellow);
}

/// 变身还剩几秒（向上取整，到期或已过返回 0）。`until_ms` 是服务端的到期毫秒时间戳，`now_ms` 是本地时钟；
/// 角色页每个 tick 重算，不存死数字（STAGE-203，`docs/protocol.md` 5.8 的 `status.metamo`）。
pub fn metamo_left_secs(until_ms: i64, now_ms: i64) -> i64 {
    let left = until_ms - now_ms;
    if left <= 0 { 0 } else { (left + 999) / 1000 }
}

/// 本地时钟的毫秒时间戳（和服务端的 `until` 同一个时间基准）。
fn now_ms() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).map_or(0, |d| i64::try_from(d.as_millis()).unwrap_or(i64::MAX))
}

#[cfg(test)]
mod tests {
    use super::metamo_left_secs;

    #[test]
    fn metamo_left_rounds_up_and_stops_at_zero() {
        assert_eq!(metamo_left_secs(60_000, 0), 60);
        assert_eq!(metamo_left_secs(60_001, 0), 61, "不足一秒也算一秒");
        assert_eq!(metamo_left_secs(500, 0), 1);
        assert_eq!(metamo_left_secs(1_000, 1_000), 0, "正好到期");
        assert_eq!(metamo_left_secs(1_000, 5_000), 0, "已经过了");
    }
}

pub(super) fn ratio(cur: u32, max: u32) -> f64 {
    if max == 0 { 0.0 } else { f64::from(cur) / f64::from(max) }
}

/// 一行进度：标签、LineGauge、数值。比例会夹在 0–1 之间（LineGauge 对越界的比例会直接 panic）。
pub(super) fn gauge_row(frame: &mut Frame, area: Rect, unicode: bool, name: &str, ratio: f64, value: &str, color: Color) {
    let [l, g, v] = Layout::horizontal([
        Constraint::Length(5),
        Constraint::Min(4),
        Constraint::Length(value.len() as u16 + 1),
    ])
    .areas(area);
    frame.render_widget(Paragraph::new(Span::styled(format!(" {name}"), Style::default().fg(Color::DarkGray))), l);
    let sym = if unicode { symbols::line::THICK_HORIZONTAL } else { symbols::line::HORIZONTAL };
    let gauge = LineGauge::default()
        .filled_style(Style::default().fg(color))
        .unfilled_style(Style::default().fg(Color::DarkGray))
        .filled_symbol(sym)
        .unfilled_symbol(sym)
        .label("")
        .ratio(ratio.clamp(0.0, 1.0));
    frame.render_widget(gauge, g);
    frame.render_widget(Paragraph::new(format!(" {value}")), v);
}

/// 能力：左列是点数，右列是衍生数值（HP、攻击、防御、敏捷、魅力、运气）。
/// 能力栏的四项（体力到速度）可以选：选中行行首 ▶，焦点栏反色（STAGE-116）。
fn render_ability(frame: &mut Frame, s: &Status, area: Rect, cursor: Option<(usize, bool)>) {
    let header = Row::new([" 能力", "点数", "衍生", "数值"])
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD));
    let d = &s.derived;
    let st = &s.stats;
    let rows = [
        ability_row(0, cursor, ["体力".into(), st.vital.to_string(), "HP".into(), format!("{}/{}", s.hp, s.max_hp)]),
        ability_row(1, cursor, ["腕力".into(), st.strength.to_string(), "攻击".into(), d.attack.to_string()]),
        ability_row(2, cursor, ["耐久力".into(), st.toughness.to_string(), "防御".into(), d.defense.to_string()]),
        ability_row(3, cursor, ["速度".into(), st.dexterity.to_string(), "敏捷".into(), d.agility.to_string()]),
        Row::new([" 未分配".to_string(), s.skill_points.to_string(), "魅力".into(), d.charm.to_string()]),
        Row::new([" ".to_string(), "".into(), "运气".into(), d.luck.to_string()]),
    ];
    let widths = [Constraint::Length(7), Constraint::Length(4), Constraint::Length(4), Constraint::Min(0)];
    let table = Table::new(rows, widths).header(header).block(Block::bordered().title("能力"));
    frame.render_widget(table, area);
}

/// 地水火风：横向柱状图，满格是 `max`（角色的属性点是 0–10，宠物的模板值是 0–100）。
/// unicode 开启用九级块，关闭退回三级块（`docs/ui.md`）。调用方自己决定要不要边框。
pub(super) fn element_chart(unicode: bool, e: &ElementPoints, max: u64) -> BarChart<'static> {
    let bars = [
        ("地", e.earth, Color::Yellow),
        ("水", e.water, Color::Cyan),
        ("火", e.fire, Color::Red),
        ("风", e.wind, Color::Green),
    ]
    .map(|(name, value, color)| Bar::with_label(name, u64::from(value)).style(Style::default().fg(color)));
    let bar_set = if unicode { symbols::bar::NINE_LEVELS } else { symbols::bar::THREE_LEVELS };
    BarChart::horizontal(bars).max(max).bar_gap(0).bar_set(bar_set)
}

fn render_elements(frame: &mut Frame, unicode: bool, s: &Status, area: Rect) {
    let chart = element_chart(unicode, &s.elements, ELEMENT_MAX).block(Block::bordered().title("地水火风"));
    frame.render_widget(chart, area);
}

/// 一行能力：选中的那一行行首 ▶，焦点栏反色、其他栏只加粗（STAGE-116）；`index` 是 0–3 的能力下标。
fn ability_row(index: usize, cursor: Option<(usize, bool)>, cells: [String; 4]) -> Row<'static> {
    let on = cursor.filter(|(i, _)| *i == index);
    let [name, point, label, value] = cells;
    let row = Row::new([format!("{}{name}", select_mark(on.is_some())), point, label, value]);
    match on {
        Some((_, focused)) => row.style(selected_style(focused)),
        None => row,
    }
}

/// 精灵（魔法）清单（STAGE-25，`docs/ui.md` 角色页 C）：5 个栏位（0–4）各一行，空栏位写 —，
/// 每项显示名字、所需 MP 和目标类型。编号与战斗里 `magic` 指令的精灵编号一致（`docs/battle-ui.md`「命令表」）。
/// 精灵栏是焦点栏时，选中的精灵效果写在底边（STAGE-116，`docs/ui.md`「页面选择」2 角色）。
fn render_magic(frame: &mut Frame, s: &Status, area: Rect, cursor: Option<(usize, bool)>) {
    let dim = Style::default().fg(Color::DarkGray);
    let rows = (0..MAGIC_SLOTS).map(|slot| {
        let on = cursor.filter(|(index, _)| *index == slot);
        let mark = select_mark(on.is_some());
        let row = match s.magic.iter().find(|m| usize::from(m.slot) == slot) {
            Some(m) => Row::new([
                format!("{mark}{slot}"),
                m.name.clone(),
                format!("MP {}", m.mp),
                m.target_type.text().to_string(),
            ]),
            None => Row::new([format!("{mark}{slot}"), "—".to_string(), String::new(), String::new()]).style(dim),
        };
        match on {
            Some((_, focused)) => row.style(selected_style(focused)),
            None => row,
        }
    });
    let effect = cursor
        .filter(|(_, focused)| *focused)
        .and_then(|(slot, _)| s.magic.iter().find(|m| usize::from(m.slot) == slot))
        .map(|m| m.effect.clone())
        .filter(|effect| !effect.is_empty());
    let mut block = Block::bordered().title("精灵（魔法）");
    if let Some(effect) = effect {
        block = block.title_bottom(effect);
    }
    let widths = [Constraint::Length(3), Constraint::Min(0), Constraint::Length(7), Constraint::Length(12)];
    let table = Table::new(rows, widths).block(block);
    frame.render_widget(table, area);
}
