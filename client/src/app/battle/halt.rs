//! 主动停机协调（STAGE-153，`docs/battle-ui.md`「观察模式与托管」）：玩家自己按下的
//! 「停止自动挂机」统一走这里——战斗菜单的管理项、战斗控制面板、系统页和 `auto off` 命令。
//!
//! 两步走：先立即关掉本机的自动出招（清 `auto_due`、重试和自动继续，本轮 tick 不再发招），
//! 再在在线且有控制权时发 `encounter off` 关掉后续遇敌。服务端状态只能由权威响应/推送确认，
//! 发出请求后写「正在关闭自动遇敌」，确认后才写「自动遇敌已关闭」。
//!
//! 关掉挂机是单向联动：挂机 → 遇敌；只关遇敌（场景页、系统页的「停止自动遇敌」）不动挂机。
//! 因失败、断线、失去控制权而停下的情况另有说明，不冒充「已经远程停止」。

use std::time::Instant;

use serde_json::json;

use crate::app::session::Pending;
use crate::app::{App, WorldState};

/// 发出 `encounter off` 后等权威确认多久。超时先查状态，再让玩家决定是否重试。
pub const HALT_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(5);

/// 主动停机的状态：停止意图在途、上一次的失败原因。跟着世界快照走（重连后重新开始）。
#[derive(Debug, Default)]
pub struct HaltState {
    /// 停止意图在途：已经发出 `encounter off`，等权威确认。
    pub stopping: bool,
    /// 发出停止请求的时刻（超时判断）。
    pub since: Option<Instant>,
    /// 超时后已经查过一次权威状态，避免每个刷新周期都发查询。
    pub queried: bool,
    /// 上一次主动停机的结果：失败或状态待确认时留着原因，系统页和消息栏据此提示。
    pub error: Option<String>,
}

impl HaltState {
    /// 停止意图在途：这时不允许重新开启挂机或遇敌。
    pub fn busy(&self) -> bool {
        self.stopping
    }

    /// 开始一次停止意图（重复按下的第二次不会再发请求）。
    fn begin(&mut self, now: Instant) {
        self.stopping = true;
        self.since = Some(now);
        self.queried = false;
        self.error = None;
    }

    /// 权威状态已经确认关闭：结束停止意图。
    fn done(&mut self) {
        self.stopping = false;
        self.since = None;
        self.queried = false;
        self.error = None;
    }

    /// 失败或状态待确认：挂机保持关闭，记下原因等玩家重试。
    fn fail(&mut self, reason: String) {
        self.stopping = false;
        self.since = None;
        self.queried = false;
        self.error = Some(reason);
    }
}

impl App {
    /// 停止意图在途（关闭中）。
    pub(crate) fn halt_busy(&self) -> bool {
        self.world.as_ref().is_some_and(|w| w.halt.busy())
    }

