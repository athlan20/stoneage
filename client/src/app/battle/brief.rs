//! 简略战斗日志：观察模式（agent 操控）默认开，`battle-log` 命令可改。
//! 逐事件的战斗文字刷得太快，聊天和系统提示几秒就被挤出日志区；简略档把普通攻击、精灵、道具、技能
//! 合成每回合一行汇总，只保留倒下、捕捉、逃跑、换宠、场地这类要紧的事。完整记录仍在 `records`（回放高亮用），
//! 装备耐久等其他消息不受影响。

use super::{slots, BattleState};
use crate::app::{App, Kind};
use crate::proto::BattleEvent;

/// 战斗日志的档位：自动（观察模式简略、自己操控完整）、强制简略、强制完整。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum LogMode {
    #[default]
    Auto,
    Brief,
    Full,
}

impl LogMode {
    pub fn parse(word: &str) -> Option<Self> {
        match word {
            "auto" | "自动" => Some(Self::Auto),
            "brief" | "简略" => Some(Self::Brief),
            "full" | "完整" | "详细" => Some(Self::Full),
            _ => None,
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Auto => "自动（agent 操控时简略，自己操控时完整）",
            Self::Brief => "简略",
            Self::Full => "完整",
        }
    }

    /// 这一档在当前是不是简略输出。
    pub fn is_brief(self, observing: bool) -> bool {
        match self {
            Self::Auto => observing,
            Self::Brief => true,
            Self::Full => false,
        }
    }
}

/// 简略档的状态：当前是否简略，以及本回合已播事件的伤害累计。
#[derive(Debug, Default, Clone)]
pub struct BriefLog {
    pub on: bool,
    dealt: i64,
    taken: i64,
    healed: i64,
}

/// 简略档下仍逐条写的事件：这些事不能被汇总吞掉。
pub fn is_key_event(kind: &str) -> bool {
    matches!(kind, "capture" | "escape" | "swap_pet" | "field" | "pet_escape" | "disobey")
}

impl BattleState {
    /// 累计一条事件的伤害和回复（按目标所在的一方分己方、敌方）。
    pub(super) fn tally_event(&mut self, ev: &BattleEvent) {
        let me = self.me_side();
        for e in &ev.effects {
            let mine = slots::cell_of(e.target).side == me;
            match (e.delta, mine) {
                (d, true) if d < 0 => self.brief.taken -= d,
                (d, false) if d < 0 => self.brief.dealt -= d,
                (d, true) if d > 0 => self.brief.healed += d,
                _ => {}
            }
        }
    }

    /// 回放播完：写一行回合汇总（只在简略档、且这回合有过非要紧事件时），并清零累计。
    pub(super) fn flush_round_summary(&mut self) {
        let b = std::mem::take(&mut self.brief);
        self.brief.on = b.on;
        if !b.on || self.turn == 0 {
            return;
        }
        let mut parts = Vec::new();
        if b.dealt > 0 {
            parts.push(format!("造成 {} 伤害", b.dealt));
        }
        if b.taken > 0 {
            parts.push(format!("受到 {} 伤害", b.taken));
        }
        if b.healed > 0 {
            parts.push(format!("回复 {}", b.healed));
        }
        if parts.is_empty() {
            return;
        }
        self.pending.push((Kind::Battle, format!("第 {} 回合：{}", self.turn, parts.join("，"))));
    }
}

impl App {
    /// `battle-log [auto|brief|full]`：不带参数查询当前档位。
    pub(in crate::app) fn battle_log_command(&mut self, rest: &str) {
        match (rest.trim(), LogMode::parse(rest.trim())) {
            ("", _) => {}
            (_, Some(mode)) => self.battle_log = mode,
            _ => {
                self.log.error("用法：battle-log auto|brief|full（简写 战斗日志）");
                return;
            }
        }
        self.log.system(format!("战斗日志：{}", self.battle_log.label()));
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{BattleEffect, BattleRound, BattleStart, BattleTurn, BattleUnit, Progress};
    use std::time::Instant;

    fn unit(no: u8, name: &str) -> BattleUnit {
        BattleUnit { no, name: name.into(), level: 3, hp_pct: 100, ..BattleUnit::default() }
    }

    fn effect(target: u8, delta: i64, down: bool) -> BattleEffect {
        BattleEffect { target, result: if delta < 0 { "hit" } else { "heal" }.into(), delta, down, ..BattleEffect::default() }
    }

    fn event(kind: &str, actor: u8, effects: Vec<BattleEffect>) -> BattleEvent {
        BattleEvent { kind: kind.into(), actor: Some(actor), effects, ..BattleEvent::default() }
    }

    /// 播完一回合的事件，取走新增的消息文字。
    fn replay(brief: bool, events: Vec<BattleEvent>) -> Vec<String> {
        let mut s = BattleState::from_start(BattleStart {
            you: 0,
            units: vec![unit(0, "勇者"), unit(5, "乌力"), unit(15, "野猪")],
            ..BattleStart::default()
        });
        s.brief.on = brief;
        let turn = BattleTurn {
            turn: 3,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: vec!["player".into()],
            progress: Progress { done: 0, total: 1 },
            ..BattleTurn::default()
        };
        s.on_turn(turn, Instant::now());
        s.take_pending();
        s.on_round(BattleRound { turn: 3, events, ..BattleRound::default() }, Instant::now());
        s.skip();
        s.take_pending().into_iter().map(|(_, t)| t).collect()
    }

    fn round() -> Vec<BattleEvent> {
        vec![
            event("attack", 0, vec![effect(15, -30, false)]),
            event("attack", 5, vec![effect(15, -12, false)]),
            event("attack", 15, vec![effect(0, -7, false)]),
            event("item", 0, vec![effect(0, 10, false)]),
        ]
    }

    #[test]
    fn brief_writes_one_summary_line_per_round() {
        assert_eq!(replay(true, round()), ["第 3 回合：造成 42 伤害，受到 7 伤害，回复 10"]);
    }

    #[test]
    fn full_keeps_every_event_and_no_summary() {
        let lines = replay(false, round());
        assert_eq!(lines.len(), 4);
        assert!(lines.iter().all(|l| !l.contains("回合")));
    }

    #[test]
    fn brief_keeps_downs_and_key_events_before_the_summary() {
        let mut events = round();
        events.push(event("attack", 0, vec![effect(15, -5, true)]));
        events.push(BattleEvent { ok: Some(true), ..event("escape", 0, vec![]) });
        let lines = replay(true, events);
        assert_eq!(lines, ["野猪 倒下了", "勇者 逃跑：成功", "第 3 回合：造成 47 伤害，受到 7 伤害，回复 10"]);
    }

    #[test]
    fn brief_with_no_damage_writes_no_summary() {
        assert!(replay(true, vec![event("guard", 0, vec![])]).is_empty());
    }
}

#[cfg(test)]
mod mode_tests {
    use super::*;

    #[test]
    fn auto_follows_the_observer_flag() {
        assert!(LogMode::Auto.is_brief(true));
        assert!(!LogMode::Auto.is_brief(false));
        assert!(LogMode::Brief.is_brief(false));
        assert!(!LogMode::Full.is_brief(true));
    }

    #[test]
    fn parse_accepts_english_and_chinese() {
        assert_eq!(LogMode::parse("简略"), Some(LogMode::Brief));
        assert_eq!(LogMode::parse("full"), Some(LogMode::Full));
        assert_eq!(LogMode::parse("x"), None);
    }
}
