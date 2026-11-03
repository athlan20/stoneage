//! STAGE-152 场景页的遇敌列：行文本、选中摘要、完整详情和「遇敌 1 次 / 无限遇敌」面板。
//! STAGE-286：能抓 1 级宠物的区域行首加标记，详情里列出这些宠物。
use super::scene_root::{dim_style, focus_highlight};
use crate::app::{EncounterChoice, SceneState};
use crate::proto::{AreaEnemy, EncounterArea};
use ratatui::{
    Frame,
    layout::{Alignment, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Clear, List, ListItem, ListState, Paragraph, Wrap},
};

fn enemy_level(e: &AreaEnemy) -> String {
    if e.level_min == e.level_max {
        format!("Lv{}", e.level_min)
    } else {
        format!("Lv{}-{}", e.level_min, e.level_max)
    }
}

/// 列表行：`#21 昆伊 Lv1 · 2只`，不可用时前面加 `[禁]`；能抓 1 级宠物的区域行首先加标记
/// （unicode 下 `◆ `，关闭时 `[宠] `，黄色；队员灰显时跟着整行变灰）。
pub fn row(area: &EncounterArea, unicode: bool, dim: bool) -> Line<'static> {
    let gate = if area.available { "" } else { "[禁] " };
    let main = area
        .main_enemy()
        .map(|e| format!("{} {}", e.name, enemy_level(e)))
        .unwrap_or_else(|| "无敌人".into());
    let mut spans = Vec::new();
    if !area.level1_pets.is_empty() {
        let mark = if unicode { "◆ " } else { "[宠] " };
        let style = if dim {
            dim_style(true)
        } else {
            Style::new().fg(Color::Yellow)
        };
        spans.push(Span::styled(mark, style));
    }
    spans.push(Span::raw(format!(
        "{gate}#{} {} · {}只",
        area.number, main, area.max_enemies
    )));
    Line::from(spans)
}

/// 详情里的 1 级宠物一行（没有时返回空串）；道具条件挡住的区域补一句怎么抓。
fn pet_line(area: &EncounterArea) -> String {
    if area.level1_pets.is_empty() {
        return String::new();
    }
    let mut line = format!("1级宠物：{}", area.level1_pets.join("、"));
    if !area.available && !area.required_items.is_empty() {
        line.push_str("（持有指定道具后可抓）");
    }
    line.push('\n');
    line
}

/// 底部摘要：区域、等级覆盖、人数、落点与可用性。
pub fn summary(area: &EncounterArea) -> String {
    let mut text = format!(
        "区域 #{} · {} · 最多 {} 只 · 落点 ({},{})",
        area.number,
        area.level_label(),
        area.max_enemies,
        area.anchor.x,
        area.anchor.y
    );
    if area.available {
        text.push_str(" · Enter 选择次数 · Tab 详情");
    } else if let Some(reason) = &area.unavailable_reason {
        text.push_str(&format!(" · 不可用：{reason}"));
    }
    text
}

/// Tab 页的完整信息：全部候选敌人（含随机替换展开项）、人数范围、道具限制与覆盖说明。
pub fn details(area: &EncounterArea, floor: u32) -> String {
    let mut text = format!(
        "区域 #{} · {} · 最多 {} 只\n地图 #{} · 矩形 ({},{})-({},{})（重叠处按优先级判定，不代表整块都生效）\n代表落点 ({},{})\n{}\n敌人（共 {} 项，权重只表示相对常见度）：\n",
        area.number,
        area.level_label(),
        area.max_enemies,
        floor,
        area.rect.x1,
        area.rect.y1,
        area.rect.x2,
        area.rect.y2,
        area.anchor.x,
        area.anchor.y,
        pet_line(area),
        area.enemies.len()
    );
    for e in &area.enemies {
        text.push_str(&format!(
            "  {} {} · 同场最多 {} · 权重 {} · 敌群 #{} · 敌人 #{}\n",
            e.name,
            enemy_level(e),
            e.create_max,
            e.weight,
            e.group_id,
            e.id
        ));
    }
    text.push_str(&format!(
        "\n每场人数是 1..{} 的随机值，上限不是固定满员。\n",
        area.max_enemies
    ));
    if !area.required_items.is_empty() {
        text.push_str(&format!("需要持有的道具：{:?}\n", area.required_items));
    }
    if !area.forbidden_items.is_empty() {
        text.push_str(&format!("不能持有的道具：{:?}\n", area.forbidden_items));
    }
    match (&area.available, &area.unavailable_reason) {
        (true, _) => text.push_str("当前可开始遇敌。\n"),
        (false, Some(r)) => text.push_str(&format!("当前不可用：{r}\n")),
        (false, None) => text.push_str("当前不可用。\n"),
    }
    text
}