    /// 主动停止自动挂机（统一入口）：先关本机自动出招，再关后续遇敌。
    /// `origin` 是入口名字，只用在消息里，说明是从哪里停的。
    pub(crate) fn stop_auto_all(&mut self, origin: &str) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let (online, observing, encounter_on) = (w.online, w.observing(), w.entry.encounter);
        self.stop_local_auto();
        if self.halt_busy() {
            self.log.system("正在关闭自动遇敌，请稍候");
            return;
        }
        if !online {
            self.fail_halt("连接已断开，无法确认远端遇敌状态");
            self.log.system(format!("自动挂机已停止（{origin}）；连接已断开，自动遇敌状态未知"));
            return;
        }
        if observing {
            self.fail_halt("agent 操控中，没有权限关闭对方的遇敌");
            self.log.error(format!("自动挂机已停止（{origin}）；agent 操控中，没有权限关闭对方的自动遇敌"));
            return;
        }
        if !encounter_on && !self.session.encounter_busy() {
            self.log.system(format!("自动挂机已停止（{origin}）；自动遇敌本来就是关闭的"));
            return;
        }
        self.request_encounter_off("自动挂机已停止；正在关闭自动遇敌");
    }

    /// 停止保护（STAGE-173 M11）：方案里的固定条件命中时，由托管自己停下。
    /// 流程和玩家主动停止完全一样（先关本机自动出招、再请求关闭后续遇敌），
    /// 区别只在消息里写明是「停止保护」以及命中了哪些条件。
    pub(crate) fn stop_by_guard(&mut self, reasons: &str) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let (online, observing, encounter_on) = (w.online, w.observing(), w.entry.encounter);
        self.stop_local_auto();
        self.log.system(format!("自动挂机已停止：停止保护（{reasons}）"));
        if self.halt_busy() {
            self.log.system("正在关闭自动遇敌，请稍候");
            return;
        }
        if !online {
            self.fail_halt("连接已断开，无法确认远端遇敌状态");
            self.log.system("停止保护命中；连接已断开，自动遇敌状态未知");
            return;
        }
        if observing {
            self.fail_halt("agent 操控中，没有权限关闭对方的遇敌");
            self.log.error("停止保护命中；agent 操控中，没有权限关闭对方的自动遇敌");
            return;
        }
        if !encounter_on && !self.session.encounter_busy() {
            self.log.system("自动遇敌本来就是关闭的；补给或条件恢复后可以重新开启自动挂机");
            return;
        }
        self.request_encounter_off("正在关闭自动遇敌");
    }

    /// 只关后续遇敌，不动本地自动挂机（场景页和系统页的「停止自动遇敌」）。
    pub(crate) fn stop_encounter_only(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let (online, observing, encounter_on) = (w.online, w.observing(), w.entry.encounter);
        if self.halt_busy() {
            self.log.system("正在关闭自动遇敌，请稍候");
            return;
        }
        if !online {
            self.log.error("连接已断开，无法停止自动遇敌；请从系统菜单重新连接");
            return;
        }
        if observing {
            self.log.error("agent 操控中，没有权限关闭对方的自动遇敌");
            return;
        }
        if !encounter_on && !self.session.encounter_busy() {
            self.log.system("自动遇敌本来就是关闭的");
            return;
        }
        self.request_encounter_off("正在关闭自动遇敌");
    }

    /// 立即关掉本机的自动出招，清掉这一阶段的所有自动调度（本轮 tick 不再发招）。
    pub(crate) fn stop_local_auto(&mut self) {
        if let Some(w) = self.world.as_mut() {
            w.auto_battle = false;
            if let Some(b) = w.battle.as_mut() {
                b.auto_due = None;
                b.auto_retries = 0;
                b.last_auto = None;
            }
        }
    }

    /// 发 `encounter off`：标记停止意图、写第一句提示；发不出去按失败处理。返回是否发出去了。
    pub(crate) fn request_encounter_off(&mut self, pending_text: &str) -> bool {
        let now = Instant::now();
        if let Some(w) = self.world.as_mut() {
            w.halt.begin(now);
        }
        if self.send("encounter", json!({"set": "off"}), Pending::EncounterStop) {
            self.log.system(pending_text);
            true
        } else {
            self.halt_failed("请求没有发出去（连接已断开）");
            false
        }
    }

    /// 每个刷新周期查一次：停止请求等太久就先查权威状态，不假设请求没执行、也不自动重放。
    pub(crate) fn halt_tick(&mut self, now: Instant) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let Some(since) = w.halt.since.filter(|_| w.halt.stopping && !w.halt.queried) else {
            return;
        };
        if now < since + HALT_TIMEOUT {
            return;
        }
        if let Some(w) = self.world.as_mut() {
            w.halt.queried = true;
        }
        self.log.error("自动遇敌关闭请求没有回应，正在查询服务端状态");
        self.send("encounter", json!({}), Pending::Encounter { set: None });
    }

    /// 权威状态（响应或推送）到了：关闭就收尾，还开着就按是否已经查证过决定下一步。
    pub(crate) fn on_halt_state(&mut self, encounter_on: bool) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        if !w.halt.stopping {
            return;
        }
        if !encounter_on {
            if let Some(w) = self.world.as_mut() {
                w.halt.done();
            }
            // off 生效前已经建立的这一场保留，照常手动打完；之后不再开新的一场。
            let in_battle = self.world.as_ref().is_some_and(|w| w.battle.is_some());
            self.log.system(if in_battle {
                "自动遇敌已关闭；这一场照常打完，之后不再遇敌"
            } else {
                "自动遇敌已关闭"
            });
            return;
        }
        if w.halt.queried {
            self.fail_halt("服务端回报自动遇敌仍在开启");
            self.log.error("自动遇敌仍在开启，可能继续开战；可在系统页重试「停止自动遇敌」");
        }
    }

    /// 停止请求失败（协议错误）：挂机保持关闭，记下原因，等玩家主动重试。
    pub(crate) fn fail_halt(&mut self, reason: &str) {
        if let Some(w) = self.world.as_mut() {
            w.halt.fail(reason.to_string());
        }
    }

    /// 停止请求失败并告诉玩家下一步：挂机不回滚，失败后不自动恢复自动出招。
    pub(crate) fn halt_failed(&mut self, reason: &str) {
        self.fail_halt(reason);
        self.log.error(format!(
            "自动遇敌关闭失败：{reason}；自动挂机保持关闭，可能继续开战；可在系统页重试「停止自动遇敌」"
        ));
    }
}

