//! 结算回放（STAGE-39、STAGE-110，`docs/battle-ui.md`「结算回放」）：一条事件分两拍播放。起手拍亮出行动者、写出字幕；
//! 命中拍改目标的血量和状态、做效果、补上字幕的结果；播完用 `state` 校正最终状态。
//! 这里只改本机的显示状态（血量、状态、在场单位、飘字、效果、记录），伤害和状态变化都由服务端算好。

mod effect;

use std::collections::VecDeque;
use std::time::{Duration, Instant};

use effect::{combo_floats, float_of, fx_kinds, slots_of};

use super::{BattleState, Float, FloatKind, Flown, Fx, FxKind, HpMove, Motion, Phase, SpellFx, SwapFx, brief, caption, slots};
use crate::app::Kind;
use crate::proto::{BattleEffect, BattleEvent, BattleField, BattleRound, BattleUnitState};

/// 时间（1x，毫秒）：起手到命中拍、一条事件的总长。捕捉和逃跑在起手和命中之间多停一拍。
const HIT_MS: u64 = 250;
const EVENT_MS: u64 = 700;
const PAUSE_MS: u64 = 400;
/// 命中拍的长度（250–600ms）：回复的血条涨完、闪避的文字回位的时间。
const HIT_SPAN_MS: u64 = 350;
/// 掉血残影缩掉的时间。
const GHOST_MS: u64 = 300;
/// 闪和抖的一段长度。
const SLOT_MS: u64 = 100;

/// 命中拍效果用到的时间，已经按速度换算好。
#[derive(Debug, Clone, Copy)]
struct Anim {
    now: Instant,
    slot: Duration,
    span: Duration,
    ghost: Duration,
    /// 整条事件结束的时刻：飘字在这之前一直显示。
    end: Instant,
}

/// 正在播的一条事件。
#[derive(Debug)]
struct Playing {
    ev: BattleEvent,
    start: Instant,
    hit: bool,
}

/// 正在播放的一回合结算。
#[derive(Debug)]
pub struct Playback {
    events: VecDeque<BattleEvent>,
    /// 回合结束时场上所有单位的状态，播完后用它校正。
    last_state: Vec<BattleUnitState>,
    /// 本回合的事件总数，以及已经开始播的条数（`结算中 3/7`）。
    total: usize,
    started: usize,
    playing: Option<Playing>,
    /// 下一条可以开始的时刻（上一条的结束时刻）。
    next_at: Instant,
}

impl Playback {
    pub fn new(round: BattleRound, now: Instant) -> Self {
        let events: VecDeque<BattleEvent> = round.events.into();
        Self { total: events.len(), events, last_state: round.state, started: 0, playing: None, next_at: now }
    }

    /// 结算进度：已经开始播的条数和总数。
    pub fn progress(&self) -> (usize, usize) {
        (self.started, self.total)
    }
}

impl BattleState {
    /// 按时间推进回放：到了节拍就推进一步。回放播完时返回 true，调用方据此发确认或弹结果。
    pub fn advance(&mut self, now: Instant) -> bool {
        if self.phase != Phase::Replay {
            return false;
        }
        self.expire(now);
        let Some(mut pb) = self.playback.take() else {
            return self.finish_playback(None);
        };
        if self.step(&mut pb, now) {
            return self.finish_playback(Some(pb));
        }
        self.playback = Some(pb);
        false
    }

    /// 立即播完（空格或回车）：正在播的那条直接命中，剩下的事件只写记录、改状态，不做动画；最后用最终状态校正。返回 true 表示已播完。
    pub fn skip(&mut self) -> bool {
        if self.phase != Phase::Replay {
            return false;
        }
        let Some(mut pb) = self.playback.take() else {
            return self.finish_playback(None);
        };
        if let Some(cur) = pb.playing.take().filter(|cur| !cur.hit) {
            self.hit(&cur.ev, None);
        }
        while let Some(ev) = pb.events.pop_front() {
            self.record(&ev);
            self.hit(&ev, None);
        }
        self.finish_playback(Some(pb))
    }

    /// 结算进度（`结算中 3/7` 里的 3 和 7）；不在回放时返回 None。
    pub fn replay_progress(&self) -> Option<(usize, usize)> {
        self.playback.as_ref().map(Playback::progress)
    }

