//! STAGE-275 验收（场景）：真人队同场战斗，回放 `tests/cases/battle_party_*.jsonl`（严格模式，客户端发出的指令要与用例逐字段一致）。
//! 覆盖：队员超时被踢出（`battle_leave` 写进战斗记录，队长的回合照常结算，下一回合只剩自己）、
//! 超时被移出战斗（`forced_out`，没有奖励）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use std::time::Instant;

use crossterm::event::KeyCode::{Char, Enter, Tab};
use stoneage_client::app::battle::{Actor, BattleState, Phase};
use stoneage_client::app::{App, Screen};
use stoneage_client::proto::Progress;
use support::{Harness, opts};

fn case(name: &str) -> String {
    format!("{}/tests/cases/{name}.jsonl", env!("CARGO_MANIFEST_DIR"))
}

fn battle(a: &App) -> Option<&BattleState> {
    a.world.as_ref()?.battle.as_ref()
}

/// 这个回合轮到这个角色出招。
fn input(a: &App, turn: u32, actor: Actor) -> bool {
    battle(a).is_some_and(|b| b.turn == turn && b.phase == Phase::Input(actor))
}

/// 登录、选角色、进入世界（同 scenarios_battle.rs）。
fn enter_world(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
}

/// 队员超时被踢出：队长交了人物指令，队员 路人乙 没交，超时被踢（`battle_leave`，`cause` 为 timeout）；
/// 这一回合照常结算，下一回合只剩队长一人。
#[test]
fn a_timed_out_teammate_leaves_and_the_round_goes_on() {
    let mut h = Harness::new(&case("battle_party_leave"), opts);
    enter_world(&mut h);
    h.wait("第 1 回合轮到人物出招", 5, |a| input(a, 1, Actor::Player));
    {
        let b = battle(&h.app).unwrap();
        assert!(b.party, "kind 为 party");
        assert!(b.is_leader(0) && b.is_mate(1), "队长是自己，路人乙是队友");
        assert!(
            b.remaining_secs(Instant::now()).is_none(),
            "第一条指令被接受之前没有倒计时"
        );
    }
    // 人物攻击敌方 3 号（协议 15 号）：1 选攻击，3 选敌方 3 号。
    h.keys(&[Char('1'), Char('3')]);
    h.wait("交齐自己的指令，等其他玩家", 5, |a| {
        battle(a).is_some_and(|b| b.phase == Phase::Waiting)
    });
    h.wait("路人乙超时被踢出", 5, |a| {
        battle(a).is_some_and(|b| !b.here.contains(&1))
    });
    {
        let b = battle(&h.app).unwrap();
        assert!(
            b.records.iter().any(|r| r == "路人乙 超时未下指令，被踢出战斗"),
            "战斗记录写原因：{:?}",
            b.records
        );
        assert!(!b.here.contains(&6), "队员的宠物随之退场");
        assert_eq!(b.progress, Progress { done: 1, total: 1 }, "进度以离场之后的数字为准");
    }
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    {
        let b = battle(&h.app).unwrap();
        assert_eq!(b.progress, Progress { done: 0, total: 1 }, "下一回合只剩队长");
        assert!(b.here.contains(&0) && !b.here.contains(&1));
    }
    h.abort();
}

/// 超时被移出战斗：队员 路人乙 交了指令，队长没交，超时后队长收到 `forced_out`，结果窗口写没有奖励、没有惩罚。
#[test]
fn a_forced_out_player_gets_a_window_without_rewards() {
    let mut h = Harness::new(&case("battle_party_forced_out"), opts);
    enter_world(&mut h);
    h.wait("战斗结束，结果窗口", 5, |a| {
        battle(a).is_some_and(|b| b.result.is_some())
    });
    {
        let b = battle(&h.app).unwrap();
        let end = b.result.as_ref().unwrap();
        assert_eq!(end.result, "forced_out");
        assert_eq!(end.rewards.gold, 0, "没有金钱");
        assert!(end.rewards.exp.is_empty(), "没有经验");
        assert!(end.penalty.is_none(), "没有惩罚");
        assert_eq!(b.phase, Phase::Ended);
    }
    h.abort();
}
