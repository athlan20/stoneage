//! 场景根页（STAGE-131，`docs/cli-ui-redesign.md`「场景与传送」）：进入世界后的默认层。
//! 场景根页的出口/NPC双列绘制见 scene_root；本模块保留地点与目录步骤。
//! 场景不是文字输入层：方向键选、Enter 打开、Esc 打开游戏菜单、`/` 打开命令栏。
//! STAGE-134：NPC 行加上到角色的距离；两个入口分别打开整页传送列表和按名字查找的表单。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem, ListState, Paragraph};

use crate::app::{App, CatalogScope, SceneStep, WorldState};
use crate::proto::NpcMatch;

const LABEL: Style = Style::new().fg(Color::DarkGray);

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let Some(world) = app.world.as_mut() else {
        return;
    };
    // 场景根页要更新滚动位置（STAGE-147），单独走可变借用；两个步骤页只读。
    if world.scene_step.is_none() {
        super::scene_root::render(frame, world, area, app.unicode);
        return;
    }
    match world.scene_step.as_ref() {
        Some(SceneStep::Catalog { text, scope, items, index, loading, .. }) => {
            render_catalog(frame, world, area, text.text(), *scope, items, *index, *loading)
        }
        None => {}
    }
}
/// NPC 目录页（STAGE-134）：搜索框 + 分类 + 候选。候选按服务端给的顺序（本图的在前），
/// 里面标出每一条在哪张地图上，本图的写「本图」。
fn render_catalog(
    frame: &mut Frame,
    world: &WorldState,
    area: Rect,
    text: &str,
    scope: CatalogScope,
    items: &[NpcMatch],
    index: usize,
    loading: bool,
) {
    let [head, list_area] = Layout::vertical([Constraint::Length(3), Constraint::Min(0)]).areas(area);
    let line = Line::from(vec![
        Span::raw("名字  "),
        Span::styled(format!("{text}_"), Style::default().fg(Color::Yellow)),
        Span::styled(
            format!(
                "    分类 {} · Tab 切换 · Enter {} · Esc 返回",
                scope.label(),
                if loading { "重新搜索" } else { "对话 / 传送" }
            ),
            LABEL,
        ),
    ]);
    let title = format!("NPC 目录 · {}（共 {}）", scope.label(), items.len());
    frame.render_widget(Paragraph::new(line).block(Block::bordered().title("查找并直接对话")), head);

    if items.is_empty() {
        let hint = if loading { " 正在查找…" } else { " 没有匹配的 NPC，换个词试试" };
        frame.render_widget(Paragraph::new(hint).style(LABEL).block(Block::bordered().title(title)), list_area);
        return;
    }
    let rows: Vec<ListItem> = items
        .iter()
        .enumerate()
        .map(|(i, n)| {
            let mut spans = vec![
                Span::styled(format!("{:>5}  ", n.id), LABEL),
                Span::raw(n.name.clone()),
                Span::styled(format!("   {}", n.type_), LABEL),
            ];
            if n.here {
                spans.push(Span::styled("   本图", Style::default().fg(Color::Green)));
            } else {
                spans.push(Span::styled(format!("   {}", n.map_name), LABEL));
            }
            let dist = distance_to(world, n.x, n.y);
            if n.here {
                spans.push(Span::styled(format!("  {dist}"), LABEL));
            }
            let item = ListItem::new(Line::from(spans));
            if i == index {
                item.style(selected_style())
            } else {
                item
            }
        })
        .collect();
    let mut state = ListState::default();
    state.select(Some(index.min(items.len() - 1)));
    frame.render_stateful_widget(List::new(rows).block(Block::bordered().title(title)), list_area, &mut state);
}

/// 两格之间的切比雪夫距离（max(|dx|, |dy|)），和服务端的 NPC 排序与对话距离判定一致（npclist.go）。
pub(super) fn chebyshev(ax: i32, ay: i32, bx: i32, by: i32) -> i32 {
    (ax - bx).abs().max((ay - by).abs())
}

/// NPC 到角色的切比雪夫距离。**只用于显示**，排序仍按服务端给的列表顺序，不做寻路。
fn distance_to(world: &WorldState, x: i32, y: i32) -> i32 {
    let p = &world.entry.position;
    chebyshev(x, y, p.x, p.y)
}

#[cfg(test)]
mod tests {
    use super::chebyshev;

    #[test]
    fn npc_distance_is_chebyshev_like_the_server() {
        // 缺陷 STAGE-263：超级饲育员 (18,13) 与落点 (16,21) 的服务端距离是 8，曼哈顿会显示 10。
        assert_eq!(chebyshev(18, 13, 16, 21), 8);
        assert_eq!(chebyshev(18, 13, 17, 14), 1);
        assert_eq!(chebyshev(18, 13, 17, 15), 2);
        assert_eq!(chebyshev(5, 5, 5, 5), 0);
        assert_eq!(chebyshev(3, 9, 7, 6), 4);
    }
}

/// 选中行：反色的黄色（和 demo 的选中项一致，`docs/ui.md`「界面风格」）。
fn selected_style() -> Style {
    Style::default().fg(Color::Black).bg(Color::Yellow).add_modifier(Modifier::BOLD)
}
