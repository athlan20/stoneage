//! 指令条（`docs/battle-ui.md`「指令输入」）：战场最下面一行，写当前轮到谁下指令、可选的项，
//! 不是自己下指令时写“等待其他玩家”或“结算中”。

use std::time::Instant;

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::Paragraph;
use unicode_width::UnicodeWidthStr;


use crate::app::battle::{Actor, BattleState, Phase, Pick, slots};
use crate::app::{App, WorldState};
use crate::proto::TargetType;

/// `target_info`：没有右侧栏时，选目标的目标信息（名字、等级、血量）写在这一行里。
pub fn render(frame: &mut Frame, app: &App, b: &BattleState, area: Rect, target_info: bool) {
    // 托管中（STAGE-173）：不再摆动作网格——一开托管就不再请玩家选行动，
    // 指令条只报“自动战斗中”和两个出口（Tab 改设置、8 停止）。
    if matches!(b.phase, Phase::Input(_))
        && app.world.as_ref().is_some_and(|w| w.auto_battle)
        && !b.submitting
    {
        frame.render_widget(Paragraph::new(auto_lines(b)), area);
        return;
    }
    if let Phase::Input(actor)=b.phase {
        if b.pick==Pick::Action && area.height>=3 && !b.submitting {
            let Some(w)=app.world.as_ref() else{return;};
            let who=if b.revising{"人物指令（改）"}else{actor.title()};
            let clones=b.has_optional();
            let back=match (b.can_revert(),clones){(true,true)=>" · Esc 改人物",(true,false)=>" · Esc 改人物指令",_=>""};
            let clone_key=if clones{" · f 分身"}else{""};
            let mut lines=vec![Line::raw(format!("{who} · ↑↓←→ 选择 · Enter 确认{back}{clone_key} · Tab 控制 · / 命令"))];
            lines.extend(grid_lines(app, w, b, actor));
            let focus_slot=b.menu(actor).get(b.action_focus).copied();
            if focus_slot.is_some_and(|s|!b.slot_enabled(w,actor,s)){lines[0]=Line::raw("本回合条件不足，Enter 查看原因 · Tab 控制 · / 命令");}
            if w.observing(){lines[0]=Line::styled("agent 出招中 · Tab 控制 · / 命令",Style::default().fg(Color::Yellow));}
            frame.render_widget(Paragraph::new(lines),area);return;
        }
    }
    let observing = app.world.as_ref().is_some_and(WorldState::observing);
    frame.render_widget(Paragraph::new(text(app, b, observing, target_info, None)), area);
}

/// 托管中的指令条：第一行是状态与两个出口，第二行说明怎么回到手动。
fn auto_lines(b: &BattleState) -> Vec<Line<'static>> {
    let eta = match b.auto_due {
        Some(due) => {
            let left = due.saturating_duration_since(Instant::now());
            if left.is_zero() { "正在出招".to_string() } else { format!("{:.1}s 后出招", left.as_secs_f32()) }
        }
        None => "准备出招".to_string(),
    };
    vec![
        Line::styled(
            format!(" 自动挂机中 · {eta} · 8 停止挂机 · 7 停止遇敌 · Tab 设置"),
            Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD),
        ),
        Line::from(Span::styled(
            " 要手动打：先按 8 停止，这一场继续手动打完",
            Style::default().fg(Color::DarkGray),
        )),
    ]
}

/// 动作层的两行网格（STAGE-153）：行列按下标算，和方向键、数字键共用 `BattleState::grid`。
/// 每格「编号 名字」按最宽的那个名字补空格对齐，宽度按终端显示列算（中文占两列）。
fn grid_lines(app: &App, w: &WorldState, b: &BattleState, actor: Actor) -> Vec<Line<'static>> {
    let grid = b.grid(actor);
    let auto_on = w.auto_battle;
    let menu = b.menu(actor);
    let widest = menu.iter().map(|s| UnicodeWidthStr::width(s.label(auto_on))).max().unwrap_or(0);
    let marker = if app.unicode { "›" } else { ">" };
    (0..grid.rows())
        .map(|row| {
            let spans = (0..grid.row_len(row))
                .filter_map(|col| grid.index(row, col))
                .map(|index| {
                    let slot = menu[index];
                    let style = if index == b.action_focus {
                        Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD | Modifier::REVERSED)
                    } else if b.slot_enabled(w, actor, slot) {
                        Style::default()
                    } else {
                        Style::default().fg(Color::DarkGray)
                    };
                    let lead = if index == b.action_focus { marker } else { " " };
                    Span::styled(format!("{lead}{} {}  ", index + 1, pad(slot.label(auto_on), widest)), style)
                })
                .collect::<Vec<_>>();
            Line::from(spans)
        })
        .collect()
}

