//! STAGE-69 验收（场景）：`talk 225` 弹出对话窗口，`choose 1` 回错误（本期对话没有
//! 选项），`Esc` 和数字键都能关窗且都发 `npc_close`；模拟服务端按用例 `ui_npc_talk` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Esc, Tab};
use stoneage_client::app::Screen;
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

#[test]
fn npc_window_opens_and_closes() {
    let mut h = Harness::new("ui_npc_talk", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();

    // talk 225：窗口弹出，标题和正文都是推送带来的。
    h.text("talk 225");
    h.key(Enter);
    h.wait("对话窗口", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.npc_window.as_ref().is_some_and(|d| d.npc == 225 && d.name == "村长家的厨师"))
    });

    // choose 1：本期对话没有选项，服务端回 NPC_BAD_CHOICE，窗口保持打开。
    h.key(Char('/'));
    h.command("choose 1");
    h.run_for(300);
    assert!(h.last_error().contains("选项"), "应提示选项错误：{}", h.last_error());
    assert!(h.app.world.as_ref().is_some_and(|w| w.npc_window.is_some()));

    // Esc：关窗并同步清掉服务端状态（发 npc_close）。
    h.key(Esc);
    h.wait("Esc 关窗", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_none()));

    // 再开一次，按数字键关掉（没有选项的窗口按数字就是关）。
    h.text("talk 225");
    h.key(Enter);
    h.wait("再次弹窗", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_some()));
    h.key(Char('3'));
    h.wait("数字键关窗", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_none()));
    h.finish();
}
