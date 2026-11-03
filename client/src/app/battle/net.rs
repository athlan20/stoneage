//! 战斗与服务端的连接（STAGE-37 至 39）：推送的接入、指令的回复、回放的定时推进，
//! 以及战斗记录写进消息栏。协议见 `docs/protocol.md` 第 10 章。

use std::time::Instant;

use serde_json::{Value, json};

use super::capture_discard::discards_in_round;
use super::{BattleState, Phase};
use crate::app::App;
use crate::app::session::Pending;
use crate::proto::{BattleEnd, BattleLeave, BattleProgress, BattleRound, BattleSnapshot, BattleStart, BattleTurn};

impl App {
    /// 战斗的推送。没有进入世界时收到的忽略。
    pub(crate) fn on_battle_push(&mut self, ty: &str, data: Value) {
        let now = Instant::now();
        // 出招延迟来自方案（STAGE-173）：先取出来，下面 `world` 借出去之后就不能再调 `self` 的方法了。
        let delay = self.auto_delay();
        // 打输后停不停、响不响，也是方案的设置（STAGE-265），同理先取出来。
        let stop_on_lose = self.auto_profile().stop_on_lose;
        let mut halt_on_lose = false;
        let Some(world) = self.world.as_mut() else {
            return;
        };
        // 资质筛选放生的宠物（STAGE-264）：不进宠物列表，只进丢弃记录，借用结束后再写。
        let discards = if ty == "battle_round" { discards_in_round(&data) } else { Vec::new() };
        match ty {
            "battle_start" => {
                // 决斗被挑战方（STAGE-307）：没有弹窗，消息栏写一句后直接进战斗页；挑战方的名字取 duel.sides[0].leader。
                let challenger = (data["kind"] == "duel" && data["you"].as_u64().is_some_and(|y| y >= 10))
                    .then(|| data["duel"]["sides"][0]["leader"].as_str().map(str::to_string))
                    .flatten();
                if let Ok(start) = serde_json::from_value::<BattleStart>(data) {
                    // 新战斗不继承上一场的控制面板（STAGE-153）；但正在改设置时不收（STAGE-173）：
                    // 设置页跟战斗是两件事，战斗继续跑，设置照旧改。
                    if world.auto_edit.is_none() {
                        world.system.battle_open = false;
                    }
                    // STAGE-284：观察方正在翻背包、宠物等页面（或弹着确认、NPC 窗口）时，新战斗不把人拉回战斗画面，
                    // 战斗留在后台（盖子 `battle_veil`），玩家退回场景根再回到战斗；自己操控时照旧。
                    if world.observing()
                        && (world.nav.depth() > 1 || world.confirm.is_some() || world.npc_window.is_some())
                    {
                        world.battle_veil = true;
                    } else {
                        world.close_battle_veil(); // 停在场景根时残留的盖子先收掉，否则新战斗会被盖住
                        world.prepare_battle_navigation();
                    }
                    world.battle = Some(BattleState::from_start(start));
                    if let Some(name) = challenger {
                        self.log.system(format!("{name} 向你发起了决斗"));
                    }
                    if let Some(b)=world.battle.as_mut(){b.speed=u32::from(self.default_battle_speed);}
                }
            }
            "battle_turn" => {
                if !world.observing() && world.nav.is(&crate::app::Layer::Command){world.nav.close_command();world.input.clear();}
                if let (Some(b), Ok(t)) = (world.battle.as_mut(), serde_json::from_value::<BattleTurn>(data)) {
                    b.on_turn(t, now);
                    // 托管中：轮到下指令就安排自动出招（STAGE-42）。
                    if world.auto_battle {
                        b.schedule_auto(now, delay);
                    }
                }
            }
            "battle_progress" => {
                if let (Some(b), Ok(p)) = (world.battle.as_mut(), serde_json::from_value::<BattleProgress>(data)) {
                    b.on_progress(p, now);
                }
            }
            "battle_round" => {
                if !world.observing() && world.nav.is(&crate::app::Layer::Command){world.nav.close_command();world.input.clear();}
                if let (Some(b), Ok(r)) = (world.battle.as_mut(), serde_json::from_value::<BattleRound>(data)) {
                    b.on_round(r, now);
                }
            }
            "battle_leave" => {
                if let (Some(b), Ok(l)) = (world.battle.as_mut(), serde_json::from_value::<BattleLeave>(data)) {
                    b.on_leave(l);
                }
            }
            "battle_end" => {
                if let (Some(b), Ok(end)) = (world.battle.as_mut(), serde_json::from_value::<BattleEnd>(data)) {
                    let lost = end.result == "lose";
                    let escaped = end.result == "escaped";
                    let duel_total = end.duel_point.map(|d| d.total);
                    b.on_end(end);
                    // 决斗点数的现值以 battle_end 为准（STAGE-307，`docs/pk.md` 2.2），角色页马上看到。
                    if let Some(total) = duel_total {
                        world.entry.duel_point = total;
                    }
                    // 战斗结束不再保留战斗控制面板，结果窗口接管（STAGE-153）；
                    // 正在改设置时不收（STAGE-173）。
                    if world.auto_edit.is_none() {
                        world.system.battle_open = false;
                    }
                    if lost {
                        // 挂机或无限遇敌在跑、方案开关开着（STAGE-265）：统一停止并响提示音，留到借用结束后做。
                        halt_on_lose = stop_on_lose && (world.auto_battle || world.entry.encounter) && !world.observing();
                        if !halt_on_lose {
                            // 打输了自动关挂机：练级循环到这里就该停（STAGE-42）。
                            world.auto_battle = false;
                            self.log.system("自动挂机已关闭：战斗失败");
                        }
                    } else if escaped && world.auto_battle {
                        // 逃跑抓的策略性逃跑（STAGE-285）：跑这一场是为了刷新目标，挂机继续跑。
                        let catch_flee = world.battle.as_ref().is_some_and(|b| b.catch_flee);
                        if catch_flee {
                            self.log.system("逃跑抓：这一场没有目标，继续等下一场");
                        } else {
                            // 逃跑成功（STAGE-173 M10）：本地自动出招到此为止，重不重开由玩家决定。
                            world.auto_battle = false;
                            self.log.system("自动挂机已关闭：已经逃离战斗");
                        }
                    }
                }
            }
            _ => {}
        }
        self.log_capture_discards(discards);
        if halt_on_lose {
            self.on_battle_lost();
        }
        self.flush_battle_log();
    }