    /// 按时间推进一步：命中拍到了就结算效果，一条的两拍走完就开始下一条。全部播完返回 true。
    fn step(&mut self, pb: &mut Playback, now: Instant) -> bool {
        let speed = self.speed.max(1);
        if let Some(mut cur) = pb.playing.take() {
            let (hit_ms, end_ms) = timing(&cur.ev);
            let end_at = cur.start + ms(end_ms, speed);
            if !cur.hit && now >= cur.start + ms(hit_ms, speed) {
                cur.hit = true;
                let anim = Anim {
                    now,
                    slot: ms(SLOT_MS, speed),
                    span: ms(HIT_SPAN_MS, speed),
                    ghost: ms(GHOST_MS, speed),
                    end: end_at,
                };
                self.hit(&cur.ev, Some(anim));
                self.caption = caption::build(self, &cur.ev, true);
            }
            if now < end_at {
                pb.playing = Some(cur);
                return false;
            }
            pb.next_at = end_at;
        }
        if pb.events.is_empty() {
            return true;
        }
        if now < pb.next_at {
            return false;
        }
        let Some(ev) = pb.events.pop_front() else {
            return true;
        };
        pb.started += 1;
        if !caption::known(&ev.kind) {
            // 不认识的事件：不写字幕、不播动画，只更新状态和记录（协议 10.7 的兼容规则）。
            if ev.kind == "field" {
                // 场地变化或恢复（STAGE-200）：状态跟着事件走，记录写“场地变成…”。
                self.field = ev.field.clone().filter(BattleField::active);
            }
            self.record(&ev);
            self.hit(&ev, None);
            return false;
        }
        self.current_lines = Some(self.record(&ev));
        self.caption = caption::build(self, &ev, false);
        if let Some(actor) = ev.actor {
            self.acting = Some((actor, now + ms(timing(&ev).1, speed)));
            self.acting_with = ev.actors.iter().copied().filter(|n| *n != actor).collect();
        }
        // 动作层（STAGE-167）：起手这一刻就把这条事件的位移/飞行/接触时间线排好，
        // 和血条、飘字用同一条时间线（接触前不会先扣展示血量）。
        let (hit_ms, end_ms) = timing(&ev);
        let hit_at = now + ms(hit_ms, speed);
        // 下一拍就是反击（被打的人还手）时，攻击者停在接触现场不回位，等攻防交换演完再走。
        let counter_next = pb
            .events
            .front()
            .filter(|next| next.kind == "counter" && ev.effects.iter().any(|e| Some(e.target) == next.actor));
        let hold = counter_next.is_some();
        let end_ms = end_ms + counter_next.map_or(0, |next| timing(next).1);
        let end_at = now + ms(end_ms, speed);
        if ev.kind == "combo" && !ev.actors.is_empty() {
            // STAGE-167：合击每个人都有身体动作，effects 与 actors 按协议一一对应。
            for (i, actor) in ev.actors.iter().enumerate() {
                let mut action = ev.clone();
                action.actor = Some(*actor);
                action.effects = ev.effects.get(i).cloned().into_iter().collect();
                if let Some(m) = Motion::from_event_holding(&action, now, hit_at, end_at, hold) {
                    self.motions.push(m);
                }
            }
        } else if let Some(m) = Motion::from_event_holding(&ev, now, hit_at, end_at, hold) {
            self.motions.push(m);
        }
        if let Some(fx) = SpellFx::from_event(&ev, now, hit_at, now + ms(timing(&ev).1, speed)) {
            self.spells.push(fx);
        }
        if let Some(fx) = SwapFx::from_event(&ev, now, hit_at, now + ms(timing(&ev).1, speed)) {
            self.swaps.push(fx);
        }
        pb.playing = Some(Playing { ev, start: now, hit: false });
        false
    }

    /// 回放播完：用最终状态校正，清掉所有动画。
    fn finish_playback(&mut self, pb: Option<Playback>) -> bool {
        self.flush_round_summary();
        if let Some(pb) = pb {
            for s in &pb.last_state {
                self.apply_state(s);
            }
        }
        self.clear_anim();
        true
    }

    /// 清掉所有动画的显示状态（行动高亮、飘字、效果、血条移动、字幕、记录高亮）；数据不动。
    pub(super) fn clear_anim(&mut self) {
        self.acting = None;
        self.acting_with.clear();
        self.floats.clear();
        self.fx.clear();
        self.hp_moves.clear();
        self.motions.clear();
        self.spells.clear();
        self.swaps.clear();
        self.caption.clear();
        self.current_lines = None;
    }

    /// 去掉过期的飘字、效果、血条移动和行动高亮。
    fn expire(&mut self, now: Instant) {
        self.floats.retain(|f| f.until > now);
        self.fx.retain(|f| f.until > now);
        self.hp_moves.retain(|m| m.until > now);
        self.motions.expire(now);
        self.spells.expire(now);
        self.swaps.expire(now);
        if self.acting.is_some_and(|(_, until)| until <= now) {
            self.acting = None;
            self.acting_with.clear();
        }
    }

    /// 写下一条事件的战斗记录，返回这几行在记录栏里的范围（右侧栏据此反色高亮）。
    fn record(&mut self, ev: &BattleEvent) -> (usize, usize) {
        let lines = self.describe(ev);
        let n = lines.len();
        // 简略档（brief.rs）：普通出招只进记录并累计伤害，“倒下了”这类排在最后的行和要紧事件照常写进日志。
        let quiet = if self.brief.on && !brief::is_key_event(&ev.kind) {
            self.tally_event(ev);
            n.saturating_sub(ev.effects.iter().filter(|e| e.down).count())
        } else {
            0
        };
        for (i, line) in lines.into_iter().enumerate() {
            if i < quiet {
                self.note_record(line);
            } else {
                self.note(line);
            }
        }
        // 装备耐久的提示（STAGE-225）排在这回合文字的后面，走消息日志，不进战斗记录。
        self.pending.extend(ev.crush.iter().map(|c| (Kind::durability(c.red), c.text.clone())));
        let end = self.records.len();
        (end.saturating_sub(n), end)
    }

