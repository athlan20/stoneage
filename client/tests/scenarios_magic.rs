//! STAGE-203 验收（场景）：`magic <栏位> [宠物编号]` 发出的请求与服务端回复对得上：
//! 空栏位、不是变幻的精灵、目标没宠物、加美被拒（错误写进消息栏），变成凯比后状态里有 metamo、消息栏有“变身成凯比！”。
//! 模拟服务端按协议用例 `magic_use_metamo` 回放（严格模式：请求的 type 和 data 都要一致），只走到变身成功为止。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Tab};
use stoneage_client::app::{App, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_pet_manage.rs）。
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

/// 当前变身的宠物名（没有变身为 None）。
fn metamo_pet(a: &App) -> Option<String> {
    a.world.as_ref()?.entry.status.metamo.as_ref().map(|m| m.pet.clone())
}

#[test]
fn magic_commands_follow_replies() {
    let mut h = Harness::new("magic_use_metamo", opts);
    login_to_characters(&mut h);
    // 角色列表回来之后再进入（列表没回来时回车会被会话占用，什么也不发）。
    h.wait("角色列表", 5, |a| a.chars.character_at(0).is_some());
    // 第一次回车打开角色的操作菜单，第二次选“进入游戏”（菜单第 0 项）。
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.enter_page();

    // 栏位是空的、不是变幻的精灵、目标没宠物、加美：服务端回复的原因写进消息栏。
    command(&mut h, "magic 2");
    h.wait("空栏位的原因", 5, |a| logged(a, "这个栏位没有精灵"));
    command(&mut h, "magic 1");
    h.wait("不是变幻的精灵的原因（STAGE-230 起用例文字）", 5, |a| logged(a, "这个精灵不能在战斗外使用"));
    command(&mut h, "magic 0 3");
    h.wait("目标没宠物的原因", 5, |a| logged(a, "3 号栏位没有宠物"));
    command(&mut h, "magic 0 1");
    h.wait("加美的原因", 5, |a| logged(a, "不能变身成加美"));
    assert!(metamo_pet(&h.app).is_none(), "被拒的变身不能留下状态");

    // magic 0 0：变成 0 号宠物（凯比），状态里有 metamo，消息栏有服务端的提示。
    command(&mut h, "magic 0 0");
    h.wait("变身状态", 5, |a| metamo_pet(a).as_deref() == Some("凯比"));
    h.wait("变身提示", 5, |a| logged(a, "变身成凯比！"));
}
