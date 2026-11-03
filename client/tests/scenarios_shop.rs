//! STAGE-70 验收（场景）：`talk 198` 弹商店菜单，选「买」看商品列表，选商品后按提示
//! `reply 2` 买入（窗口关闭、背包和金币推送刷新）；模拟服务端按用例 `ui_shop` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Tab, Right};
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
fn shop_menu_and_buy_flow() {
    let mut h = Harness::new("ui_shop", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    // 特产品贩卖员在萨姆吉尔村（1000），先传送过去（等地点表到位、传送完成）。
    h.wait("地点表", 5, |a| !a.world.as_ref().map_or(true, |w| w.places.is_empty()));
    h.text("warp 1");
    h.key(Enter);
    h.wait("到了萨姆吉尔村", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.entry.position.map_name == "萨姆吉尔村")
    });

    // talk 198：商店菜单弹出，标题是店名，选项有「买」「卖」。
    h.text("talk 198");
    h.key(Enter);
    h.wait("商店菜单", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window.as_ref().is_some_and(|d| d.npc == 198 && d.options.len() == 2 && d.options[0].label == "买")
        })
    });

    // 按 1 选「买」：商品列表弹出（数字键在窗口打开、输入行为空时是选选项，不切菜单）。
    h.key(Enter);
    h.wait("商品列表", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window
                .as_ref()
                .is_some_and(|d| d.options.len() >= 2 && d.options[0].label.contains("25 金币"))
        })
    });

    // 选 1 号商品：数量询问，提示 reply 数量。
    h.key(Enter);
    h.wait("数量询问", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window.as_ref().is_some_and(|d| d.options.is_empty() && d.text.contains("reply"))
        })
    });

    // reply 2：买入两个，窗口随 CHOSEN 关闭（金币和背包由推送刷新，剧本里已带）。
    h.text("2");
    h.key(Enter);
    h.wait("报价", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.as_ref().is_some_and(|n| n.trade.is_some())));
    assert!(!h.app.world.as_ref().unwrap().dialog.confirm);
    h.key(Right);
    h.key(Enter);
    h.wait("买入成交", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.as_ref().is_some_and(|n| n.window_id == "shop-5")));
    h.finish();
}