    /// 每个刷新周期推进一次回放。
    pub(crate) fn battle_tick(&mut self, now: Instant) {
        // 「跳过」档（STAGE-173）：不播回放动画，下完指令就直接出结果、进下一回合。
        if self.skip_replay && self.world.as_ref().and_then(|w| w.battle.as_ref()).is_some_and(|b| b.phase == Phase::Replay) {
            self.jump_replay();
            return;
        }
        let done = self.world.as_mut().and_then(|w| w.battle.as_mut()).is_some_and(|b| b.advance(now));
        if done {
            self.after_replay();
        }
        self.flush_battle_log();
    }

    /// 回放播完：战斗已经结束就弹结果窗口（结束之后不再确认）；否则向服务端确认这一回合，等下一回合的推送。
    pub(crate) fn after_replay(&mut self) {
        let Some(b) = self.world.as_mut().and_then(|w| w.battle.as_mut()) else {
            return;
        };
        // STAGE-282：只有正在回放时才算“播完”。战斗已经结束（结果窗口显示中）后不再确认，服务端早把战斗摘掉了，
        // 再发只会换来红字“当前不在战斗中”。
        if b.phase != Phase::Replay {
            return;
        }
        if let Some(end) = b.end_after_replay.take() {
            b.show_result(end);
            self.flush_battle_log();
            return;
        }
        b.phase = Phase::Waiting;
        let turn = b.turn;
        self.flush_battle_log();
        self.send("battle_ack", json!({"turn": turn}), Pending::BattleAck);
    }

    /// 指令被接受（回复里的 `next`）：`pet` 轮到宠物指令，`wait` 等其他玩家或结算。
    pub(crate) fn on_battle_command_ok(&mut self, data: Value) {
        let next = data.get("next").and_then(Value::as_str).unwrap_or("wait");
        let now = Instant::now();
        let (auto, delay) = (self.world.as_ref().is_some_and(|w| w.auto_battle), self.auto_delay());
        let Some(b) = self.world.as_mut().and_then(|w| w.battle.as_mut()) else {
            return;
        };
        b.on_accepted(next);
        // 托管中：人物指令交完轮到宠物，接着安排宠物的自动出招（STAGE-42）。
        if next == "pet" && auto {
            b.schedule_auto(now, delay);
        }
    }

