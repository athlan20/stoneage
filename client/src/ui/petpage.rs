//! 主界面第 3 页「宠物」（STAGE-31，`docs/ui.md` 宠物页 A；STAGE-114 接入方向键）：左边是 20 个栏位的表（STAGE-346），
//! 右边是选中那只宠物的详情。默认选出战的，没有就第一只；`↑↓` 和 `pet status <编号>` 移动同一个选中项。
//! 左右并排：页面最窄是 80 列，上下叠放时详情放不下（设计取舍，见 docs/ui.md）。
//! STAGE-133：`Enter` 打开对象动作菜单（出战 / 待命 / 休息 / 改名 / 技能 / 放生），流程面板画在下方。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Cell, List, ListItem, ListState, Paragraph, Row, Table, TableState};

use super::petdetail;
use super::petmerge;
use super::widgets::{select_mark, selected_style};
use crate::app::{App, PET_ACTS, PetStep, WorldState, is_merge_skill, pet_disabled_reason};
use crate::proto::PetStatus;

const LIST_WIDTH: u16 = 36;
pub(super) const HEADER_STYLE: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);
pub(super) const LABEL_STYLE: Style = Style::new().fg(Color::DarkGray);

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let unicode = app.unicode;
    // 选材料要看很多行：整页让给材料清单，宠物栏位和详情先收起来（STAGE-208）。
    if app.world.as_ref().is_some_and(|w| matches!(w.pet_step, Some(PetStep::Merge { .. }))) {
        return render_merge_page(frame, app, area);
    }
    // 动作流程开着时，底部留出面板（STAGE-133）。STAGE-249：面板先给宠物表和详情各留 8 行，剩下的才给面板。
    let panel = app
        .world
        .as_ref()
        .map_or(0, |w| pet_panel_height(w, area.height))
        .min(area.height.saturating_sub(6));
    let [body, panel_area] = Layout::vertical([Constraint::Min(0), Constraint::Length(panel)]).areas(area);
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let [list, detail] = Layout::horizontal([Constraint::Length(LIST_WIDTH), Constraint::Min(0)]).areas(body);
    render_list(frame, world, list);
    match world.shown_pet() {
        Some(pet) => petdetail::render(frame, unicode, pet, detail, panel > 0),
        None => render_empty(frame, detail),
    }
    if panel > 0 {
        render_step(frame, app, panel_area);
    }
}

// ---- 对象动作流程的面板（STAGE-133） ----

/// STAGE-249：宠物表（5 栏 + 表头 + 边框）和详情（表头、四项属性、汇总 + 边框）各要的最少行数。
const PET_BODY_MIN: u16 = 8;

/// 动作面板占的高度（含边框）；没有动作流程时是 0。`room` 是中间区域的高度。
fn pet_panel_height(world: &WorldState, room: u16) -> u16 {
    match world.pet_step.as_ref() {
        None => 0,
        Some(PetStep::Actions { .. }) => list_panel_height(PET_ACTS.len() as u16 + 2, room),
        Some(PetStep::Skills { .. }) => list_panel_height(world.pet_step_rows() as u16 + 2, room),
        Some(PetStep::Rename { .. }) => 3,
        Some(PetStep::Merge { .. }) => 0, // 整页渲染，不走底部面板
        Some(PetStep::MergeResult { report, .. }) => petmerge::result_panel_height(report),
    }
}

/// STAGE-249：列表面板的高度。想要 `want` 行（含边框），最多 10 行（8 项 + 边框），最少 4 行（2 行可见 + 边框），
/// 并且给宠物表和详情各留 `PET_BODY_MIN` 行。80×24 下中间区域只有 12 行，面板就是 4 行，可见 2 项，其余靠滚动。
fn list_panel_height(want: u16, room: u16) -> u16 {
    want.clamp(4, 10).min(room.saturating_sub(PET_BODY_MIN))
}

/// 可选行的样式：选中的反色，禁用的灰字。
fn row_style(selected: bool, disabled: bool) -> Style {
    if selected {
        selected_style(true)
    } else if disabled {
        LABEL_STYLE
    } else {
        Style::default()
    }
}

fn render_merge_page(frame: &mut Frame, app: &App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let Some(PetStep::Merge { slot, skill, cursor, picked, .. }) = world.pet_step.as_ref() else {
        return;
    };
    let pet = world.pet_at(*slot);
    let name = pet.as_ref().map(|p| p.name.clone()).unwrap_or_default();
    let skill_name = pet.and_then(|p| p.skills.iter().find(|s| s.slot == *skill).map(|s| s.name.clone())).unwrap_or_default();
    petmerge::render_merge(frame, world, format!("{skill_name}选材料 · {name}"), *cursor, picked, area);
}

