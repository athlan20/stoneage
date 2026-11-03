//! STAGE-123 验收（场景）：`pet ride 1` 骑上配套虎（服务端推 pets_changed，宠物页状态变成骑乘），
//! `pet ride 2` 被拒（已经在骑 1 号栏的宠物），`pet ride off` 骑下来；模拟服务端按用例 `ui_pet_ride` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Tab};
use stoneage_client::app::Screen;
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

#[test]
fn ride_and_unride_from_the_command_line() {
    let mut h = Harness::new("ui_pet_ride", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);

    h.text("pet ride 1");
    h.key(Enter);
    h.wait("骑上", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.pets.pets.iter().any(|p| p.slot == 1 && p.state == "ride"))
    });
    h.wait("骑上消息", 5, |a| a.log.all().iter().any(|m| m.text.contains("1 号宠物已骑上")));

    h.text("pet ride 2");
    h.key(Enter);
    h.wait("已在骑", 5, |a| a.log.all().iter().any(|m| m.text.contains("已经在骑 1 号栏的宠物")));

    h.text("pet ride off");
    h.key(Enter);
    h.wait("下骑", 5, |a| a.log.all().iter().any(|m| m.text.contains("宠物已骑下来")));
    h.wait("休息", 5, |a| a.world.as_ref().is_some_and(|w| w.pets.pets.iter().any(|p| p.slot == 1 && p.state == "rest")));
    h.finish();
}
