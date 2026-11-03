//! 出手动作层（STAGE-167，`docs/battle-ui.md`「谁在打谁」）：把一条结算事件翻译成「谁从哪移动到哪、
//! 用什么方式、什么时候接触、接触点在哪」，供战场渲染画位移、投射物和接触标记。
//!
//! 这里只按服务端给的事实算动作：`weapon` 决定形态、`effects[].intent` 决定朝谁去、
//! `effects[].target` 决定伤害落在谁身上、`row` 决定回旋镖扫哪一排。客户端不补目标、不改顺序、
//! 不因为动画好看多打一下。
//!
//! 终端只有字符格，所以「接近」是从站位跨排走到目标身旁的位移，「接触」是一个字符标记；
//! 关掉 Unicode 时标记退回 ASCII。

use std::time::{Duration, Instant};

pub use super::glyph::{contact_glyph, projectile_glyph};

use crate::proto::BattleEvent;

/// 一次出手的动作形态。由 `weapon` 决定，认不出来时用 `Neutral`（还是照常画接触和结果）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Style {
    /// 近战：接近目标再打（空手、斧、棍、爪）。
    Melee,
    /// 直刺：接近目标但停在稍远处（长矛）。
    Thrust,
    /// 远程：留在原地，飞行物从自己飞向目标（弓、投斧、投石）。
    Ranged,
    /// 回旋镖：原地出手，飞行物横扫一排再回手。
    Boomerang,
    /// 法术：原地起手，作用到真实范围。
    Cast,
    /// 反击：被打的人在接触现场还手，不退回出生位重新起跑（STAGE-167）。
    Counter,
    /// 认不出来的武器：不动，只在接触时画标记。
    Neutral,
}

impl Style {
    /// 按协议里的 `weapon` 取值判断形态。
    pub fn of(weapon: &str) -> Self {
        match weapon {
            "fist" | "axe" | "club" | "claw" => Style::Melee,
            "spear" => Style::Thrust,
            "bow" | "throw_axe" | "throw_stone" => Style::Ranged,
            "boomerang" => Style::Boomerang,
            _ => Style::Neutral,
        }
    }

    /// 出手者会不会离开自己的位置。远程和法术留在原地。
    pub fn moves(self) -> bool {
        matches!(self, Style::Melee | Style::Thrust)
    }
}

/// 接触点表达的结果（STAGE-167，A05）：命中、暴击、闪开、落空、挡住各不相同。
/// 闪避和未命中不共用一个标记——一个是身体离开了接触点，一个是攻击落在旁边。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Outcome {
    Hit,
    Crit,
    Dodge,
    Miss,
    Block,
}

impl Outcome {
    /// 一个事件里的多次效果合成一个「最重要」的接触结果：暴击 > 挡住 > 闪开 > 落空 > 命中。
    fn strongest(results: impl Iterator<Item = String>) -> Self {
        let mut best = Outcome::Hit;
        let rank = |o: Outcome| match o {
            Outcome::Hit => 0,
            Outcome::Miss => 1,
            Outcome::Dodge => 2,
            Outcome::Block => 3,
            Outcome::Crit => 4,
        };
        for r in results {
            let o = match r.as_str() {
                "crit" => Outcome::Crit,
                "block" | "vanish" => Outcome::Block,
                "dodge" => Outcome::Dodge,
                "miss" => Outcome::Miss,
                _ => Outcome::Hit,
            };
            if rank(o) > rank(best) {
                best = o;
            }
        }
        best
    }
}

/// 一条出手动作，从 `start` 开始、`hit` 接触、`end` 结束。
#[derive(Debug, Clone)]
pub struct Motion {
    /// 出手者（可能多人合击，这里每人都有一条）。
    pub no: u8,
    /// 朝谁去：优先 `intent`（本来要打的人），没有就是实际承伤者。
    pub toward: u8,
    pub style: Style,
    /// 武器身份原样留着（`bow`、`boomerang`…）：渲染挑飞行物轮廓时要用。
    pub weapon: String,
    /// 回旋镖这一趟扫过的目标（按事件顺序），用来画沿线接触点。
    pub sweep: Vec<u8>,
    pub start: Instant,
    pub hit: Instant,
    pub end: Instant,
    /// 这一个事件里最重要的一次结果，决定接触标记的样子。
    pub outcome: Outcome,
    /// 每个实际承伤者各自的命中结果；不把暴击复制给同排的其他目标。
    pub contacts: Vec<(u8, Outcome)>,
    /// 接触之后停在目标身边不回位（STAGE-167）：被打的人马上要反击时，
    /// 攻击者留在接触现场完成攻防交换，不是退回出生位再冲一次。
    pub hold: bool,
}

impl Motion {
    /// 从一条事件推出动作。没有出手者（系统毒伤之类）或者没有目标时返回 None。
    pub fn from_event(ev: &BattleEvent, start: Instant, hit: Instant, end: Instant) -> Option<Self> {
        Self::from_event_holding(ev, start, hit, end, false)
    }

