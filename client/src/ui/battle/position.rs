//! STAGE-167：身体在战场上的实际坐标。近战走到目标身旁，换宠回到事件指定的主人。
use super::field::Look;
use crate::app::battle::{BattleState, Style as MotionStyle};
use ratatui::layout::Rect;
use std::collections::BTreeMap;

pub(super) fn center(r: &Rect) -> (f32, f32) {
    (
        r.x as f32
            + if r.height >= 5 {
                r.width as f32 / 2.0
            } else {
                1.0
            },
        r.y as f32 + if r.height >= 5 { 2.0 } else { 1.0 },
    )
}

pub(super) fn lerp(a: f32, b: f32, p: f32) -> f32 {
    a + (b - a) * p.clamp(0.0, 1.0)
}

fn between(from: Rect, to: Rect, p: f32, area: Rect) -> Rect {
    Rect::new(
        lerp(from.x as f32, to.x as f32, p).round().clamp(
            area.x as f32,
            area.right().saturating_sub(from.width) as f32,
        ) as u16,
        lerp(from.y as f32, to.y as f32, p).round().clamp(
            area.y as f32,
            area.bottom().saturating_sub(from.height) as f32,
        ) as u16,
        from.width,
        from.height,
    )
}

fn contact(at: Rect, to: Rect, no: u8, style: MotionStyle, area: Rect) -> Rect {
    let dx = if at.x < to.x {
        -1.0
    } else if at.x > to.x {
        1.0
    } else if no % 2 == 0 {
        -1.0
    } else {
        1.0
    };
    let reach = if style == MotionStyle::Thrust {
        10.0
    } else {
        7.0
    };
    let dy = if to.y <= area.y {
        1.0
    } else if at.y > to.y {
        1.0
    } else {
        -1.0
    };
    Rect::new(
        (to.x as f32 + dx * reach)
            .clamp(area.x as f32, area.right().saturating_sub(at.width) as f32) as u16,
        (to.y as f32 + dy).clamp(
            area.y as f32,
            area.bottom().saturating_sub(at.height) as f32,
        ) as u16,
        at.width,
        at.height,
    )
}

pub(super) fn poses(
    b: &BattleState,
    home: &BTreeMap<u8, Rect>,
    area: Rect,
    look: &Look,
) -> BTreeMap<u8, Rect> {
    let mut out = home.clone();
    if look.reduce_motion {
        return out;
    }
    for (&no, &at) in home {
        if let Some(m) = b
            .motions
            .of(no)
            .filter(|m| m.alive(look.now) && m.style.moves())
        {
            if let Some(&to) = home.get(&m.toward) {
                let near = contact(at, to, no, m.style, area);
                let mut p = m.approach(look.now);
                if m.hold {
                    if let Some(counter) = b
                        .motions
                        .all()
                        .iter()
                        .find(|c| c.style == MotionStyle::Counter && c.toward == no)
                    {
                        if look.now >= counter.hit {
                            p = 1.0
                                - look
                                    .now
                                    .saturating_duration_since(counter.hit)
                                    .as_secs_f32()
                                    / counter
                                        .end
                                        .saturating_duration_since(counter.hit)
                                        .as_secs_f32()
                                        .max(f32::EPSILON);
                        }
                    }
                }
                out.insert(no, between(at, near, p, area));
            }
        }
    }
    // 忠犬代受：出手方向仍是 intent，代受者移动到该目标身前，反馈落在真正承伤的身体上。
    for m in b.motions.all().iter().filter(|m| m.alive(look.now)) {
        for &(target, _) in &m.contacts {
            if target == m.toward {
                continue;
            }
            if let (Some(&guard), Some(&intended)) = (home.get(&target), home.get(&m.toward)) {
                out.insert(
                    target,
                    between(guard, intended, m.approach(look.now) * 0.8, area),
                );
            }
        }
    }
    // 反击者站在原地向已经贴近自己的攻击者还手，不再朝攻击者的出生位挥击。
    for s in b.swaps.all().iter().filter(|s| s.alive(look.now)) {
        let Some(&owner) = home.get(&s.owner) else {
            continue;
        };
        let pet = if look.now < s.hit { s.old } else { s.new };
        if let Some(no) = pet {
            if let Some(&at) = home.get(&no) {
                let p = if look.now < s.hit {
                    s.reclaim(look.now)
                } else {
                    1.0 - s.release(look.now)
                };
                // 身体的中心真正抵达主人中心；姓名另由角色层避让。
                out.insert(no, between(at, owner, p, area));
            }
        }
    }
    for (&no, f) in &b.flown {
        let Some(&at) = out.get(&no) else { continue };
        let p = look.now.saturating_duration_since(f.at).as_secs_f32() / 0.45;
        if p >= 1.0 {
            out.remove(&no);
            continue;
        }
        let edge = Rect::new(
            if f.dir >= 0 {
                area.right().saturating_sub(at.width)
            } else {
                area.x
            },
            at.y,
            at.width,
            at.height,
        );
        out.insert(no, between(at, edge, p, area));
    }
    out
}

#[cfg(test)]
#[path = "position_tests.rs"]
mod tests;
