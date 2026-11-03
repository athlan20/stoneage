//! 主界面第 6 页「社交」（STAGE-162，`docs/ui.md`「社交」）：目前只有「队伍」一栏（名片簿、邮件、社交开关还没做）。
//! 没有队伍时是一行入口「组成分身队」加规则说明；有队伍时是带黄色表头的成员表，宽屏（≥110 列）右边并排显示选中成员的详情，
//! 窄屏时详情只在 `Layer::Detail` 里整页替换列表。`Enter` 打开动作菜单（面板画在下方，和宠物页一样）。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Cell, List, ListItem, Paragraph, Row, Table, Wrap};

use super::widgets::{select_mark, selected_style};
use crate::app::{App, PartyStep, WorldState, party_detail_rows, party_menu};
use crate::proto::{Party, PartyMember};
use crate::app::party_players;
use crate::app::social_settings;
use super::social_settings as settings_view;
use std::time::Instant;

const HEADER_STYLE: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);
const LABEL_STYLE: Style = Style::new().fg(Color::DarkGray);
/// 成员表的宽度（列）：宽屏时右边留给详情。
const TABLE_WIDTH: u16 = 78;
/// HP 迷你条的宽度。
const BAR_WIDTH: usize = 6;

const RULES: [&str; 3] = [
    "你当队长，另外四个是“当前状态的自己”的分身，一起走路、一起打架。分身会复制等级、能力、HP/MP、装备、精灵和出战宠物（创建那一刻的快照），不复制背包、石币和任务进度。",
    "分身自动出招，只会普通攻击，不领奖励：经验和掉落只给队长。",
    "队长解散队伍、所有连接断开，分身就全部消失，重新登录不保留。",
];

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    let unicode = app.unicode;
    let Some(world) = app.world.as_ref() else {
        return;
    };
    // 动作流程开着时，底部留出面板。
    let panel = match world.party_step {
        Some(PartyStep::Actions { member, .. }) => {
            (party_menu(world, member).len() as u16 + 2).min(area.height.saturating_sub(6))
        }
        _ => 0,
    };
    let [body, panel_area] = Layout::vertical([Constraint::Min(0), Constraint::Length(panel)]).areas(area);
    if party_players::on_request_column(world) {
        render_requests(frame, world, body);
        if panel > 0 {
            render_actions(frame, world, panel_area);
        }
        return;
    }
    if social_settings::on_settings_column(world) {
        settings_view::render(frame, world, body);
        return;
    }
    match (world.party(), world.party_step.as_ref()) {
        (None, _) => render_entry(frame, world, body),
        (Some(p), Some(PartyStep::Detail { member, scroll })) => match p.member(*member) {
            Some(m) => render_detail(frame, m, *scroll, body),
            None => render_table(frame, world, p, unicode, body),
        },
        (Some(p), _) if world.is_narrow() => render_table(frame, world, p, unicode, body),
        (Some(p), _) => {
            let [list, side] = Layout::horizontal([Constraint::Length(TABLE_WIDTH), Constraint::Min(0)]).areas(body);
            render_table(frame, world, p, unicode, list);
            let selected = world.selected_party_index().and_then(|i| p.member(i));
            if let Some(m) = selected.filter(|_| p.kind != "players") {
                render_detail(frame, m, 0, side);
            }
        }
    }
    if panel > 0 {
        render_actions(frame, world, panel_area);
    }
}

/// 没有队伍：入口一行（不能组队时灰显并写明原因）加规则说明。
fn render_entry(frame: &mut Frame, world: &WorldState, area: Rect) {
    let block = Block::bordered().title("队伍 · 还没有队伍");
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let reason = if !world.online {
        Some("连接已断开")
    } else if world.observing() {
        Some("agent 正在操控，输入 take 收回控制权")
    } else {
        world.clone_blocked()
    };
    let entry = match reason {
        None => Line::from(Span::styled(
            format!("{} 组成分身队", select_mark(true)),
            selected_style(true),
        )),
        Some(why) => Line::from(vec![
            Span::styled(format!("{} 组成分身队", select_mark(true)), LABEL_STYLE),
            Span::styled(format!("  （{why}）"), LABEL_STYLE),
        ]),
    };
    let [row, text] = Layout::vertical([Constraint::Length(2), Constraint::Min(0)]).areas(inner);
    frame.render_widget(Paragraph::new(entry), row);
    let lines: Vec<Line> = RULES.iter().flat_map(|r| [Line::from(*r), Line::default()]).collect();
    frame.render_widget(Paragraph::new(lines).wrap(Wrap { trim: true }).style(LABEL_STYLE), text);
}

