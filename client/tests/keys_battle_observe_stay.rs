//! STAGE-284：观察模式（agent 操控）下，玩家在背包、宠物等页面或菜单里时，新战斗、结果窗口不把人拉回战斗画面。
//! 自己操控时 battle_start 仍直接进入战斗前台。不连服务端：用推送数据喂状态，看本地状态。

use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use serde_json::json;
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::net::NetEvent;
use stoneage_client::proto::Incoming;
use stoneage_client::proto::WorldEntry;

/// 结果窗口停留时长（与 `battle/auto.rs` 的 `RESULT_HOLD` 一致）：测试里直接把时钟拨过去。
const HOLD_PASSED: Duration = Duration::from_millis(2600);

/// 进入世界、没有战斗；`observer` 为真表示 agent 在操控。
fn world_app(observer: bool) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.screen = Screen::World;
    if observer {
        a.world.as_mut().unwrap().entry.control.you = "observer".into();
    }
    a
}

fn battle_start_json() -> serde_json::Value {
    json!({
        "scene": "野外",
        "you": 0,
        "allow": {"escape": true, "capture": true},
        "units": [
            {"no": 0, "kind": "player", "name": "勇者小明", "level": 3, "hp_pct": 100},
            {"no": 5, "kind": "pet", "name": "乌力", "level": 3, "hp_pct": 100},
            {"no": 15, "kind": "monster", "name": "野猪", "level": 3, "hp_pct": 100}
        ]
    })
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

/// 喂一条服务端推送（连接代数从 0 开始）。
fn push(a: &mut App, ty: &str, data: serde_json::Value) {
    a.on_net(NetEvent::Message {
        gen_id: 0,
        msg: Incoming::Push { ty: ty.into(), data },
    });
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

/// 从场景根打开游戏菜单，选中第 2 项进入功能页（同 `keys_battle_observe.rs` 的写法）。
fn open_a_page(a: &mut App) {
    key(a, KeyCode::Esc);
    key(a, KeyCode::Down);
    key(a, KeyCode::Enter);
    assert!(matches!(world(a).nav.top(), Layer::Page(_)), "进了功能页");
}

#[test]
fn observer_new_battle_does_not_pull_the_player_back_from_a_page() {
    let mut a = world_app(true);
    open_a_page(&mut a);
    let page = world(&a).nav.top().clone();
    push(&mut a, "battle_start", battle_start_json());
    assert!(world(&a).battle.is_some(), "战斗已建");
    assert!(world(&a).battle_veil, "新战斗留在后台");
    assert!(!world(&a).battle_front(), "没有把人拉回战斗画面");
    assert_eq!(world(&a).nav.top(), &page, "页面不变");
}

#[test]
fn non_observer_battle_start_still_enters_the_battle_front() {
    let mut a = world_app(false);
    open_a_page(&mut a);
    push(&mut a, "battle_start", battle_start_json());
    assert!(world(&a).battle.is_some());
    assert!(!world(&a).battle_veil, "自己操控不盖菜单");
    assert!(world(&a).battle_front(), "自己操控的战斗照旧进入战斗前台");
}

#[test]
fn observer_result_auto_closes_behind_the_menu_then_leaving_the_menu_shows_no_result() {
    let mut a = world_app(true);
    push(&mut a, "battle_start", battle_start_json());
    key(&mut a, KeyCode::Esc); // 战斗中按 Esc 打开菜单（STAGE-182）
    assert!(world(&a).battle_veil && world(&a).nav.is(&Layer::Menu));
    push(
        &mut a,
        "battle_end",
        json!({"battle_id": "b_1", "result": "escaped", "turn": 2}),
    );
    a.on_tick(Instant::now() + HOLD_PASSED);
    assert!(world(&a).battle.is_none(), "结果窗口看够时间后自动关，不依赖托管");
    assert!(world(&a).battle_veil, "菜单仍盖着");
    key(&mut a, KeyCode::Esc); // 菜单退回场景
    assert!(!world(&a).battle_veil && world(&a).nav.is(&Layer::Scene));
    assert!(!world(&a).battle_front(), "退出菜单不弹已结束战斗的结果");
}

#[test]
fn leaving_the_menu_after_battle_end_closes_the_battle_quietly() {
    let mut a = world_app(true);
    push(&mut a, "battle_start", battle_start_json());
    key(&mut a, KeyCode::Esc);
    push(
        &mut a,
        "battle_end",
        json!({"battle_id": "b_1", "result": "win", "turn": 2}),
    );
    key(&mut a, KeyCode::Esc); // 还没到 RESULT_HOLD 就退出菜单
    assert!(world(&a).battle.is_none(), "已结束的战斗直接收掉");
    assert!(world(&a).nav.is(&Layer::Scene));
    assert!(!world(&a).battle_front());
}

#[test]
fn battle_gone_but_veil_left_does_not_kick_the_player_out_of_the_menu() {
    let mut a = world_app(true);
    push(&mut a, "battle_start", battle_start_json());
    key(&mut a, KeyCode::Esc);
    push(
        &mut a,
        "battle_end",
        json!({"battle_id": "b_1", "result": "win", "turn": 2}),
    );
    a.on_tick(Instant::now() + HOLD_PASSED);
    assert!(world(&a).battle.is_none() && world(&a).battle_veil);
    key(&mut a, KeyCode::Down); // 菜单里按任意键
    assert!(world(&a).battle_veil && world(&a).nav.is(&Layer::Menu), "不被踢回场景");
}
