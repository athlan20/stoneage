//! STAGE-143 验收（场景）：`talk 1763` 弹宠物技能店的技能列表（第一项是待机），数字键选攻击（第 2 项），
//! `choose 1` 选凯比，数字键选 3 号技能栏位，确认页按 1 学习；窗口回到技能列表，宠物页和金币随推送刷新。
//! 模拟服务端按用例 `ui_petskill_shop` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
use stoneage_client::app::Screen;
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_shop.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

#[test]
fn petskill_shop_learn_flow() {
    let mut h = Harness::new("ui_petskill_shop", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);

    // talk 1763：技能列表弹出，9 个技能，第一项是待机（500 金币），第二项是攻击（1000 金币）。
    h.text("talk 1763");
    h.key(Enter);
    h.wait("技能列表", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window.as_ref().is_some_and(|d| {
                d.npc == 1763
                    && d.options.len() == 9
                    && d.options[0].label.contains("500 金币")
                    && d.options[1].label.contains("1000 金币")
            })
        })
    });

    // 数字键 2：选攻击，弹出随身宠物列表（编号 1–5，同宠物页）。
    h.key(Char('2'));
    h.wait("宠物列表", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window
                .as_ref()
                .is_some_and(|d| d.options.len() == 1 && d.text.contains("教哪一只"))
        })
    });

    // choose 1：凯比（1 号），弹出技能栏位列表。
    h.text("choose 1");
    h.key(Enter);
    h.wait("技能栏位列表", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window
                .as_ref()
                .is_some_and(|d| d.options.len() == 6 && d.text.contains("技能栏位"))
        })
    });

    // 数字键 3：3 号栏位，弹出确认页（只有一个「学习」选项）。
    h.key(Char('3'));
    h.wait("确认页", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window
                .as_ref()
                .is_some_and(|d| d.options.len() == 1 && d.options[0].label == "学习")
        })
    });

    // 数字键 1：学习。宠物页和金币推送刷新，窗口回到技能列表。
    h.key(Char('1'));
    h.wait("回到技能列表", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.npc_window.as_ref().is_some_and(|d| d.options.len() == 9))
    });
    h.finish();
}
