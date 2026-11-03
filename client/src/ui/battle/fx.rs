//! 命中拍的视觉效果（STAGE-110，`docs/battle-ui.md`「卡片效果」）：闪色、抖动、文字让位和血条的残影、涨血。
//! 这里只算这一帧该怎么画，不改数据；效果到时间就停，卡片回到常态。

use std::time::Instant;

use ratatui::style::Color;

use crate::app::battle::{BattleState, Fx, FxKind};

/// 受伤抖一下：先右、再左、回位（左右各一格，`put` 里用右移和列裁切实现）。暴击抖两个来回。
const HURT_SHAKE: [i16; 3] = [1, -1, 0];
const CRIT_SHAKE: [i16; 4] = [1, -1, 1, 0];

/// 血条要画的两段：实心到 `fill`，残影从 `fill` 到 `ghost` 之间画成 `▒`。没有残影时两者相等。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Gauge {
    pub fill: u32,
    pub ghost: u32,
}

/// 单位身上还没结束的效果。
fn live(b: &BattleState, no: u8, now: Instant) -> impl Iterator<Item = &Fx> {
    b.fx.iter().filter(move |f| f.no == no && f.from <= now && now < f.until)
}

/// 效果进行到第几段（每段一个 `slot`）。
fn slot_index(f: &Fx, now: Instant) -> usize {
    let slot = f.slot.as_nanos().max(1);
    (now.saturating_duration_since(f.from).as_nanos() / slot) as usize
}

/// 回放中正在行动的单位（卡片标题加 `▶` 和高亮）：事件的行动者，合击时包括起头以外的出手者（STAGE-126）。
pub fn acting(b: &BattleState, no: u8, now: Instant) -> bool {
    b.acting
        .is_some_and(|(who, until)| until > now && (who == no || b.acting_with.contains(&no)))
}

/// 闪色：受伤、暴击、倒下是红色，全防是白色，回复是绿色。一段亮、一段灭，闪的段数由效果决定。
/// 新增状态的品红闪不在这里（只闪状态标记，见 `status_flash`）。
pub fn flash(b: &BattleState, no: u8, now: Instant) -> Option<Color> {
    live(b, no, now).find_map(|f| {
        let color = match f.kind {
            FxKind::Hurt | FxKind::Crit | FxKind::Down => Color::Red,
            FxKind::Block => Color::White,
            FxKind::Heal => Color::Green,
            FxKind::Dodge(_) | FxKind::Status => return None,
        };
        (slot_index(f, now) % 2 == 0).then_some(color)
    })
}

/// 新增的状态标记是否正在品红闪（只闪标记，不闪整张卡片）。
pub fn status_flash(b: &BattleState, no: u8, now: Instant) -> bool {
    live(b, no, now).any(|f| f.kind == FxKind::Status && slot_index(f, now) % 2 == 0)
}

/// 倒下的单位还在闪红：这时卡片还不变灰（闪完才变灰）。
pub fn down_flashing(b: &BattleState, no: u8, now: Instant) -> bool {
    live(b, no, now).any(|f| f.kind == FxKind::Down)
}

/// 文字的左右偏移（格）：受伤、暴击按段抖动，闪避的文字一直让开一格。没有效果时是 0。
pub fn shake(b: &BattleState, no: u8, now: Instant) -> i16 {
    live(b, no, now)
        .map(|f| match f.kind {
            FxKind::Hurt => HURT_SHAKE.get(slot_index(f, now)).copied().unwrap_or(0),
            FxKind::Crit => CRIT_SHAKE.get(slot_index(f, now)).copied().unwrap_or(0),
            FxKind::Dodge(dir) => i16::from(dir),
            _ => 0,
        })
        .find(|&dx| dx != 0)
        .unwrap_or(0)
}

