//! STAGE-167：聚集在施法者身边，岩柱、水环、火舌、风弧落在真实目标身体周围。
use super::{field::Look, paint, position::center};
use crate::app::battle::{Elem, SpellFx, SpellKind, effect_glyph, element_glyph};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use std::collections::BTreeMap;

fn color(s: &SpellFx) -> Color {
    match s.kind {
        SpellKind::HealSelf | SpellKind::HealOther | SpellKind::Revive => Color::Green,
        SpellKind::Cure => Color::Cyan,
        SpellKind::Status => Color::Magenta,
        _ => match s.elem {
            Elem::Earth => Color::Yellow,
            Elem::Water => Color::Cyan,
            Elem::Fire => Color::Red,
            Elem::Wind => Color::Green,
            Elem::None => Color::Magenta,
        },
    }
}

pub(super) fn draw(
    frame: &mut Frame,
    s: &SpellFx,
    poses: &BTreeMap<u8, Rect>,
    area: Rect,
    look: &Look,
) {
    let tone = color(s);
    if look.now >= s.start && look.now < s.hit && !look.reduce_motion {
        if let Some(r) = poses.get(&s.caster) {
            let (cx, cy) = center(r);
            let radius = (5.0 - s.gather(look.now) * 2.0).round() as i32;
            let (x, y) = (cx as i32, cy as i32);
            paint::glyph(
                frame,
                area,
                x - radius,
                y,
                element_glyph(s.elem, look.unicode),
                tone,
            );
            paint::glyph(frame, area, x + radius, y, '+', tone);
            if r.height >= 5 {
                paint::text(
                    frame,
                    area,
                    x - 2,
                    y + 1,
                    "(---)",
                    Style::default().fg(tone),
                );
            }
        }
    }
    if !s.showing(look.now) {
        return;
    }
    for no in &s.targets {
        let Some(r) = poses.get(no) else { continue };
        let (cx, cy) = center(r);
        let (x, y) = (cx as i32, cy as i32);
        let glyph = effect_glyph(s, look.unicode);
        // 紧凑档仍保留完整身体和实际目标，不以整格闪色代替角色。
        paint::glyph(
            frame,
            area,
            x - if r.height >= 5 { 4 } else { 2 },
            y,
            glyph,
            tone,
        );
        if r.height < 5 || look.reduce_motion {
            continue;
        }
        let phase = look.now.saturating_duration_since(s.hit).as_millis() / 80 % 2;
        let (top, bottom) = match s.kind {
            SpellKind::Damage => match s.elem {
                Elem::Earth => (" /\\   /\\ ", "|_|___|_|"),
                Elem::Water => (
                    " .  .  . ",
                    if phase == 0 { "(_______)" } else { " (_____) " },
                ),
                Elem::Fire => (
                    if phase == 0 { " ' ^ ^ ' " } else { "  ^ ' ^  " },
                    " \\_|_|_/ ",
                ),
                Elem::Wind => (" /-----) ", " (-----/ "),
                Elem::None => (" .     . ", " (-----) "),
            },
            SpellKind::HealSelf | SpellKind::HealOther => (" +     + ", " '  +  ' "),
            SpellKind::Revive => (" |     | ", " \\_____/ "),
            SpellKind::Cure => (" .     . ", " '     ' "),
            SpellKind::Status => match s.status.as_deref() {
                Some("poison") => (" :     : ", " .     . "),
                Some("paralysis") => (" /     / ", " /     / "),
                Some("sleep") => (" z     z ", "         "),
                Some("stone") => (" [     ] ", " [_____] "),
                Some("drunk") => (" ~     ~ ", "         "),
                Some("confusion") => (" >     < ", "         "),
                _ => (" .     . ", "         "),
            },
            _ => (" .     . ", " (-----) "),
        };
        // 只画两侧和脚边，不覆盖身体中心、姓名、血条。
        paint::text(
            frame,
            area,
            x - 4,
            y - 1,
            &top[..2],
            Style::default().fg(tone),
        );
        paint::text(
            frame,
            area,
            x + 3,
            y - 1,
            &top[top.len().saturating_sub(2)..],
            Style::default().fg(tone),
        );
        paint::text(frame, area, x - 4, y + 1, bottom, Style::default().fg(tone));
    }
}
