//! 打输后的停止（STAGE-265，`docs/auto-battle-settings.md`「打输后提示并停止」）：挂机开着或无限遇敌
//! 进行中，战斗结果为打输（原地复活、被打飞都算）时，响一声提示音，停止自动挂机、关闭自动遇敌，
//! 消息栏写一行原因。胜利、逃跑、平局不触发；手动战斗打输不触发；观察方（agent 操控中）不处理。
//!
//! 触发判断在 `net.rs` 的 `battle_end` 分支里做（借用 `world` 的时候不能调 `self` 的方法），
//! 这里只管触发之后怎么停：和玩家按 8 一样，先关本机自动出招，再在在线时发 `encounter off`。

use crate::app::App;
use crate::app::alert::play_lose_alert;

/// 消息栏里那一行（测试也靠它断言）。
pub const LOSE_NOTE: &str = "被打输了，已停止自动挂机和自动遇敌";

impl App {
    /// 打输后停下（调用方已经判断过触发条件）。提示音只响这一次，消息只写这一行。
    pub(crate) fn on_battle_lost(&mut self) {
        play_lose_alert();
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let (online, encounter_on) = (w.online, w.entry.encounter);
        self.stop_local_auto();
        // 原因永远先写一行，后面的发送结果（成功、失败）接在它后面。
        self.log.system(LOSE_NOTE);
        if self.halt_busy() {
            self.log.system("正在关闭自动遇敌，请稍候");
            return;
        }
        if !online {
            self.fail_halt("连接已断开，无法确认远端遇敌状态");
            self.log.system("连接已断开，自动遇敌状态未知");
            return;
        }
        if !encounter_on && !self.session.encounter_busy() {
            // 只开着挂机：遇敌本来就是关的，不发请求。
            return;
        }
        self.request_encounter_off("正在关闭自动遇敌");
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::LOSE_NOTE;
    use crate::app::alert::lose_rings;
    use crate::app::auto_battle::AutoProfile;
    use crate::app::auto_battle::store::Store;
    use crate::app::battle::{Actor, BattleState, Phase};
    use crate::app::{App, Options, Screen, WorldState};
    use crate::proto::{BattleStart, WorldEntry};

    /// 在线的战斗中玩家。`auto` 是挂机开关，`encounter` 是无限遇敌，`stop_on` 是打输后停止的设置。
    fn fight(auto: bool, encounter: bool, stop_on: bool) -> App {
        let mut a = App::new(Options::default());
        a.screen = Screen::World;
        let mut w = WorldState::new(WorldEntry::default());
        w.online = true;
        w.entry.encounter = encounter;
        w.auto_battle = auto;
        let mut b = BattleState::from_start(BattleStart::default());
        b.phase = Phase::Input(Actor::Player);
        w.battle = Some(b);
        a.world = Some(w);
        let dir = std::env::temp_dir().join(format!("stoneage-loss-test-{}-{stop_on}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        let mut store = Store::open_in(&dir, "测试服", "勇者");
        store.profiles.active_mut().stop_on_lose = stop_on;
        a.auto_store = Some(store);
        a
    }

    fn texts(a: &App) -> Vec<String> {
        a.log.all().iter().map(|m| m.text.clone()).collect()
    }

    /// 推一条 `battle_end`，返回这次响了几声。
    fn push_end(a: &mut App, result: &str, penalty: Option<serde_json::Value>) -> usize {
        let before = lose_rings();
        let mut data = json!({ "result": result });
        if let Some(p) = penalty {
            data["penalty"] = p;
        }
        a.on_battle_push("battle_end", data);
        lose_rings() - before
    }

    fn encounter_off_attempted(a: &App) -> bool {
        // 没有真实连接，发不出去时会留下失败原因：说明停止请求已经走到了发送这一步。
        a.world
            .as_ref()
            .is_some_and(|w| w.halt.error.as_deref().is_some_and(|e| e.contains("请求没有发出去")))
    }

    #[test]
    fn losing_while_encounter_runs_stops_both_and_rings_once() {
        let mut a = fight(false, true, true);
        assert_eq!(push_end(&mut a, "lose", Some(json!({"in_place": true}))), 1);
        assert!(!a.world.as_ref().unwrap().auto_battle);
        assert!(encounter_off_attempted(&a), "无限遇敌要发 encounter off");
        assert!(texts(&a).iter().any(|t| t == LOSE_NOTE), "{:?}", texts(&a));
        assert!(
            !texts(&a).iter().any(|t| t == "自动挂机已关闭：战斗失败"),
            "新路径不再写旧的那一行"
        );
    }

    #[test]
    fn losing_with_only_auto_on_stops_auto_without_encounter_request() {
        let mut a = fight(true, false, true);
        assert_eq!(push_end(&mut a, "lose", None), 1);
        assert!(!a.world.as_ref().unwrap().auto_battle);
        assert!(!encounter_off_attempted(&a), "遇敌本来是关的，不发请求");
        assert!(texts(&a).iter().any(|t| t == LOSE_NOTE), "{:?}", texts(&a));
    }

    #[test]
    fn being_flown_back_triggers_the_same_stop() {
        // 被打飞：服务端省略 in_place，penalty 里只有送回的地点。
        let mut a = fight(true, true, true);
        assert_eq!(
            push_end(&mut a, "lose", Some(json!({"respawn": {"map": 1, "x": 2, "y": 3}}))),
            1
        );
        assert!(!a.world.as_ref().unwrap().auto_battle);
        assert!(encounter_off_attempted(&a));
    }

    #[test]
    fn manual_loss_does_not_ring_or_change_anything_new() {
        let mut a = fight(false, false, true);
        assert_eq!(push_end(&mut a, "lose", None), 0);
        assert!(!texts(&a).iter().any(|t| t == LOSE_NOTE));
        assert!(
            texts(&a).iter().any(|t| t == "自动挂机已关闭：战斗失败"),
            "旧行为照旧：{:?}",
            texts(&a)
        );
    }

    #[test]
    fn win_escape_and_draw_never_trigger() {
        for result in ["win", "escaped", "draw"] {
            let mut a = fight(true, true, true);
            assert_eq!(push_end(&mut a, result, None), 0, "{result}");
            assert!(!texts(&a).iter().any(|t| t == LOSE_NOTE), "{result}");
        }
    }

    #[test]
    fn switch_off_keeps_the_old_behavior() {
        let mut a = fight(true, true, false);
        assert_eq!(push_end(&mut a, "lose", None), 0);
        assert!(!a.world.as_ref().unwrap().auto_battle, "旧行为：打输照样关挂机");
        assert!(
            texts(&a).iter().any(|t| t == "自动挂机已关闭：战斗失败"),
            "{:?}",
            texts(&a)
        );
        assert!(!encounter_off_attempted(&a));
    }

    #[test]
    fn observer_is_left_to_the_agent_rules() {
        let mut a = fight(true, true, true);
        if let Some(w) = a.world.as_mut() {
            w.entry.control.you = "observer".into();
        }
        assert_eq!(push_end(&mut a, "lose", None), 0);
        assert!(!encounter_off_attempted(&a));
    }

    #[test]
    fn old_saved_profile_without_the_field_defaults_to_on() {
        let mut v = serde_json::to_value(AutoProfile::default()).unwrap();
        v.as_object_mut().unwrap().remove("stop_on_lose");
        let p: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(p.stop_on_lose);
        assert!(AutoProfile::default().stop_on_lose);
    }

    #[test]
    fn saved_switch_off_is_kept_as_written() {
        let mut v = serde_json::to_value(AutoProfile::default()).unwrap();
        v["stop_on_lose"] = json!(false);
        let p: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(!p.stop_on_lose);
    }
}
