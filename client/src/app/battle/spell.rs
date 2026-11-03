//! 精灵表现层（STAGE-167，`docs/battle-ui.md`「精灵与状态」）：施法起手/聚集 → 作用范围 → 生效/持续。
//!
//! 形态全部按服务端下发的 `magic.func` / `elem` / `all` / `status` 选，不看精灵名字：
//! 地水火风各有自己的字符轮廓（岩柱、水珠、火舌、风弧），治疗往上、复活从倒下位置起身、
//! 净化把已有状态分解掉、六种异常各有自己的施加形态。认不出的精灵用中性标记，
//! 但起手、范围、生效时刻照常，不因为认不出而丢效果。
//!
//! 这里只负责「这一帧该在谁身上画什么」；范围来自事件真实的 `effects`，
//! 客户端不扩大范围、不给没被作用的目标补反馈。

use std::time::Instant;

pub use super::glyph::{effect_glyph, element_glyph, status_glyph};

use crate::proto::BattleEvent;

/// 精灵属性。攻击系才有；治疗、复活、解状态没有属性。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Elem {
    Earth,
    Water,
    Fire,
    Wind,
    None,
}

impl Elem {
    fn of(value: &str) -> Self {
        match value {
            "earth" => Elem::Earth,
            "water" => Elem::Water,
            "fire" => Elem::Fire,
            "wind" => Elem::Wind,
            _ => Elem::None,
        }
    }
}

/// 精灵的表现类别（由 `magic.func` 决定）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SpellKind {
    /// 攻击（单体或全体）。
    Damage,
    /// 治愈自己（含我方全体）。
    HealSelf,
    /// 治愈别人（滋润、恩惠一类）。
    HealOther,
    /// 复活。
    Revive,
    /// 解状态（净化）。
    Cure,
    /// 施加异常状态。
    Status,
    /// 认不出的类别：中性标记，但照常起手和生效。
    Neutral,
}

impl SpellKind {
    fn of(value: &str) -> Self {
        match value {
            "attack" => SpellKind::Damage,
            "heal" => SpellKind::HealSelf,
            "heal_other" => SpellKind::HealOther,
            "revive" => SpellKind::Revive,
            "cure" => SpellKind::Cure,
            "status" => SpellKind::Status,
            _ => SpellKind::Neutral,
        }
    }

    /// 治疗类要先涨血再有光粒，攻击类先有冲击再扣血，这里只区分「往上的」和「往下的」。
    pub fn rises(self) -> bool {
        matches!(self, SpellKind::HealSelf | SpellKind::HealOther | SpellKind::Revive)
    }
}

/// 一次施法的表现时间线。
#[derive(Debug, Clone)]
pub struct SpellFx {
    pub caster: u8,
    /// 真实被作用的目标（来自事件的 `effects`，按服务端顺序）。
    pub targets: Vec<u8>,
    pub kind: SpellKind,
    pub elem: Elem,
    /// 是不是全体（协议里的 `magic.all`）；只用来在字幕/详情里说明，范围仍以 `targets` 为准。
    pub all: bool,
    /// 状态精灵挂的状态、解状态精灵解的状态。
    pub status: Option<String>,
    pub start: Instant,
    /// 生效时刻（命中拍）。
    pub hit: Instant,
    pub end: Instant,
}

/// 生效标记留多久（接触之后）。比整条事件短，免得压住下一拍。
const EFFECT_MS: u64 = 320;

impl SpellFx {
    /// 从一条 `magic` 事件推出表现。没有施法者时返回 None；旧服务端不带 `magic` 时
    /// 退化成 `Neutral`/`None` 的中性表现，仍然起手和生效。
    pub fn from_event(ev: &BattleEvent, start: Instant, hit: Instant, end: Instant) -> Option<Self> {
        if ev.kind != "magic" {
            return None;
        }
        let caster = ev.actor?;
        let view = ev.magic.clone().unwrap_or_default();
        if view.func == "field" {
            return None; // 场地属性（STAGE-200）没有对准谁，不做施法特效；场地的变化写在记录和顶部
        }
        let kind = SpellKind::of(&view.func);
        let mut targets: Vec<u8> = ev.effects.iter().map(|e| e.target).collect();
        if targets.is_empty() {
            // 没有效果（例如全都被挡住）时至少标出施法者自己，避免整条动作凭空消失。
            targets.push(ev.target.unwrap_or(caster));
        }
        let status = (!view.status.is_empty()).then_some(view.status.clone());
        Some(Self {
            caster,
            targets,
            kind,
            elem: Elem::of(&view.elem),
            all: view.all,
            status,
            start,
            hit,
            end,
        })
    }

