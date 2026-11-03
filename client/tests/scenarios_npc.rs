//! STAGE-68 验收（场景）：进入世界取本地图 NPC（村长家 2 个：厨师和 STAGE-90 收录的任务 NPC 村长），传送到萨姆吉尔村后
//! 列表跟着换成村里的 43 个（STAGE-69 收录报时 NPC、STAGE-90 收录任务 NPC、STAGE-266 补主城精灵王、STAGE-308 补强者表后）；模拟服务端按用例 `ui_npc_follow` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
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
fn npcs_follow_the_current_map() {
    let mut h = Harness::new("ui_npc_follow", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    // 村长家（1006）有村长家的厨师（对话）和萨姆吉尔的村长（任务，STAGE-90），进世界后自动取到。
    h.wait("村长家的厨师", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npcs.len() == 2
                && w.npcs.iter().any(|n| n.name == "村长家的厨师" && n.type_ == "对话")
                && w.npcs.iter().any(|n| n.name == "萨姆吉尔的村长" && n.type_ == "任务")
        })
    });
    // 传送到萨姆吉尔村：换了地图就重新取，列表换成村里的 43 个（含商店、存档点、报时、任务、转生、强者表）。
    h.text("warp 1");
    h.key(Enter);
    h.wait("村里的 NPC 列表", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npcs.len() == 43
                && w.npcs.iter().any(|n| n.name == "特产品贩卖员" && n.type_ == "商店")
                && w.npcs.iter().any(|n| n.type_ == "报时")
        })
    });
    h.finish();
}
