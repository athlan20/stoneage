//! 主界面第 4 页「背包」（STAGE-28，`docs/ui.md` 背包页 A；STAGE-114 接入方向键）：左边装备表（5 格），
//! 右边物品表（15 格，高度不够时跟着选中项滚动），空格子也占一行、名称写灰色的 —，
//! 这样编号始终固定；石币写在「物品」面板的标题里。选中项的完整说明写在它所在面板的底边。
//! STAGE-132 起 `Enter` 打开对象动作菜单，动作流程画在下方的面板里（动作 / 目标 / 目的格 / 数量）。

use std::collections::HashMap;

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Cell, List, ListItem, Paragraph, Row, Table};

use super::durability::durability_spans;
use super::inventpage_inlay::render_inlay_pick;
use super::widgets::{select_mark, selected_style};
use crate::app::{
    App, BAG_LAST_SLOT, BagStep, EQUIP_PARTS, FIRST_BAG_SLOT, UseTargets, WorldState, actions, disabled_reason,
    use_targets,
};
use crate::proto::{Inventory, InventorySlot};

const EQUIP_WIDTH: u16 = 40;
const HEADER_STYLE: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);
/// 焦点栏编号：物品栏是 0，装备栏是 1（与 `bag_select::bag_columns` 的顺序一致）。
const BAG_COLUMN: usize = 0;
const EQUIP_COLUMN: usize = 1;
/// 移动目的格的网格：5 列 × 3 行铺满 5–19 共 15 个背包格。
const MOVE_COLS: usize = 5;

/// 槽位 → 物品。装备 5 格和背包 15 格都从这里找。
fn slot_map(inv: &Inventory) -> HashMap<u8, &InventorySlot> {
    inv.slots.iter().map(|s| (s.slot, s)).collect()
}

/// 选中的格子的详情写在面板底边：耐久行（STAGE-225）在前，然后是完整说明（表里的说明列会截断）。
fn detail(it: &InventorySlot) -> Option<Line<'static>> {
    let mut spans = durability_spans(it).unwrap_or_default();
    if !it.effect.is_empty() {
        if !spans.is_empty() {
            spans.push(Span::raw(" · "));
        }
        spans.push(Span::raw(it.effect.clone()));
    }
    if spans.is_empty() {
        return None;
    }
    spans.insert(0, Span::raw(" "));
    spans.push(Span::raw(" "));
    Some(Line::from(spans))
}

fn with_note(block: Block<'static>, note: Option<Line<'static>>) -> Block<'static> {
    match note {
        Some(line) => block.title_bottom(line),
        None => block,
    }
}

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    // 对象动作流程开着时，底部留出动作 / 目标 / 目的格 / 数量面板（STAGE-132）。
    let panel = app.world.as_ref().map_or(0, step_panel_height).min(area.height.saturating_sub(6));
    let [tables, panel_area] = Layout::vertical([Constraint::Min(0), Constraint::Length(panel)]).areas(area);
    render_tables(frame, app, tables);
    if panel > 0 {
        render_step(frame, app, panel_area);
    }
}

