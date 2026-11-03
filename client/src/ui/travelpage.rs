//! 快捷传送页（STAGE-161，`docs/ui.md`「快捷传送」）：按地图包含层级逐级选择，选定就直达。
//!
//! 布局沿用 demo 的带边框面板：左边是当前层的列表，右边（宽度够时）是选中目的地的详情。
//! 标题写当前路径，底边写这一层有几项，行尾标出地图编号和「← 当前」。
//! 页面的状态、层级和光标都在 `app/travel.rs`，这里只负责画。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, ListItem, ListState, Paragraph, Wrap};

use super::leveling_dir_view;
use super::pet_dir_view;
use super::widgets::{select_mark, selected_style};
use crate::app::{App, TravelState, TravelStatus};

/// 内容区宽到这个值才把详情放在右边；更窄时详情用列表底部的两行代替。
const SIDE_BY_SIDE: u16 = 88;

const DIM: Style = Style::new().fg(Color::DarkGray);
const HERE: Style = Style::new().fg(Color::Green);

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let unicode = app.unicode;
    let Some(world) = app.world.as_mut() else {
        return;
    };
    let here = format!(
        "{} ({})",
        world.entry.position.map_name, world.entry.position.floor
    );
    let travel = &mut world.travel;
    // 「1级宠物」目录（STAGE-251）打开时占满内容区；画面细节见 pet_dir_view.rs。
    if travel.pets.is_open() {
        pet_dir_view::render(frame, &mut travel.pets, area, unicode);
        return;
    }
    // 「练级传送」（STAGE-279）打开时同样占满内容区。
    if travel.leveling.is_open() {
        leveling_dir_view::render(frame, &mut travel.leveling, area, unicode);
        return;
    }
    let path = travel.path();
    let mut title = String::from("快捷传送");
    if !path.is_empty() {
        title.push_str(" › ");
        title.push_str(&path.join(" › "));
    }
    if travel.ready() {
        title.push_str(&format!("（{} 项）", travel.len()));
    }

    // 目录没取到（加载中、不可用）时，上面写一行说明，下面照样列出根层的「1级宠物」（STAGE-251）。
    let block = Block::bordered().title(title);
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let list_area = match travel.status.clone() {
        TravelStatus::Ready => inner,
        TravelStatus::Loading => {
            let [note, rest] = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).areas(inner);
            frame.render_widget(Paragraph::new("  正在取快捷传送目录…").style(DIM), note);
            rest
        }
        TravelStatus::Unavailable(reason) => {
            let text = format!(
                "  快捷传送目录不可用：{reason}\n\n  Enter 重新获取（光标在「1级宠物」上时进入它） · Esc 返回上一页\n\n  \
                 目录由服务端提供；旧版服务端不返回层级数据，无法在这里按地图逐级选择。"
            );
            let [note, rest] = Layout::vertical([Constraint::Length(7), Constraint::Min(0)]).areas(inner);
            frame.render_widget(
                Paragraph::new(text).style(Style::default().fg(Color::Yellow)).wrap(Wrap { trim: false }),
                note,
            );
            rest
        }
    };
    render_list(frame, travel, list_area, &here);
}

