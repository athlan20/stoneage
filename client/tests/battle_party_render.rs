//! STAGE-275 界面测试（真人队同场战斗，`docs/battle-ui.md`「真人队同场战斗」）：不经过网络，战场直接用 `BattleState` 搭出来。
//! 检查队友的卡片（名字、血量、蓝量、队长标记）、等待进度和倒计时、队员离场的战斗记录、`forced_out` 的结果窗口，
//! 以及队友不能被选为目标。主要尺寸是 80×24。

use std::time::{Duration, Instant};

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use serde_json::json;
use stoneage_client::app::battle::party::{countdown_text, wait_text};
use stoneage_client::app::battle::{Act, Actor, BattleState, Phase};
use stoneage_client::app::{App, Kind, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleEnd, BattleLeave, BattleRound, BattleStart, Character, Location, Progress, TargetType, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    })
}

/// 把整个画面按行拼成文字：宽字符占两列，后面的占位格跳过（同 render_battle.rs）。
fn draw(app: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..h)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < w {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 真人队同场的开局：队长 勇者小明（0 号，自己）、队员 路人乙（1 号）及其宠物乌力（6 号），敌方野猪（15 号）。
fn party_start() -> BattleStart {
    serde_json::from_value(json!({
        "battle_id": "b_1", "kind": "party", "scene": "野外", "you": 0,
        "allow": {"escape": true, "capture": true},
        "units": [
            {"no": 0, "kind": "player", "name": "勇者小明", "level": 1, "hp": 44, "max_hp": 44, "hp_pct": 100,
             "mp": 100, "max_mp": 100, "status": [], "ride": false},
            {"no": 1, "kind": "player", "name": "路人乙", "level": 1, "hp": 40, "max_hp": 40, "hp_pct": 100,
             "mp": 60, "max_mp": 60, "status": [], "ride": false},
            {"no": 5, "kind": "pet", "name": "小豆", "level": 5, "hp": 30, "max_hp": 30, "hp_pct": 100, "status": [], "ride": false},
            {"no": 6, "kind": "pet", "name": "乌力", "level": 5, "hp": 35, "max_hp": 35, "hp_pct": 100, "status": [], "ride": false},
            {"no": 15, "kind": "monster", "name": "野猪", "level": 3, "hp_pct": 100, "status": [], "ride": false}
        ],
        "party": {"members": [{"no": 0, "name": "勇者小明", "leader": true}, {"no": 1, "name": "路人乙", "leader": false}]}
    }))
    .unwrap()
}

/// 进行中的真人队战斗：轮到自己出人物指令，还没有人交指令（没有截止时间）。
fn party_battle() -> BattleState {
    let mut b = BattleState::from_start(party_start());
    b.turn = 1;
    b.phase = Phase::Input(Actor::Player);
    b.progress = Progress { done: 0, total: 2 };
    b
}

/// 把战斗放进一个处于世界中的 App（同 render_battle.rs）。
fn battle_app(battle: BattleState) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance {
            body: 3,
            color: 2,
            eye: 1,
            mouth: 4,
        },
        location: Location {
            floor: 1000,
            map_name: "萨姆吉尔村".into(),
        },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(Layer::Page(0));
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a.flush_battle_log();
    a
}

#[test]
fn teammate_card_shows_name_hp_and_leader_mark_at_80x24() {
    let mut a = battle_app(party_battle());
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("路人乙"), "{screen}");
    assert!(screen.contains("40/40"), "队友的血量：{screen}");
    // 80×24 是紧凑档，队长标记写成一个“长”字（完整档写“队长”）。
    assert!(screen.contains("长"), "队长要有标记：{screen}");
    assert!(!screen.contains("自动→"), "队友不是分身，不写“自动”：{screen}");
}

#[test]
fn teammate_mp_shows_on_the_full_layout() {
    let mut a = battle_app(party_battle());
    // 120×40 的日志区占了行数，落到紧凑档；完整档要 44 行（见 render_battle.rs 的说明）。
    let screen = draw(&mut a, 120, 44);
    assert!(screen.contains("队长"), "完整档写“队长”：{screen}");
    assert!(screen.contains("MP60/60"), "队友的蓝量：{screen}");
}

#[test]
fn teammate_is_mate_not_clone_and_leader_is_marked() {
    let b = party_battle();
    assert!(b.party);
    assert!(b.is_mate(1), "队员是队友");
    assert!(b.is_mate(6), "队员的宠物也是队友");
    assert!(!b.is_mate(0), "自己不是队友");
    assert!(!b.is_mate(5), "自己的宠物不是队友");
    assert!(b.is_leader(0));
    assert!(!b.is_leader(1));
}

#[test]
fn waiting_line_shows_done_of_total() {
    let mut b = party_battle();
    b.phase = Phase::Waiting;
    b.progress = Progress { done: 1, total: 3 };
    let mut a = battle_app(b);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("等待其他玩家 1/3"), "{screen}");
    assert_eq!(wait_text(Progress { done: 2, total: 3 }), "等待其他玩家 2/3");
}

