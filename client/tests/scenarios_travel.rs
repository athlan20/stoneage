//! STAGE-161 验收（场景）：进世界后服务端在 `maps` 里返回节点树，快捷传送页直接用它逐级浏览。
//! 模拟服务端按用例 `ui_travel` 回放；矩阵里的请求（maps / inventory / pets / quests / npcs / exits）
//! 由模拟服务端自动应答，所以本用例只走「进世界 → 打开快捷传送 → 下钻一层 → 离开世界」。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Down, Enter, Esc, Tab};
use stoneage_client::app::{Layer, MAP_MENU, Screen};
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 场景 → 游戏菜单 → 快捷传送（菜单第 5 项）。
fn open_travel(h: &mut Harness) {
    h.key(Esc);
    h.repeat(Down, 4);
    h.key(Enter);
}

#[test]
fn travel_page_reads_the_catalog_from_the_server() {
    let mut h = Harness::new("ui_travel", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);

    // 目录进世界就取了，页面打开时应该已经是 Ready，而不是先转一圈再取。
    h.wait("拿到快捷传送目录", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.travel.ready())
    });
    let version = h.app.world.as_ref().unwrap().travel.version.clone();
    assert_eq!(version, "mock-1", "目录版本号来自服务端");

    open_travel(&mut h);
    h.wait("打开快捷传送页", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.menu == MAP_MENU && w.nav.is(&Layer::Page(MAP_MENU)))
    });

    let names: Vec<String> = h
        .app
        .world
        .as_ref()
        .unwrap()
        .travel
        .view()
        .into_iter()
        .map(|r| r.name)
        .collect();
    assert_eq!(
        names,
        vec!["萨伊那斯", "沙姆岛", "1级宠物", "练级传送"],
        "根层列出来自服务端的岛，末尾是「1级宠物」（STAGE-251）和「练级传送」（STAGE-279）"
    );

    // 下钻一层：萨伊那斯这一层的首项是「传送到本图」，后面是萨姆吉尔村和洞窟分组。
    h.key(Enter);
    let path = h.app.world.as_ref().unwrap().travel.path();
    assert_eq!(path, vec!["萨伊那斯".to_string()]);
    let rows = h.app.world.as_ref().unwrap().travel.view();
    assert!(rows[0].here, "地图节点自己有下级时，首项是「传送到本图」");
    assert_eq!(rows[0].floor, Some(100));

    // Esc 逐级返回：回到根层，再退一层回游戏菜单，再退才回场景。
    h.key(Esc);
    assert!(h.app.world.as_ref().unwrap().travel.path().is_empty());
    h.key(Esc);
    assert!(h.app.world.as_ref().unwrap().nav.is(&Layer::Menu), "从菜单进来的，先回菜单");
    h.key(Esc);
    assert!(h.app.world.as_ref().unwrap().nav.is(&Layer::Scene), "最后回场景");

    h.command("leave");
    h.finish();
}

#[test]
fn travel_page_is_the_same_page_from_the_scene_entry() {
    let mut h = Harness::new("ui_travel", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.wait("拿到快捷传送目录", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.travel.ready())
    });

    // 场景第一项固定入口就是「快捷传送」，Enter 打开的是同一个页面。
    h.key(Enter);
    h.wait("从场景进入快捷传送", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.menu == MAP_MENU && w.nav.is(&Layer::Page(MAP_MENU)))
    });
    h.key(Esc);
    assert!(h.app.world.as_ref().unwrap().nav.is(&Layer::Scene));

    h.command("leave");
    h.finish();
}
