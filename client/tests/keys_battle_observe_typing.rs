//! 观察模式（agent 操控）下，玩家在命令栏里打字时，agent 的战斗推送（新回合、回合结算）
//! 不关命令栏、不清输入；自己操控时新回合仍按旧规则收起命令栏。不连服务端：用推送数据喂状态。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use serde_json::json;
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::net::NetEvent;
use stoneage_client::proto::{Incoming, WorldEntry};

fn world_app(observer: bool) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.screen = Screen::World;
    if observer {
        a.world.as_mut().unwrap().entry.control.you = "observer".into();
    }
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn push(a: &mut App, ty: &str, data: serde_json::Value) {
    a.on_net(NetEvent::Message { gen_id: 0, msg: Incoming::Push { ty: ty.into(), data } });
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn start() -> serde_json::Value {
    json!({
        "scene": "野外", "you": 0,
        "allow": {"escape": true, "capture": true},
        "units": [
            {"no": 0, "kind": "player", "name": "勇者小明", "level": 3, "hp_pct": 100},
            {"no": 15, "kind": "monster", "name": "野猪", "level": 3, "hp_pct": 100}
        ]
    })
}

fn turn(n: u32) -> serde_json::Value {
    json!({
        "battle_id": "b_1", "turn": n, "deadline_ms": 1_030_000u64, "server_time_ms": 1_000_000u64,
        "needs": ["player"], "optional": [], "progress": {"done": 0, "total": 1}
    })
}

fn type_text(a: &mut App, s: &str) {
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

fn assert_still_typing(a: &App, draft: &str, when: &str) {
    assert!(world(a).nav.is(&Layer::Command), "{when}：命令栏被关了");
    assert_eq!(world(a).input.text(), draft, "{when}：草稿被清了");
}

#[test]
fn observer_keeps_the_command_bar_while_the_agent_fights() {
    let mut a = world_app(true);
    push(&mut a, "battle_start", start());
    push(&mut a, "battle_turn", turn(1));
    key(&mut a, KeyCode::Char('/'));
    type_text(&mut a, "say hello");
    assert_still_typing(&a, "say hello", "打字");
    push(&mut a, "battle_turn", turn(2));
    assert_still_typing(&a, "say hello", "新回合");
    push(&mut a, "battle_round", json!({"battle_id": "b_1", "turn": 2, "events": []}));
    assert_still_typing(&a, "say hello", "回合结算");
}

#[test]
fn own_battle_turn_still_closes_the_command_bar() {
    let mut a = world_app(false);
    push(&mut a, "battle_start", start());
    push(&mut a, "battle_turn", turn(1));
    key(&mut a, KeyCode::Char('/'));
    type_text(&mut a, "x");
    push(&mut a, "battle_turn", turn(2));
    assert!(!world(&a).nav.is(&Layer::Command), "自己操控时新回合收起命令栏");
    assert_eq!(world(&a).input.text(), "");
}
