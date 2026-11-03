//! STAGE-29 验收（场景）：穿上斧头后攻击从 5 变成 14（status_changed 推送，不用刷新）、卸下后恢复；
//! 丢弃不带 yes 只提示、带 yes 才丢；格子编号不对在本地报错。模拟服务端按用例 `ui_item_manage` 回放（严格模式）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Tab};
use stoneage_client::app::{App, Screen};
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

/// 背包的某一格是不是这件物品。
fn bag_has(a: &App, slot: u8, name: &str) -> bool {
    a.world.as_ref().is_some_and(|w| w.inventory.slots.iter().any(|s| s.slot == slot && s.name == name))
}

/// 角色页上的衍生数值（攻击、防御、敏捷）。
fn derived(a: &App) -> Option<(u32, u32, u32)> {
    let d = a.world.as_ref()?.entry.status.derived;
    Some((d.attack, d.defense, d.agility))
}

#[test]
fn item_commands_follow_pushes() {
    let mut h = Harness::new("ui_item_manage", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("管理命令发的斧头在第 5 格", 5, |a| bag_has(a, 5, "小斧头"));
    assert_eq!(derived(&h.app), Some((5, 5, 4)), "没穿装备时是基础数值");

    // item equip 5：穿到手上，攻击 +9、防御 -3、敏捷 -3 立刻变化。
    command(&mut h, "item equip 5");
    h.wait("穿上斧头后攻击 14", 5, |a| derived(a) == Some((14, 2, 1)));
    h.wait("穿上的消息", 5, |a| logged(a, "已穿上 小斧头"));
    assert!(bag_has(&h.app, 2, "小斧头"), "斧头在手上（2 号格）");

    // item unequip 2：卸下，数值恢复，斧头回到第一个空背包格（5 号）。
    command(&mut h, "item unequip 2");
    h.wait("卸下后数值恢复", 5, |a| derived(a) == Some((5, 5, 4)));
    h.wait("卸下的消息", 5, |a| logged(a, "已卸下 小斧头"));
    assert!(bag_has(&h.app, 5, "小斧头"));

    // item drop 5：没有 yes 先问一句，不发请求（剧本里没有这一步，发了模拟服务端会报错）。
    command(&mut h, "item drop 5");
    h.wait("丢弃要确认", 5, |a| logged(a, "丢弃后不能恢复：小斧头（5 号）。输入 y 确认"));
    assert!(bag_has(&h.app, 5, "小斧头"), "没确认，东西还在");

    // 输入 y：真正丢弃，格子空了（命令末尾带 yes 的直接丢弃写法见单元测试 keys_global.rs）。
    command(&mut h, "y");
    h.wait("丢弃后背包里没有了", 5, |a| a.world.as_ref().is_some_and(|w| w.inventory.slots.is_empty()));
    h.wait("丢弃的消息", 5, |a| logged(a, "已丢弃 小斧头（5 号）"));

    // 本地检查：穿上要选背包格，空格子不发请求。
    command(&mut h, "item equip 2");
    h.wait("穿上要选背包格", 5, |a| logged(a, "穿上要选背包格"));
    command(&mut h, "item move 5 6");
    h.wait("空格子不能移动", 5, |a| logged(a, "5 号格子没有物品"));
    h.finish();
}

/// STAGE-132：同一套剧本（`ui_item_manage`）改用背包菜单动作走一遍——不发命令，
/// 全用方向键 + Enter 完成穿上、卸下、丢弃。请求和推送与 `item` 命令完全一致。
#[test]
fn item_actions_do_the_same_as_the_commands() {
    use crossterm::event::KeyCode::{Char, Down};
    let mut h = Harness::new("ui_item_manage", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.key(Char('4')); // 切到背包页
    h.wait("管理命令发的斧头在第 5 格", 5, |a| bag_has(a, 5, "小斧头"));
    assert_eq!(derived(&h.app), Some((5, 5, 4)), "没穿装备时是基础数值");

    // 物品栏默认选中 5 号小斧头：Enter 打开动作菜单，第一项就是「装备」。
    h.key(Enter);
    h.key(Enter);
    h.wait("穿上斧头后攻击 14", 5, |a| derived(a) == Some((14, 2, 1)));
    h.wait("穿上的消息", 5, |a| logged(a, "已穿上 小斧头"));
    h.wait("斧头在手上（2 号格）", 5, |a| bag_has(a, 2, "小斧头"));

    // 穿上后物品栏空了，焦点自动落到装备栏（选中手上的斧头）：Enter → 「卸下」→ Enter。
    h.keys(&[Enter, Enter]);
    h.wait("卸下后数值恢复", 5, |a| derived(a) == Some((5, 5, 4)));
    h.wait("卸下的消息", 5, |a| logged(a, "已卸下 小斧头"));
    h.wait("斧头回到 5 号背包格", 5, |a| bag_has(a, 5, "小斧头"));

    // 丢弃：卸下后焦点回到物品栏（斧头在 5 号）。Enter → 动作列表 → 下两下到「丢弃」→ Enter 进数量表单。
    h.keys(&[Enter, Down, Down, Enter]);
    h.text("1");
    h.key(Enter);
    h.wait("丢弃要确认", 5, |a| a.world.as_ref().is_some_and(|w|w.confirm.is_some()&&w.confirm_native&&!w.confirm_choice));
    assert!(bag_has(&h.app, 5, "小斧头"), "还没确认，东西还在");
    h.key(crossterm::event::KeyCode::Right);h.key(Enter);
    h.wait("丢弃后背包里没有了", 5, |a| a.world.as_ref().is_some_and(|w| w.inventory.slots.is_empty()));
    h.wait("丢弃的消息", 5, |a| logged(a, "已丢弃 小斧头（5 号）"));
    h.finish();
}