impl WorldState {
    /// 自动挂机状态的一句话（系统页、面板用）：开启 / 关闭。
    pub fn auto_text(&self) -> &'static str {
        if self.auto_battle { "开启" } else { "关闭" }
    }

    /// 自动遇敌状态的一句话：开启 / 关闭 / 正在关闭（停止意图在途）。
    pub fn encounter_text(&self) -> &'static str {
        if self.halt.busy() {
            "正在关闭"
        } else if self.entry.encounter {
            "开启"
        } else {
            "关闭"
        }
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use crate::app::battle::{Actor, Phase, Pick};
    use crate::app::{App, Options, WorldState};
    use crate::proto::{BattleStart, TargetType};

    /// 战斗中的玩家，挂机开着，这一阶段已经安排了一次自动出招。
    fn app() -> App {
        let mut a = App::new(Options::default());
        let mut w = WorldState::new(Default::default());
        let mut b = crate::app::battle::BattleState::from_start(BattleStart::default());
        b.phase = Phase::Input(Actor::Player);
        b.auto_due = Some(Instant::now());
        w.battle = Some(b);
        w.auto_battle = true;
        a.world = Some(w);
        a
    }

    fn texts(a: &App) -> Vec<String> {
        a.log.all().iter().map(|m| m.text.clone()).collect()
    }

    #[test]
    fn stop_turns_off_the_local_auto_first() {
        let mut a = app();
        // 遇敌开着：停止要先关本地挂机，再去关遇敌。
        if let Some(w) = a.world.as_mut() {
            w.entry.encounter = true;
        }
        a.stop_auto_all("测试");
        let w = a.world.as_ref().unwrap();
        assert!(!w.auto_battle, "第一步就关掉本机自动出招");
        assert!(w.battle.as_ref().unwrap().auto_due.is_none(), "这一阶段的自动出招安排也清掉");
        // 没有连接：不能假装已经远程停止，失败要说清楚。
        assert!(texts(&a).iter().any(|t| t.contains("自动遇敌关闭失败")), "{:?}", texts(&a));
        assert!(w.halt.error.is_some() && !w.halt.busy());
    }

    #[test]
    fn repeated_stop_does_not_send_again() {
        let mut a = app();
        let since = Instant::now() - Duration::from_millis(100);
        if let Some(w) = a.world.as_mut() {
            w.halt.stopping = true;
            w.halt.since = Some(since);
        }
        a.stop_auto_all("测试");
        let w = a.world.as_ref().unwrap();
        assert!(!w.auto_battle, "重复按下仍然先关本地挂机");
        assert_eq!(w.halt.since, Some(since), "在途的停止请求不重发");
        assert!(texts(&a).iter().any(|t| t == "正在关闭自动遇敌，请稍候"), "{:?}", texts(&a));
    }

    #[test]
    fn start_is_refused_while_stopping() {
        let mut a = app();
        if let Some(w) = a.world.as_mut() {
            w.auto_battle = false;
            w.halt.stopping = true;
        }
        a.start_auto("测试");
        assert!(!a.world.as_ref().unwrap().auto_battle, "关闭中不允许重新开启");
        assert!(texts(&a).iter().any(|t| t.contains("请稍候再开始自动挂机")), "{:?}", texts(&a));
    }

    #[test]
    fn authoritative_off_finishes_the_stop() {
        let mut a = app();
        if let Some(w) = a.world.as_mut() {
            w.halt.stopping = true;
            w.halt.since = Some(Instant::now());
        }
        a.on_halt_state(false);
        let w = a.world.as_ref().unwrap();
        assert!(!w.halt.busy() && w.halt.error.is_none());
        assert!(texts(&a).iter().any(|t| t.contains("自动遇敌已关闭")), "{:?}", texts(&a));
    }

    #[test]
    fn still_on_waits_until_the_state_is_queried() {
        let mut a = app();
        if let Some(w) = a.world.as_mut() {
            w.halt.stopping = true;
            w.halt.since = Some(Instant::now());
        }
        a.on_halt_state(true);
        assert!(a.world.as_ref().unwrap().halt.busy(), "还没查证就继续等权威状态");
        // 超时查过一次、结果仍然开着：停下来如实报告，让玩家自己决定重试。
        if let Some(w) = a.world.as_mut() {
            w.halt.queried = true;
        }
        a.on_halt_state(true);
        let w = a.world.as_ref().unwrap();
        assert!(!w.halt.busy());
        assert!(w.halt.error.as_deref().is_some_and(|e| e.contains("仍在开启")));
        assert!(texts(&a).iter().any(|t| t.contains("可能继续开战")), "{:?}", texts(&a));
    }

    #[test]
    fn start_clears_the_pending_pick_and_keeps_the_focus() {
        let mut a = app();
        if let Some(w) = a.world.as_mut() {
            w.auto_battle = false;
            let b = w.battle.as_mut().unwrap();
            b.action_focus = 7;
            b.pick = Pick::Target { act: crate::app::battle::Act::Attack, extra: None, kind: TargetType::Enemy };
            b.prepared_row = Some((
                Pick::Item,
                crate::app::battle::list::Row {
                    number: Some(5),
                    key: 5,
                    name: "回复药".into(),
                    extra: String::new(),
                    target: "己方一个",
                    blocked: None,
                    desc: String::new(),
                    heals: true,
                },
            ));
        }
        a.start_auto("测试");
        let b = a.world.as_ref().unwrap().battle.as_ref().unwrap();
        assert!(a.world.as_ref().unwrap().auto_battle);
        assert_eq!(b.pick, Pick::Action, "开始挂机回到安全动作层");
        assert!(b.prepared_row.is_none(), "尚未提交的旧选物被清掉");
        assert_eq!(b.action_focus, 7, "焦点仍在管理项上");
    }
}
