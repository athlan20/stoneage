//! STAGE-71 验收（场景）：`talk 16` 弹护士确认窗口（宠物受伤时有「治疗」选项），
//! 选治疗宠物回满、窗口关闭；模拟服务端按用例 `ui_healer` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
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
fn healer_heals_and_closes() {
    let mut h = Harness::new("ui_healer", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    // 森林的护士在萨伊那斯（地图 100），先传送过去（等地点表到位）。
    h.wait("地点表", 5, |a| !a.world.as_ref().map_or(true, |w| w.places.is_empty()));
    h.text("warp 5");
    h.key(Enter);
    h.run_for(400);

    // talk 16：确认窗口弹出（宠物被剧本弄伤，有「治疗」选项）。
    h.text("talk 16");
    h.key(Enter);
    h.wait("护士窗口", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window.as_ref().is_some_and(|d| d.npc == 16 && d.options.iter().any(|o| o.label == "治疗"))
        })
    });

    // 按 1 选治疗：窗口随 CHOSEN 关闭（宠物回满由剧本推送带回）。
    h.key(Char('1'));
    h.wait("治疗完成", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_none()));
    h.finish();
}