/// 成员表：序号、名字（队长、分身带标记）、Lv、HP（迷你条加数字）、MP、出战宠物。
fn render_table(frame: &mut Frame, world: &WorldState, party: &Party, unicode: bool, area: Rect) {
    let selected = world.selected_party_index();
    let header = Row::new(["", "#", "名字", "Lv", "HP", "MP", "出战宠物"]).style(HEADER_STYLE);
    let rows: Vec<Row> = party
        .members
        .iter()
        .map(|m| {
            let on = selected == Some(m.index);
            let tag = if m.leader {
                Span::styled(" [队长]", Style::default().fg(Color::Yellow))
            } else if m.clone {
                Span::styled(" [分身]", Style::default().fg(Color::Cyan))
            } else {
                Span::styled(" [队员]", Style::default().fg(Color::Cyan))
            };
            let pet = m.pet.as_ref().map_or_else(
                || "—".to_string(),
                |p| format!("{} Lv{} {}/{}", p.name, p.level, p.hp, p.max_hp),
            );
            let row = Row::new(vec![
                Cell::from(select_mark(on)),
                Cell::from((m.index + 1).to_string()),
                Cell::from(Line::from(vec![Span::raw(m.name.clone()), tag, battle_tag(m)])),
                Cell::from(m.level.to_string()),
                Cell::from(hp_line(m, unicode)),
                Cell::from(format!("{}/{}", m.mp, m.max_mp)),
                Cell::from(pet),
            ]);
            if on { row.style(selected_style(true)) } else { row }
        })
        .collect();
    let widths = [
        Constraint::Length(1),
        Constraint::Length(2),
        Constraint::Length(22),
        Constraint::Length(3),
        Constraint::Length(BAR_WIDTH as u16 + 8),
        Constraint::Length(7),
        Constraint::Min(8),
    ];
    let title = if party.kind == "players" {
        format!("队伍 · {}/5 真人", party.size())
    } else {
        format!(
            "队伍 · {} 队长 + {} 分身",
            party.size() - party.clones(),
            party.clones()
        )
    };
    frame.render_widget(
        Table::new(rows, widths)
            .header(header)
            .block(Block::bordered().title(title)),
        area,
    );
}

/// HP 迷你条加 `当前/上限`：满格绿、过半黄、不到两成红，倒下是灰的。
fn hp_line(m: &PartyMember, unicode: bool) -> Line<'static> {
    let pct = if m.max_hp == 0 {
        0
    } else {
        (u64::from(m.hp) * 100 / u64::from(m.max_hp)).min(100) as usize
    };
    let filled = (pct * BAR_WIDTH).div_ceil(100).min(BAR_WIDTH);
    let (full, rest) = if unicode { ("█", "░") } else { ("#", "-") };
    let color = match pct {
        0 => Color::DarkGray,
        1..=20 => Color::Red,
        21..=50 => Color::Yellow,
        _ => Color::Green,
    };
    Line::from(vec![
        Span::styled(full.repeat(filled), Style::default().fg(color)),
        Span::styled(rest.repeat(BAR_WIDTH - filled), LABEL_STYLE),
        Span::raw(format!(" {}/{}", m.hp, m.max_hp)),
    ])
}

/// 成员详情：一行「名 值」，宽屏时并排显示、`Layer::Detail` 时整页显示（可滚动）。
fn render_detail(frame: &mut Frame, m: &PartyMember, scroll: usize, area: Rect) {
    let kind = if m.leader { "队长" } else { "分身" };
    let block = Block::bordered().title(format!("{} · {kind}", m.name));
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let lines: Vec<Line> = party_detail_rows(m)
        .into_iter()
        .map(|(label, value)| Line::from(vec![Span::styled(format!(" {label}  "), LABEL_STYLE), Span::raw(value)]))
        .collect();
    let scroll = scroll.min(lines.len().saturating_sub(1)) as u16;
    frame.render_widget(
        Paragraph::new(lines).wrap(Wrap { trim: false }).scroll((scroll, 0)),
        inner,
    );
}

