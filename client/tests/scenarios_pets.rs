//! STAGE-31 验收（场景）：宠物页跟随推送，离开再进入宠物还在。模拟服务端按用例 `world_pets_give` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Screen};
use stoneage_client::ui;
use support::{Harness, opts};

/// 整屏文字（120×40），宽字符按两列取。
fn draw(app: &mut App) -> String {
    let mut term = Terminal::new(TestBackend::new(120, 40)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..40u16)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < 120 {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += unicode_width::UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 登录并等到角色选择页（同 scenarios.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 进入世界后取宠物；服务端推送 `pets_changed` 后宠物页不用刷新就出现乌力；
/// 离开再进入世界宠物还在（用例 `world_pets_give`，give 由模拟服务端按剧本回放）。
#[test]
fn pet_page_follows_pets_changes() {
    let mut h = Harness::new("world_pets_give", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    // 用例先回空宠物、紧接着推 pets_changed：等最终状态。
    h.wait("宠物与发宠物推送", 5, |a| a.world.as_ref().is_some_and(|w| w.pets.pets.len() == 1));
    let w = h.app.world.as_ref().unwrap();
    let p = &w.pets.pets[0];
    assert_eq!((p.slot, p.species.as_str(), p.level, p.state.as_str()), (0, "乌力", 5, "rest"), "乌力应放进第一个空栏位");
    assert_eq!(p.next_exp, 67, "下一级需要的累计经验");
    h.key(Char('3'));
    h.wait("宠物页显示乌力", 5, |a| a.world.as_ref().is_some_and(|w| w.shown_pet().is_some()));
    // STAGE-217：详情按 growth、quality 渲染。给的时候是 5 级、还没升过级：表头写起始等级，成长率是 —，素质档位照发。
    let screen = draw(&mut h.app);
    assert!(screen.contains("基础属性（成长/级，自 Lv5 起）"), "捕获时已是 5 级，表头写起始等级\n{screen}");
    assert!(screen.contains("HP 37/37 (—)"), "还没升过级，成长率是 —\n{screen}");
    assert!(screen.contains("-3") && screen.contains("满"), "素质档位：-3（场景是 ASCII 模式）和满\n{screen}");
    // 离开再进入：宠物还在（重新取一次 pets）。
    h.text("leave");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("重进后宠物仍在", 5, |a| a.world.as_ref().is_some_and(|w| w.pets.pets.len() == 1));
    h.finish();
}