#[test]
fn countdown_is_dash_until_someone_submits_then_counts_down() {
    let mut a = battle_app(party_battle());
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("倒计时 ——"), "还没有人交指令时写“——”：{screen}");
    assert_eq!(countdown_text(None), "——");

    // 第一条指令被接受后有了截止时间：按剩余毫秒换算（这里约 90 秒）。
    let now = Instant::now();
    let b = a.world.as_mut().unwrap().battle.as_mut().unwrap();
    b.deadline = Some(now + Duration::from_secs(90));
    b.turn_window = Duration::from_secs(120);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains(" 90s"), "倒计时按剩余秒数显示：{screen}");
    assert_eq!(countdown_text(Some(90)), "90s");
}

#[test]
fn leave_by_timeout_is_written_to_the_battle_log_with_its_cause() {
    let mut b = party_battle();
    b.on_leave(BattleLeave {
        battle_id: "b_1".into(),
        no: 1,
        name: "路人乙".into(),
        cause: "timeout".into(),
        left: vec![1, 6],
        progress: Progress { done: 1, total: 1 },
    });
    let text = "路人乙 超时未下指令，被踢出战斗";
    assert!(b.records.iter().any(|r| r == text), "{:?}", b.records);
    assert!(
        b.take_pending().contains(&(Kind::Party, text.to_string())),
        "写进消息日志，类别是队伍"
    );
    assert!(!b.here.contains(&1), "离场的队员不在场上");
    assert!(!b.here.contains(&6), "他的宠物也离场");
    assert_eq!(b.progress, Progress { done: 1, total: 1 }, "进度以离场之后的数字为准");
}

#[test]
fn leave_by_offline_says_the_player_left_the_battle() {
    let mut b = party_battle();
    b.on_leave(BattleLeave {
        no: 1,
        name: "路人乙".into(),
        cause: "offline".into(),
        left: vec![1, 6],
        progress: Progress { done: 1, total: 1 },
        ..BattleLeave::default()
    });
    assert!(
        b.records.iter().any(|r| r == "路人乙 掉线，退出战斗"),
        "{:?}",
        b.records
    );
}

#[test]
fn teammate_escape_in_a_round_is_logged() {
    let mut b = party_battle();
    let round: BattleRound = serde_json::from_value(json!({
        "battle_id": "b_1", "turn": 1,
        "events": [{"kind": "escape", "actor": 1, "ok": true, "left": [1, 6]}],
        "state": []
    }))
    .unwrap();
    b.on_round(round, Instant::now());
    assert!(
        b.records.iter().any(|r| r == "路人乙 逃离了战斗，已离队"),
        "{:?}",
        b.records
    );
}

#[test]
fn forced_out_window_says_no_reward_and_no_penalty() {
    let mut b = party_battle();
    b.on_end(BattleEnd { duel_point: None,
        battle_id: "b_1".into(),
        result: "forced_out".into(),
        turn: 1,
        ..BattleEnd::default()
    });
    let mut a = battle_app(b);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("被移出战斗"), "{screen}");
    assert!(screen.contains("因超时被移出战斗，无奖励无惩罚"), "{screen}");
}

#[test]
fn teammates_cannot_be_picked_as_targets() {
    let b = party_battle();
    assert!(b.can_pick(TargetType::Ally, Act::Item, 0), "自己可以");
    assert!(b.can_pick(TargetType::Ally, Act::Item, 5), "自己的宠物可以");
    assert!(b.can_pick(TargetType::Ally, Act::Item, 1), "回复道具可以选队友（STAGE-275）");
    assert!(b.can_pick(TargetType::Ally, Act::Item, 6), "回复道具可以选队友的宠物");
    assert!(!b.can_pick(TargetType::Any, Act::Item, 1), "属性反转（any）不能选队友");
    assert!(!b.can_pick(TargetType::Enemy, Act::Attack, 1), "攻击不能选队友");
    assert!(b.can_pick(TargetType::Enemy, Act::Attack, 15), "敌方照常");
}

#[test]
fn single_player_battle_is_unchanged() {
    // 单人战斗：没有 party，没有队友，倒计时没有截止时间时不写“——”（现有行为）。
    let start: BattleStart = serde_json::from_value(json!({
        "battle_id": "b_1", "kind": "wild", "scene": "野外", "you": 0,
        "allow": {"escape": true, "capture": true},
        "units": [{"no": 0, "kind": "player", "name": "勇者小明", "level": 1, "hp": 44, "max_hp": 44, "hp_pct": 100, "status": [], "ride": false},
                  {"no": 15, "kind": "monster", "name": "野猪", "level": 3, "hp_pct": 100, "status": [], "ride": false}]
    }))
    .unwrap();
    let mut b = BattleState::from_start(start);
    b.phase = Phase::Input(Actor::Player);
    assert!(!b.party);
    assert!(!b.is_mate(0));
    let mut a = battle_app(b);
    let screen = draw(&mut a, 80, 24);
    assert!(!screen.contains("倒计时 ——"), "{screen}");
    assert!(!screen.contains("队长"), "{screen}");
}
