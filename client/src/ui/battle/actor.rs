//! STAGE-167：可辨认的人形、兽形和骑乘身体，以及走动、出手、施法、倒地姿势。
use super::{Tier, card, field::Look, fx, gauge, paint};
use crate::{
    app::battle::{
        BattleState,
        slots::{self, Cell},
    },
    proto::BattleUnit,
};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::Paragraph,
};
use unicode_width::UnicodeWidthStr;

fn body(
    unit: &BattleUnit,
    compact: bool,
    casting: bool,
    striking: bool,
    step: bool,
    down: bool,
) -> Vec<&'static str> {
    if down {
        return if compact {
            vec!["   ", "_x_"]
        } else {
            vec!["       ", "       ", "_x___x_"]
        };
    }
    if compact {
        return match unit.kind.as_str() {
            "player" if unit.ride => vec![" o ", "mMm"],
            "player" if casting => vec!["\\o/", " | "],
            "player" => vec![" o ", if striking { "/|=" } else { "/|\\" }],
            "pet" => vec!["oo>", if step { "/| " } else { "|\\ " }],
            _ => vec!["^o^", if step { "/| " } else { "|\\ " }],
        };
    }
    if unit.ride {
        return vec![
            "   O   ",
            " (oo)> ",
            if step { " /| /| " } else { " /||\\  " },
        ];
    }
    match unit.kind.as_str() {
        "player" if casting => vec!["  \\O/  ", "   |   ", "  / \\  "],
        "player" => vec![
            "   O   ",
            if striking { "  /|-- " } else { "  /|\\  " },
            if step { "  / >  " } else { "  / \\  " },
        ],
        "pet" => vec![
            "  __   ",
            if striking { " (oo)=>" } else { " (oo)> " },
            if step { " /| /| " } else { " /||\\  " },
        ],
        _ => vec![
            " /^^\\  ",
            if striking { " (OO)=>" } else { " (OO)> " },
            if step { " /| /| " } else { " /||\\  " },
        ],
    }
}