fn render_step(frame: &mut Frame, app: &App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let Some(step) = world.pet_step.as_ref() else {
        return;
    };
    let name = world.pet_at(step.slot()).map(|p| p.name).unwrap_or_default();
    let title = step.title(&name);
    match step {
        PetStep::Actions { slot, index } => {
            let pet = world.pet_at(*slot);
            let ride = world.ride_context();
            let rows: Vec<ListItem> = PET_ACTS
                .iter()
                .enumerate()
                .map(|(i, act)| {
                    let reason = pet.as_ref().and_then(|p| pet_disabled_reason(*act, p, ride));
                    let mut spans = vec![Span::raw(act.label_for(pet.as_ref()))];
                    if let Some(reason) = &reason {
                        spans.push(Span::styled(format!("  （{reason}）"), LABEL_STYLE));
                    }
                    ListItem::new(Line::from(spans)).style(row_style(i == *index, reason.is_some()))
                })
                .collect();
            // STAGE-239：80×24 下面板放不下全部动作，带上选中项让列表跟着光标滚动（STAGE-243 起是 8 项，第 8 项「放生」靠滚动选中）。
            let mut state = ListState::default().with_selected(Some(*index));
            frame.render_stateful_widget(List::new(rows).block(Block::bordered().title(title)), area, &mut state);
        }
        PetStep::Skills { slot, index } => {
            let skills = world.pet_at(*slot).map(|p| p.skills).unwrap_or_default();
            let rows: Vec<ListItem> = if skills.is_empty() {
                vec![ListItem::new(Line::from(Span::styled("还没有学会技能", LABEL_STYLE)))]
            } else {
                skills
                    .iter()
                    .enumerate()
                    .map(|(i, s)| {
                        ListItem::new(Line::from(vec![
                            Span::raw(format!("{} {}  ", s.slot, s.name)),
                            Span::styled(
                                if is_merge_skill(s.id) {
                                    "（合成 · Enter 选材料）".to_string()
                                } else {
                                    format!("（{}）", s.target_type.text())
                                },
                                LABEL_STYLE,
                            ),
                        ]))
                        .style(row_style(i == *index, false))
                    })
                    .collect()
            };
            frame.render_widget(List::new(rows).block(Block::bordered().title(title)), area);
        }
        PetStep::Merge { .. } => {} // 整页渲染，见 render_merge_page
        PetStep::MergeResult { report, .. } => petmerge::render_result(frame, title, report, area),
        PetStep::Rename { text, .. } => {
            let line = Line::from(vec![
                Span::raw("名字  "),
                Span::styled(format!("{}_", text.text()), Style::default().fg(Color::Yellow)),
                Span::styled("    留空表示恢复种族名 · Enter 确定 · Esc 取消", LABEL_STYLE),
            ]);
            frame.render_widget(Paragraph::new(line).block(Block::bordered().title(title)), area);
        }
    }
}

/// 状态的文字和颜色：出战绿、待命黄、骑乘青、休息默认色、已倒下红（`docs/ui.md` 宠物页 A）。
pub(super) fn status_label(status: PetStatus) -> (&'static str, Option<Color>) {
    match status {
        PetStatus::Battle => ("出战", Some(Color::Green)),
        PetStatus::Standby => ("待命", Some(Color::Yellow)),
        PetStatus::Ride => ("骑乘", Some(Color::Cyan)),
        PetStatus::Rest => ("休息", None),
        PetStatus::Fainted => ("已倒下", Some(Color::Red)),
    }
}

/// 左边栏位表：按服务端的 `max_slots` 画 20 行，超出一屏时跟着选中项滚动（STAGE-346），空栏位写灰色的 —。
/// 选中的那一行有行首 `▶`，样式同各页列表（`selected_style`）。
fn render_list(frame: &mut Frame, world: &WorldState, area: Rect) {
    let selected = world.selected_pet_slot();
    let header = Row::new(["", "编号", "名字", "Lv", "HP", "状态"]).style(HEADER_STYLE);
    let rows: Vec<Row> = (0..world.pets.max_slots)
        .map(|slot| {
            let is_selected = selected == Some(i64::from(slot));
            let mark = select_mark(is_selected);
            let row = match world.pets.pets.iter().find(|p| p.slot == slot) {
                Some(p) => {
                    let (label, color) = status_label(p.status());
                    let mut status = Cell::from(label);
                    if let Some(color) = color {
                        status = status.style(Style::default().fg(color));
                    }
                    Row::new(vec![
                        Cell::from(mark),
                        Cell::from(slot.to_string()),
                        Cell::from(p.name.clone()),
                        Cell::from(p.level.to_string()),
                        Cell::from(format!("{}/{}", p.hp, p.max_hp)),
                        status,
                    ])
                }
                None => Row::new(vec![
                    Cell::from(mark),
                    Cell::from(slot.to_string()),
                    Cell::from("—"),
                    Cell::from(""),
                    Cell::from(""),
                    Cell::from(""),
                ])
                .style(LABEL_STYLE),
            };
            if is_selected { row.style(selected_style(true)) } else { row }
        })
        .collect();
    let widths = [
        Constraint::Length(1),
        Constraint::Length(4),
        Constraint::Length(8),
        Constraint::Length(3),
        Constraint::Length(7),
        Constraint::Length(4),
    ];
    // STAGE-346：20 个栏位在 80×24 下放不进一屏，带选中项让表格跟着光标滚动。
    let mut state = TableState::default().with_selected(selected.map(|slot| slot as usize));
    frame.render_stateful_widget(
        Table::new(rows, widths).header(header).block(Block::bordered().title("宠物栏位")),
        area,
        &mut state,
    );
}

fn render_empty(frame: &mut Frame, area: Rect) {
    let block = Block::bordered().title("宠物");
    let inner = block.inner(area);
    frame.render_widget(block, area);
    frame.render_widget(Paragraph::new("  还没有宠物。").style(LABEL_STYLE), inner);
}
