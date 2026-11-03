//! STAGE-96 验收（场景）：`talk 1735` 对里昂蛙王弹出两行确认（1 是、2 否）；数字键 2 关窗、不开战；再 talk，数字键 1
//! 开战，战斗记录第一行是 NPC 的开战台词。模拟服务端按用例 `npc_enemy_frog` 回放（客户端发出的请求字段要与用例一致）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
use stoneage_client::app::Screen;
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_npc_talk.rs）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

#[test]
fn enemy_npc_asks_declines_then_fights() {
    let mut h = Harness::new("npc_enemy_frog", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();

    // talk 1735：两行确认，有两个选项。
    h.text("talk 1735");
    h.key(Enter);
    h.wait("确认窗口", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window
                .as_ref()
                .is_some_and(|d| d.npc == 1735 && d.name == "里昂蛙王" && d.options.len() == 2)
        })
    });

    // 数字键 2（否）：关窗，不开战。
    h.key(Char('2'));
    h.wait("否：关窗", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.npc_window.is_none())
    });
    assert!(
        h.app.world.as_ref().is_some_and(|w| w.battle.is_none()),
        "选否不应开战"
    );

    // 再 talk，数字键 1（是）：开战，战斗记录第一行是开战台词。
    h.text("talk 1735");
    h.key(Enter);
    h.wait("再次确认", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.npc_window.is_some())
    });
    h.key(Char('1'));
    h.wait("开战", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.battle.is_some())
    });
    h.wait("开战台词写进战斗记录", 5, |a| {
        a.log
            .all()
            .iter()
            .any(|m| m.text == "里昂蛙王：看我怎么修理你们这些人类！")
    });
}
