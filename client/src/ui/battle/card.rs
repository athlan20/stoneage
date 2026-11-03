//! STAGE-167：角色标签、状态、血量文字与显示宽度处理；身体绘制在 actor.rs。
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};

use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

use crate::app::battle::FloatKind;
use crate::proto::BattleUnit;

/// 完整档第 1 行：`Lv.5 名字 ....标记`（STAGE-173：不再带「敌 / 我 + 编号」前缀）。
/// 等级在名字前面；放不下时先让出等级、保住名字与状态标记。
pub(super) fn name_line(
    name: &str,
    level: &str,
    state: &str,
    style: Style,
    level_style: Style,
    state_style: Style,
    width: usize,
) -> Line<'static> {
    let state_w = UnicodeWidthStr::width(state);
    let state_room = if state_w > 0 { state_w + 1 } else { 0 };
    let name_w = UnicodeWidthStr::width(name);
    let lvl_w = UnicodeWidthStr::width(level);
    // 等级在名字前面，但**不能挤掉名字**：先试「Lv.5 名字」，再试省掉空格的「Lv.5名字」，
    // 都放不下就不显示等级（窄卡片优先保住名字与状态标记）。
    let spaced = lvl_w > 0 && lvl_w + 1 + name_w + state_room <= width;
    let tight = !spaced && lvl_w > 0 && lvl_w + name_w + state_room <= width;
    let (keep_lvl, lvl_room) = (spaced || tight, if spaced { lvl_w + 1 } else if tight { lvl_w } else { 0 });
    let room = width.saturating_sub(state_room + lvl_room);
    let shown = clip(name, room);
    let gap = width.saturating_sub(UnicodeWidthStr::width(shown.as_str()) + lvl_room + state_w);
    let mut spans = Vec::new();
    if keep_lvl {
        spans.push(Span::styled(level.to_string(), level_style));
        if spaced {
            spans.push(Span::raw(" "));
        }
    }
    spans.push(Span::styled(shown, style));
    spans.push(Span::raw(" ".repeat(gap)));
    spans.push(Span::styled(state.to_string(), state_style));
    Line::from(spans)
}

/// 血量文字：自己的单位显示数字，对方只显示百分比（协议 10.2 的隐藏规则）。
pub(super) fn hp_text(unit: &BattleUnit, mine: bool) -> String {
    match (mine, unit.hp, unit.max_hp) {
        (true, Some(hp), Some(max)) => format!("{hp}/{max}"),
        _ => format!("{}%", unit.hp_pct),
    }
}

/// 血量的颜色：> 50% 绿，20%–50% 黄，< 20% 红（血条和顶部信息条共用）。
pub fn hp_color(pct: u32) -> Color {
    if pct > 50 {
        Color::Green
    } else if pct > 20 {
        Color::Yellow
    } else {
        Color::Red
    }
}

/// 状态标记：毒、麻、睡、石、醉、乱、虚、障、默（STAGE-201），防御 ◇，骑乘 骑，增益 +，减益 -，防御次数吸/反/无（STAGE-199），属性反转 反（STAGE-200）；打飞的单位再加一个离场箭头。
pub(super) fn marks(unit: &BattleUnit, flown: bool, unicode: bool) -> String {
    let mut s = String::new();
    for st in &unit.status {
        s.push_str(match st.as_str() {
            "poison" => "毒",
            "paralysis" => "麻",
            "sleep" => "睡",
            "stone" => "石",
            "drunk" => "醉",
            "confusion" => "乱",
            "weaken" => "虚",
            "barrier" => "障",
            "nocast" => "默",
            "guard" => "◇",
            "reversed" => "反",
            "buff" => "+",
            "debuff" => "-",
            "absorb" => "吸",
            "reflect" => "反",
            "vanish" => "无",
            // STAGE-229：魔抗系（STAGE-228）；火、电、冰抗只占状态槽，标记和魔抗一样各一个字。
            "magic_resist" => "魔",
            "fire_resist" => "火",
            "thunder_resist" => "电",
            "ice_resist" => "冰",
            _ => "",
        });
    }
    if unit.ride {
        s.push('骑');
    }
    if flown {
        s.push(if unicode { '↗' } else { '>' });
    }
    s
}

pub(super) fn float_style(kind: FloatKind) -> Style {
    match kind {
        FloatKind::Damage => Style::default().fg(Color::Red),
        FloatKind::Crit => Style::default()
            .fg(Color::Yellow)
            .add_modifier(Modifier::BOLD),
        FloatKind::Miss | FloatKind::Block => Style::default().fg(Color::DarkGray),
        FloatKind::Heal => Style::default().fg(Color::Green),
    }
}

