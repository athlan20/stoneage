//! STAGE-26 验收（场景）：命令表分发、Tab 补全、↑↓ 历史、help。补全、历史和 help 都在客户端
//! 本地完成，用 `world_enter` 用例进入世界（地点表由模拟服务端自动应答），全程不发用例之外的请求。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Down, Enter, Esc, Tab, Up};
use stoneage_client::app::Screen;
use support::{Harness, opts};

/// 登录并进入世界。
fn login_to_world(h: &mut Harness) {
    h.text("alice");
    h.key(Tab);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
}

/// 输入行当前的内容。
fn input(h: &Harness) -> String {
    h.app.world.as_ref().unwrap().input.text().to_string()
}

/// 消息栏里有没有包含 text 的消息。
fn logged(a: &stoneage_client::app::App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

#[test]
fn completion_history_and_help() {
    let mut h = Harness::new("world_enter", opts);
    login_to_world(&mut h);
    h.wait("地点表已取到", 5, |a| a.world.as_ref().is_some_and(|w| !w.places.is_empty()));

    // Tab 补全命令名：唯一候选补全并加空格。
    h.text("he");
    h.key(Tab);
    assert_eq!(input(&h), "help ");
    h.key(Enter);
    h.wait("help 列出全部命令", 5, |a| logged(a, "help 命令名 看详细用法"));

    // help 命令名：详细用法。
    h.text("help warp");
    h.key(Enter);
    h.wait("warp 的详细用法", 5, |a| logged(a, "用法：warp 地点编号|地点名|地图编号"));

    // Tab 补全参数：地点名完整匹配唯一时直接补全（不同分支的地点表都能过）。
    h.text("warp 萨姆吉尔村");
    h.key(Tab);
    assert_eq!(input(&h), "warp 萨姆吉尔村 ");
    h.repeat(crossterm::event::KeyCode::Backspace,input(&h).chars().count());
    h.key(Esc);

    // 多个候选补到公共前缀（这里没有进展），把候选写进消息栏。
    h.text("a");
    h.key(Tab);
    assert_eq!(input(&h), "a", "addpoint 和 ag 的公共前缀还是 a");
    assert!(logged(&h.app, "候选：addpoint"), "{}", h.app.log.all().iter().last().unwrap().text);
    h.repeat(crossterm::event::KeyCode::Backspace,input(&h).chars().count());
    h.key(Esc);

    // 未知命令按命令表提示。
    h.text("foo bar");
    h.key(Enter);
    h.wait("未知命令的提示", 5, |a| logged(a, "未知命令 foo"));

    // 地图页的地点是能选的列表，空行的 ↑↓ 移动选中项（STAGE-116）；历史在没有列表的社交页上翻，先切过去。
    // 当前已处于辅助命令栏，历史直接在这里浏览。
    // ↑↓ 翻历史：从新到旧，越过最新时清空。
    h.key(Up);
    assert_eq!(input(&h), "foo bar");
    h.key(Up);
    assert_eq!(input(&h), "help warp");
    h.key(Down);
    assert_eq!(input(&h), "foo bar");
    h.key(Down);
    assert_eq!(input(&h), "", "回到最新，输入行清空");

    // 输入到一半去翻历史，只翻以打的字开头的（STAGE-113）：没有以 wip 开头的，↑ 不动。
    h.text("wip");
    h.key(Up);
    assert_eq!(input(&h), "wip");
    h.repeat(crossterm::event::KeyCode::Backspace,input(&h).chars().count());
    h.key(Esc);
    h.text("fo");
    h.key(Up);
    assert_eq!(input(&h), "foo bar");
    h.key(Down);
    assert_eq!(input(&h), "fo", "越过最新一条，还原成打的字");
    h.repeat(crossterm::event::KeyCode::Backspace,input(&h).chars().count());
    h.key(Esc);
    h.finish();
}
