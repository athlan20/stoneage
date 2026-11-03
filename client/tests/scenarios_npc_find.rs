//! STAGE-84 验收（场景）：`npc 村长家的厨师` 在同图直接弹出对话窗口；`npc 糊涂小孩` 在别的地图上
//! 先问是否传送，输入 `Y` 后 warp 到那张地图再弹窗，输入其他内容则取消；模拟服务端按用例 `ui_npc_find` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Esc, Tab};
use stoneage_client::app::{Confirm, Kind, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 消息栏里最近一条系统消息的正文。
fn last_system(h: &Harness) -> String {
    h.app.log.all().iter().rev().find(|m| m.kind == Kind::System).map(|m| m.text.clone()).unwrap_or_default()
}

#[test]
fn npc_command_talks_here_and_travels_after_yes() {
    let mut h = Harness::new("ui_npc_find", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();

    // 同图：直接对话，窗口弹出。
    h.text("npc 村长家的厨师");
    h.key(Enter);
    h.wait("同图对话", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.as_ref().is_some_and(|d| d.npc == 225)));
    h.key(Esc);
    h.wait("Esc 关窗", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_none()));

    // 别的地图：先问是否传送，不先动。
    h.text("npc 糊涂小孩");
    h.key(Enter);
    h.wait("问是否传送", 5, |a| matches!(a.world.as_ref().and_then(|w| w.confirm.as_ref()), Some(Confirm::NpcVisit(_))));
    assert!(last_system(&h).contains("要传送过去吗"), "应问是否传送：{}", last_system(&h));
    assert!(h.app.world.as_ref().is_some_and(|w| w.npc_window.is_none()), "没有确认前不能开窗");

    // 原生确认：选确认后传送到避难洞窟，再直接对话。
    h.key(crossterm::event::KeyCode::Right);
    h.key(Enter);
    h.wait("传送后对话", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.as_ref().is_some_and(|d| d.npc == 635)));
    assert!(h.app.world.as_ref().is_some_and(|w| w.confirm.is_none()), "确认后问题应清掉");
    h.key(Esc);
    h.wait("关窗", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_none()));
    h.finish();
}
