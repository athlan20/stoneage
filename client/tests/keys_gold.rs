//! 丢弃石币的命令与确认（STAGE-328，`docs/protocol.md` 5.76）：`gold drop <数量> [yes]`。
//! 不连服务端：只检查本地的确认状态和消息栏；请求的收发见协议用例 gold_drop_*。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{App, Confirm, Options, Screen, WorldState};
use stoneage_client::proto::WorldEntry;

fn world_app() -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

/// 输入一整行并回车（和手敲命令一样）。
fn line(a: &mut App, s: &str) {
    if a.world.as_ref().is_some_and(|w| {
        w.npc_window.is_none() && w.confirm.is_none() && !w.nav.is(&stoneage_client::app::Layer::Command)
    }) {
        key(a, KeyCode::Char('/'));
    }
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
    key(a, KeyCode::Enter);
}

fn confirm(a: &App) -> Option<&Confirm> {
    a.world.as_ref().expect("已进入世界").confirm.as_ref()
}

fn logged(a: &App, s: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(s))
}

#[test]
fn gold_drop_asks_and_y_confirms() {
    let mut a = world_app();
    line(&mut a, "gold drop 1000");
    assert!(matches!(confirm(&a), Some(Confirm::GoldDrop { amount: 1000 })));
    assert!(logged(&a, "丢弃后不能恢复：1000 石币"));
    line(&mut a, "y");
    assert!(confirm(&a).is_none());
}

#[test]
fn gold_drop_with_trailing_yes_skips_the_question() {
    let mut a = world_app();
    line(&mut a, "gold drop 1000 yes");
    assert!(confirm(&a).is_none());
    assert!(!logged(&a, "输入 y 确认"));
}

#[test]
fn gold_drop_rejects_bad_amount_without_asking() {
    let mut a = world_app();
    for bad in ["gold drop 0", "gold drop -5", "gold drop 1.5 yes", "gold drop abc"] {
        line(&mut a, bad);
        assert!(confirm(&a).is_none(), "{bad}");
    }
    assert!(logged(&a, "数量要是正整数"));
}