fn render_list(frame: &mut Frame, travel: &mut TravelState, area: Rect, here: &str) {
    let rows = travel.view();
    let wide = area.width >= SIDE_BY_SIDE;
    let (list_area, detail_area) = if wide {
        let [list, detail] =
            Layout::horizontal([Constraint::Percentage(58), Constraint::Min(30)]).areas(area);
        (list, Some(detail))
    } else {
        (area, None)
    };

    // 一屏能放多少行记下来给 PgUp/PgDn 用；光标走出可见范围时列表跟着滚（STAGE-147 的做法）。
    let visible = usize::from(list_area.height.saturating_sub(2)).max(1);
    travel.rows = visible;
    let cursor = travel.cursor();
    let mut scroll = travel.scroll();
    if cursor < scroll {
        scroll = cursor;
    } else if cursor >= scroll + visible {
        scroll = cursor + 1 - visible;
    }
    // 最后几项时不要留一大片空白：光标贴底也能看到。
    scroll = scroll.min(rows.len().saturating_sub(visible));
    travel.set_scroll(scroll);

    if rows.is_empty() {
        frame.render_widget(
            Paragraph::new("  这一层没有可去的目录").style(DIM).block(Block::bordered().title("目录")),
            list_area,
        );
        return;
    }

    let items: Vec<ListItem> = rows
        .iter()
        .enumerate()
        .map(|(i, row)| {
            let mut spans = vec![
                Span::raw(select_mark(i == cursor)),
                Span::raw(" "),
            ];
            if row.here {
                spans.push(Span::styled(
                    format!("传送到本图 · {}", row.name),
                    Style::default().fg(Color::Yellow),
                ));
            } else {
                spans.push(Span::raw(row.name.clone()));
                if row.has_children {
                    spans.push(Span::styled(" ›", DIM));
                }
            }
            if let Some(floor) = row.floor {
                spans.push(Span::styled(format!("   {floor}"), DIM));
            }
            if row.current {
                spans.push(Span::styled("   ← 当前", HERE));
            }
            let item = ListItem::new(Line::from(spans));
            if i == cursor {
                item.style(selected_style(true))
            } else {
                item
            }
        })
        .collect();

    let mut state = ListState::default();
    state.select(Some(cursor));
    let block = Block::bordered()
        .title("目录")
        .title_bottom(Line::from(Span::styled(
            format!(" {} / {} ", cursor + 1, rows.len()),
            DIM,
        )));
    frame.render_stateful_widget(List::new(items).block(block), list_area, &mut state);

    if let Some(detail) = detail_area {
        render_detail(frame, travel, &rows, detail, here);
    }
}

/// 右侧详情：名字、地图编号、下级数量、是不是已经在这里。
fn render_detail(
    frame: &mut Frame,
    travel: &TravelState,
    rows: &[crate::app::TravelRow],
    area: Rect,
    here: &str,
) {
    let block = Block::bordered().title("目的地");
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let Some(row) = rows.get(travel.cursor()) else {
        return;
    };
    // 「1级宠物」入口不是地图：详情说明它是干什么的，不套用“分组”的说法（STAGE-251）。
    if row.leveling {
        let lines = vec![
            Line::from(Span::styled(
                row.name.clone(),
                Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
            )),
            Line::from(""),
            Line::from("按 10 级一段看全地图的野外遇敌点，选中后可以直接传送开战。"),
        ];
        frame.render_widget(Paragraph::new(lines).wrap(Wrap { trim: false }), inner);
        return;
    }
    if row.pets {
        let lines = vec![
            Line::from(Span::styled(
                row.name.clone(),
                Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD),
            )),
            Line::from(""),
            Line::from("按宠物查看能遇到它的地点，选中后可以直接传送开战。"),
        ];
        frame.render_widget(Paragraph::new(lines).wrap(Wrap { trim: false }), inner);
        return;
    }
    let mut lines = vec![
        Line::from(Span::styled(
            row.name.clone(),
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        )),
        Line::from(""),
    ];
    match row.floor {
        Some(floor) => {
            lines.push(Line::from(vec![
                Span::styled("地图编号  ", DIM),
                Span::raw(floor.to_string()),
            ]));
            lines.push(Line::from(vec![
                Span::styled("下级      ", DIM),
                Span::raw(if row.has_children {
                    format!("{} 项", row.children)
                } else {
                    "无（Enter 直接传送）".to_string()
                }),
            ]));
            if row.current {
                lines.push(Line::from(Span::styled("已经在这里", HERE)));
            } else {
                lines.push(Line::from(Span::styled("Enter 传送过去", DIM)));
            }
        }
        None => {
            lines.push(Line::from(Span::styled(
                "分组（不能直接传送）",
                Style::default().fg(Color::Yellow),
            )));
            lines.push(Line::from(vec![
                Span::styled("下级      ", DIM),
                Span::raw(format!("{} 项", row.children)),
            ]));
            lines.push(Line::from(Span::styled("Enter 进入", DIM)));
        }
    }
    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled("当前位置", DIM)));
    lines.push(Line::from(Span::styled(here.to_string(), HERE)));
    if !travel.version.is_empty() {
        lines.push(Line::from(""));
        lines.push(Line::from(vec![
            Span::styled("目录版本  ", DIM),
            Span::styled(travel.version.clone(), DIM),
        ]));
    }
    frame.render_widget(Paragraph::new(lines).wrap(Wrap { trim: false }), inner);
}