/// 按显示宽度截断（中文占 2 列）。
pub(super) fn clip(text: &str, width: usize) -> String {
    let mut out = String::new();
    let mut used = 0;
    for c in text.chars() {
        let w = UnicodeWidthChar::width(c).unwrap_or(0);
        if used + w > width {
            break;
        }
        used += w;
        out.push(c);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn clip_counts_wide_characters_as_two_columns() {
        assert_eq!(clip("勇者小明一二", 6), "勇者小");
        assert_eq!(clip("abc", 2), "ab");
    }

    #[test]
    fn hp_text_hides_enemy_numbers() {
        let mine = BattleUnit {
            hp: Some(45),
            max_hp: Some(60),
            hp_pct: 75,
            ..BattleUnit::default()
        };
        assert_eq!(hp_text(&mine, true), "45/60");
        assert_eq!(hp_text(&mine, false), "75%");
    }

    #[test]
    fn marks_follow_status_names() {
        let u = BattleUnit {
            status: vec!["poison".into(), "guard".into(), "sleep".into()],
            ride: true,
            ..BattleUnit::default()
        };
        assert_eq!(marks(&u, false, true), "毒◇睡骑");
        assert_eq!(
            marks(&u, true, true),
            "毒◇睡骑↗",
            "打飞的单位多一个离场箭头"
        );
        assert_eq!(marks(&u, true, false), "毒◇睡骑>", "关掉 Unicode 用 ASCII");
    }

    /// STAGE-199：防御次数标记各占一个字（吸、反、无），和毒、睡等排在一起。
    #[test]
    fn defense_marks_are_one_char_each() {
        let u = BattleUnit {
            status: vec!["absorb".into(), "reflect".into(), "vanish".into()],
            ..BattleUnit::default()
        };
        assert_eq!(marks(&u, false, true), "吸反无");
    }

    #[test]
    fn reversed_units_get_the_fan_mark() {
        let u = BattleUnit { status: vec!["reversed".into()], ..BattleUnit::default() };
        assert_eq!(marks(&u, false, true), "反", "属性反转的标志画一个“反”字（STAGE-200）");
    }

    #[test]
    fn new_statuses_have_one_mark_each() {
        let u = BattleUnit {
            status: vec!["weaken".into(), "barrier".into(), "nocast".into()],
            ..BattleUnit::default()
        };
        assert_eq!(marks(&u, false, true), "虚障默", "虚弱、魔障、沉默各一个字");
    }

    #[test]
    fn magic_resist_statuses_each_have_one_mark() {
        // STAGE-229：魔抗、火抗、电抗、冰抗各一个字，按挂上的顺序排。
        let u = BattleUnit {
            status: vec![
                "magic_resist".into(),
                "fire_resist".into(),
                "thunder_resist".into(),
                "ice_resist".into(),
            ],
            ..BattleUnit::default()
        };
        assert_eq!(marks(&u, false, true), "魔火电冰");
        assert_eq!(
            marks(&u, false, false),
            "魔火电冰",
            "关掉 Unicode 也是中文字，不降级"
        );
    }

    #[test]
    fn name_line_puts_marks_on_the_right_and_never_pushes_them_out() {
        let text = |l: Line<'static>| {
            l.spans
                .iter()
                .map(|s| s.content.to_string())
                .collect::<String>()
        };
        let plain = Style::default();
        assert_eq!(
            text(name_line("勇者小明", "", "", plain, plain, plain, 12)),
            "勇者小明    "
        );
        assert_eq!(
            text(name_line("勇者小明", "", "毒", plain, plain, plain, 12)),
            "勇者小明  毒"
        );
        assert_eq!(
            text(name_line("勇者小明一二", "", "毒", plain, plain, plain, 12)),
            "勇者小明  毒",
            "名字截断，标记不动"
        );
    }

    /// STAGE-173：等级在名字前面（`Lv.3 野猪`），宽度不够时先让等级、再截名字。
    #[test]
    fn name_line_puts_the_level_before_the_name() {
        let text = |l: Line<'static>| {
            l.spans
                .iter()
                .map(|s| s.content.to_string())
                .collect::<String>()
        };
        let plain = Style::default();
        let full = text(name_line("野猪", "Lv.3", "", plain, plain, plain, 14));
        assert_eq!(full, "Lv.3 野猪     ");
        let marked = text(name_line("野猪", "Lv.3", "毒", plain, plain, plain, 14));
        assert!(marked.starts_with("Lv.3 野猪"), "{marked}");
        assert!(marked.ends_with('毒'), "{marked}");
        assert_eq!(UnicodeWidthStr::width(marked.as_str()), 14, "不超宽：{marked}");
        // 差一列时省掉空格贴在一起写，名字仍然完整。
        let tight = text(name_line("野猪", "Lv.3", "毒", plain, plain, plain, 11));
        assert!(tight.starts_with("Lv.3野猪"), "{tight}");
        // 名字 + 标记已经把这一行占满：等级让位，名字完整保留。
        let crowded = text(name_line("勇者小明", "Lv.12", "毒", plain, plain, plain, 11));
        assert!(!crowded.contains("Lv"), "{crowded}");
        assert!(crowded.starts_with("勇者小明"), "{crowded}");
        assert!(crowded.ends_with('毒'), "{crowded}");
    }
}
