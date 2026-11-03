//! STAGE-24 验收（场景）：加点命令的反馈、管理命令发的点数不用刷新就显示、点数不够时报错且数值不变、
//! 离开再进入加点结果还在。模拟服务端按用例 `ui_addpoint` 回放（严格模式：客户端发的每一条请求都要和剧本一致）。

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

/// 当前未分配的点数（进入世界之后才有）。
fn points(a: &App) -> Option<u32> {
    a.world.as_ref().map(|w| w.entry.status.skill_points)
}

#[test]
fn addpoint_follows_status_changes() {
    let mut h = Harness::new("ui_addpoint", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();

    // 管理命令发的 3 点：不用刷新就显示，消息栏提示怎么加。
    h.wait("在线推送：未分配点数 3", 5, |a| points(a) == Some(3));
    h.wait("消息栏提示获得点数", 5, |a| logged(a, "获得 3 点未分配点数"));

    // 加 2 点到体力：数值与剩余点数立刻更新，消息栏写明结果。
    command(&mut h, "addpoint 体力 2");
    h.wait("体力加到 10", 5, |a| a.world.as_ref().is_some_and(|w| w.entry.status.stats.vital == 10));
    h.wait("加点结果写进消息栏", 5, |a| logged(a, "已加点：体力 +2，未分配点数还剩 1"));
    assert_eq!(points(&h.app), Some(1));

    // 点数不够：消息栏报错，数值不变。
    command(&mut h, "addpoint 腕力 5");
    h.wait("点数不够的提示", 5, |a| logged(a, "未分配的点数不够，现在有 1 点"));
    let w = h.app.world.as_ref().unwrap();
    assert_eq!(w.entry.status.stats.strength, 4, "失败的加点不改变腕力");

    // 省略点数的“速度”：加 1 点，剩 0 点。
    command(&mut h, "addpoint 速度");
    h.wait("速度加到 5", 5, |a| a.world.as_ref().is_some_and(|w| w.entry.status.stats.dexterity == 5));
    assert_eq!(points(&h.app), Some(0));

    // 没有点数了再加：服务端回复点数不够，数值不变。
    command(&mut h, "addpoint 体力 1");
    h.wait("没有点数时的提示", 5, |a| logged(a, "未分配的点数不够，现在有 0 点"));

    // 能力名不对：客户端在本地拦下，不发请求（剧本里没有这一步，发出去会失败）。
    command(&mut h, "addpoint 力量 1");
    h.wait("能力名提示", 5, |a| logged(a, "没有“力量”这项能力"));

    // 离开再进入：离线时管理命令发的 1 点在进入世界后可见，加点结果还在。
    command(&mut h, "leave");
    h.wait_screen(Screen::Characters);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("离线发的点数进入世界后可见", 5, |a| points(a) == Some(1));
    let w = h.app.world.as_ref().unwrap();
    assert_eq!((w.entry.status.stats.vital, w.entry.status.stats.dexterity), (10, 5), "加点结果离开再进入还在");
    h.finish();
}
