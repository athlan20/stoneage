//! STAGE-148/152：出口 / NPC / 遇敌三列；STAGE-273 起在 NPC 与遇敌之间加「附近玩家」列，各自独立滚动与选择。
use crate::app::SceneColumn;
use crate::app::{WorldState, party_rules, wrap_text};
use crate::ui::{scene_encounter, scene_players};
use ratatui::{
    Frame,
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::Line,
    widgets::{Block, List, ListItem, ListState, Paragraph, Wrap},
};
const EXTRAS: [&str; 3] = ["快捷传送", "查找并直接对话", "回城（存档点）"];

pub fn render(frame: &mut Frame, w: &mut WorldState, area: Rect, unicode: bool) {
    if w.scene.details {
        details(frame, w, area);
        return;
    }
    let [head, body, foot] = Layout::vertical([
        Constraint::Length(1),
        Constraint::Min(0),
        Constraint::Length(3),
    ])
    .areas(area);
    let p = &w.entry.position;
    let place = if p.map_name.is_empty() {
        format!("地图 #{}", p.floor)
    } else {
        p.map_name.clone()
    };
    frame.render_widget(
        Paragraph::new(format!("{place} #{} · {},{}", p.floor, p.x, p.y)),
        head,
    );
    // STAGE-273：四列 24/26/22/28（出口 / NPC / 附近玩家 / 遇敌）。
    let [left, mid, players, right] = Layout::horizontal([
        Constraint::Percentage(24),
        Constraint::Percentage(26),
        Constraint::Percentage(22),
        Constraint::Percentage(28),
    ])
    .areas(body);
    w.scene.rows = body.height.saturating_sub(2) as usize;
    let count = w.scene.exits.len();
    // STAGE-274：队员（真人队跟随中）的出口、固定入口「快捷传送」「回城」和遇敌列整列灰显。
    let dim = party_rules::follower_reason(w).is_some();
    let mut exits: Vec<ListItem> = w
        .scene
        .exits
        .iter()
        .map(|e| {
            let cell = e
                .trigger_cells
                .first()
                .map_or(String::new(), |c| format!(" ({},{})", c.x, c.y));
            let multi = if e.trigger_cells.len() > 1 {
                format!(" {} 格", e.trigger_cells.len())
            } else {
                String::new()
            };
            let closed = if e.available { "" } else { "[关] " };
            let oneway = if e.one_way { " 单程" } else { "" };
            let gate = if e.time_gate == "all" {
                ""
            } else {
                e.gate_label()
            };
            ListItem::new(format!("{closed}{}{cell}{multi} {gate}{oneway}", e.label()))
                .style(dim_style(dim))
        })
        .collect();
    // Status and separator have no logical row: navigation can never execute them.
    let placeholder = w.scene.exit_load.placeholder(count == 0);
    let insert = if let Some(text) = placeholder {
        exits.insert(
            0,
            ListItem::new(text).style(Style::new().fg(Color::DarkGray)),
        );
        1
    } else {
        0
    };
    exits.push(
        ListItem::new(if unicode {
            "──────── 其他地点与查找 ────────"
        } else {
            "-------- 其他地点与查找 --------"
        })
        .style(Style::new().fg(Color::DarkGray)),
    );
    // 「查找并直接对话」不受组队限制，保持青色。
    for label in EXTRAS {
        let style = match label {
            "查找并直接对话" => Style::new().fg(Color::Cyan),
            _ => dim_style(dim),
        };
        exits.push(ListItem::new(label).style(style));
    }
    let logical = w.scene_index.min(count + 2);
    let physical = logical + insert + usize::from(logical >= count);
    draw_list(
        frame,
        left,
        "出口",
        exits,
        Some(physical),
        &mut w.scene_scroll,
        w.scene.column == SceneColumn::Exit,
        unicode,
        dim,
    );
    let mut npcs: Vec<ListItem> = w
        .npcs
        .iter()
        .map(|n| {
            // 切比雪夫距离，和服务端判定对话距离一致（STAGE-263）。
            let distance = super::scene::chebyshev(n.x, n.y, p.x, p.y);
            ListItem::new(format!("{} · {} · {distance}", n.name, n.type_))
        })
        .collect();
    if let Some(text) = w.scene.npc_load.placeholder(npcs.is_empty()) {
        npcs.insert(
            0,
            ListItem::new(text).style(Style::new().fg(Color::DarkGray)),
        );
        // Cached NPCs can be browsed during refresh, but cannot be executed.
        let selected = (!w.npcs.is_empty())
            .then_some(w.scene.npc_index.min(w.npcs.len().saturating_sub(1)) + 1);
        draw_list(
            frame,
            mid,
            "NPC",
            npcs,
            selected,
            &mut w.scene.npc_top,
            w.scene.column == SceneColumn::Npc,
            unicode,
            false,
        );
    } else {
        let selected =
            (!w.npcs.is_empty()).then_some(w.scene.npc_index.min(w.npcs.len().saturating_sub(1)));
        draw_list(
            frame,
            mid,
            "NPC",
            npcs,
            selected,
            &mut w.scene.npc_top,
            w.scene.column == SceneColumn::Npc,
            unicode,
            false,
        );
    }
    scene_players::draw(frame, w, players, unicode);
    scene_encounter::draw_areas(frame, &mut w.scene, right, unicode, dim);
    frame.render_widget(
        Paragraph::new(selected_summary(w))
            .wrap(Wrap { trim: false })
            .block(Block::bordered().title("选中项 · Tab 详情 · F5 刷新")),
        foot,
    );
    if w.scene.prompt.is_some() {
        scene_encounter::draw_prompt(frame, &w.scene, area);
    }
    if w.scene.players.prompt.is_some() {
        scene_players::draw_prompt(frame, w, area);
    }
}
pub(super) fn draw_list(
    frame: &mut Frame,
    area: Rect,
    title: &str,
    items: Vec<ListItem>,
    selected: Option<usize>,
    top: &mut usize,
    focus: bool,
    unicode: bool,
    dim: bool,
) {
    let style = if focus {
        Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD)
    } else {
        Style::new().fg(Color::Gray)
    };
    let title = if focus {
        format!("* {title}")
    } else {
        title.into()
    };
    let block = Block::bordered()
        .title(title)
        .title_style(style)
        .border_style(style);
    let mut state = ListState::default()
        .with_offset(*top)
        .with_selected(selected);
    let marker = if focus && unicode {
        "▶ "
    } else if focus {
        "> "
    } else {
        ": "
    };
    frame.render_stateful_widget(
        List::new(items)
            .block(block)
            .highlight_symbol(marker)
            .highlight_style(focus_highlight(focus, dim)),
        area,
        &mut state,
    );
    *top = state.offset();
}