    /// 同上，`hold` 表示接触之后停在目标身边（下一拍是反击时用）。
    pub fn from_event_holding(
        ev: &BattleEvent,
        start: Instant,
        hit: Instant,
        end: Instant,
        hold: bool,
    ) -> Option<Self> {
        let no = ev.actor?;
        let style = match ev.kind.as_str() {
            "magic" => Style::Cast,
            "counter" => Style::Counter,
            _ => Style::of(&ev.weapon),
        };
        let sweep: Vec<u8> = ev.effects.iter().map(|e| e.target).collect();
        let toward = ev
            .effects
            .iter()
            .find_map(|e| e.intent.or(Some(e.target)))
            .or(ev.target)
            .filter(|t| *t != no)?;
        let outcome = Outcome::strongest(ev.effects.iter().map(|e| e.result.clone()));
        let contacts = ev.effects.iter().map(|e| (e.target, Outcome::strongest(std::iter::once(e.result.clone())))).collect();
        Some(Self {
            no,
            toward,
            style,
            weapon: ev.weapon.clone(),
            sweep,
            start,
            hit,
            end,
            outcome,
            contacts,
            hold,
        })
    }

    /// 0 到 1 的「贴近」进度：接触时到 1，之后回位。
    /// 回位用同样的时长，所以整条动作是「去一半、回一半」。
    pub fn approach(&self, now: Instant) -> f32 {
        if now <= self.start || self.hit <= self.start {
            return 0.0;
        }
        if now < self.hit {
            let span = self.hit.duration_since(self.start);
            let done = now.duration_since(self.start);
            return done.as_secs_f32() / span.as_secs_f32().max(f32::EPSILON);
        }
        if self.hold {
            // 停在接触现场等下一拍（反击），不往回走。
            return 1.0;
        }
        let back = self.end.saturating_duration_since(self.hit);
        let done = now.duration_since(self.hit);
        if back.is_zero() {
            return 0.0;
        }
        1.0 - (done.as_secs_f32() / back.as_secs_f32()).clamp(0.0, 1.0)
    }

    /// 这一条动作现在还在不在演。
    pub fn alive(&self, now: Instant) -> bool {
        now >= self.start && now < self.end
    }

    /// 飞行物现在在 0 到 1 之间的哪个位置（只在远程/回旋镖去程有意义）。
    pub fn flight(&self, now: Instant) -> f32 {
        if !matches!(self.style, Style::Ranged | Style::Boomerang) {
            return 0.0;
        }
        if now <= self.start || self.hit <= self.start {
            return 0.0;
        }
        if now >= self.hit {
            return 1.0;
        }
        let span = self.hit.duration_since(self.start);
        (now.duration_since(self.start).as_secs_f32() / span.as_secs_f32().max(f32::EPSILON))
            .clamp(0.0, 1.0)
    }
}

/// 动作层的一次性状态：当前在演的动作。
#[derive(Debug, Clone, Default)]
pub struct Motions {
    items: Vec<Motion>,
}

impl Motions {
    pub fn clear(&mut self) {
        self.items.clear();
    }

    pub fn push(&mut self, m: Motion) {
        // 同一个出手者只留一条最新的动作，避免上一条没演完就开始下一条时两个卡片一起飘。
        self.items.retain(|old| old.no != m.no);
        self.items.push(m);
    }

    /// 去掉演完的动作。
    pub fn expire(&mut self, now: Instant) {
        self.items.retain(|m| m.alive(now));
    }

    /// 某个单位现在在演的动作。
    pub fn of(&self, no: u8) -> Option<&Motion> {
        self.items.iter().find(|m| m.no == no)
    }

    pub fn all(&self) -> &[Motion] {
        &self.items
    }

    pub fn is_empty(&self) -> bool {
        self.items.is_empty()
    }