/// 画遇敌列（含加载中/失败/空三种占位），末尾固定一个「停止遇敌」。
/// `dim` 为真（队员）时整列灰显（STAGE-274）。
pub fn draw_areas(frame: &mut Frame, s: &mut SceneState, area: Rect, unicode: bool, dim: bool) {
    let focus = s.column.is_encounter();
    let count = s.areas.len();
    let mut items: Vec<ListItem> = s
        .areas
        .iter()
        .map(|a| ListItem::new(row(a, unicode, dim)).style(dim_style(dim)))
        .collect();
    let mut insert = 0;
    let mut selected = if s.areas.is_empty() {
        None
    } else {
        Some(s.area_index.min(count - 1))
    };
    if let Some(text) = s.area_load.area_placeholder(s.areas.is_empty()) {
        items.insert(
            0,
            ListItem::new(text).style(Style::new().fg(Color::DarkGray)),
        );
        insert = 1;
        // 占位行不能被选中，但缓存下来的区域仍可浏览。
        selected = selected.map(|i| i + 1);
    }
    items.push(
        ListItem::new(if unicode {
            "──────── 停止遇敌 ────────"
        } else {
            "-------- 停止遇敌 --------"
        })
        .style(Style::new().fg(Color::DarkGray)),
    );
    let stop_style = if dim { dim_style(true) } else { Style::new().fg(Color::Cyan) };
    items.push(ListItem::new("停止遇敌").style(stop_style));
    // 逻辑项 = 区域 + 末尾固定项；分隔行在区域之后、固定项之前，没有逻辑位置。
    let logical = s.area_index.min(count);
    if logical >= count {
        selected = Some(logical + insert + 1);
    }
    let style = if focus {
        Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD)
    } else {
        Style::new().fg(Color::Gray)
    };
    let title = if focus {
        format!("* 遇敌")
    } else {
        "遇敌".to_string()
    };
    let block = Block::bordered()
        .title(title)
        .title_style(style)
        .border_style(style);
    let mut state = ListState::default()
        .with_offset(s.area_top)
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
    s.area_top = state.offset();
}

/// 动作面板：区域、敌人等级、目标坐标与三个选项；默认「遇敌 1 次」。
pub fn draw_prompt(frame: &mut Frame, s: &SceneState, area: Rect) {
    let Some(area_data) = s.selected_area() else {
        return;
    };
    let choice = s.prompt.map_or(EncounterChoice::Once, |p| p.choice);
    let width = area.width.min(44).max(28);
    // 边框内要放下：区域、落点、空行、三个选项、空行、提示 = 8 行。
    let height = 10u16.min(area.height);
    let popup = Rect {
        x: area.x + (area.width.saturating_sub(width)) / 2,
        y: area.y + (area.height.saturating_sub(height)) / 2,
        width,
        height,
    };
    frame.render_widget(Clear, popup);
    let mut lines = vec![
        Line::raw(format!("区域 #{} · {}", area_data.number, area_data.level_label())),
        Line::raw(format!("落点 ({},{})", area_data.anchor.x, area_data.anchor.y)),
        Line::raw(""),
    ];
    for c in [EncounterChoice::Once, EncounterChoice::Infinite, EncounterChoice::Cancel] {
        let mark = if c == choice { "▶ " } else { "  " };
        lines.push(Line::raw(format!("{mark}{}", c.label())).style(if c == choice {
            Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD)
        } else {
            Style::new()
        }));
    }
    lines.push(Line::raw(""));
    lines.push(
        Line::raw(if choice == EncounterChoice::Infinite {
            "持续到手动停止或自动停止"
        } else {
            "到此区域后立即遇敌"
        })
        .style(Style::new().fg(Color::DarkGray)),
    );
    frame.render_widget(
        Paragraph::new(lines)
            .alignment(Alignment::Left)
            .wrap(Wrap { trim: true })
            .block(Block::bordered().title("选择遇敌次数 · Esc 取消")),
        popup,
    );
}
