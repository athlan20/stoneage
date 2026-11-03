//! STAGE-273：场景页第三列「附近玩家」的渲染（设计 1.3）：行格式、标记、颜色、详情与动作面板。
use crate::app::{PLAYER_ACTIONS, WorldState};
use crate::proto::NearbyPlayer;
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
    text::Line,
    widgets::{Block, Clear, ListItem, Paragraph, Wrap},
};
use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

/// 列的标题与列表；列宽 24/26/22/28 中的第三列，内宽按边框和高亮符号扣除。
pub fn draw(frame: &mut Frame, w: &mut WorldState, area: Rect, unicode: bool) {
    let focus = w.scene.column == crate::app::SceneColumn::Players;
    let inner = area.width.saturating_sub(4) as usize;
    let p = &w.scene.players;
    let mut items: Vec<ListItem> = p.list.iter().map(|x| row(x, inner, unicode)).collect();
    let mut offset = 0;
    if let Some(text) = p.placeholder() {
        items.insert(0, ListItem::new(text).style(Style::new().fg(Color::DarkGray)));
        offset = 1;
    }
    // 加载中、失败时缓存的玩家仍可浏览，但 Enter 会先检查资料是否就绪（scene_keys）。
    // STAGE-273：用 then 而非 then_some，空列表时不能先算 len() - 1（会下溢）。
    // STAGE-278：末尾固定一行「聊天…」，编号等于玩家数，光标可以落在它上面。
    items.push(ListItem::new("聊天…").style(Style::new().fg(Color::Cyan)));
    let selected = Some(p.index.min(p.list.len()) + offset);
    let title = if p.truncated {
        format!("附近玩家 {}+", p.list.len())
    } else {
        format!("附近玩家 {}", p.list.len())
    };
    super::scene_root::draw_list(
        frame,
        area,
        &title,
        items,
        selected,
        &mut w.scene.players.top,
        focus,
        unicode,
        false,
    );
}

/// 一行：`标记 名字 Lv等级`，行尾的标记：战斗中 `⚔`（ASCII 为 `!`）优先；否则接受决斗的人标 `决`（ASCII 为 `D`）。
/// 名字过长先截断，等级和行尾标记必须保留。
fn row(p: &NearbyPlayer, width: usize, unicode: bool) -> ListItem<'static> {
    let mark = match (p.party.is_some(), unicode) {
        (true, true) => "· ",
        (true, false) => "- ",
        (false, _) => "  ",
    };
    let battle = match (p.in_battle, p.duel, unicode) {
        (true, _, true) => " ⚔",
        (true, _, false) => " !",
        (false, true, true) => " 决",
        (false, true, false) => " D",
        (false, false, _) => "",
    };
    let lv = format!(" Lv{}", p.level);
    let budget = width.saturating_sub(mark.width() + lv.width() + battle.width());
    let text = format!("{mark}{}{lv}{battle}", clip(&p.name, budget));
    let style = if p.in_battle {
        Style::new().fg(Color::DarkGray)
    } else if p.party.is_some() {
        Style::new().fg(Color::Yellow)
    } else {
        Style::new().fg(Color::White)
    };
    ListItem::new(text).style(style)
}

/// 按终端显示宽度截断，超出时末尾加 `…`（一格）。
fn clip(text: &str, max: usize) -> String {
    if text.width() <= max {
        return text.to_string();
    }
    if max == 0 {
        return String::new();
    }
    let mut out = String::new();
    let mut used = 0;
    for c in text.chars() {
        let cw = c.width().unwrap_or(0);
        if used + cw + 1 > max {
            break;
        }
        out.push(c);
        used += cw;
    }
    out.push('…');
    out
}

/// 底部的选中项说明。
pub fn summary(w: &WorldState) -> String {
    match w.scene.players.selected() {
        Some(p) => format!(
            "{} · Lv{} · 转生 {}{} · Enter 动作 · Tab 详情",
            p.name,
            p.level,
            p.transmigration,
            if p.in_battle { " · 战斗中" } else { "" }
        ),
        None => w
            .scene
            .players
            .placeholder()
            .unwrap_or_else(|| "没有选中玩家".into()),
    }
}

/// Tab 详情：名字、称号、等级、转生、队伍、战斗状态。
pub fn details(p: &NearbyPlayer) -> String {
    let title = if p.title.is_empty() { "无" } else { p.title.as_str() };
    // 队伍摘要 {"leader","size","max"}（STAGE-274）；字段缺失时只写“有”。
    let party = match &p.party {
        Some(v) => match (v["leader"].as_str(), v["size"].as_u64(), v["max"].as_u64()) {
            (Some(l), Some(n), Some(m)) => format!("有（队长 {l}，{n}/{m} 人）"),
            _ => "有".to_string(),
        },
        None => "无".to_string(),
    };
    format!(
        "{}\n等级 Lv{}\n转生 {} 次\n称号 {title}\n队伍 {party}\n状态 {}",
        p.name,
        p.level,
        p.transmigration,
        if p.in_battle { "战斗中" } else { "空闲" }
    )
}

/// 动作面板（Enter 打开）：灰掉的动作后面跟原因。
pub fn draw_prompt(frame: &mut Frame, w: &WorldState, area: Rect) {
    let Some(p) = w.scene.players.selected() else {
        return;
    };
    let Some(sel) = w.scene.players.prompt else {
        return;
    };
    let width = area.width.min(44).max(28);
    let height = ((PLAYER_ACTIONS.len() + 4) as u16).min(area.height);
    let popup = Rect {
        x: area.x + (area.width.saturating_sub(width)) / 2,
        y: area.y + (area.height.saturating_sub(height)) / 2,
        width,
        height,
    };
    frame.render_widget(Clear, popup);
    let mut lines = vec![Line::raw(format!("{} · Lv{}", p.name, p.level)), Line::raw("")];
    for (i, action) in PLAYER_ACTIONS.iter().enumerate() {
        let mark = if i == sel { "▶ " } else { "  " };
        let blocked = action.reason(w, p);
        let text = match blocked {
            Some(reason) => format!("{mark}{} · {reason}", action.label()),
            None => format!("{mark}{}", action.label()),
        };
        let style = if blocked.is_some() {
            Style::new().fg(Color::DarkGray)
        } else if i == sel {
            Style::new().fg(Color::Yellow)
        } else {
            Style::new()
        };
        lines.push(Line::raw(text).style(style));
    }
    frame.render_widget(
        Paragraph::new(lines)
            .wrap(Wrap { trim: true })
            .block(Block::bordered().title("附近玩家 · Esc 关闭")),
        popup,
    );
}

#[cfg(test)]
mod tests {
    use super::clip;

    #[test]
    fn clip_keeps_width_and_marks_cut() {
        assert_eq!(clip("阿牛", 4), "阿牛");
        assert_eq!(clip("勇者小明一号", 5), "勇者…");
        assert_eq!(clip("abc", 0), "");
    }
}
