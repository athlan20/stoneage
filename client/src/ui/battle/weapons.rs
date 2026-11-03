//! STAGE-167：手持武器轮廓、飞行轨迹和回旋镖沿排往返。
use super::{
    field::Look,
    paint,
    position::{center, lerp},
};
use crate::app::battle::{Motion, Style as MotionStyle, projectile_glyph};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use std::{collections::BTreeMap, time::Instant};

fn along(points: &[(f32, f32)], p: f32) -> (f32, f32) {
    let t = p.clamp(0.0, 1.0) * (points.len() - 1) as f32;
    let i = (t.floor() as usize).min(points.len() - 2);
    (
        lerp(points[i].0, points[i + 1].0, t - i as f32),
        lerp(points[i].1, points[i + 1].1, t - i as f32),
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{BattleEffect, BattleEvent};
    use std::time::Duration;

    #[test]
    fn boomerang_sweeps_one_physical_column_and_returns_for_both_armies() {
        for me_side in [0, 1] {
            for front in [false, true] {
                let actor = me_side * 10;
                let enemy = (1 - me_side) * 10;
                let targets: Vec<_> = [3, 1, 0, 2, 4]
                    .into_iter()
                    .map(|slot| enemy + if front { 5 } else { 0 } + slot)
                    .collect();
                let mut poses = BTreeMap::from([(actor, Rect::new(90, 10, 14, 5))]);
                for (row, &no) in targets.iter().enumerate() {
                    poses.insert(
                        no,
                        Rect::new(if front { 30 } else { 5 }, row as u16 * 5, 14, 5),
                    );
                }
                let start = Instant::now();
                let hit = start + Duration::from_millis(500);
                let end = start + Duration::from_millis(1000);
                let ev = BattleEvent {
                    actor: Some(actor),
                    weapon: "boomerang".into(),
                    effects: targets
                        .iter()
                        .map(|&target| BattleEffect {
                            target,
                            ..Default::default()
                        })
                        .collect(),
                    ..Default::default()
                };
                let m = Motion::from_event(&ev, start, hit, end).unwrap();
                assert_eq!(m.sweep, targets);
                for ms in [100, 200, 300, 400] {
                    let (x, _, back) =
                        flight_at(&m, &poses, start + Duration::from_millis(ms)).unwrap();
                    assert_eq!(x, center(&poses[&targets[0]]).0, "只扫指定的前列或后列");
                    assert!(!back);
                }
                let (x, y, back) = flight_at(&m, &poses, end - Duration::from_millis(1)).unwrap();
                let hand = center(&poses[&actor]);
                assert!(back && (x - hand.0).abs() < 1.0 && (y - hand.1).abs() < 1.0);
                assert!(flight_at(&m, &poses, end).is_none());
            }
        }
    }
}

pub(super) fn flight_at(
    m: &Motion,
    poses: &BTreeMap<u8, Rect>,
    now: Instant,
) -> Option<(f32, f32, bool)> {
    let from = center(poses.get(&m.no)?);
    if m.style == MotionStyle::Ranged {
        if now >= m.hit {
            return None;
        }
        let to = center(poses.get(&m.toward)?);
        return Some((
            lerp(from.0, to.0, m.flight(now)),
            lerp(from.1, to.1, m.flight(now)),
            false,
        ));
    }
    let mut points = vec![from];
    // 只按服务端的实际目标和顺序扫一排，不补齐空位、不扩到另一排。
    for no in &m.sweep {
        if let Some(r) = poses.get(no) {
            points.push(center(r));
        }
    }
    if points.len() < 2 {
        points.push(center(poses.get(&m.toward)?));
    }
    if now < m.hit {
        let (x, y) = along(&points, m.flight(now));
        Some((x, y, false))
    } else if now < m.end {
        let p = now.saturating_duration_since(m.hit).as_secs_f32()
            / m.end.duration_since(m.hit).as_secs_f32().max(f32::EPSILON);
        let last = points.last()?;
        Some((lerp(last.0, from.0, p), lerp(last.1, from.1, p), true))
    } else {
        None
    }
}

pub(super) fn draw(
    frame: &mut Frame,
    m: &Motion,
    poses: &BTreeMap<u8, Rect>,
    area: Rect,
    look: &Look,
) {
    let Some(from) = poses.get(&m.no).map(center) else {
        return;
    };
    let Some(to) = poses.get(&m.toward).map(center) else {
        return;
    };
    let right = from.0 <= to.0;
    let left_weapon = match m.weapon.as_str() {
        "spear" => "<---",
        "axe" | "throw_axe" => "P-",
        "club" => "O==",
        "claw" => "\\\\\\",
        "bow" => "{(",
        "throw_stone" => "o",
        "boomerang" => "<",
        "fist" => "o-",
        _ => "",
    };
    let right_weapon = match m.weapon.as_str() {
        "spear" => "--->",
        "axe" | "throw_axe" => "-P",
        "club" => "==O",
        "claw" => "///",
        "bow" => ")}",
        "throw_stone" => "o",
        "boomerang" => ">",
        "fist" => "-o",
        _ => "",
    };
    if m.style != MotionStyle::Cast {
        let shape = if right { right_weapon } else { left_weapon };
        let x = if right {
            from.0 as i32 + 2
        } else {
            from.0 as i32 - 2 - shape.len() as i32
        };
        paint::text(
            frame,
            area,
            x,
            from.1 as i32,
            shape,
            Style::default().fg(Color::Yellow),
        );
    }
    if look.reduce_motion || !matches!(m.style, MotionStyle::Ranged | MotionStyle::Boomerang) {
        return;
    }
    if let Some((x, y, back)) = flight_at(m, poses, look.now) {
        let color = if back { Color::Cyan } else { Color::Yellow };
        // 短尾迹附着飞行物，箭、旋转斧、石块与回旋镖保留不同主体。
        let tail = if m.weapon == "bow" {
            if right { "--" } else { "==" }
        } else {
            ":"
        };
        paint::text(
            frame,
            area,
            x.round() as i32 - tail.len() as i32,
            y.round() as i32,
            tail,
            Style::default().fg(Color::DarkGray),
        );
        paint::glyph(
            frame,
            area,
            x.round() as i32,
            y.round() as i32,
            if m.weapon == "bow" && !right {
                if look.unicode { '←' } else { '<' }
            } else {
                projectile_glyph(&m.weapon, look.unicode)
            },
            color,
        );
    }
}
