//! 连接代次、未知结果和刷新屏障的边界回归。
use super::{App, Layer, Options, Screen, WorldState, session::Pending};
use crate::net::NetEvent;
use crate::proto::{Incoming, WorldEntry};
use serde_json::json;
#[test]
fn refresh_barrier_keeps_old_view_until_every_required_reply() {
    let mut a = App::new(Options::default());
    let mut old = WorldEntry::default();
    old.character.name = "勇者".into();
    old.status.gold = 1;
    a.world = Some(WorldState::new(old));
    a.screen = Screen::World;
    a.begin_reconnect();
    assert!(a.stage_reconnect("勇者", false));
    let mut new = WorldEntry::default();
    new.character.name = "勇者".into();
    new.status.gold = 99;
    a.world = Some(WorldState::new(new));
    a.world.as_mut().unwrap().online = false;
    for p in [
        Pending::Places,
        Pending::Maps,
        Pending::Inventory,
        Pending::Pets,
        Pending::Quests,
    ] {
        a.on_ok(p, json!({}));
        assert!(!a.world.as_ref().unwrap().online);
        assert!(a.reconnect.as_ref().unwrap().old.is_some());
    }
    let c=super::SceneQuery{floor:0,epoch:0,query:0};
    a.on_ok(Pending::Npcs(c), json!({"floor":0,"npcs":[]}));
    assert!(!a.world.as_ref().unwrap().online);
    a.on_ok(Pending::Exits(c),json!({"floor":0,"exits":[],"server_time_ms":0,"next_change_ms":null}));
    assert!(a.world.as_ref().unwrap().online);
    assert_eq!(a.world.as_ref().unwrap().entry.status.gold, 99);
    assert!(a.reconnect.is_none());
}
#[test]
fn refresh_error_restores_the_complete_old_snapshot() {
    let mut a = App::new(Options::default());
    let mut e = WorldEntry::default();
    e.character.name = "勇者".into();
    e.status.gold = 7;
    a.world = Some(WorldState::new(e));
    a.begin_reconnect();
    assert!(a.stage_reconnect("勇者", false));
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.offline("资料刷新失败", false);
    assert_eq!(a.world.as_ref().unwrap().entry.status.gold, 7);
    assert!(!a.world.as_ref().unwrap().online);
    assert_eq!(a.screen, Screen::World);
}

#[test]
fn cancelling_edited_login_restores_snapshot_account(){
    let mut a=App::new(Options::default());a.login.account.set("original");a.world=Some(WorldState::new(WorldEntry::default()));a.begin_reconnect();a.login.account.set("edited");
    assert!(a.cancel_reconnect());assert_eq!(a.login.account.text(),"original");assert!(a.world.as_ref().is_some_and(|w|!w.online));
}
#[test]
fn readonly_discards_mutations_but_retains_data_and_message_navigation() {
    let mut w = WorldState::new(WorldEntry::default());
    w.auto_battle = true;
    w.nav.show_page(super::BAG_MENU);
    w.nav.push(Layer::Command);
    w.input.set("item drop 5");
    w.readonly();
    assert!(!w.online && !w.auto_battle);
    assert!(w.input.is_empty());
    assert!(w.nav.is(&Layer::Page(super::BAG_MENU)));
}

#[test]
fn native_confirmation_defaults_to_cancel_and_rechecks_revision() {
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.ask_confirm(
        super::Confirm::ItemDrop {
            slot: 5,
            name: "药".into(),
            count: None,
        },
        "丢弃药？",
    );
    assert!(a.world.as_ref().unwrap().confirm_native);
    a.on_key(KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE));
    assert!(a.world.as_ref().unwrap().confirm.is_none());
    assert!(!a.session.busy());
    a.ask_confirm(
        super::Confirm::ItemDrop {
            slot: 5,
            name: "药".into(),
            count: None,
        },
        "丢弃药？",
    );
    a.world.as_mut().unwrap().inventory_epoch += 1;
    for key in [KeyCode::Right, KeyCode::Enter] {
        a.on_key(KeyEvent::new(key, KeyModifiers::NONE));
    }
    assert!(
        a.log
            .all()
            .iter()
            .any(|m| m.text.contains("对象资料已经变化"))
    );
    assert!(!a.session.busy());
}

