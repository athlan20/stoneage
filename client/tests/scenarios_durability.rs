//! STAGE-225 验收（场景）：服务端下发的耐久字段原样进背包，有耐久的小斧头带 durability、max_durability，
//! 没有耐久的小块肉两项都是 0（界面据此不显示耐久行）。模拟服务端按用例 `world_item_durability` 回放（严格模式）。
//! 界面的文字和颜色在 `render_durability.rs`。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Tab};
use stoneage_client::app::{App, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_items.rs）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 背包第 `slot` 格的耐久字段：(当前, 最大, 受损等级)。
fn durability_of(a: &App, slot: u8) -> Option<(u32, u32, u8)> {
    let s = a.world.as_ref()?.inventory.slots.iter().find(|s| s.slot == slot)?;
    Some((s.durability, s.max_durability, s.crush_level))
}

#[test]
fn durability_fields_come_from_the_server() {
    let mut h = Harness::new("world_item_durability", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("背包里的小斧头带耐久", 5, |a| durability_of(a, 5) == Some((200000, 200000, 0)));
    assert_eq!(durability_of(&h.app, 6), Some((0, 0, 0)), "小块肉没有耐久，两项都是 0");
    h.finish();
}
