#[allow(dead_code)]
mod support;
use crossterm::event::KeyCode::*;
use stoneage_client::app::{LoadState, Screen};
use support::{Harness, opts};
#[test]
fn both_village_doors_send_selected_ids_and_wait_for_position_push() {
    let mut h = Harness::new("ui_exits", opts);
    h.text("alice");
    h.key(Tab);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.command("warp 2000");
    h.wait("渔村13个出口", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.entry.position.floor == 2000
                && w.scene.exits.len() == 13
                && w.scene.exit_load == LoadState::Ready
        })
    });
    h.key(Home);
    h.key(Enter);
    h.key(Enter); // repeated Enter before a response must not resubmit
    h.wait("第一个村口落点", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            (
                w.entry.position.floor,
                w.entry.position.x,
                w.entry.position.y,
            ) == (100, 79, 614)
        })
    });
    h.command("warp 2000");
    h.wait("返回渔村出口重新加载", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.entry.position.floor == 2000 && w.scene.exits.len() == 13 && !w.scene.using_exit
        })
    });
    h.key(Home);
    h.key(Down);
    h.key(Enter);
    h.wait("第二个村口落点", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            (
                w.entry.position.floor,
                w.entry.position.x,
                w.entry.position.y,
            ) == (100, 114, 660)
        })
    });
    h.finish();
}
