//! 换宠动作链（STAGE-167，`docs/battle-ui.md`「换宠」）：旧宠物先收回**自己的主人**身上，
//! 收起之后新宠物才从同一个主人处放出、走到该主人的出战位。
//!
//! 关节点和原版一致（`battle.c:6715` 的 `BATTLE_COM_PETOUT` 先 `BATTLE_PetIn` 再 `BATTLE_PetOut`），
//! 所以这里按「收回 → 放出」两段排时间线，中间那一下正好落在事件的命中拍：
//! 命中拍之前旧宠物还在场（服务端还没应用 `left`），命中拍之后新宠物才上场（`joined` 已应用）。
//!
//! 主人取事件的 `actor`，不是本地玩家：本人、队友、敌方换宠都回各自的主人。主人不在场
//! （倒下、已离场、数据缺失）时这一段不动，宠物留在原位收起，安全降级而不画向别人。

use std::time::Instant;

use crate::proto::BattleEvent;

/// 一次换宠的两段动作。
#[derive(Debug, Clone)]
pub struct SwapFx {
    /// 主人（事件的 `actor`）。旧宠物收回这里、新宠物从这里放出。
    pub owner: u8,
    /// 要收回的旧宠物（`left` 的第一项）。
    pub old: Option<u8>,
    /// 要放出的新宠物（`joined` 的第一项）。
    pub new: Option<u8>,
    pub start: Instant,
    /// 旧宠物收起的时刻（= 事件的命中拍）。
    pub hit: Instant,
    /// 新宠物走到出战位的时刻。
    pub end: Instant,
}

impl SwapFx {
    /// 从一条 `swap_pet` 事件推出动作。没有主人，或者既没收回也没放出时返回 None。
    pub fn from_event(ev: &BattleEvent, start: Instant, hit: Instant, end: Instant) -> Option<Self> {
        if ev.kind != "swap_pet" {
            return None;
        }
        let owner = ev.actor?;
        let old = ev.left.first().copied();
        let new = ev.joined.first().map(|u| u.no);
        if old.is_none() && new.is_none() {
            return None;
        }
        Some(Self { owner, old, new, start, hit, end })
    }

    /// 收回进度：0 是还在出战位，1 是已经收到主人身上（命中拍）。
    pub fn reclaim(&self, now: Instant) -> f32 {
        progress(self.start, self.hit, now)
    }

    /// 放出进度：0 是刚在主人身上出现（命中拍），1 是走到自己的出战位。
    pub fn release(&self, now: Instant) -> f32 {
        progress(self.hit, self.end, now)
    }

    /// 这条换宠现在还在演。
    pub fn alive(&self, now: Instant) -> bool {
        now >= self.start && now < self.end
    }
}

/// 两段各自的进度，接触点之前是去程、之后是回程，都用同一套插值。
fn progress(from: Instant, to: Instant, now: Instant) -> f32 {
    if now <= from {
        return 0.0;
    }
    if now >= to {
        return 1.0;
    }
    let span = to.saturating_duration_since(from);
    if span.is_zero() {
        return 1.0;
    }
    (now.saturating_duration_since(from).as_secs_f32() / span.as_secs_f32()).clamp(0.0, 1.0)
}

/// 换宠动作的一次性状态。
#[derive(Debug, Clone, Default)]
pub struct Swaps {
    items: Vec<SwapFx>,
}

impl Swaps {
    pub fn clear(&mut self) {
        self.items.clear();
    }

    pub fn push(&mut self, s: SwapFx) {
        // 同一个主人只留最新的一条：连着换两次时不要两条动作叠着演。
        self.items.retain(|old| old.owner != s.owner);
        self.items.push(s);
    }

    pub fn expire(&mut self, now: Instant) {
        self.items.retain(|s| s.alive(now));
    }

    pub fn all(&self) -> &[SwapFx] {
        &self.items
    }

    /// 某个单位现在是不是正在被收回（旧宠物）。
    pub fn reclaiming(&self, no: u8, now: Instant) -> Option<&SwapFx> {
        self.items
            .iter()
            .find(|s| s.old == Some(no) && now >= s.start && now < s.hit)
    }

