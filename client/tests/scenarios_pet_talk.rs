//! STAGE-239 验收（场景）：`pet talk 3` 和出战的玛蕾对话，台词进消息栏（村长家 1006 楼层，EVENTRUN31 不改上限）；
//! `pet talk 1` 对休息中的宠物被服务端拒绝（WRONG_STATE），错误说明进消息栏。模拟服务端按用例 `ui_pet_talk` 回放。
//! 另一个测试用 `--unicode false` 与 80×24 画面检查同一流程的降级显示。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Down, Enter, Esc, Tab, Up};
use ratatui::{Terminal, backend::TestBackend};
use stoneage_client::app::{App, Options, PETS_MENU, PetStep, Screen};
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 登录、进入世界，然后出战 3 号栏的玛蕾并对话两次。
fn run_talk_flow(h: &mut Harness) {
    login_to_characters(h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);

    h.text("pet battle 3");
    h.key(Enter);
    h.wait("出战", 5, |a| a.world.as_ref().is_some_and(|w| w.pets.pets.iter().any(|p| p.slot == 3 && p.state == "battle")));

    h.text("pet talk 3");
    h.key(Enter);
    h.wait("台词进消息栏", 5, |a| a.log.all().iter().any(|m| m.text.contains("你们人类的世界真是好奇怪啊")));

    h.text("pet talk 1");
    h.key(Enter);
    h.wait("休息中被拒", 5, |a| a.log.all().iter().any(|m| m.text.contains("不在身边，不能对话")));
}

/// 场景 Esc 打开游戏菜单，下一下到「宠物」，Enter 进页。
fn open_pets(h: &mut Harness) {
    h.key(Esc);
    h.key(Down);
    h.key(Enter);
    assert_eq!(h.app.world.as_ref().map(|w| w.menu), Some(PETS_MENU));
}

fn selected_slot(a: &App) -> Option<i64> {
    a.world.as_ref().and_then(|w| w.selected_pet_slot())
}

/// 在宠物页选中 slot 栏的宠物，Enter 打开动作菜单，移到「对话」（第 6 项）。
fn open_talk_action(h: &mut Harness, slot: i64) {
    while selected_slot(&h.app).is_some_and(|s| s < slot) {
        h.key(Down);
    }
    while selected_slot(&h.app).is_some_and(|s| s > slot) {
        h.key(Up);
    }
    assert_eq!(selected_slot(&h.app), Some(slot));
    h.key(Enter);
    assert!(matches!(h.app.world.as_ref().and_then(|w| w.pet_step.as_ref()), Some(PetStep::Actions { .. })));
    h.repeat(Down, 6); // 骑乘之后：对话是第 7 项
    assert!(matches!(h.app.world.as_ref().and_then(|w| w.pet_step.as_ref()), Some(PetStep::Actions { index: 6, .. })));
}

/// 按终端尺寸画一帧（中文每格占两列，比较前去掉空格）。
fn draw(a: &mut App, w: u16, h: u16) -> String {
    let mut t = Terminal::new(TestBackend::new(w, h)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, a)).unwrap();
    t.backend().buffer().content.iter().map(|c| c.symbol()).collect::<String>().replace(' ', "")
}

#[test]
fn talk_from_the_command_line() {
    let mut h = Harness::new("ui_pet_talk", opts);
    run_talk_flow(&mut h);
    h.finish();
}

#[test]
fn talk_screen_at_80x24_without_unicode() {
    let mut h = Harness::new("ui_pet_talk", |addr| Options { server: addr, unicode: false, ..Options::default() });
    run_talk_flow(&mut h);
    let screen = draw(&mut h.app, 80, 24);
    assert!(screen.contains("不在身边，不能对话"), "{screen}");
    h.finish();
}

/// 宠物页菜单走按键：3 号栏玛蕾出战后选「对话」，台词进消息栏；1 号栏休息中选「对话」被拒；
/// 80×24 下动作菜单和宠物页没被挤坏。
#[test]
fn talk_from_the_pet_menu_keys() {
    let mut h = Harness::new("ui_pet_talk", |addr| Options { server: addr, unicode: false, ..Options::default() });
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.text("pet battle 3");
    h.key(Enter);
    h.wait("出战", 5, |a| a.world.as_ref().is_some_and(|w| w.pets.pets.iter().any(|p| p.slot == 3 && p.state == "battle")));

    open_pets(&mut h);
    open_talk_action(&mut h, 3);
    let screen = draw(&mut h.app, 80, 24);
    for want in ["动作", "对话"] {
        assert!(screen.contains(want), "动作菜单缺少「{want}」\n{screen}");
    }
    h.key(Enter);
    h.wait("台词进消息栏", 5, |a| a.log.all().iter().any(|m| m.text.contains("你们人类的世界真是好奇怪啊")));
    assert!(h.app.world.as_ref().is_some_and(|w| w.pet_step.is_none()), "对话后回宠物页");
    let screen = draw(&mut h.app, 80, 24);
    assert!(screen.contains("玛蕾") && screen.contains("好奇怪"), "宠物页仍显示宠物，消息栏有台词\n{screen}");

    open_talk_action(&mut h, 1);
    h.key(Enter);
    // 菜单里休息中的宠物在本地就被拦下（不发请求），原因进消息栏，菜单保持打开。
    assert!(h.app.log.all().iter().any(|m| m.text.contains("休息中的宠物不在身边")));
    assert!(matches!(h.app.world.as_ref().and_then(|w| w.pet_step.as_ref()), Some(PetStep::Actions { .. })));
    h.key(Esc);
    // 回到命令路径：服务端对休息中的 1 号同样回 WRONG_STATE。
    h.text("pet talk 1");
    h.key(Enter);
    h.wait("休息中被服务端拒绝", 5, |a| a.log.all().iter().any(|m| m.text.contains("不在身边，不能对话")));
    h.finish();
}
