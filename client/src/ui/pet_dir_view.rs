//! STAGE-251：快捷传送页「1级宠物」目录的画面（`docs/ui.md`「快捷传送页」）。
//! 内容区宽 ≥ 88 列时右侧放详情（`pet_dir_detail.rs`）；窄屏（80×24）只有列表。
//! 长名字按显示宽度截断（中文占两列），光标行始终可见（沿用 `app::scroll::follow_top`）。
//! `--unicode` 关闭时光标、省略号、筛选光标退回 ASCII。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph};
use unicode_width::UnicodeWidthStr;

use super::pet_dir_detail::detail_lines;
use super::widgets::selected_style;
use crate::app::pet_dir::{ACTIONS, PetDir, PetDirStatus, PetLevel};
use crate::app::scroll::follow_top;
use crate::proto::{PetEntry, PetSpot};

/// 宽屏切分的宽度下限（与快捷传送页一致）。
const SIDE_BY_SIDE: u16 = 88;
const DIM: Style = Style::new().fg(Color::DarkGray);
const HEAD: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);

pub fn render(frame: &mut Frame, pets: &mut PetDir, area: Rect, unicode: bool) {
    let block = Block::bordered().title(title(pets));
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let (left, right) = if inner.width >= SIDE_BY_SIDE {
        let cols = Layout::horizontal([Constraint::Percentage(60), Constraint::Percentage(40)]).split(inner);
        (cols[0], Some(cols[1]))
    } else {
        (inner, None)
    };
    let [head, list] = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).areas(left);
    frame.render_widget(Paragraph::new(head_line(pets, unicode)), head);
    pets.rows = list.height as usize;
    let lines = list_lines(pets, list.height as usize, list.width as usize, unicode);
    frame.render_widget(Paragraph::new(lines), list);
    if let Some(detail) = right {
        frame.render_widget(
            Paragraph::new(detail_lines(pets)).block(Block::bordered().title("详情")),
            detail,
        );
    }
}

/// 标题：L1 写命中数（`1级宠物 · 筛选“人龙” · 5 只`），L2 写宠物名，L3 写宠物名和遇敌点。
fn title(pets: &PetDir) -> Span<'static> {
    let text = match pets.level() {
        PetLevel::Pets => {
            let n = pets.visible_pets().len();
            if pets.filter.is_empty() {
                format!(" 1级宠物 · {n} 只 ")
            } else {
                format!(" 1级宠物 · 筛选“{}” · {n} 只 ", pets.filter)
            }
        }
        PetLevel::Spots { pet_id } => {
            format!(
                " {} · {} 个遇敌点 ",
                pet_name(pets, *pet_id),
                pets.spots_of(*pet_id).len()
            )
        }
        PetLevel::Actions { pet_id, .. } => {
            let spot = pets
                .current_spot()
                .map(|s| format!(" · {}（{},{}）", s.map_name, s.anchor.x, s.anchor.y));
            format!(" {}{} ", pet_name(pets, *pet_id), spot.unwrap_or_default())
        }
    };
    Span::styled(text, HEAD)
}

/// 第一行：L1 是筛选框（焦点在框里时带光标），L2/L3 是操作提示。
fn head_line(pets: &PetDir, unicode: bool) -> Line<'static> {
    match (&pets.status, pets.level()) {
        (PetDirStatus::Loading | PetDirStatus::Idle, _) => Line::from(Span::styled("目录加载中…", DIM)),
        (PetDirStatus::Failed(reason), _) => {
            Line::from(Span::styled(format!("目录不可用：{reason}（Enter 重试）"), DIM))
        }
        (PetDirStatus::Ready, PetLevel::Pets) => {
            let caret = if unicode { "▌" } else { "_" };
            if pets.typing {
                Line::from(vec![
                    Span::styled("筛选：", HEAD),
                    Span::raw(format!("{}{caret}", pets.filter)),
                ])
            } else if pets.filter.is_empty() {
                Line::from(Span::styled("按 / 或 f 筛选（名字、俗称、系列）", DIM))
            } else {
                Line::from(vec![Span::styled("筛选：", HEAD), Span::raw(pets.filter.clone())])
            }
        }
        (PetDirStatus::Ready, _) => Line::from(Span::styled("↑↓ 选择 · Enter 进入 · Esc 返回", DIM)),
    }
}

/// 列表的行：只画光标附近的一屏，光标始终在可见范围内。
fn list_lines(pets: &mut PetDir, height: usize, width: usize, unicode: bool) -> Vec<Line<'static>> {
    if pets.status != PetDirStatus::Ready || height == 0 {
        return Vec::new();
    }
    let rows = rows_for(pets);
    if rows.is_empty() {
        let text = match pets.level() {
            PetLevel::Pets if !pets.filter.is_empty() => "没有匹配的宠物 · Esc 清空筛选",
            PetLevel::Pets => "没有可以显示的宠物",
            _ => "这只宠物没有遇敌点",
        };
        return vec![Line::from(Span::styled(text, DIM))];
    }
    let cursor = pets.cursor().min(rows.len() - 1);
    let top = follow_top(pets.scroll(), cursor, height, rows.len());
    pets.set_scroll(top);
    rows.into_iter()
        .enumerate()
        .skip(top)
        .take(height)
        .map(|(i, (left, right))| row_line(&left, &right, width, i == cursor, unicode))
        .collect()
}