/// 血条这一帧该画的样子：血量移动中就用移动的值，否则是单位的实际血量。
/// 伤害：`fill` 立即是新值，`ghost` 从旧值缩到新值；回复：`fill` 从旧值涨到新值。
pub fn gauge(b: &BattleState, no: u8, pct: u32, now: Instant) -> Gauge {
    let moving = b.hp_moves.iter().rev().find(|m| m.no == no && m.start <= now && now < m.until);
    let Some(m) = moving else {
        return Gauge { fill: pct, ghost: pct };
    };
    let total = m.until.saturating_duration_since(m.start).as_secs_f64().max(f64::EPSILON);
    let done = (now.saturating_duration_since(m.start).as_secs_f64() / total).clamp(0.0, 1.0);
    if m.to < m.from {
        let ghost = f64::from(m.from) - f64::from(m.from - m.to) * done;
        Gauge { fill: m.to, ghost: ghost.round() as u32 }
    } else {
        let fill = f64::from(m.from) + f64::from(m.to - m.from) * done;
        Gauge { fill: fill.round() as u32, ghost: fill.round() as u32 }
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::app::battle::HpMove;
    use crate::proto::{BattleAllow, BattleStart, BattleUnit};

    fn battle() -> BattleState {
        let unit = BattleUnit { no: 15, name: "野猪".into(), hp_pct: 100, ..BattleUnit::default() };
        let start = BattleStart {
            scene: "野外".into(),
            you: 0,
            allow: BattleAllow::default(),
            units: vec![unit],
            ..BattleStart::default()
        };
        BattleState::from_start(start)
    }

    /// 一段 100ms 的效果，共 `slots` 段。
    fn fx(kind: FxKind, from: Instant, slots: u64) -> Fx {
        let slot = Duration::from_millis(100);
        Fx { no: 15, kind, from, until: from + slot * slots as u32, slot }
    }

    fn at(t0: Instant, millis: u64) -> Instant {
        t0 + Duration::from_millis(millis)
    }

    #[test]
    fn hurt_flashes_red_on_off_on_then_stops() {
        let t0 = Instant::now();
        let mut b = battle();
        b.fx.push(fx(FxKind::Hurt, t0, 3));
        assert_eq!(flash(&b, 15, at(t0, 0)), Some(Color::Red));
        assert_eq!(flash(&b, 15, at(t0, 100)), None, "灭");
        assert_eq!(flash(&b, 15, at(t0, 200)), Some(Color::Red), "再亮");
        assert_eq!(flash(&b, 15, at(t0, 300)), None, "300ms 后不再闪");
        assert_eq!(flash(&b, 14, at(t0, 0)), None, "别的单位不闪");
    }

    #[test]
    fn block_flashes_white_once_and_heal_green_once() {
        let t0 = Instant::now();
        let mut b = battle();
        b.fx.push(fx(FxKind::Block, t0, 2));
        b.fx.push(fx(FxKind::Heal, at(t0, 500), 2));
        assert_eq!(flash(&b, 15, at(t0, 0)), Some(Color::White));
        assert_eq!(flash(&b, 15, at(t0, 100)), None);
        assert_eq!(flash(&b, 15, at(t0, 500)), Some(Color::Green));
        assert_eq!(flash(&b, 15, at(t0, 600)), None);
    }

    #[test]
    fn status_flashes_only_the_marks_twice() {
        let t0 = Instant::now();
        let mut b = battle();
        b.fx.push(fx(FxKind::Status, t0, 4));
        assert!(status_flash(&b, 15, at(t0, 0)));
        assert!(!status_flash(&b, 15, at(t0, 100)));
        assert!(status_flash(&b, 15, at(t0, 200)), "第二下");
        assert!(!status_flash(&b, 15, at(t0, 300)));
        assert_eq!(flash(&b, 15, at(t0, 0)), None, "状态的闪不闪整张卡片");
    }

    #[test]
    fn hurt_and_crit_shake_by_segment_and_dodge_leans_away() {
        let t0 = Instant::now();
        let mut b = battle();
        b.fx.push(fx(FxKind::Hurt, t0, 3));
        let hurt: Vec<i16> = [0, 100, 200, 300].iter().map(|&ms| shake(&b, 15, at(t0, ms))).collect();
        assert_eq!(hurt, [1, -1, 0, 0], "受伤抖一下回位");

        let mut b = battle();
        b.fx.push(fx(FxKind::Crit, t0, 4));
        let crit: Vec<i16> = [0, 100, 200, 300].iter().map(|&ms| shake(&b, 15, at(t0, ms))).collect();
        assert_eq!(crit, [1, -1, 1, 0], "暴击抖两个来回");

        let mut b = battle();
        b.fx.push(Fx {
            no: 15,
            kind: FxKind::Dodge(-1),
            from: t0,
            until: at(t0, 350),
            slot: Duration::from_millis(100),
        });
        assert_eq!(shake(&b, 15, at(t0, 0)), -1, "闪避的文字让开一格");
        assert_eq!(shake(&b, 15, at(t0, 350)), 0, "命中拍结束回位");
    }

    #[test]
    fn down_flashes_red_before_the_card_turns_grey() {
        let t0 = Instant::now();
        let mut b = battle();
        b.fx.push(fx(FxKind::Down, t0, 3));
        assert!(down_flashing(&b, 15, at(t0, 0)));
        assert!(!down_flashing(&b, 15, at(t0, 300)));
    }

    #[test]
    fn damage_ghost_shrinks_from_the_old_value_to_the_new() {
        let t0 = Instant::now();
        let mut b = battle();
        b.hp_moves.push(HpMove { no: 15, from: 40, to: 10, start: t0, until: at(t0, 300) });
        assert_eq!(gauge(&b, 15, 10, t0), Gauge { fill: 10, ghost: 40 }, "新值立即生效，残影是旧值");
        assert_eq!(gauge(&b, 15, 10, at(t0, 150)), Gauge { fill: 10, ghost: 25 }, "残影缩到一半");
        assert_eq!(gauge(&b, 15, 10, at(t0, 300)), Gauge { fill: 10, ghost: 10 }, "缩完就没有残影");
    }

    #[test]
    fn heal_fills_up_over_the_hit_beat() {
        let t0 = Instant::now();
        let mut b = battle();
        b.hp_moves.push(HpMove { no: 15, from: 10, to: 40, start: t0, until: at(t0, 350) });
        assert_eq!(gauge(&b, 15, 40, t0), Gauge { fill: 10, ghost: 10 });
        assert_eq!(gauge(&b, 15, 40, at(t0, 175)), Gauge { fill: 25, ghost: 25 }, "涨到一半");
        assert_eq!(gauge(&b, 15, 40, at(t0, 350)), Gauge { fill: 40, ghost: 40 });
    }
}
