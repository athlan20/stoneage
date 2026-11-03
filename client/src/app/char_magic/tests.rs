//! 角色页精灵「使用」动作的单元测试（STAGE-231）：分类、MP 挡一下、Esc 逐层退、变幻的精灵填命令。
use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};

use crate::app::world::CHAR_MENU;
use crate::app::{App, CharStep, Layer, Options, Screen, WorldState};
use crate::proto::{MagicView, Status, TargetType, WorldEntry};

use super::super::magic_use::{MagicUse, magic_use_kind};

fn spirit(slot: u8, id: u32, name: &str, mp: u32) -> MagicView {
    MagicView {
        slot,
        id,
        name: name.into(),
        mp,
        target_type: TargetType::Ally,
        ..Default::default()
    }
}

/// 角色页上有这些精灵、这点 MP 的 App（没有宠物，没有连接）。
fn app_with(magic: Vec<MagicView>, mp: u32) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    let mut entry = WorldEntry::default();
    entry.status = Status {
        mp,
        magic,
        ..Status::default()
    };
    let mut w = WorldState::new(entry);
    w.menu = CHAR_MENU;
    w.nav.push(Layer::Page(CHAR_MENU));
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn step(a: &App) -> Option<&CharStep> {
    a.world.as_ref().and_then(|w| w.char_step.as_ref())
}

#[test]
fn kinds_follow_the_protocol_ids() {
    assert_eq!(
        magic_use_kind(&spirit(0, 0, "治愈的精灵 Lv1", 6)),
        MagicUse::Direct,
        "治愈只回自己，直接发"
    );
    assert_eq!(
        magic_use_kind(&spirit(1, 10, "滋润的精灵 Lv1", 6)),
        MagicUse::Pick,
        "滋润要选自己或宠物"
    );
    assert_eq!(magic_use_kind(&spirit(2, 15, "滋润的精灵 Lv6", 6)), MagicUse::Pick);
    assert_eq!(
        magic_use_kind(&spirit(3, 20, "恩惠的精灵 Lv1", 6)),
        MagicUse::Direct,
        "恩惠只回自己"
    );
    assert_eq!(magic_use_kind(&spirit(4, 350, "变幻的精灵 Lv1", 8)), MagicUse::Metamo);
    assert_eq!(magic_use_kind(&spirit(4, 410, "变幻的精灵 Lv2", 8)), MagicUse::Metamo);
    assert_eq!(
        magic_use_kind(&spirit(0, 301, "岩石撞击", 10)),
        MagicUse::Direct,
        "攻击精灵交给服务端回 MAGIC_UNUSABLE"
    );
}

#[test]
fn enter_on_a_spirit_opens_the_action_list() {
    let mut a = app_with(vec![spirit(1, 10, "滋润的精灵 Lv1", 6)], 100);
    // 焦点落在精灵栏第一个能选的栏位上（这里是 1 号）。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(step(&a), Some(CharStep::MagicAction { slot: 1 })),
        "Enter 打开 1 号精灵的动作列表"
    );
    assert!(
        a.world.as_ref().unwrap().nav.is(&Layer::Action),
        "动作流程压一层 Action"
    );
}

#[test]
fn pick_spirit_moves_to_the_target_list_and_esc_goes_back() {
    let mut a = app_with(vec![spirit(1, 10, "滋润的精灵 Lv1", 6)], 100);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(step(&a), Some(CharStep::MagicTarget { slot: 1, index: 0 })),
        "滋润进入对象列表，默认选自己"
    );
    key(&mut a, KeyCode::Esc);
    assert!(
        matches!(step(&a), Some(CharStep::MagicAction { slot: 1 })),
        "Esc 回动作列表"
    );
    key(&mut a, KeyCode::Esc);
    assert!(step(&a).is_none(), "再 Esc 退掉整个流程");
    assert!(
        !a.world.as_ref().unwrap().nav.is(&Layer::Action),
        "导航栈上的 Action 也弹掉"
    );
}

#[test]
fn not_enough_mp_explains_and_stays_put() {
    let mut a = app_with(vec![spirit(1, 10, "滋润的精灵 Lv1", 6)], 5);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(step(&a), Some(CharStep::MagicAction { slot: 1 })),
        "MP 不够不进对象列表"
    );
    assert!(
        a.log
            .all()
            .iter()
            .any(|m| m.text.contains("MP 不够") && m.text.contains("需要 MP 6")),
        "消息栏写明需要多少 MP"
    );
}

#[test]
fn metamo_spirit_fills_the_command_line_instead() {
    let mut a = app_with(vec![spirit(0, 350, "变幻的精灵 Lv1", 8)], 100);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(step(&a).is_none(), "变幻的精灵不开对象列表，流程结束");
    let input = a.world.as_ref().unwrap().input.text();
    assert_eq!(
        input, "magic 0 ",
        "变幻保持命令写法：填进 magic <栏位> ，宠物编号由玩家接着敲"
    );
}

#[test]
fn target_list_has_self_then_pets_only() {
    let mut a = app_with(vec![spirit(1, 10, "滋润的精灵 Lv1", 6)], 100);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    let rows = a.world.as_ref().unwrap().magic_step_rows();
    assert_eq!(rows, 1, "没有宠物时对象列表只有自己，不列队友");
    key(&mut a, KeyCode::Down);
    assert!(
        matches!(step(&a), Some(CharStep::MagicTarget { index: 0, .. })),
        "只有一项时 ↓ 停住"
    );
}
