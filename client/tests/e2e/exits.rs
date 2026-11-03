//! STAGE-148 real App + server + isolated PostgreSQL schema.
use super::{
    PASSWORD,
    driver::{Client, unique},
    server::Server,
};
use crossterm::event::KeyCode::*;
use stoneage_client::app::{Layer, LoadState, Screen};
fn command(c: &mut Client, text: &str) {
    c.key(Char('/'));
    c.text(text);
    c.key(Enter);
}
fn loaded(c: &mut Client, floor: u32) {
    c.wait("当前地图出口与NPC完成刷新", 15, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.entry.position.floor == floor
                && w.scene.exit_load == LoadState::Ready
                && w.scene.npc_load == LoadState::Ready
                && !w.scene.using_exit
        })
    });
}
#[test]
fn village_doors_npc_named_warp_catalog_and_home() {
    let Some(mut server) = Server::start() else {
        return;
    };
    let account = unique("exit_");
    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    c.submit_password(PASSWORD);
    c.wait_screen(Screen::Characters);
    c.create_character("门口勇者");
    c.wait("建角", 15, |a| {
        a.screen == Screen::Characters && a.chars.list.characters.len() == 1
    });
    c.key(Enter);
    c.key(Enter);
    c.wait_screen(Screen::World);
    command(&mut c, "warp 2000");
    loaded(&mut c, 2000);
    assert_eq!(c.app.world.as_ref().unwrap().scene.exits.len(), 13);
    c.key(Home);
    c.key(Enter);
    c.key(Enter);
    loaded(&mut c, 100);
    assert_eq!((c.world_position().1, c.world_position().2), (79, 614));
    command(&mut c, "warp 2000");
    loaded(&mut c, 2000);
    c.key(Home);
    c.key(Down);
    c.key(Enter);
    loaded(&mut c, 100);
    assert_eq!((c.world_position().1, c.world_position().2), (114, 660));
    command(&mut c, "warp 1006");
    loaded(&mut c, 1006);
    c.key(Right);
    c.key(Home);
    c.key(Enter);
    c.wait("右列直接对话", 15, |a| {
        a.world.as_ref().unwrap().npc_window.is_some()
    });
    c.key(Esc);
    c.wait("关窗", 15, |a| {
        a.world.as_ref().unwrap().npc_window.is_none()
    });
    command(&mut c, "warp 加加村");
    loaded(&mut c, 3000);
    assert!(c.screen_text().contains("加加村"));
    // Fixed catalogue entry remains accessible, and Esc returns to the scene.
    c.key(Left);
    c.key(End);
    c.key(Up);
    c.key(Enter);
    c.wait("查人目录", 15, |a| {
        matches!(
            &a.world.as_ref().unwrap().scene_step,
            Some(stoneage_client::app::SceneStep::Catalog { loading: false, .. })
        )
    });
    c.key(Esc);
    assert!(c.app.world.as_ref().unwrap().nav.is(&Layer::Scene));
    let home = c
        .app
        .world
        .as_ref()
        .unwrap()
        .entry
        .savepoint
        .as_ref()
        .unwrap()
        .floor;
    c.key(End);
    c.key(Enter);
    loaded(&mut c, home);
    drop(c);
    server.stop();
    server.assert_no_plaintext(&[PASSWORD]);
}
