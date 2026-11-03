//! 战斗中顶部的信息条（STAGE-106，`docs/battle-ui.md`「总体布局」）：战斗时菜单标签的那一行整条换成它。
//! 从左到右：回合和阶段、自己的血量和蓝量、出战宠物的血量、托管和 agent 操控的标记；放不下从右往左截掉。

use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

use super::card::hp_color;
use crate::app::battle::slots;
use crate::app::battle::{BattleState, Phase};
use crate::proto::BattleUnit;

/// 信息条里各段之间的分隔。
const SEP: &str = " │ ";

/// STAGE-250：内容区高度不低于这个值时，骑着宠物的战斗顶栏才用 2 行显示骑宠（整屏 27 行及以上，120×40 等大窗口都满足）。
pub const MOUNT_BAR_MIN_HEIGHT: u16 = 24;

/// 标准顶栏的高度（含边框）：内容区是整屏去掉这一条以后剩下的高度，80×24 时是 21 行。
const STANDARD_TOP_HEIGHT: u16 = 3;

/// STAGE-250：顶栏的高度（含边框）。战斗中骑着宠物、内容区够高时是 4 行（边框加 2 行内容），否则 3 行，与原来一致。
/// `area_height` 是整屏（布局拿到的 area）的高度。
pub fn top_height(b: Option<&BattleState>, area_height: u16) -> u16 {
    let content = area_height.saturating_sub(STANDARD_TOP_HEIGHT);
    match b {
        Some(b) if content >= MOUNT_BAR_MIN_HEIGHT && b.units.get(&b.you).is_some_and(|u| u.mount.is_some()) => 4,
        _ => STANDARD_TOP_HEIGHT,
    }
}

/// STAGE-250：顶栏第二行，骑着的宠物（`骑 贝鲁卡 HP 30/40`）。没骑乘时返回 None。
/// 名字过长时截成 `…`，血量不让步；骑宠归零时红字，后面写「已倒下，伤害全由人承担」。
pub fn mount_line(b: &BattleState, width: usize) -> Option<Line<'static>> {
    let m = b.units.get(&b.you)?.mount.as_ref()?;
    let down = m.hp_pct == 0;
    let hp = match (m.hp, m.max_hp) {
        (Some(hp), Some(max)) => format!("HP {hp}/{max}"),
        _ => format!("HP {}%", m.hp_pct),
    };
    let tail = if down { " 已倒下，伤害全由人承担" } else { "" };
    let color = if down { Color::Red } else { hp_color(m.hp_pct) };
    // 固定部分是前缀「 骑 」、名字后面的空格、血量和归零提示；名字只用剩下的宽度。
    let fixed = " 骑  ".width() + hp.width() + tail.width();
    let name = truncate(&m.name, width.saturating_sub(fixed));
    Some(Line::from(vec![
        Span::raw(" "),
        Span::styled("骑", Style::default().fg(Color::Cyan)),
        Span::raw(format!(" {name} ")),
        Span::styled(hp, Style::default().fg(color).add_modifier(Modifier::BOLD)),
        Span::styled(tail, Style::default().fg(Color::Red).add_modifier(Modifier::BOLD)),
    ]))
}

/// 名字按显示宽度截到 `max` 列，截掉的部分用 `…` 代替（`…` 占一列）。
fn truncate(name: &str, max: usize) -> String {
    if max == 0 {
        return String::new();
    }
    if name.width() <= max {
        return name.to_string();
    }
    let mut out = String::new();
    let mut w = 0;
    for c in name.chars() {
        let cw = c.width().unwrap_or(0);
        if w + cw + 1 > max {
            break;
        }
        out.push(c);
        w += cw;
    }
    out.push('…');
    out
}