#[test]
fn pages_do_not_type_hidden_commands_or_execute_closed_drafts() {
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    a.world = Some(WorldState::new(WorldEntry::default()));
    for menu in [
        super::CHAR_MENU,
        super::BAG_MENU,
        super::PETS_MENU,
        super::SYSTEM_MENU,
    ] {
        let w = a.world.as_mut().unwrap();
        w.menu = menu;
        w.nav.show_page(menu);
        w.input.clear();
        a.on_key(KeyEvent::new(KeyCode::Char('a'), KeyModifiers::NONE));
        assert!(a.world.as_ref().unwrap().input.is_empty());
    }
    a.world.as_mut().unwrap().input.set("warp 1");
    a.on_key(KeyEvent::new(KeyCode::Down, KeyModifiers::NONE));
    assert_eq!(a.world.as_ref().unwrap().input.text(), "warp 1");
    assert!(!a.session.busy());
}
#[test]
fn old_connection_push_cannot_overwrite_readonly_snapshot() {
    let mut a = App::new(Options::default());
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.screen = Screen::World;
    a.offline("连接断开", true);
    a.on_net(NetEvent::Message {
        gen_id: 0,
        msg: Incoming::Push {
            ty: "status_changed".into(),
            data: json!({"gold":999}),
        },
    });
    assert_eq!(a.world.as_ref().unwrap().entry.status.gold, 0);
    assert!(a.log.all().iter().any(|m| m.text.contains("结果未知")));
}

/// STAGE-162：重连前有分身队、重连后的世界里没有（队长所有连接断开后分身已消失）：
/// 界面不残留队伍，消息栏说明“分身已随断线消失”。
#[test]
fn reconnect_clears_a_party_that_vanished_with_the_disconnect() {
    use crate::proto::{Party, PartyMember};
    let mut a = App::new(Options::default());
    let mut old = WorldEntry::default();
    old.character.name = "勇者".into();
    old.party = Some(Party { members: vec![PartyMember::default(); 5], ..Default::default() });
    a.world = Some(WorldState::new(old));
    a.screen = Screen::World;
    a.begin_reconnect();
    assert!(a.stage_reconnect("勇者", false));
    let mut new = WorldEntry::default();
    new.character.name = "勇者".into(); // 重连后的 enter_world 里没有 party
    a.world = Some(WorldState::new(new));
    a.world.as_mut().unwrap().online = false;
    for p in [Pending::Places, Pending::Maps, Pending::Inventory, Pending::Pets, Pending::Quests] {
        a.on_ok(p, json!({}));
    }
    let c = super::SceneQuery { floor: 0, epoch: 0, query: 0 };
    a.on_ok(Pending::Npcs(c), json!({"floor":0,"npcs":[]}));
    a.on_ok(Pending::Exits(c), json!({"floor":0,"exits":[],"server_time_ms":0,"next_change_ms":null}));
    let w = a.world.as_ref().unwrap();
    assert!(w.online && w.party().is_none(), "重连完成后没有队伍");
    assert!(a.log.all().iter().any(|m| m.kind == super::Kind::Party && m.text.contains("分身已随断线消失")));
}

/// 重连后队伍还在（agent 一直在线保住了它）：不说“消失”。
#[test]
fn reconnect_keeps_a_party_that_survived() {
    use crate::proto::{Party, PartyMember};
    let mut a = App::new(Options::default());
    let party = Party { members: vec![PartyMember::default(); 5], ..Default::default() };
    let mut old = WorldEntry::default();
    old.character.name = "勇者".into();
    old.party = Some(party.clone());
    a.world = Some(WorldState::new(old));
    a.screen = Screen::World;
    a.begin_reconnect();
    assert!(a.stage_reconnect("勇者", false));
    let mut new = WorldEntry::default();
    new.character.name = "勇者".into();
    new.party = Some(party);
    a.world = Some(WorldState::new(new));
    a.world.as_mut().unwrap().online = false;
    for p in [Pending::Places, Pending::Maps, Pending::Inventory, Pending::Pets, Pending::Quests] {
        a.on_ok(p, json!({}));
    }
    let c = super::SceneQuery { floor: 0, epoch: 0, query: 0 };
    a.on_ok(Pending::Npcs(c), json!({"floor":0,"npcs":[]}));
    a.on_ok(Pending::Exits(c), json!({"floor":0,"exits":[],"server_time_ms":0,"next_change_ms":null}));
    assert!(a.world.as_ref().unwrap().party().is_some());
    assert!(!a.log.all().iter().any(|m| m.text.contains("随断线消失")));
}
