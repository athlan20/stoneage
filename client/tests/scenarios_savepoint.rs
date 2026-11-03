//! STAGE-72 验收（场景）：`talk 205` 弹记录窗口（要一件凯比特的肉），没有物品时选记录
//! 收到错误提示；用例 `ui_savepoint` 由模拟服务端回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
use stoneage_client::app::Screen;
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

#[test]
fn savepoint_window_and_reject() {
    let mut h = Harness::new("ui_savepoint", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    // 储存点在萨姆吉尔村。
    h.wait("地点表", 5, |a| !a.world.as_ref().map_or(true, |w| w.places.is_empty()));
    h.text("warp 1");
    h.key(Enter);
    h.run_for(400);

    // talk 205：记录窗口弹出（Request 文案）。
    h.text("talk 205");
    h.key(Enter);
    h.wait("记录窗口", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window.as_ref().is_some_and(|d| d.npc == 205 && d.options.iter().any(|o| o.label == "记录"))
        })
    });

    // 没有候选物品时选记录：错误提示，窗口不动。
    h.key(Char('1'));
    h.run_for(300);
    assert!(h.last_error().contains("肉"), "应提示要物品：{}", h.last_error());
    assert!(h.app.world.as_ref().is_some_and(|w| w.npc_window.is_some()));
    h.finish();
}