/// 生成顶部信息条，`width` 是内部宽度（边框以内）。`observing` 是 agent 在操控，`auto_on` 是托管中。
pub fn line(b: &BattleState, observing: bool, auto_on: bool, width: usize) -> Line<'static> {
    let yellow = Style::default().fg(Color::Yellow);
    let mut segs: Vec<Vec<Span<'static>>> = vec![vec![Span::raw(format!("第 {} 回合 · {}", b.turn, phase_text(b)))]];
    if let Some(f) = b.field.as_ref().filter(|f| f.active()) {
        // 场地属性（STAGE-200）：双方共用，写在回合之后。
        let text = format!("场地 {} {}% 剩 {} 回合", f.attr_text(), f.pow, f.left);
        segs.push(vec![Span::styled(text, Style::default().fg(Color::Cyan))]);
    }
    if let Some(u) = b.units.get(&b.you) {
        segs.push(self_segment(u));
    }
    if let Some(pet) = b.unit_at(slots::cell_of(b.you + 5)) {
        segs.push(vec![Span::raw(format!("{} ", pet.name)), hp_span(pet)]);
    }
    if auto_on {
        segs.push(vec![Span::styled("自动挂机中", yellow)]);
    }
    if observing {
        segs.push(vec![Span::styled("agent 操控中", yellow)]);
    }
    // 放不下就从右边的段开始去掉，第一段（回合和阶段）留到最后。
    let mut n = segs.len();
    while n > 1 && build(&segs[..n]).width() > width {
        n -= 1;
    }
    build(&segs[..n])
}

/// 阶段的文字：人物指令 / 宠物指令 / 等待 2/3 / 结算中 3/7 / 战斗结束。
fn phase_text(b: &BattleState) -> String {
    match b.phase {
        Phase::Input(actor) => actor.title().to_string(),
        Phase::Waiting if b.progress.total > 1 => format!("等待 {}/{}", b.progress.done, b.progress.total),
        Phase::Waiting => "结算中".to_string(),
        Phase::Replay => super::bar::replay_title(b),
        Phase::Ended => "战斗结束".to_string(),
    }
}

/// 自己的一段：名字、血量和蓝量（协议只给自己的人物 `mp`）。
fn self_segment(u: &BattleUnit) -> Vec<Span<'static>> {
    let mut seg = vec![Span::raw(format!("{} ", u.name)), hp_span(u)];
    if let (Some(mp), Some(max_mp)) = (u.mp, u.max_mp) {
        seg.push(Span::raw(format!(" MP {mp}/{max_mp}")));
    }
    seg
}

/// 血量文字（己方的人和宠物都有数字），颜色按血条的规则。
fn hp_span(u: &BattleUnit) -> Span<'static> {
    let text = match (u.hp, u.max_hp) {
        (Some(hp), Some(max)) => format!("HP {hp}/{max}"),
        _ => format!("HP {}%", u.hp_pct),
    };
    Span::styled(text, Style::default().fg(hp_color(u.hp_pct)).add_modifier(Modifier::BOLD))
}