    /// 聚集进度：起手到生效，0 → 1。
    pub fn gather(&self, now: Instant) -> f32 {
        if now <= self.start {
            return 0.0;
        }
        if now >= self.hit {
            return 1.0;
        }
        let span = self.hit.saturating_duration_since(self.start);
        if span.is_zero() {
            return 1.0;
        }
        (now.saturating_duration_since(self.start).as_secs_f32() / span.as_secs_f32()).clamp(0.0, 1.0)
    }

    /// 生效标记还该不该画（接触之后的一小段）。
    pub fn showing(&self, now: Instant) -> bool {
        now >= self.hit && now < self.hit + std::time::Duration::from_millis(EFFECT_MS)
    }

    pub fn alive(&self, now: Instant) -> bool {
        now >= self.start && now < self.end
    }

    /// 起手时画在施法者身上的聚集标记：按时间呼吸，两段亮一段灭，不闪整张卡。
    pub fn gathering_glyph(&self, now: Instant, unicode: bool) -> Option<char> {
        if now >= self.hit {
            return None;
        }
        let span = self.hit.saturating_duration_since(self.start).as_millis().max(1);
        let step = 120u128;
        let on = (now.saturating_duration_since(self.start).as_millis() / step) % 2 == 0;
        if !on {
            return None;
        }
        let _ = span;
        Some(match (self.kind, unicode) {
            (SpellKind::Damage, true) => element_glyph(self.elem, true),
            (SpellKind::Damage, false) => element_glyph(self.elem, false),
            (_, true) => '✧',
            (_, false) => '*',
        })
    }
}

/// 精灵动作的一次性状态。
#[derive(Debug, Clone, Default)]
pub struct Spells {
    items: Vec<SpellFx>,
}

impl Spells {
    pub fn clear(&mut self) {
        self.items.clear();
    }

    pub fn push(&mut self, s: SpellFx) {
        // 同一个施法者只留最新一条，避免上一条没演完下一条就叠着亮。
        self.items.retain(|old| old.caster != s.caster);
        self.items.push(s);
    }

    pub fn expire(&mut self, now: Instant) {
        self.items.retain(|s| s.alive(now) || s.showing(now));
    }

    pub fn all(&self) -> &[SpellFx] {
        &self.items
    }