/// 选中行的高亮：焦点在灰显列上时仍用灰字（不变成黄底黑字），否则沿用黄底黑字。
pub(super) fn focus_highlight(focus: bool, dim: bool) -> Style {
    if focus && dim {
        Style::new().fg(Color::DarkGray).add_modifier(Modifier::BOLD)
    } else if focus {
        Style::new()
            .fg(Color::Black)
            .bg(Color::Yellow)
            .add_modifier(Modifier::BOLD)
    } else {
        Style::new().add_modifier(Modifier::UNDERLINED)
    }
}

/// 灰显列的行样式（队员时）；不灰时用默认样式。
pub(super) fn dim_style(dim: bool) -> Style {
    if dim { Style::new().fg(Color::DarkGray) } else { Style::new() }
}

/// 队员时出口、遇敌列的底部说明先写灰显原因（docs/ui.md「队员灰显」）。
fn selected_summary(w: &WorldState) -> String {
    let base = selected_summary_base(w);
    match party_rules::follower_reason(w) {
        Some(why) if matches!(w.scene.column, SceneColumn::Exit | SceneColumn::Encounter) => format!("{why} · {base}"),
        _ => base,
    }
}

fn selected_summary_base(w: &WorldState) -> String {
    if w.scene.column == SceneColumn::Players {
        return scene_players::summary(w);
    }
    if w.scene.column == SceneColumn::Encounter {
        if w.scene.stop_selected() {
            return "停止遇敌 · Enter 停掉后续场次（当前这场照常打完）".into();
        }
        return w
            .scene
            .selected_area()
            .map(scene_encounter::summary)
            .unwrap_or_else(|| "没有选中区域".into());
    }
    if w.scene.column == SceneColumn::Npc {
        return w
            .npcs
            .get(w.scene.npc_index)
            .map_or("没有选中 NPC".into(), |n| {
                format!(
                    "{} · {} ({},{}) · Enter 沿用直接对话",
                    n.name, n.type_, n.x, n.y
                )
            });
    }
    if let Some(e) = w.scene.exits.get(w.scene_index) {
        return format!(
            "{} · {} · {}{}",
            e.label(),
            e.gate_label(),
            e.unavailable_reason.as_deref().unwrap_or("Enter 直接通过"),
            if e.one_way {
                " · 单程（按普通出口表判断）"
            } else {
                ""
            }
        );
    }
    EXTRAS
        .get(w.scene_index.saturating_sub(w.scene.exits.len()))
        .unwrap_or(&"")
        .to_string()
}
fn details(frame: &mut Frame, w: &mut WorldState, area: Rect) {
    let mut text = selected_summary(w);
    if w.scene.column == SceneColumn::Encounter {
        if let Some(a) = w.scene.selected_area() {
            text = scene_encounter::details(a, w.entry.position.floor);
        }
    } else if w.scene.column == SceneColumn::Npc {
        if let Some(n) = w.npcs.get(w.scene.npc_index) {
            text.push_str(&format!(
                "\nNPC #{}\n距离 {}（仅显示，不检查距离）",
                n.id,
                super::scene::chebyshev(n.x, n.y, w.entry.position.x, w.entry.position.y)
            ));
        }
    } else if w.scene.column == SceneColumn::Players {
        if let Some(p) = w.scene.players.selected() {
            text = scene_players::details(p);
        }
    } else if let Some(e) = w.scene.exits.get(w.scene_index) {
        text.push_str(&format!(
            "\n目标地图 #{}\n{}\n触发格（{} 格；固定使用按 x、y 最小的第一格对应落点）：\n",
            e.to_floor,
            e.id,
            e.trigger_cells.len()
        ));
        for c in &e.trigger_cells {
            text.push_str(&format!("({}, {})\n", c.x, c.y));
        }
        text.push_str("\n单程仅按普通出口表判断；不承诺原路返回，不含 NPC、船或任务回程。");
    }
    let lines: Vec<Line> = text
        .split('\n')
        .flat_map(|line| wrap_text(line, area.width.saturating_sub(2) as usize))
        .map(|s| Line::raw(s.to_string()))
        .collect();
    let rows = area.height.saturating_sub(2) as usize;
    w.scene.detail_scroll = w
        .scene
        .detail_scroll
        .min(lines.len().saturating_sub(rows) as u16);
    frame.render_widget(
        Paragraph::new(lines)
            .scroll((w.scene.detail_scroll, 0))
            .block(Block::bordered().title("完整信息 · ↑↓/翻页阅读 · Tab / Esc 返回")),
        area,
    );
}