    /// 某个单位现在是不是正在被放出（新宠物）。
    pub fn releasing(&self, no: u8, now: Instant) -> Option<&SwapFx> {
        self.items
            .iter()
            .find(|s| s.new == Some(no) && now >= s.hit && now < s.end)
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::proto::BattleUnit;

    fn swap_event(owner: u8, old: Option<u8>, new: Option<u8>) -> BattleEvent {
        let mut ev = BattleEvent {
            kind: "swap_pet".into(),
            actor: Some(owner),
            ..Default::default()
        };
        if let Some(no) = old {
            ev.left.push(no);
        }
        if let Some(no) = new {
            let mut u = BattleUnit::default();
            u.no = no;
            ev.joined.push(u);
        }
        ev
    }

    fn timeline() -> (Instant, Instant, Instant) {
        let start = Instant::now();
        (start, start + Duration::from_millis(250), start + Duration::from_millis(700))
    }

    #[test]
    fn full_swap_has_two_stages_in_order() {
        let (start, hit, end) = timeline();
        let fx = SwapFx::from_event(&swap_event(0, Some(5), Some(6)), start, hit, end).unwrap();
        assert_eq!(fx.owner, 0, "主人取事件的 actor");
        assert_eq!(fx.old, Some(5));
        assert_eq!(fx.new, Some(6));
        assert_eq!(fx.reclaim(start), 0.0);
        assert_eq!(fx.reclaim(hit), 1.0, "命中拍时旧宠物已经收在主人物身上");
        assert_eq!(fx.release(hit), 0.0, "同一时刻新宠物还在主人身上");
        assert_eq!(fx.release(end), 1.0, "到结束时走到出战位");
        assert!((fx.release(start + Duration::from_millis(475)) - 0.5).abs() < 0.01);
    }

    #[test]
    fn reclaim_only_and_release_only() {
        let (start, hit, end) = timeline();
        let only_in = SwapFx::from_event(&swap_event(0, Some(5), None), start, hit, end).unwrap();
        assert_eq!(only_in.new, None, "只收回时不制造一只待放出的宠物");
        let only_out = SwapFx::from_event(&swap_event(0, None, Some(6)), start, hit, end).unwrap();
        assert_eq!(only_out.old, None, "只放出时没有待回收的旧宠物");
    }

    #[test]
    fn owner_comes_from_the_event_not_from_the_local_player() {
        let (start, hit, end) = timeline();
        // 队友（5 号位是别人的宠物排）换宠：主人是 14，不是本地玩家 0。
        let fx = SwapFx::from_event(&swap_event(14, Some(15), Some(16)), start, hit, end).unwrap();
        assert_eq!(fx.owner, 14);
    }

    #[test]
    fn a_swap_without_any_pet_is_not_an_animation() {
        let (start, hit, end) = timeline();
        assert!(SwapFx::from_event(&swap_event(0, None, None), start, hit, end).is_none());
        assert!(SwapFx::from_event(&swap_event(0, Some(5), Some(6)).clone(), start, hit, end).is_some());
    }

    #[test]
    fn newest_swap_per_owner_replaces_the_old_one() {
        let (start, hit, end) = timeline();
        let mut swaps = Swaps::default();
        swaps.push(SwapFx::from_event(&swap_event(0, Some(5), Some(6)), start, hit, end).unwrap());
        swaps.push(SwapFx::from_event(&swap_event(0, Some(6), Some(7)), start, hit, end).unwrap());
        assert_eq!(swaps.all().len(), 1, "同一主人只留最新一条");
    }

    #[test]
    fn stages_are_visible_only_inside_their_window() {
        let (start, hit, end) = timeline();
        let mut swaps = Swaps::default();
        swaps.push(SwapFx::from_event(&swap_event(0, Some(5), Some(6)), start, hit, end).unwrap());
        assert!(swaps.reclaiming(5, start).is_some());
        assert!(swaps.reclaiming(5, hit).is_none(), "命中拍之后旧宠物已经收起");
        assert!(swaps.releasing(6, hit).is_some());
        assert!(swaps.releasing(6, end).is_none(), "走到位之后不再单独处理");
        swaps.expire(end);
        assert!(swaps.all().is_empty());
    }
}
