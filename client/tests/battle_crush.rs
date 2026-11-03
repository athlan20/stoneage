//! 装备耐久在战斗里（STAGE-225，`docs/protocol.md` 10.7、`docs/durability.md`）：事件的 `crush` 数组读进来，
//! 回放时按顺序排进队列；消息栏的颜色：受损、毁坏等黄色，消失、不堪使用等红色（以 `red` 为准）。

use std::time::{Duration, Instant};

use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, Kind, Options};
use stoneage_client::proto::{
    BattleAllow, BattleEffect, BattleEvent, BattleRound, BattleStart, BattleTurn, BattleUnit, CrushNote, Progress,
};

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

fn unit(no: u8, kind: &str, name: &str) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level: 5, hp_pct: 100, ..BattleUnit::default() }
}

/// 我方勇者对敌方野猪，回合 1 等人物出招。
fn battle() -> BattleState {
    let mut hero = unit(0, "player", "勇者小明");
    hero.hp = Some(45);
    hero.max_hp = Some(60);
    let start = BattleStart { party: None,
        field: None,
        battle_id: "b_1".into(),
        kind: "wild".into(),
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![hero, unit(15, "monster", "野猪")],
        npc: None,
    };
    let mut b = BattleState::from_start(start);
    let turn = BattleTurn {
        field: None,
        battle_id: "b_1".into(),
        turn: 1,
        deadline_ms: 1_030_000,
        server_time_ms: 1_000_000,
        needs: vec!["player".into()],
        optional: vec![],
        progress: Progress { done: 0, total: 1 },
    };
    b.on_turn(turn, Instant::now());
    b
}

/// 野猪打中勇者一下，身上的铁甲受损、头盔消失（两条提示）。
fn round_with_crush() -> BattleRound {
    let ev = BattleEvent {
        kind: "attack".into(),
        actor: Some(15),
        effects: vec![BattleEffect { target: 0, result: "hit".into(), delta: -7, hp_pct: Some(88), ..Default::default() }],
        crush: vec![
            CrushNote { text: "铁甲是受损的。".into(), red: true },
            CrushNote { text: "皮帽因过度损坏而消失。".into(), red: false },
        ],
        ..BattleEvent::default()
    };
    BattleRound { turn: 1, events: vec![ev], ..BattleRound::default() }
}

#[test]
fn crush_array_is_read_from_the_attack_event() {
    let json = serde_json::json!({
        "kind": "attack",
        "actor": 15,
        "effects": [{"target": 0, "result": "hit", "delta": -7}],
        "crush": [{"text": "铁甲是受损的。", "red": true}, {"text": "皮帽因过度损坏而消失。"}]
    });
    let ev: BattleEvent = serde_json::from_value(json).unwrap();
    assert_eq!(ev.crush.len(), 2);
    assert_eq!(ev.crush[0], CrushNote { text: "铁甲是受损的。".into(), red: true });
    assert!(!ev.crush[1].red, "没有 red 的按黄色，缺省为假");
    let plain: BattleEvent = serde_json::from_value(serde_json::json!({"kind": "attack"})).unwrap();
    assert!(plain.crush.is_empty(), "没有变化时不带 crush");
}

#[test]
fn crush_notes_are_queued_in_order_when_the_round_plays() {
    let t0 = Instant::now();
    let mut b = battle();
    b.on_round(round_with_crush(), t0);
    // 按时间推到回放结束（第 2 次 advance 起才会真正开始播）。
    let mut now = t0;
    for _ in 0..100 {
        if b.advance(now) {
            break;
        }
        now += Duration::from_millis(100);
    }
    // STAGE-248：耐久提示和战斗文字共用一个有序队列，这里只看耐久的几条，顺序仍按发生的先后。
    let got: Vec<(Kind, String)> = b.take_pending().into_iter().filter(|(k, _)| matches!(k, Kind::Durability | Kind::DurabilityRed)).collect();
    assert_eq!(got, vec![(Kind::DurabilityRed, "铁甲是受损的。".to_string()), (Kind::Durability, "皮帽因过度损坏而消失。".to_string())]);
    assert!(b.take_pending().is_empty(), "取走后队列清空，不会重复进消息栏");
}

#[test]
fn crush_kinds_follow_the_red_flag() {
    let mut a = app();
    a.log.crush("皮帽因过度损坏而消失。", false);
    a.log.crush("铁甲是受损的。", true);
    let msgs = a.log.all();
    let gone = msgs.iter().find(|m| m.text.contains("消失")).expect("消失的提示进消息");
    let worn = msgs.iter().find(|m| m.text.contains("受损")).expect("受损的提示进消息");
    assert_eq!(gone.kind, Kind::Durability);
    assert_eq!(gone.kind.color(), ratatui::style::Color::Yellow, "red 为假：黄色");
    assert_eq!(worn.kind, Kind::DurabilityRed);
    assert_eq!(worn.kind.color(), ratatui::style::Color::Red, "red 为真：红色");
    assert_eq!(gone.kind.label(), "耐久  ", "标签宽度 6 列，和其他种类对齐");
    assert_eq!(worn.kind.label(), "耐久  ");
}
