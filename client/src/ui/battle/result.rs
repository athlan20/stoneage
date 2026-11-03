//! 战斗结束的结果窗口（STAGE-39，`docs/battle-ui.md`「战斗结束」）：居中叠在战场上，Enter、Space 或 Esc 关闭。
//! 内容来自 `battle_end`（`docs/protocol.md` 10.9）：胜利写奖励，失败写被送回的地点和损失，逃跑和平局只有一句话。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Clear, Paragraph};

use crate::proto::{BattleEnd, DuelPoint, Penalty, Rewards};
use crate::ui::widgets::centered;

/// 窗口的宽度（列）；高度随内容变化。
const WIDTH: u16 = 50;

pub fn render(frame: &mut Frame, area: Rect, end: &BattleEnd, clones: bool, observing: bool) {
    let (title, color, mut lines) = content(end);
    // 带着分身队打赢时注明一句，免得玩家以为漏发了分身的奖励（STAGE-162）。
    if clones && end.result == "win" {
        lines.push(Line::styled("分身不领奖励：经验和掉落只给队长", Style::default().fg(Color::DarkGray)));
    }
    lines.push(Line::default());
    // STAGE-284：观察模式下结果窗口约 2.5 秒后自己关闭，提示里写出来。
    let hint = if observing { "[继续] Enter · Space/Esc 继续（约 2.5 秒后自动关闭）" } else { "[继续] Enter · Space/Esc 继续" };
    lines.push(Line::styled(hint, Style::default().fg(Color::Yellow)));
    let height = (lines.len() as u16 + 2).min(area.height);
    let rect = centered(area, WIDTH, height);
    frame.render_widget(Clear, rect);
    let block = Block::bordered().title(title).border_style(Style::default().fg(color).add_modifier(Modifier::BOLD));
    frame.render_widget(Paragraph::new(lines).block(block), rect);
}

fn content(end: &BattleEnd) -> (&'static str, Color, Vec<Line<'static>>) {
    if let Some(dp) = end.duel_point {
        return duel_content(&end.result, dp);
    }
    match end.result.as_str() {
        "win" => ("战斗胜利", Color::Green, win_lines(&end.rewards)),
        "lose" => ("战斗失败", Color::Red, lose_lines(end.penalty.as_ref())),
        "escaped" => ("逃离战斗", Color::Yellow, vec![Line::from("你逃离了战斗")]),
        // 真人队同场时超时没交指令（STAGE-275，10.9）：没有奖励，也没有惩罚。
        "forced_out" => ("被移出战斗", Color::Yellow, vec![Line::from("因超时被移出战斗，无奖励无惩罚")]),
        _ => ("战斗结束", Color::Yellow, vec![Line::from("没有分出胜负")]),
    }
}

/// 决斗（STAGE-307）：没有经验、石币和掉落，只写决斗点数的增减和现值（`docs/pk.md` 5.2）。
fn duel_content(result: &str, dp: DuelPoint) -> (&'static str, Color, Vec<Line<'static>>) {
    let (title, color) = match result {
        "win" => ("决斗胜利", Color::Green),
        "lose" => ("决斗失败", Color::Red),
        "escaped" => ("逃离决斗", Color::Yellow),
        "forced_out" => ("被移出决斗", Color::Yellow),
        _ => ("决斗结束", Color::Yellow),
    };
    let (change, style) = match dp.change {
        0 => ("不变".to_string(), Style::default()),
        c if c > 0 => (format!("+{c}"), Style::default().fg(Color::Green)),
        c => (c.to_string(), Style::default().fg(Color::Red)),
    };
    let line = Line::from(vec![
        Span::raw("决斗点数 "),
        Span::styled(change, style),
        Span::raw(format!("（现在 {}）", dp.total)),
    ]);
    (title, color, vec![line])
}

/// 胜利：每个人物和宠物一行经验，升级时用黄色标出“Lv 12 → 13”和获得的能力点，然后是石币、掉落和捕获。
fn win_lines(r: &Rewards) -> Vec<Line<'static>> {
    let mut out = vec![];
    for g in &r.exp {
        let who = if g.who == "pet" { format!("宠物 {}", g.name) } else { g.name.clone() };
        let mut spans = vec![Span::raw(format!("{who}  经验 +{}", g.gain))];
        if g.level_to > g.level_from {
            spans.push(Span::styled(
                format!("  Lv {} → {}", g.level_from, g.level_to),
                Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD),
            ));
        }
        if g.skill_points > 0 {
            spans.push(Span::raw(format!("  能力点 +{}", g.skill_points)));
        }
        out.push(Line::from(spans));
    }
    if r.gold > 0 {
        out.push(Line::from(format!("石币 +{}", r.gold)));
    }
    for it in &r.items {
        out.push(Line::from(format!("获得 {} ×{}", it.name, it.count)));
    }
    for p in &r.pets {
        out.push(Line::from(format!("捕获 {} Lv{}（宠物栏 {}）", p.name, p.level, p.slot + 1)));
    }
    if out.is_empty() {
        out.push(Line::from("没有奖励"));
    }
    out
}

/// 失败：被打死原地复活（体力剩 1，位置不变），或被打飞送回的地点，以及按原版规则的损失。
fn lose_lines(p: Option<&Penalty>) -> Vec<Line<'static>> {
    let Some(p) = p else {
        return vec![Line::from("你倒下了")];
    };
    let mut out = vec![];
    if p.in_place {
        out.push(Line::from("原地复活，体力剩 1"));
    } else if let Some(pos) = &p.respawn {
        out.push(Line::from(format!("被送回：{}", pos.map_name)));
    }
    out.push(Line::from(format!("损失石币 {}，经验 {}", p.gold_lost, p.exp_lost)));
    out
}