/// 把若干段用分隔拼成一行，左边留一格空白（和边框之间）。
fn build(segs: &[Vec<Span<'static>>]) -> Line<'static> {
    let mut spans = vec![Span::raw(" ")];
    for (i, seg) in segs.iter().enumerate() {
        if i > 0 {
            spans.push(Span::styled(SEP, Style::default().fg(Color::DarkGray)));
        }
        spans.extend(seg.iter().cloned());
    }
    Line::from(spans)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::app::battle::Actor;
    use crate::proto::{BattleAllow, BattleStart, Mount};

    /// 自己（勇者 45/60，蓝 100/100）和出战宠物（乌力 35/35）。
    fn battle() -> BattleState {
        let hero = BattleUnit {
            no: 0,
            kind: "player".into(),
            name: "勇者小明".into(),
            level: 12,
            hp: Some(45),
            max_hp: Some(60),
            hp_pct: 75,
            mp: Some(100),
            max_mp: Some(100),
            ..BattleUnit::default()
        };
        let pet = BattleUnit {
            no: 5,
            kind: "pet".into(),
            name: "乌力".into(),
            level: 5,
            hp: Some(35),
            max_hp: Some(35),
            hp_pct: 100,
            ..BattleUnit::default()
        };
        let start = BattleStart { party: None,
            battle_id: "b_1".into(),
            kind: "wild".into(),
            scene: "野外".into(),
            you: 0,
            allow: BattleAllow::default(),
            units: vec![hero, pet],
            npc: None,
            field: None,
        };
        let mut b = BattleState::from_start(start);
        b.turn = 3;
        b.phase = Phase::Input(Actor::Player);
        b
    }

    fn text(line: &Line) -> String {
        line.spans.iter().map(|s| s.content.as_ref()).collect()
    }

    #[test]
    fn wide_bar_shows_every_segment() {
        let mut b = battle();
        let s = text(&line(&b, false, true, 120));
        assert_eq!(s, " 第 3 回合 · 人物指令 │ 勇者小明 HP 45/60 MP 100/100 │ 乌力 HP 35/35 │ 自动挂机中");
        b.phase = Phase::Input(Actor::Pet);
        assert!(text(&line(&b, true, false, 120)).contains("宠物指令"));
        assert!(text(&line(&b, true, false, 120)).ends_with("agent 操控中"));
    }

    #[test]
    fn narrow_bar_drops_segments_from_the_right() {
        let b = battle();
        assert!(!text(&line(&b, false, true, 40)).contains("勇者"), "放不下自己的段就从右边去掉");
        let s = text(&line(&b, false, true, 60));
        assert!(s.contains("勇者小明") && !s.contains("乌力"), "{s}");
        let s = text(&line(&b, false, true, 70));
        assert!(s.contains("乌力") && !s.contains("自动挂机中"), "{s}");
        assert!(text(&line(&b, false, true, 5)).contains("人物指令"), "第一段留到最后");
    }

    #[test]
    fn field_shows_right_after_the_turn_and_only_when_it_has_an_attribute() {
        use crate::proto::BattleField;
        let mut b = battle();
        assert!(!text(&line(&b, false, false, 120)).contains("场地"), "无属性时不显示");
        b.field = Some(BattleField { attr: "fire".into(), pow: 60, left: 3 });
        let s = text(&line(&b, false, false, 120));
        assert!(s.starts_with(" 第 3 回合 · 人物指令 │ 场地 火 60% 剩 3 回合 │ 勇者小明"), "{s}");
        b.field = Some(BattleField { attr: "none".into(), ..BattleField::default() });
        assert!(!text(&line(&b, false, false, 120)).contains("场地"), "none 也是无属性");
    }

    #[test]
    fn mount_line_keeps_the_hp_and_cuts_the_name() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().ride = true;
        b.units.get_mut(&0).unwrap().mount = Some(Mount { name: "超级无敌长名字的贝鲁卡".into(), hp: Some(30), max_hp: Some(40), hp_pct: 75 });
        let s = text(&mount_line(&b, 30).unwrap());
        assert!(s.ends_with("HP 30/40") && s.contains('…'), "{s}");
        assert!(s.width() <= 30, "不超过给的宽度：{s}");
        assert_eq!(text(&mount_line(&b, 120).unwrap()), " 骑 超级无敌长名字的贝鲁卡 HP 30/40");
    }

    #[test]
    fn fainted_mount_says_damage_falls_on_the_player() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().mount = Some(Mount { name: "贝鲁卡".into(), hp: Some(0), max_hp: Some(40), hp_pct: 0 });
        let line = mount_line(&b, 120).unwrap();
        assert_eq!(text(&line), " 骑 贝鲁卡 HP 0/40 已倒下，伤害全由人承担");
        assert_eq!(line.spans.last().unwrap().style.fg, Some(Color::Red));
        b.units.get_mut(&0).unwrap().mount = None;
        assert!(mount_line(&b, 120).is_none(), "没骑乘没有第二行");
    }

    #[test]
    fn top_is_two_lines_only_while_riding_and_tall_enough() {
        let mut b = battle();
        assert_eq!(top_height(Some(&b), 40), 3, "没骑乘是 1 行");
        b.units.get_mut(&0).unwrap().mount = Some(Mount { name: "贝鲁卡".into(), hp: Some(30), max_hp: Some(40), hp_pct: 75 });
        assert_eq!(top_height(Some(&b), 40), 4);
        assert_eq!(top_height(Some(&b), 27), 4, "整屏 27 行，内容区 24 行");
        assert_eq!(top_height(Some(&b), 26), 3, "内容区 23 行，不够高");
        assert_eq!(top_height(Some(&b), 24), 3, "80×24");
        assert_eq!(top_height(None, 40), 3);
    }

    #[test]
    fn pet_segment_is_skipped_when_no_pet_is_out() {
        let mut b = battle();
        b.here.remove(&5);
        assert!(!text(&line(&b, false, false, 120)).contains("乌力"));
    }
}