/// 按终端显示列补空格（中文两列），让上下两行的格子对齐。
fn pad(text: &str, width: usize) -> String {
    let mut s = text.to_string();
    let mut now = UnicodeWidthStr::width(text);
    while now < width {
        s.push(' ');
        now += 1;
    }
    s
}

/// 结算的标题：`结算中 3/7`（第几条、一共几条），没有进度时只写“结算中”。
pub fn replay_title(b: &BattleState) -> String {
    match b.replay_progress() {
        Some((done, total)) => format!("结算中 {done}/{total}"),
        None => "结算中".to_string(),
    }
}

fn text(app: &App, b: &BattleState, observing: bool, target_info: bool, _caption_room: Option<usize>) -> Line<'static> {
    // 托管中（STAGE-42）：指令条开头的黄色标记，玩家仍可手动出招接管。
    let auto_on = app.world.as_ref().is_some_and(|w| w.auto_battle);
    match b.phase {
        Phase::Ended if observing => Line::from(" 战斗结束 · 约 2.5 秒后自动关闭"), // STAGE-284
        Phase::Ended => Line::from(" 战斗结束 · 按 Enter 关闭"),
        // 自动战斗时（STAGE-173）：结算阶段只写状态与停止入口，不写进度和按键。
        Phase::Replay if auto_on => Line::styled(" 自动挂机中 · 8 停止自动挂机 · 7 停止自动遇敌", Style::default().fg(Color::Yellow)),
        Phase::Replay => Line::raw(format!(" {}  ", replay_title(b))),
        Phase::Waiting if auto_on => Line::styled(" 自动挂机中 · 8 停止自动挂机 · 7 停止自动遇敌", Style::default().fg(Color::Yellow)),
        Phase::Waiting => {
            let p = b.progress;
            if p.total > 1 && p.done < p.total {
                Line::raw(format!(" 等待其他玩家 {}/{}", p.done, p.total))
            } else {
                Line::raw(" 结算中…")
            }
        }
        Phase::Input(_) if observing => Line::styled(" agent 出招中", Style::default().fg(Color::Yellow)),
        // 托管中（STAGE-173）：指令条只报状态与出口，不列动作（窄到画不出网格时走这里）。
        Phase::Input(_) if auto_on => auto_lines(b).into_iter().next().unwrap_or_default(),
        Phase::Input(actor) => input_line(app, b, actor, target_info),
    }
}

/// 轮到自己下指令时的指令条：按当前的选择（指令、道具、换宠、技能或目标）列出可选的项。
fn input_line(app: &App, b: &BattleState, actor: Actor, target_info: bool) -> Line<'static> {
    let dim = Style::default().fg(Color::DarkGray);
    let key = Style::default().fg(Color::Yellow);
    let mut spans = vec![];
    if b.submitting {
        spans.push(Span::styled(" 发送中… ", dim));
    }
    let Some(w) = app.world.as_ref() else {
        return Line::from(spans);
    };
    match b.pick {
        Pick::Action => {
            // 从宠物指令退回来重选时写“人物指令（改）”，提醒玩家先交的那条仍然有效（STAGE-109）。
            let title = if b.revising { "人物指令（改）" } else { actor.title() };
            spans.push(Span::styled(format!(" {title}  "), key));
            let auto_on = w.auto_battle;
            for (i, &slot) in b.menu(actor).iter().enumerate() {
                let style = if !b.slot_enabled(w, actor, slot) {
                    dim
                } else if i == b.action_focus {
                    Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD | Modifier::REVERSED)
                } else {
                    Style::default()
                };
                spans.push(Span::styled(format!("{} {}", i + 1, slot.label(auto_on)), style));
                spans.push(Span::raw("  "));
            }
        }
        // 列表的内容在右侧（或浮层）的列表面板里（STAGE-107），这里只写怎么选。
        Pick::Item | Pick::Magic | Pick::Pet | Pick::Skill => {
            let (title, hint) = list_hint(b.pick);
            spans.push(Span::styled(format!(" {title}  "), key));
            spans.push(Span::styled(hint, dim));
        }
        // 分身面板在战场右侧（clone_panel.rs），这里只写怎么选。
        Pick::Clones => {
            spans.push(Span::styled(" 分身  ", key));
            spans.push(Span::styled("数字 1–8 直选 · ↑↓ Enter · Esc 返回（不选的按合击顺序打第一个）", dim));
        }
        Pick::CloneAim { who } => {
            let head = match (target_info, b.unit_at(b.cursor)) {
                (true, Some(u)) => {
                    format!(" {} 选目标  {} {} Lv.{} 血 {}%  ", b.name(who), slots::label(b.me_side(), u.no), u.name, u.level, u.hp_pct)
                }
                _ => format!(" {} 选目标  ", b.name(who)),
            };
            spans.push(Span::styled(format!("{head}只能选敌方 · 数字或方向键 · Enter 确认 · Esc 返回"), key));
        }
        Pick::Target { kind, .. } => {
            // 能选的只有一方（由目标类型决定），写在指令条上。
            let only = match kind {
                TargetType::Ally => "只能选己方",
                TargetType::AllyDown => "只能选倒下的己方",
                TargetType::Any => "己方敌方都能选",
                _ => "只能选敌方",
            };
            // 目标信息放在最前面：放不下时从右边截掉的是按键提示。
            let head = match (target_info, b.unit_at(b.cursor)) {
                (true, Some(u)) => {
                    format!(" 选目标  {} {} Lv.{} 血 {}%  ", slots::label(b.me_side(), u.no), u.name, u.level, u.hp_pct)
                }
                _ => " 选目标  ".to_string(),
            };
            spans.push(Span::styled(format!("{head}{only} · 数字或方向键 · Enter 确认 · Esc 返回"), key));
        }
    }
    Line::from(spans)
}

