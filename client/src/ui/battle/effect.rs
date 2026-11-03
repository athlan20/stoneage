//! STAGE-167：身体之上的武器、命中和精灵效果，全部共用这一帧的实际坐标。
use super::{Tier, field::Look, paint, position::center, spell_effect, weapons};
use crate::app::battle::{BattleState, Outcome, Style as MotionStyle, contact_glyph};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use std::collections::BTreeMap;

pub fn draw(
    frame: &mut Frame,
    b: &BattleState,
    poses: &BTreeMap<u8, Rect>,
    area: Rect,
    _tier: Tier,
    look: &Look,
) {
    for m in b.motions.all().iter().filter(|m| m.alive(look.now)) {
        weapons::draw(frame, m, poses, area, look);
        if look.now < m.hit || m.style == MotionStyle::Cast {
            continue;
        }
        let contacts = if m.contacts.is_empty() {
            vec![(m.toward, m.outcome)]
        } else {
            m.contacts.clone()
        };
        for (no, outcome) in contacts {
            let Some(r) = poses.get(&no) else { continue };
            let (cx, cy) = center(r);
            let (x, y) = (cx as i32, cy as i32);
            let color = if outcome == Outcome::Crit {
                Color::Yellow
            } else {
                Color::White
            };
            paint::glyph(
                frame,
                area,
                x + if poses.get(&m.no).is_some_and(|from| center(from).0 < cx) {
                    -4
                } else {
                    4
                },
                y,
                contact_glyph(outcome, look.unicode),
                color,
            );
            if r.height >= 5 {
                let shape = match outcome {
                    Outcome::Crit => "\\ | /",
                    Outcome::Block => "[ ]",
                    Outcome::Dodge => ">>",
                    Outcome::Miss => "' '",
                    Outcome::Hit => "-+-",
                };
                paint::text(frame, area, x - 2, y - 1, shape, Style::default().fg(color));
            }
        }
    }
    for s in b.spells.all() {
        spell_effect::draw(frame, s, poses, area, look);
    }
    for s in b.swaps.all().iter().filter(|s| s.alive(look.now)) {
        if let Some(r) = poses.get(&s.owner) {
            let (x, y) = center(r);
            paint::glyph(
                frame,
                area,
                x as i32 - 4,
                y as i32,
                if s.owner / 10 == b.me_side() {
                    '>'
                } else {
                    '<'
                },
                Color::Cyan,
            );
        }
    }
}
