//! STAGE-182：agent 操控战斗（观察模式）时，玩家按 Esc 打开游戏菜单看背包、宠物等页面，战斗留在后台。
//! 自己操控时仍锁在战斗画面。不连服务端：看本地状态和渲染出来的文字。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{BattleState, Phase};
use stoneage_client::app::{App, Kind, Layer, Options, Screen, WorldState};
use stoneage_client::net::NetEvent;
use stoneage_client::proto::Incoming;
use stoneage_client::proto::{
    BattleAllow, BattleStart, BattleTurn, BattleUnit, Progress, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

const WIDTH: u16 = 120;
const HEIGHT: u16 = 40;

fn unit(no: u8, kind: &str, name: &str) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level: 3, hp_pct: 100, ..BattleUnit::default() }
}

/// 一场战斗正在进行；`observer` 为真表示 agent 在操控。
fn battle_app(observer: bool) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.screen = Screen::World;
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![unit(0, "player", "勇者小明"), unit(5, "pet", "乌力"), unit(15, "monster", "野猪")],
        ..BattleStart::default()
    };
    let mut battle = BattleState::from_start(start);
    battle.on_turn(
        BattleTurn {
                field: None,
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: vec!["player".into()],
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        std::time::Instant::now(),
    );
    let w = a.world.as_mut().unwrap();
    w.battle = Some(battle);
    if observer {
        w.entry.control.you = "observer".into();
    }
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

/// 喂一条服务端推送（连接代数从 0 开始，没连过服务端的应用还是 0）。
fn push(a: &mut App, ty: &str, data: serde_json::Value) {
    a.on_net(NetEvent::Message { gen_id: 0, msg: Incoming::Push { ty: ty.into(), data } });
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn draw(a: &mut App) -> String {
    let mut term = Terminal::new(TestBackend::new(WIDTH, HEIGHT)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..HEIGHT)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < WIDTH {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn input_row(a: &mut App) -> String {
    draw(a).lines().rev().nth(1).unwrap_or_default().to_string()
}

fn last_error(a: &App) -> String {
    a.log.all().iter().rev().find(|m| m.kind == Kind::Error).map(|m| m.text.clone()).unwrap_or_default()
}

#[test]
fn observer_esc_opens_game_menu_and_battle_bar_stays() {
    let mut a = battle_app(true);
    assert!(input_row(&mut a).contains("Esc 菜单"), "提示行写出入口：{}", input_row(&mut a));
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).battle_veil && world(&a).battle.is_some(), "战斗留在后台");
    assert!(world(&a).nav.is(&Layer::Menu));
    let screen = draw(&mut a);
    assert!(screen.contains("战斗中"), "顶部战斗信息条保留");
    assert!(screen.contains("背包") && screen.contains("宠物"), "中间是游戏菜单：\n{screen}");
}

#[test]
fn observer_can_open_pages_from_the_menu_and_come_back_with_esc() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Esc);
    // 菜单里选一项进功能页（第 1 项是快捷传送，往下选到背包不依赖顺序：直接用页命令）。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).nav.top(), Layer::Page(_)), "进了功能页");
    assert!(world(&a).battle_veil);
    key(&mut a, KeyCode::Esc); // 回菜单
    assert!(world(&a).battle_veil && world(&a).nav.is(&Layer::Menu));
    key(&mut a, KeyCode::Esc); // 回战斗
    assert!(!world(&a).battle_veil, "菜单退回场景就是回到战斗画面");
    assert!(world(&a).battle.is_some());
}

#[test]
fn observer_operations_in_menu_are_still_blocked() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Esc);
    for c in "/turn 1".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("输入 take 收回控制权"), "{}", last_error(&a));
}

#[test]
fn own_battle_esc_does_not_leave_the_battle() {
    let mut a = battle_app(false);
    key(&mut a, KeyCode::Esc);
    assert!(!world(&a).battle_veil, "自己操控时 Esc 不开菜单");
    assert!(world(&a).battle_front());
}

#[test]
fn tab_panel_still_works_from_the_menu() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Tab);
    assert!(!world(&a).battle_veil && world(&a).system.battle_open, "先回战斗画面再开控制面板");
    key(&mut a, KeyCode::Esc);
    assert!(!world(&a).system.battle_open);
}

#[test]
fn take_back_control_closes_the_menu() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Esc);
    push(&mut a, "control_changed", serde_json::json!({"controller": "player", "you": "controller", "reason": "TAKEN_BY_PLAYER"}));
    assert!(!world(&a).battle_veil, "收回控制权后菜单自动退出");
    assert!(world(&a).battle_front());
}

#[test]
fn battle_ending_behind_the_menu_does_not_pull_the_player_back() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    let page = world(&a).nav.top().clone();
    push(&mut a, "battle_end", serde_json::json!({"battle_id": "b_1", "result": "win", "turn": 2}));
    assert_eq!(world(&a).battle.as_ref().unwrap().phase, Phase::Ended);
    assert_eq!(world(&a).nav.top(), &page, "结果窗口留着，不抢页面");
    assert!(a.log.all().iter().any(|m| m.text.contains("战斗胜利")), "消息栏有战斗结束提示");
}