/// 每一行的（左边文字，右边文字）：L1 是宠物名与俗称系列、属性与可遇点数；L2 是地图与坐标、区域和上限；L3 是动作。
fn rows_for(pets: &PetDir) -> Vec<(String, String)> {
    match pets.level() {
        PetLevel::Pets => pets
            .visible_pets()
            .iter()
            .map(|p| (pet_label(p), pet_right(p)))
            .collect(),
        PetLevel::Spots { pet_id } => {
            let spots = pets.spots_of(*pet_id);
            spots
                .iter()
                .map(|s| {
                    let same_name = spots.iter().filter(|o| o.map_name == s.map_name).count() > 1;
                    (spot_label(s, same_name), spot_right(s))
                })
                .collect()
        }
        PetLevel::Actions { .. } => ACTIONS.iter().map(|a| ((*a).to_string(), String::new())).collect(),
    }
}

/// 一行：`› 名字 ……  右边`。右边最多占半行，超出截断；左边占剩下的，超出截断。
fn row_line(left: &str, right: &str, width: usize, selected: bool, unicode: bool) -> Line<'static> {
    let mark = match (selected, unicode) {
        (true, true) => "▶ ",
        (true, false) => "> ",
        (false, _) => "  ",
    };
    let avail = width.saturating_sub(UnicodeWidthStr::width(mark));
    let right = clip(right, avail / 2, unicode);
    let gap = if right.is_empty() { 0 } else { 1 };
    let left = clip(
        left,
        avail.saturating_sub(UnicodeWidthStr::width(right.as_str()) + gap),
        unicode,
    );
    let pad = avail
        .saturating_sub(UnicodeWidthStr::width(left.as_str()))
        .saturating_sub(UnicodeWidthStr::width(right.as_str()));
    let right_style = if selected { Style::new() } else { DIM };
    let line = Line::from(vec![
        Span::raw(mark),
        Span::raw(left),
        Span::raw(" ".repeat(pad)),
        Span::styled(right, right_style),
    ]);
    if selected {
        line.style(selected_style(true))
    } else {
        line
    }
}

/// `扬奇洛斯（蓝人龙 · 人龙系）`：俗称与系列为空就不带括号。
fn pet_label(p: &PetEntry) -> String {
    let mut tags = Vec::new();
    if !p.aliases.is_empty() {
        tags.push(p.aliases.join("、"));
    }
    if !p.series.is_empty() {
        tags.push(p.series.clone());
    }
    if tags.is_empty() {
        p.name.clone()
    } else {
        format!("{}（{}）", p.name, tags.join(" · "))
    }
}

/// 右边：最强的属性与数值（`风 100`，全为 0 写 `—`），再加可遇点数。
fn pet_right(p: &PetEntry) -> String {
    let attrs = [
        ("地", p.atoms.earth),
        ("水", p.atoms.water),
        ("火", p.atoms.fire),
        ("风", p.atoms.wind),
    ];
    let attr = attrs
        .iter()
        .filter(|(_, v)| *v > 0)
        .max_by_key(|(_, v)| *v)
        .map_or_else(|| "—".to_string(), |(name, v)| format!("{name} {v}"));
    format!("{attr} · {}处", p.spot_count)
}

/// L2 的行：地图名（同一只宠物有同名地图时补 ` · floor`）加坐标。
fn spot_label(s: &PetSpot, same_name: bool) -> String {
    let name = if same_name {
        format!("{} · {}", s.map_name, s.floor)
    } else {
        s.map_name.clone()
    };
    format!("{name} ({},{})", s.anchor.x, s.anchor.y)
}

/// L2 的右边：区域原编号、队伍上限，同点还有别的 1 级宠物就带上名字。
fn spot_right(s: &PetSpot) -> String {
    let mut text = format!("区域 {} · 上限 {}", s.number, s.max_enemies);
    if !s.also_level1.is_empty() {
        text.push_str(&format!(" · 同点 {}", s.also_level1.join("、")));
    }
    text
}

fn pet_name(pets: &PetDir, pet_id: i64) -> String {
    pets.catalog
        .as_ref()
        .and_then(|c| c.pets.iter().find(|p| p.pet_id == pet_id))
        .map_or_else(|| format!("#{pet_id}"), |p| p.name.clone())
}

/// 按显示宽度截断（中文按 2 列算），超出的部分用省略号代替（`…` 或 ASCII 的 `..`）。
fn clip(text: &str, width: usize, unicode: bool) -> String {
    use unicode_width::UnicodeWidthChar;
    if UnicodeWidthStr::width(text) <= width {
        return text.to_string();
    }
    let ellipsis = if unicode { "…" } else { ".." };
    let ell_w = UnicodeWidthStr::width(ellipsis);
    if width < ell_w {
        return String::new();
    }
    let mut out = String::new();
    let mut used = 0;
    for c in text.chars() {
        let w = c.width().unwrap_or(0);
        if used + w + ell_w > width {
            break;
        }
        out.push(c);
        used += w;
    }
    out.push_str(ellipsis);
    out
}