    /// 指令被拒绝：错误已经在消息栏里，阶段不变，玩家可以重新选。
    /// 托管发出的指令再换个来源重试（道具被拒就本场拉黑，STAGE-42）。
    pub(crate) fn on_battle_command_failed(&mut self) {
        self.on_auto_command_failed(Instant::now());
        if let Some(b) = self.world.as_mut().and_then(|w| w.battle.as_mut()) {
            b.on_rejected();
        }
    }

    /// `battle` 查询的回复（文字命令 `battle`）：不在战斗中就说明；在战斗中而本机没有战场，就补上。
    pub(crate) fn on_battle_query(&mut self, data: Value) {
        let snap: BattleSnapshot = serde_json::from_value(data).unwrap_or_default();
        if !snap.in_battle {
            if self.reconnect.as_ref().is_some_and(|r|r.refreshing()){if let Some(w)=self.world.as_mut(){w.battle=None;}}
            self.log.system("当前不在战斗中");
            return;
        }
        self.log.system(format!("战斗中：第 {} 回合，{}", snap.turn, snap.scene));
        if self.world.as_ref().is_some_and(|w| w.battle.is_none()) || self.reconnect.as_ref().is_some_and(|r|r.refreshing()) {
            self.resume_battle(snap);
        }
    }

    /// 中途加入（进入世界时正在战斗，或查询得知）：补上战场；已经结算、等确认的回合直接确认。
    pub(crate) fn resume_battle(&mut self, snap: BattleSnapshot) {
        let delay = self.auto_delay();
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let auto = w.auto_battle;
        w.prepare_battle_navigation();
        let mut battle = BattleState::resume(snap, Instant::now());
        battle.speed=u32::from(self.default_battle_speed);
        let replay = battle.phase == Phase::Replay;
        if auto && matches!(battle.phase, Phase::Input(_)) {
            battle.schedule_auto(Instant::now(), delay);
        }
        w.battle = Some(battle);
        if replay && w.online {
            self.after_replay();
        }
        self.flush_battle_log();
    }

    /// 把战斗新增的消息按发生顺序写进消息日志（STAGE-248：战斗日志区和消息页读的是同一份）。
    pub fn flush_battle_log(&mut self) {
        let mode = self.battle_log;
        if let Some(w) = self.world.as_mut() {
            let on = mode.is_brief(w.observing());
            if let Some(b) = w.battle.as_mut() {
                b.brief.on = on;
            }
        }
        let pending = self.world.as_mut().and_then(|w| w.battle.as_mut()).map(BattleState::take_pending).unwrap_or_default();
        for (kind, text) in pending {
            self.log.push(kind, text);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::app::battle::{Actor, BattleState};
    use crate::app::{Options, WorldState};

    /// 造一个「托管开着、正在战斗」的客户端；`catch_flee` 表示这次逃跑是逃跑抓的策略性逃跑。
    fn app_with(catch_flee: bool) -> App {
        let mut a = App::new(Options::default());
        let mut w = WorldState::new(Default::default());
        let mut b = BattleState::from_start(BattleStart::default());
        b.phase = Phase::Input(Actor::Player);
        b.catch_flee = catch_flee;
        w.battle = Some(b);
        w.auto_battle = true;
        a.world = Some(w);
        a
    }

    fn texts(a: &App) -> Vec<String> {
        a.log.all().iter().map(|m| m.text.clone()).collect()
    }

    /// STAGE-285：逃跑抓跑掉的那一场要留着挂机继续刷，不能被「已经逃离战斗」停掉。
    #[test]
    fn catch_flee_keeps_the_auto_running_after_escaping() {
        let mut a = app_with(true);
        a.on_battle_push("battle_end", json!({"result": "escaped"}));
        assert!(a.world.as_ref().unwrap().auto_battle, "逃跑抓不许停挂机");
        let texts = texts(&a);
        assert!(texts.iter().any(|t| t.contains("继续等下一场")), "{texts:?}");
        assert!(!texts.iter().any(|t| t.contains("已经逃离战斗")), "{texts:?}");
    }

    /// 保命/名单的普通逃跑照旧收工：跑掉以后不再开新的一场。
    #[test]
    fn ordinary_escape_still_stops_the_auto() {
        let mut a = app_with(false);
        a.on_battle_push("battle_end", json!({"result": "escaped"}));
        assert!(!a.world.as_ref().unwrap().auto_battle, "老行为不能改");
        assert!(texts(&a).iter().any(|t| t.contains("已经逃离战斗")), "{:?}", texts(&a));
    }
}