pub(super) fn draw(
    frame: &mut Frame,
    b: &BattleState,
    u: &BattleUnit,
    cell: Cell,
    at: Rect,
    stage: Rect,
    tier: Tier,
    look: &Look,
) {
    let now = look.now;
    let compact = tier == Tier::Compact;
    let motion = b.motions.of(u.no).filter(|m| m.alive(now));
    let casting = b
        .spells
        .all()
        .iter()
        .any(|s| s.caster == u.no && now >= s.start && now < s.hit);
    let active = fx::acting(b, u.no, now) || motion.is_some() || casting;
    let focused = look.focus == Some(cell);
    let down = u.hp_pct == 0 && !fx::down_flashing(b, u.no, now);
    let side = if cell.side == b.me_side() {
        Color::Green
    } else {
        Color::Red
    };
    let color = if down {
        Color::DarkGray
    } else if focused || active {
        Color::Yellow
    } else if b.commander() == Some(u.no) || u.auto {
        Color::Cyan
    } else {
        side
    };
    let mut style = Style::default().fg(color);
    if focused || active {
        style = style.add_modifier(Modifier::BOLD);
    }
    let mut body_style = style.fg(fx::flash(b, u.no, now).unwrap_or(color));
    if b.swaps.reclaiming(u.no, now).is_some() {
        body_style = body_style.add_modifier(Modifier::DIM);
    }
    let stepping = !look.reduce_motion
        && motion.is_some_and(|m| {
            m.style.moves() && now.saturating_duration_since(m.start).as_millis() / 65 % 2 == 1
        });
    let striking = motion.is_some_and(|m| {
        m.style != crate::app::battle::Style::Cast && (now >= m.hit || m.approach(now) > 0.75)
    });
    let shape = body(u, compact, casting, striking, stepping, down);
    let body_x = if compact {
        at.x
    } else {
        at.x + at.width.saturating_sub(7) / 2
    };
    let body_y = at.y + u16::from(!compact);
    let shake = if look.reduce_motion {
        0
    } else {
        fx::shake(b, u.no, now)
    };
    for (dy, line) in shape.into_iter().enumerate() {
        let line = if cell.side == b.me_side() {
            mirror(line)
        } else {
            line.to_string()
        };
        paint::text(
            frame,
            stage,
            body_x as i32 + shake as i32,
            body_y as i32 + dy as i32,
            &line,
            body_style,
        );
    }
    let name = if down {
        format!("{} {}", u.name, if look.unicode { "✕" } else { "x" })
    } else {
        u.name.clone()
    };
    let mut marks = card::marks(u, b.flown.contains_key(&u.no), look.unicode);
    if u.auto {
        // 玩家这回合给它指定了目标：写成“自动→3”（STAGE-186）；没指定就只写“自动”，按合击顺序打第一个（STAGE-312）。
        let aim = b.clone_targets.get(&u.no).map_or_else(String::new, |&t| {
            format!("→{}", slots::number_of(slots::cell_of(t)))
        });
        marks = format!("{}{}{}", if compact { "自" } else { "自动" }, aim, marks);
    }
    // 队长标记（STAGE-275）：真人队同场时，队员表里 `leader` 为真的人物（自己也可能是队长）。
    if b.is_leader(u.no) {
        marks = format!("{}{}", if compact { "长" } else { "队长" }, marks);
    }
    if b.flown.get(&u.no).is_some_and(|f| f.dir < 0) {
        marks = marks.replace(
            if look.unicode { '↗' } else { '>' },
            if look.unicode { "↖" } else { "<" },
        );
    }
    let travelling = !look.reduce_motion
        && (motion.is_some_and(|m| m.style.moves())
            || b.swaps.reclaiming(u.no, now).is_some()
            || b.swaps.releasing(u.no, now).is_some()
            || b.flown.contains_key(&u.no));
    // 名字行不再带「敌 / 我 + 编号」前缀（STAGE-173）：整行就是「Lv.xx 名字」，
    // 站位与敌我已经由左右两侧的舞台分区和颜色区分，编号在「选目标」那一行仍能看到。
    // 紧凑档卡片左侧 3 列是身体头顶（`^o^`），名字仍从第 4 列起，免得把头像盖掉。
    let label_x = if travelling && !compact {
        body_x
    } else {
        at.x + if compact { 4 } else { 0 }
    };
    let label_w = if travelling && !compact {
        9
    } else {
        at.width.saturating_sub(if compact { 4 } else { 0 })
    };
    let head = name;
    // 紧凑档名字区只有 10 列，省掉「Lv.」的点才塞得下等级（`Lv3野猪`）。
    let level = if compact { format!("Lv{}", u.level) } else { format!("Lv.{}", u.level) };
    let state_style = Style::default().fg(if fx::status_flash(b, u.no, now) {
        Color::Magenta
    } else {
        color
    });
    frame.render_widget(
        Paragraph::new(card::name_line(
            &head,
            &level,
            &marks,
            style,
            style.remove_modifier(Modifier::BOLD),
            state_style,
            label_w as usize,
        )),
        Rect::new(label_x, at.y, label_w, 1),
    );
    let float = b
        .floats
        .iter()
        .rev()
        .find(|f| f.no == u.no && f.until > now);
    // 等级已经写在名字行（`Lv.xx 名字`），这里只画浮动数字，不再重复一个 Lv。
    let info = float.map(|f| f.text.clone()).unwrap_or_default();
    if !compact && !travelling && !info.is_empty() {
        let info_w = UnicodeWidthStr::width(info.as_str()) as u16;
        paint::text(
            frame,
            stage,
            (at.right().saturating_sub(info_w)) as i32,
            (at.y + 1) as i32,
            &info,
            float.map_or(Style::default().fg(Color::DarkGray), |f| {
                card::float_style(f.kind)
            }),
        );
    }
    let hp = if travelling {
        format!("{}%", u.hp_pct)
    } else {
        card::hp_text(u, cell.side == b.me_side())
    };
    // 队友的蓝量（STAGE-275）：自己的蓝量在顶部信息条，队友的写在血量后面。
    let hp = match (u.mp, u.max_mp) {
        (Some(mp), Some(max)) if b.is_mate(u.no) && !travelling && !compact => format!("{hp} MP{mp}/{max}"),
        _ => hp,
    };
    let hp_w = UnicodeWidthStr::width(hp.as_str()) as u16;
    let start = if travelling && !compact {
        body_x
    } else {
        at.x + if compact { 4 } else { 0 }
    };
    let width = if travelling && !compact {
        9
    } else {
        at.right().saturating_sub(start)
    };
    let bar_w = width
        .saturating_sub(hp_w + 1)
        .min(if travelling { 4 } else { 6 });
    let band_end = start + width;
    let start = start + width.saturating_sub(bar_w + hp_w + 1) / 2;
    let y = at.y + at.height - 1;
    if bar_w > 0 {
        frame.render_widget(
            Paragraph::new(gauge::thin(
                fx::gauge(b, u.no, u.hp_pct, now),
                bar_w as usize,
                look.unicode,
                down,
            )),
            Rect::new(start, y, bar_w, 1),
        );
    }
    let hp_x = start + bar_w + u16::from(bar_w > 0);
    let text = if compact {
        float.map_or(hp.as_str(), |f| f.text.as_str())
    } else {
        hp.as_str()
    };
    frame.render_widget(
        Paragraph::new(Line::from(Span::styled(
            text.to_string(),
            Style::default().fg(card::hp_color(u.hp_pct)),
        ))),
        Rect::new(hp_x, y, band_end.saturating_sub(hp_x), 1),
    );
    if focused {
        paint::text(
            frame,
            stage,
            body_x as i32 - 1,
            body_y as i32,
            "[",
            Style::default().fg(Color::White),
        );
        paint::text(
            frame,
            stage,
            body_x as i32 + if compact { 3 } else { 7 },
            body_y as i32,
            "]",
            Style::default().fg(Color::White),
        );
    }
}

/// 身体与手臂朝向中场；文字标签不翻转。
fn mirror(line: &str) -> String {
    line.chars()
        .rev()
        .map(|ch| match ch {
            '/' => '\\',
            '\\' => '/',
            '(' => ')',
            ')' => '(',
            '<' => '>',
            '>' => '<',
            _ => ch,
        })
        .collect()
}