/// 上方两张表：装备（固定 5 行）与物品（15 格，高度不够时跟着选中项滚动）。
fn render_tables(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_mut() else {
        return;
    };
    let [equip_area, bag_area] =
        Layout::horizontal([Constraint::Length(EQUIP_WIDTH), Constraint::Min(20)]).areas(area);

    let (focus_column, marks) = world.bag_marks();
    let selected = |column: usize, slot: usize| marks.get(column).copied().flatten() == Some(slot as i64);
    let inv = &world.inventory;
    let slots = slot_map(inv);
    // 焦点栏选中格子的说明，写在它所在的面板底边。
    let note = |column: usize| {
        if column != focus_column {
            return None;
        }
        let key = marks.get(column).copied().flatten()?;
        slots.get(&(key as u8)).and_then(|it| detail(it))
    };

    // 装备：固定 5 行，不滚动。
    let header = Row::new(["", "编号", "部位", "名称", "说明"]).style(HEADER_STYLE);
    let rows: Vec<Row> = (0..5)
        .map(|slot| {
            let mark = select_mark(selected(EQUIP_COLUMN, slot));
            let part = EQUIP_PARTS[slot];
            let row = match slots.get(&(slot as u8)) {
                Some(it) => Row::new(vec![
                    Cell::from(mark),
                    Cell::from(slot.to_string()),
                    Cell::from(part),
                    Cell::from(it.name.clone()),
                    Cell::from(it.effect.clone()),
                ]),
                None => Row::new(vec![
                    Cell::from(mark),
                    Cell::from(slot.to_string()),
                    Cell::from(part),
                    Cell::from("—"),
                    Cell::from(""),
                ])
                .style(Style::default().fg(Color::DarkGray)),
            };
            if selected(EQUIP_COLUMN, slot) { row.style(selected_style(focus_column == EQUIP_COLUMN)) } else { row }
        })
        .collect();
    let widths = [
        Constraint::Length(1),
        Constraint::Length(4),
        Constraint::Length(6),
        Constraint::Length(12),
        Constraint::Min(8),
    ];
    let block = with_note(Block::bordered().title("装备"), note(EQUIP_COLUMN));
    frame.render_widget(Table::new(rows, widths).header(header).block(block), equip_area);

    // 物品：15 行，高度不够时整页滚动（标题写当前范围）。边框尺寸与标题无关，先用它量出可见行数。
    let inner = Block::bordered().inner(bag_area);
    let data_rows = usize::from(inner.height.saturating_sub(1)); // 扣掉表头
    world.bag_rows = data_rows;
    let total = BAG_LAST_SLOT - 4; // 15 个背包格
    let max_top = (5 + total).saturating_sub(data_rows).max(5);
    let mut top = world.bag_scroll.clamp(5, max_top);
    // 选中的背包格不在可见范围内时，表跟着滚到它。
    if let Some(key) = marks.get(BAG_COLUMN).copied().flatten() {
        let slot = key as usize;
        if slot < top {
            top = slot;
        } else if slot >= top + data_rows {
            top = slot + 1 - data_rows;
        }
    }
    let top = top.clamp(5, max_top);
    world.bag_scroll = top;
    let bottom = (top + data_rows - 1).min(BAG_LAST_SLOT);

    let title = if data_rows < total {
        format!("物品 · 石币 {} · {}–{} / {}", inv.gold, top, bottom, BAG_LAST_SLOT)
    } else {
        format!("物品 · 石币 {}", inv.gold)
    };
    let header = Row::new(["", "编号", "名称", "数量", "说明"]).style(HEADER_STYLE);
    let rows: Vec<Row> = (top..=bottom)
        .map(|slot| {
            let mark = select_mark(selected(BAG_COLUMN, slot));
            let row = match slots.get(&(slot as u8)) {
                Some(it) => Row::new(vec![
                    Cell::from(mark),
                    Cell::from(slot.to_string()),
                    Cell::from(it.name.clone()),
                    Cell::from(it.count.to_string()),
                    Cell::from(it.effect.clone()),
                ]),
                None => Row::new(vec![
                    Cell::from(mark),
                    Cell::from(slot.to_string()),
                    Cell::from("—"),
                    Cell::from(""),
                    Cell::from(""),
                ])
                .style(Style::default().fg(Color::DarkGray)),
            };
            if selected(BAG_COLUMN, slot) { row.style(selected_style(focus_column == BAG_COLUMN)) } else { row }
        })
        .collect();
    let widths = [
        Constraint::Length(1),
        Constraint::Length(4),
        Constraint::Min(10),
        Constraint::Length(4),
        Constraint::Min(10),
    ];
    let block = with_note(Block::bordered().title(title), note(BAG_COLUMN));
    frame.render_widget(Table::new(rows, widths).header(header).block(block), bag_area);
}

// ---- 对象动作流程的面板（STAGE-132） ----

/// 动作面板占的高度（含边框）；没有动作流程时是 0。
fn step_panel_height(world: &WorldState) -> u16 {
    let rows = world.bag_step_rows() as u16;
    match world.bag_step.as_ref() {
        None => 0,
        Some(BagStep::Actions { .. }) => (rows + 2).clamp(4, 8),
        Some(BagStep::Target { .. }) => (rows + 2).clamp(4, 9),
        Some(BagStep::MoveTo { .. }) => 5,
        Some(BagStep::DropCount { .. }) => 3,
        Some(BagStep::InlayPick { .. }) => (rows + 2).clamp(4, 9),
    }
}

/// 这一步作用在哪个格子上。
fn step_slot(step: &BagStep) -> u8 {
    match step {
        BagStep::Actions { slot, .. }
        | BagStep::Target { slot, .. }
        | BagStep::MoveTo { slot, .. }
        | BagStep::DropCount { slot, .. }
        | BagStep::InlayPick { slot, .. } => *slot,
    }
}