    /// 命中拍：换下、换上的单位，和每个效果。`anim` 为 None 时（跳过、不认识的事件）不做效果。
    fn hit(&mut self, ev: &BattleEvent, anim: Option<Anim>) {
        for no in &ev.left {
            self.here.remove(no);
        }
        for u in &ev.joined {
            self.units.insert(u.no, u.clone());
            self.here.insert(u.no);
        }
        // 合击（STAGE-126）：几下打在同一个目标上，飘字合成一个总数，免得后一个盖住前一个。
        let combo = ev.kind == "combo";
        for eff in &ev.effects {
            self.apply_effect(eff, ev.actor, anim, !combo);
        }
        if let (true, Some(a)) = (combo, anim) {
            for (no, text, kind) in combo_floats(&ev.effects) {
                self.push_float(no, text, kind, a.end);
            }
        }
    }

    fn apply_effect(&mut self, eff: &BattleEffect, actor: Option<u8>, anim: Option<Anim>, float: bool) {
        if eff.flown {
            // STAGE-167：按左右物理列计算，敌方朝左离场、我方朝右离场。
            let target_column = slots::screen_column(self.me_side(), eff.target);
            let dir = actor.map_or(if eff.target / 10 == self.me_side() {1} else {-1}, |a| {
                let from = slots::screen_column(self.me_side(), a);
                if target_column > from {1} else if target_column < from {-1}
                else if eff.target / 10 == self.me_side() {1} else {-1}
            });
            let at = anim.map(|a| a.now).unwrap_or_else(Instant::now);
            self.flown.insert(eff.target, Flown { at, dir });
        }
        let before = self.units.get(&eff.target).map(|u| u.hp_pct);
        if let Some(u) = self.units.get_mut(&eff.target) {
            if let Some(pct) = eff.hp_pct {
                u.hp_pct = pct;
            }
            if let Some(hp) = eff.hp {
                u.hp = Some(hp);
            }
            if eff.down {
                u.hp_pct = 0;
            }
            u.status.retain(|s| !eff.status_remove.contains(s));
            for s in &eff.status_add {
                if !u.status.contains(s) {
                    u.status.push(s.clone());
                }
            }
        }
        let Some(a) = anim else {
            return;
        };
        let after = self.units.get(&eff.target).map(|u| u.hp_pct);
        if let (Some(from), Some(to)) = (before, after) {
            if from != to {
                // 回复：血条在命中拍内涨到新值；伤害：新值立即生效，多出来的一段残影慢慢缩掉。
                let until = if to > from { a.now + a.span } else { a.now + a.ghost };
                self.hp_moves.push(HpMove { no: eff.target, from, to, start: a.now, until });
            }
        }
        for kind in fx_kinds(eff, actor, self.me_side()) {
            let until = match kind {
                FxKind::Dodge(_) => a.now + a.span,
                _ => a.now + a.slot * slots_of(kind),
            };
            self.fx.push(Fx { no: eff.target, kind, from: a.now, until, slot: a.slot });
        }
        if let Some((text, kind)) = float_of(eff).filter(|_| float) {
            self.push_float(eff.target, text, kind, a.end);
        }
    }

    fn push_float(&mut self, no: u8, text: String, kind: FloatKind, until: Instant) {
        self.floats.push(Float { no, text, kind, until });
    }

    /// 回合结束时的最终状态（只校正，不做动画）。
    fn apply_state(&mut self, s: &BattleUnitState) {
        if let Some(u) = self.units.get_mut(&s.no) {
            u.hp_pct = if s.down { 0 } else { s.hp_pct };
            if let Some(hp) = s.hp {
                u.hp = Some(hp);
            }
            if let Some(mp) = s.mp {
                u.mp = Some(mp);
            }
            u.status = s.status.clone();
            u.mount = s.mount.clone(); // STAGE-250：骑宠血量每回合校正，没骑乘时清掉
        }
    }
}

/// 一条事件的两拍：命中拍开始的时刻和整条的长度（毫秒，1x）。
fn timing(ev: &BattleEvent) -> (u64, u64) {
    if matches!(ev.kind.as_str(), "capture" | "escape") {
        (HIT_MS + PAUSE_MS, EVENT_MS + PAUSE_MS)
    } else {
        (HIT_MS, EVENT_MS)
    }
}

/// 按速度换算时长：2x 时减半。
fn ms(v: u64, speed: u32) -> Duration {
    Duration::from_millis(v / u64::from(speed.max(1)))
}

#[cfg(test)]
mod tests;
