//! STAGE-32 验收（场景）：宠物管理命令发出的请求与推送对得上、设出战时原出战宠变休息、改名与恢复种族名、
//! 放生要先输入 yes 确认、栏位超出范围在本地报错。模拟服务端按用例 `ui_pet_manage` 回放（严格模式：请求的 type 和 data 都要一致）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Tab};
use stoneage_client::app::{App, Confirm, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_pets.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 在底部输入行敲一条命令并回车。
fn command(h: &mut Harness, line: &str) {
    h.text(line);
    h.key(Enter);
}

/// 消息栏里有没有一条包含 text 的消息。
fn logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

/// 某个栏位宠物的状态（没有宠物为 None）。
fn state_of(a: &App, slot: u8) -> Option<String> {
    a.world.as_ref()?.pets.pets.iter().find(|p| p.slot == slot).map(|p| p.state.clone())
}

/// 某个栏位宠物的名字（没有宠物为 None）。
fn name_of(a: &App, slot: u8) -> Option<String> {
    a.world.as_ref()?.pets.pets.iter().find(|p| p.slot == slot).map(|p| p.name.clone())
}

/// 当前有几只宠物。
fn pet_count(a: &App) -> usize {
    a.world.as_ref().map_or(0, |w| w.pets.pets.len())
}

/// 放生等待确认时，待放生的栏位（没有等待为 None）。
fn pending_drop(a: &App) -> Option<u8> {
    match a.world.as_ref()?.confirm.as_ref()? {
        Confirm::PetDrop { slot, .. } => Some(*slot),
        _ => None,
    }
}

#[test]
fn pet_manage_commands_follow_pushes() {
    let mut h = Harness::new("ui_pet_manage", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("管理命令给的两只宠物", 5, |a| pet_count(a) == 2);

    // pet battle 0：0 号出战。
    command(&mut h, "pet battle 0");
    h.wait("0 号出战", 5, |a| state_of(a, 0).as_deref() == Some("battle"));
    h.wait("设出战的消息", 5, |a| logged(a, "0 号宠物已设为出战"));

    // pet battle 1：1 号出战，原来的 0 号自动变成休息。
    command(&mut h, "pet battle 1");
    h.wait("1 号出战、0 号休息", 5, |a| state_of(a, 1).as_deref() == Some("battle") && state_of(a, 0).as_deref() == Some("rest"));

    // pet rename 1 小黑 与 pet rename 1（不带名字恢复种族名）。
    command(&mut h, "pet rename 1 小黑");
    h.wait("改名", 5, |a| name_of(a, 1).as_deref() == Some("小黑"));
    h.wait("改名的消息", 5, |a| logged(a, "1 号宠物改名为 小黑"));
    command(&mut h, "pet rename 1");
    h.wait("恢复种族名", 5, |a| name_of(a, 1).as_deref() == Some("乌力"));
    h.wait("恢复种族名的消息", 5, |a| logged(a, "1 号宠物已恢复种族名"));

    // 栏位超出范围：本地报错，不发请求（剧本里没有这一步，发了模拟服务端会报失败）。
    command(&mut h, "pet battle 20");
    h.wait("栏位范围的提示", 5, |a| logged(a, "宠物栏位要在 0–19 之间"));

    // 放生：先提示，输入别的内容取消；再输入 y 才真正放生。
    command(&mut h, "pet drop 0");
    h.wait("放生的提示", 5, |a| logged(a, "放生后不能恢复：乌力（0 号）"));
    assert_eq!(pending_drop(&h.app), Some(0), "等待确认时记下要放生的栏位");
    assert_eq!(pet_count(&h.app), 2, "还没确认，宠物都在");
    command(&mut h, "n");
    h.wait("输入其他内容取消放生", 5, |a| logged(a, "已取消放生"));
    assert_eq!(pending_drop(&h.app), None);
    command(&mut h, "pet drop 0");
    command(&mut h, "是");
    h.wait("放生后只剩 1 只", 5, |a| pet_count(a) == 1);
    h.wait("放生的消息", 5, |a| logged(a, "乌力（0 号）已放生"));
    assert_eq!(state_of(&h.app, 1).as_deref(), Some("battle"), "放生别的宠物不影响出战的 1 号");
    h.finish();
}