    /// 某个单位身上的生效标记（多条时取最新的一条）。
    pub fn effect_on(&self, no: u8, now: Instant) -> Option<&SpellFx> {
        self.items
            .iter()
            .rev()
            .find(|s| s.showing(now) && s.targets.contains(&no))
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::proto::{BattleEffect, BattleEvent, SpellView};

    fn magic(func: &str, elem: &str, all: bool, status: &str, targets: &[u8]) -> BattleEvent {
        BattleEvent {
            kind: "magic".into(),
            actor: Some(0),
            magic: Some(SpellView {
                id: 1,
                func: func.into(),
                elem: elem.into(),
                all,
                status: status.into(),
            }),
            effects: targets
                .iter()
                .map(|t| BattleEffect {
                    target: *t,
                    result: "none".into(),
                    ..Default::default()
                })
                .collect(),
            ..Default::default()
        }
    }

    fn span() -> (Instant, Instant, Instant) {
        let start = Instant::now();
        (start, start + Duration::from_millis(250), start + Duration::from_millis(700))
    }

    #[test]
    fn four_elements_have_four_shapes() {
        let shapes: Vec<char> = [Elem::Earth, Elem::Water, Elem::Fire, Elem::Wind]
            .iter()
            .map(|e| element_glyph(*e, true))
            .collect();
        let unique: std::collections::HashSet<char> = shapes.iter().copied().collect();
        assert_eq!(unique.len(), 4, "地水火风不能共用一个符号：{shapes:?}");
        let ascii: Vec<char> = [Elem::Earth, Elem::Water, Elem::Fire, Elem::Wind]
            .iter()
            .map(|e| element_glyph(*e, false))
            .collect();
        let unique: std::collections::HashSet<char> = ascii.iter().copied().collect();
        assert_eq!(unique.len(), 4, "关掉 Unicode 也要能分开：{ascii:?}");
    }

    #[test]
    fn all_targets_are_the_events_own_effects() {
        let (start, hit, end) = span();
        let ev = magic("attack", "fire", true, "", &[10, 11, 12]);
        let fx = SpellFx::from_event(&ev, start, hit, end).unwrap();
        assert_eq!(fx.targets, vec![10, 11, 12], "范围来自事件真实效果，不自己扩");
        assert!(fx.all);
        assert_eq!(fx.kind, SpellKind::Damage);
        assert_eq!(fx.elem, Elem::Fire);
    }

    #[test]
    fn self_heal_starts_and_ends_on_the_caster() {
        let (start, hit, end) = span();
        let mut ev = magic("heal", "", false, "", &[]);
        ev.effects.clear();
        let fx = SpellFx::from_event(&ev, start, hit, end).unwrap();
        assert_eq!(fx.targets, vec![0], "没有效果时至少标出施法者，不画指向敌人的假轨迹");
        assert!(fx.kind.rises());
    }

    #[test]
    fn revive_and_cure_look_different_from_heal() {
        let (start, hit, end) = span();
        let revive = SpellFx::from_event(&magic("revive", "", false, "", &[6]), start, hit, end).unwrap();
        let cure = SpellFx::from_event(&magic("cure", "", false, "poison", &[6]), start, hit, end).unwrap();
        let heal = SpellFx::from_event(&magic("heal_other", "", false, "", &[6]), start, hit, end).unwrap();
        let glyphs = [effect_glyph(&revive, true), effect_glyph(&cure, true), effect_glyph(&heal, true)];
        let unique: std::collections::HashSet<char> = glyphs.iter().copied().collect();
        assert_eq!(unique.len(), 3, "复活、净化、治疗要分得开：{glyphs:?}");
    }

    #[test]
    fn six_statuses_have_six_shapes() {
        let shapes: Vec<char> = ["poison", "paralysis", "sleep", "stone", "drunk", "confusion"]
            .iter()
            .map(|s| status_glyph(s, true))
            .collect();
        let unique: std::collections::HashSet<char> = shapes.iter().copied().collect();
        assert_eq!(unique.len(), 6, "六种异常不能共用一个符号：{shapes:?}");
    }

    #[test]
    fn gather_runs_up_to_the_hit_and_then_shows_the_effect() {
        let (start, hit, end) = span();
        let fx = SpellFx::from_event(&magic("attack", "water", false, "", &[15]), start, hit, end).unwrap();
        assert_eq!(fx.gather(start), 0.0);
        assert!((fx.gather(start + Duration::from_millis(125)) - 0.5).abs() < 0.01);
        assert_eq!(fx.gather(hit), 1.0);
        assert!(fx.showing(hit));
        assert!(!fx.showing(hit + Duration::from_millis(EFFECT_MS)), "生效标记不能一直挂着");
    }

    #[test]
    fn old_servers_without_magic_data_still_play_neutrally() {
        let (start, hit, end) = span();
        let ev = BattleEvent {
            kind: "magic".into(),
            actor: Some(0),
            effects: vec![BattleEffect { target: 15, result: "hit".into(), ..Default::default() }],
            ..Default::default()
        };
        let fx = SpellFx::from_event(&ev, start, hit, end).expect("没有 magic 也要有中性表现");
        assert_eq!(fx.kind, SpellKind::Neutral);
        assert_eq!(fx.elem, Elem::None);
        assert_eq!(fx.targets, vec![15]);
        assert_eq!(effect_glyph(&fx, true), '◆');
    }
}