/// 动作流程的面板：动作列表、使用目标、移动目的格、丢弃数量。
fn render_step(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let Some(step) = world.bag_step.as_ref() else {
        return;
    };
    let name = world.item_at(step_slot(step)).map(|it| it.name).unwrap_or_default();
    let title = step.title(&name);
    match step {
        BagStep::Actions { slot, index } => render_actions(frame, world, area, title, *slot, *index),
        BagStep::Target { slot, index } => render_targets(frame, world, area, title, *slot, *index),
        BagStep::MoveTo { slot, index } => render_move(frame, world, area, title, *slot, *index),
        BagStep::DropCount { text, .. } => render_count(frame, area, title, text.text()),
        BagStep::InlayPick { slot, index, picked } => {
            let free = world.item_at(*slot).and_then(|it| it.inlay.map(|i| i.free)).unwrap_or(0);
            render_inlay_pick(frame, world, area, title, free, *index, picked);
        }
    }
}

/// 一个可选行的样式：选中的反色，禁用的灰字。
pub(super) fn row_style(selected: bool, disabled: bool) -> Style {
    if selected {
        selected_style(true)
    } else if disabled {
        Style::default().fg(Color::DarkGray)
    } else {
        Style::default()
    }
}

/// 动作列表：不能做的动作也显示，并在后面解释原因（STAGE-132 验收标准 6）。
fn render_actions(frame: &mut Frame, world: &WorldState, area: Rect, title: String, slot: u8, index: usize) {
    let it = world.item_at(slot);
    let acts = it.as_ref().map(|it| actions(slot, it)).unwrap_or_default();
    let rows: Vec<ListItem> = acts
        .iter()
        .enumerate()
        .map(|(i, act)| {
            let reason = it.as_ref().and_then(|it| disabled_reason(*act, it)).or_else(|| {
                // STAGE-299：agent 操控时精工灰掉（动作本身照常显示，回车只提示）。
                (act.label() == "精工" && world.observing()).then_some("agent 正在操控，输入 take 收回控制权")
            });
            let mut spans = vec![Span::raw(act.label())];
            if let Some(reason) = reason {
                spans.push(Span::styled(format!("  （{reason}）"), Style::default().fg(Color::DarkGray)));
            }
            ListItem::new(Line::from(spans)).style(row_style(i == index, reason.is_some()))
        })
        .collect();
    frame.render_widget(List::new(rows).block(Block::bordered().title(title)), area);
}

/// 使用目标：自己在前，然后是能选的宠物。
fn render_targets(frame: &mut Frame, world: &WorldState, area: Rect, title: String, slot: u8, index: usize) {
    let Some(it) = world.item_at(slot) else {
        return;
    };
    let down_only = matches!(use_targets(&it), UseTargets::Pick { down_only: true });
    let mut labels = vec![format!("自己　HP {}/{}", world.entry.status.hp, world.entry.status.max_hp)];
    for pet in world.bag_target_pets(down_only) {
        labels.push(format!("{}（{} 号）", world.pet_name(pet), pet));
    }
    if down_only && labels.len() == 1 {
        labels.push("没有倒下的宠物".to_string());
    }
    let rows: Vec<ListItem> = labels
        .iter()
        .enumerate()
        .map(|(i, label)| {
            let disabled = down_only && i == 1 && world.bag_target_pets(true).is_empty();
            ListItem::new(Line::from(Span::raw(label.clone()))).style(row_style(i == index, disabled))
        })
        .collect();
    frame.render_widget(List::new(rows).block(Block::bordered().title(title)), area);
}

/// 移动目的格：5 列 × 3 行铺出 5–19 全部背包格；当前格灰掉，有物品的格子标一个 ◆（可交换）。
fn render_move(frame: &mut Frame, world: &WorldState, area: Rect, title: String, from: u8, index: usize) {
    let mut lines = Vec::new();
    for row in 0..(15usize.div_ceil(MOVE_COLS)) {
        let mut spans = Vec::new();
        for col in 0..MOVE_COLS {
            let i = row * MOVE_COLS + col;
            let slot = (FIRST_BAG_SLOT + i) as u8;
            let mark = if world.item_at(slot).is_some() { "◆" } else { "·" };
            let style = if slot == from {
                Style::default().fg(Color::DarkGray)
            } else {
                row_style(i == index, false)
            };
            spans.push(Span::styled(format!("{:>2}{mark}  ", slot), style));
        }
        lines.push(Line::from(spans));
    }
    frame.render_widget(Paragraph::new(lines).block(Block::bordered().title(title)), area);
}

/// 丢弃数量表单：数字直接进输入框，`Enter` 继续到确认，`Esc` 取消。
fn render_count(frame: &mut Frame, area: Rect, title: String, text: &str) {
    let line = Line::from(vec![
        Span::raw("数量  "),
        Span::styled(format!("{text}_"), Style::default().fg(Color::Yellow)),
        Span::styled("    1–持有数 · Enter 继续 · Esc 取消", Style::default().fg(Color::DarkGray)),
    ]);
    frame.render_widget(Paragraph::new(line).block(Block::bordered().title(title)), area);
}