/// 列表选择时指令条上的标题和按键提示：道具不直选编号，精灵、换宠、技能直选（`docs/battle-ui.md`「编号规则」）。
fn list_hint(pick: Pick) -> (&'static str, &'static str) {
    match pick {
        Pick::Item => ("道具", "↑↓ 选择 · Enter 确认 · Esc 返回"),
        Pick::Magic => ("精灵", "数字 0–4 直选 · ↑↓ Enter · Esc 返回"),
        Pick::Pet => ("换宠", "数字 0–9 直选 · ↑↓ 选 10–19 · Enter · Esc 返回"),
        Pick::Skill => ("宠物技能", "数字 1–7 直选 · ↑↓ Enter · Esc 返回"),
        Pick::Action | Pick::Target { .. } | Pick::Clones | Pick::CloneAim { .. } => ("", ""),
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use super::*;
    use crate::app::{Options, WorldState};
    use crate::proto::{BattleAllow, BattleEffect, BattleEvent, BattleRound, BattleStart, BattleUnit};

    /// 回放到命中拍的战斗：我方 0 号攻击敌方 15 号，打掉 23 点。
    fn replaying() -> BattleState {
        let unit =
            |no, name: &str| BattleUnit { no, name: name.into(), level: 3, hp_pct: 100, ..BattleUnit::default() };
        let start = BattleStart {
            scene: "野外".into(),
            you: 0,
            allow: BattleAllow::default(),
            units: vec![unit(0, "勇者小明"), unit(15, "野猪")],
            ..BattleStart::default()
        };
        let mut b = BattleState::from_start(start);
        let ev = BattleEvent {
            kind: "attack".into(),
            actor: Some(0),
            effects: vec![BattleEffect {
                target: 15,
                result: "hit".into(),
                delta: -23,
                hp_pct: Some(70),
                ..BattleEffect::default()
            }],
            ..BattleEvent::default()
        };
        let t0 = Instant::now();
        b.on_round(BattleRound { turn: 1, events: vec![ev], ..BattleRound::default() }, t0);
        assert!(!b.advance(t0));
        assert!(!b.advance(t0 + Duration::from_millis(250)));
        b
    }

    fn plain(line: &Line<'static>) -> String {
        line.spans.iter().map(|s| s.content.to_string()).collect()
    }

    #[test]
    fn replay_bar_never_draws_the_action_caption() {
        let app = App::new(Options { unicode: true, ..Options::default() });
        let b = replaying();
        let line = plain(&text(&app, &b, false, false, Some(120)));
        assert!(!line.contains("━━") && !line.contains("勇者小明"), "战报只进日志：{line}");
        // 回放速度和“跳过”只在系统页设（STAGE-173），指令条上不再有按键提示。
        assert!(!line.contains("跳过") && !line.contains("倍速"), "回放阶段没有按键提示：{line}");
    }

    /// 托管中结算阶段（STAGE-173）：指令条只写状态与停止入口。
    #[test]
    fn auto_battle_replay_bar_only_shows_the_stop_hint() {
        let mut app = App::new(Options { unicode: true, ..Options::default() });
        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.auto_battle = true;
        app.world = Some(w);
        let b = replaying();
        let line = plain(&text(&app, &b, false, false, None));
        assert_eq!(line, " 自动挂机中 · 8 停止自动挂机 · 7 停止自动遇敌", "{line}");
    }

    #[test]
    fn bar_has_no_caption_when_the_field_draws_it() {
        let app = App::new(Options { unicode: true, ..Options::default() });
        let b = replaying();
        let line = plain(&text(&app, &b, false, false, None));
        assert!(!line.contains("━━"), "{line}");
        assert!(line.starts_with(" 结算中 1/1"), "{line}");
    }
}