/// 动作面板：做不了的动作保留显示，后面用灰字解释原因。
fn render_actions(frame: &mut Frame, world: &WorldState, area: Rect) {
    let Some(PartyStep::Actions { member, index }) = world.party_step.as_ref() else {
        return;
    };
    let name = world
        .party()
        .and_then(|p| p.member(*member))
        .map(|m| m.name.clone())
        .unwrap_or_default();
    let rows: Vec<ListItem> = party_menu(world, *member)
        .into_iter()
        .enumerate()
        .map(|(i, (act, reason))| {
            let mut spans = vec![Span::raw(act.label())];
            if let Some(reason) = reason {
                spans.push(Span::styled(format!("  （{reason}）"), LABEL_STYLE));
            }
            let style = if i == *index {
                selected_style(true)
            } else if reason.is_some() {
                LABEL_STYLE
            } else {
                Style::default()
            };
            ListItem::new(Line::from(spans)).style(style)
        })
        .collect();
    frame.render_widget(
        List::new(rows).block(Block::bordered().title(format!("动作 · {name}"))),
        area,
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    fn text(line: Line<'static>) -> String {
        line.spans.iter().map(|s| s.content.to_string()).collect()
    }

    fn member(hp: u32, max: u32) -> PartyMember {
        PartyMember {
            hp,
            max_hp: max,
            ..Default::default()
        }
    }

    #[test]
    fn hp_bar_fills_proportionally_and_falls_back_to_ascii() {
        assert_eq!(text(hp_line(&member(44, 44), true)), "██████ 44/44");
        assert_eq!(text(hp_line(&member(22, 44), true)), "███░░░ 22/44");
        assert_eq!(text(hp_line(&member(22, 44), false)), "###--- 22/44");
        assert_eq!(text(hp_line(&member(0, 44), false)), "------ 0/44");
        assert_eq!(text(hp_line(&member(0, 0), false)), "------ 0/0", "上限为 0 不能除零");
    }
}

/// 成员战斗中的标记（服务端给了 `in_battle` 才有）。
fn battle_tag(m: &PartyMember) -> Span<'static> {
    if m.in_battle == Some(true) {
        Span::styled(" 战斗中", Style::default().fg(Color::Red))
    } else {
        Span::raw("")
    }
}

/// 「请求」栏（STAGE-274）：收到的（可答复）和发出的（可撤回），每条带剩余秒数（`docs/ui.md`）。
fn render_requests(frame: &mut Frame, world: &WorldState, area: Rect) {
    let now = Instant::now();
    let r = &world.party_requests;
    let selected = party_players::selected_request(world);
    let incoming = r.incoming.iter().map(|p| ("收到", p));
    let outgoing = r.outgoing.iter().map(|p| ("发出", p));
    let rows: Vec<Row> = incoming
        .chain(outgoing)
        .enumerate()
        .map(|(i, (dir, p))| {
            let on = selected == Some(i);
            let kind = match p.req.kind.as_str() {
                "apply" => "申请",
                crate::app::trade::TRADE_KIND => "易 交易",
                _ => "邀请",
            };
            let who = if dir == "收到" { &p.req.from } else { &p.req.to };
            let status = if p.answering {
                "处理中"
            } else if dir == "收到" {
                "Enter 打开弹窗"
            } else {
                "Enter 撤回"
            };
            let row = Row::new(vec![
                Cell::from(select_mark(on)),
                Cell::from(dir),
                Cell::from(kind),
                Cell::from(who.clone()),
                Cell::from(format!("{} 秒", p.remaining_secs(now))),
                Cell::from(status),
            ]);
            if on { row.style(selected_style(true)) } else { row }
        })
        .collect();
    let title = format!("请求 · 收到 {} · 发出 {}", r.incoming.len(), r.outgoing.len());
    if rows.is_empty() {
        let block = Block::bordered().title(title);
        let inner = block.inner(area);
        frame.render_widget(block, area);
        frame.render_widget(Paragraph::new("没有待处理的组队请求").style(LABEL_STYLE), inner);
        return;
    }
    let header = Row::new(["", "方向", "类型", "对方", "剩余", "状态"]).style(HEADER_STYLE);
    let widths = [
        Constraint::Length(1),
        Constraint::Length(4),
        Constraint::Length(4),
        Constraint::Length(16),
        Constraint::Length(8),
        Constraint::Min(8),
    ];
    frame.render_widget(
        Table::new(rows, widths)
            .header(header)
            .block(Block::bordered().title(title)),
        area,
    );
}
