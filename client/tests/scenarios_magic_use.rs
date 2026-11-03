//! STAGE-231 验收（场景）：角色页精灵清单的「使用」走通：选 0 号治愈的精灵直接使用（台词进消息栏）；
//! 选 1 号滋润的精灵，对象选 1 号宠物（倒下的加美加血）。模拟服务端按用例 `ui_magic_use` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Down, Enter, Tab};
use stoneage_client::app::{App, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_magic.rs）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 消息栏里有没有一条包含 text 的消息。
fn logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

#[test]
fn spirit_use_on_the_character_page_follows_the_replies() {
    let mut h = Harness::new("ui_magic_use", opts);
    login_to_characters(&mut h);
    h.wait("角色列表", 5, |a| a.chars.character_at(0).is_some());
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.wait("宠物列表", 5, |a| a.world.as_ref().is_some_and(|w| w.pets.pets.len() == 2));

    // 进角色页：游戏菜单里按 2（第 2 页）。
    h.enter_page();
    h.key(Char('2'));
    h.wait("角色页", 5, |a| a.world.as_ref().is_some_and(|w| w.menu == 1));

    // 焦点在 0 号治愈的精灵：Enter 打开「使用」，再 Enter 执行，满血台词是 0。
    h.key(Enter);
    h.key(Enter);
    h.wait("治愈的台词", 5, |a| logged(a, "耐久力回复0"));

    // ↓ 到 1 号滋润的精灵：Enter 打开动作，Enter 进入对象列表（自己、0 号凯比、1 号加美）。
    h.key(Down);
    h.key(Enter);
    h.key(Enter);
    h.wait("对象列表", 5, |a| a.world.as_ref().is_some_and(|w| w.magic_step_rows() == 3));
    h.keys(&[Down, Down]);
    h.key(Enter);
    h.wait("滋润加美的台词", 5, |a| logged(a, "加美的耐久力回复"));
    h.wait("加美加血后血量大于 0", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.pets.pets.iter().any(|p| p.name == "加美" && p.hp > 0))
    });
    h.finish();
}