    /// 接触点标记是否该画（接触前后各一小段）。
    pub fn at_contact(&self, m: &Motion, now: Instant, window: Duration) -> bool {
        let d = if now >= m.hit {
            now.duration_since(m.hit)
        } else {
            m.hit.duration_since(now)
        };
        d <= window
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{BattleEffect, BattleEvent};

    fn event(kind: &str, weapon: &str, actor: u8, target: u8) -> BattleEvent {
        BattleEvent {
            kind: kind.into(),
            actor: Some(actor),
            weapon: weapon.into(),
            effects: vec![BattleEffect {
                target,
                result: "hit".into(),
                ..Default::default()
            }],
            ..Default::default()
        }
    }

    #[test]
    fn styles_follow_the_weapon_not_the_name() {
        assert_eq!(Style::of("axe"), Style::Melee);
        assert_eq!(Style::of("spear"), Style::Thrust);
        assert_eq!(Style::of("bow"), Style::Ranged);
        assert_eq!(Style::of("boomerang"), Style::Boomerang);
        assert_eq!(Style::of("unknown-weapon"), Style::Neutral);
        assert!(Style::Melee.moves() && !Style::Ranged.moves());
    }

    #[test]
    fn different_weapons_do_not_share_one_glyph() {
        let glyphs: Vec<char> = ["bow", "throw_axe", "throw_stone", "boomerang"]
            .iter()
            .map(|w| projectile_glyph(w, true))
            .collect();
        let unique: std::collections::HashSet<char> = glyphs.iter().copied().collect();
        assert_eq!(unique.len(), glyphs.len(), "四种飞行物要有各自的轮廓：{glyphs:?}");
    }

    #[test]
    fn motion_moves_out_and_comes_back() {
        let start = Instant::now();
        let hit = start + Duration::from_millis(250);
        let end = start + Duration::from_millis(700);
        let ev = event("attack", "axe", 0, 15);
        let m = Motion::from_event(&ev, start, hit, end).expect("有出手者和目标");
        assert_eq!(m.style, Style::Melee);
        assert_eq!(m.approach(start), 0.0);
        assert!((m.approach(hit) - 1.0).abs() < f32::EPSILON, "接触时贴到目标");
        assert!((m.approach(end) - 0.0).abs() < f32::EPSILON, "结束时回到原位");
        assert!(m.approach(start + Duration::from_millis(125)) > 0.4);
    }

    #[test]
    fn ranged_weapons_stay_home_and_fly() {
        let start = Instant::now();
        let hit = start + Duration::from_millis(250);
        let end = start + Duration::from_millis(700);
        let ev = event("attack", "bow", 0, 15);
        let m = Motion::from_event(&ev, start, hit, end).expect("有出手者和目标");
        assert!(!m.style.moves(), "拿弓的不离开原位");
        assert_eq!(m.approach(hit), 1.0);
        assert!((m.flight(start + Duration::from_millis(125)) - 0.5).abs() < 0.01);
    }

    #[test]
    fn intent_wins_over_the_actual_body() {
        let mut ev = event("attack", "axe", 0, 17);
        ev.effects[0].intent = Some(15);
        let m = Motion::from_event(
            &ev,
            Instant::now(),
            Instant::now() + Duration::from_millis(250),
            Instant::now() + Duration::from_millis(700),
        )
        .unwrap();
        assert_eq!(m.toward, 15, "忠犬代受时动作朝原目标去");
    }

    #[test]
    fn five_outcomes_have_five_marks() {
        let marks: Vec<char> = [Outcome::Hit, Outcome::Crit, Outcome::Dodge, Outcome::Miss, Outcome::Block]
            .iter()
            .map(|o| contact_glyph(*o, true))
            .collect();
        let unique: std::collections::HashSet<char> = marks.iter().copied().collect();
        assert_eq!(unique.len(), 5, "命中、暴击、闪开、落空、挡住不能共用标记：{marks:?}");
        let ascii: Vec<char> = [Outcome::Hit, Outcome::Crit, Outcome::Dodge, Outcome::Miss, Outcome::Block]
            .iter()
            .map(|o| contact_glyph(*o, false))
            .collect();
        let unique: std::collections::HashSet<char> = ascii.iter().copied().collect();
        assert_eq!(unique.len(), 5, "关掉 Unicode 也要分得开：{ascii:?}");
    }

    #[test]
    fn strongest_result_wins_the_contact_mark() {
        let ev = BattleEvent {
            kind: "attack".into(),
            actor: Some(0),
            weapon: "axe".into(),
            effects: vec![
                BattleEffect { target: 15, result: "hit".into(), ..Default::default() },
                BattleEffect { target: 16, result: "crit".into(), ..Default::default() },
            ],
            ..Default::default()
        };
        let m = Motion::from_event(&ev, Instant::now(), Instant::now(), Instant::now()).unwrap();
        assert_eq!(m.outcome, Outcome::Crit, "同一次出手里暴击优先");
        assert_eq!(m.sweep, vec![15, 16], "扫过的目标顺序照服务端给");
    }

    #[test]
    fn system_events_have_no_motion() {
        let mut ev = event("status_tick", "", 0, 0);
        ev.actor = None;
        assert!(Motion::from_event(&ev, Instant::now(), Instant::now(), Instant::now()).is_none());
    }

    #[test]
    fn only_one_motion_per_actor() {
        let start = Instant::now();
        let m = |no: u8, toward: u8| Motion {
            no,
            toward,
            style: Style::Melee,
            weapon: "axe".into(),
            sweep: vec![toward],
            start,
            hit: start + Duration::from_millis(250),
            end: start + Duration::from_millis(700),
            outcome: Outcome::Hit,
            contacts: vec![(toward, Outcome::Hit)],
            hold: false,
        };
        let mut ms = Motions::default();
        ms.push(m(0, 15));
        ms.push(m(0, 16));
        assert_eq!(ms.all().len(), 1, "同一出手者只留最新的一条");
        assert_eq!(ms.of(0).unwrap().toward, 16);
    }
}
