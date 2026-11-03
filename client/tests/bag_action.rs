//! 背包对象动作流程（STAGE-132，`docs/cli-ui-redesign.md`「对象、动作、参数」）：
//! 动作列表、使用目标、移动目的格、丢弃数量表单的按键与校验。
//! 不连服务端：只走选择与校验，真正发出去的请求见 `scenarios_items.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{App, BAG_MENU, BagStep, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{Inventory, InventorySlot, Pet, Pets, TargetType, WorldEntry};

fn app(inv: Vec<InventorySlot>, pets: Vec<Pet>) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    let mut w = WorldState::new(WorldEntry::default());
    w.menu = BAG_MENU;
    w.nav.push(Layer::Page(BAG_MENU));
    w.inventory = Inventory { gold: 100, slots: inv };
    w.pets = Pets { pets, ..Pets::default() };
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn item(slot: u8, name: &str, kind: u32, count: u32, target: TargetType) -> InventorySlot {
    InventorySlot { slot, name: name.into(), kind, count, target_type: target, ..Default::default() }
}

fn pet(slot: u8, name: &str, hp: u32) -> Pet {
    Pet { slot, name: name.into(), hp, max_hp: 40, ..Pet::default() }
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn logged(a: &App, s: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(s))
}

// ---- 打开与返回 ----

#[test]
fn enter_opens_the_action_list_and_esc_walks_back() {
    let mut a = app(vec![item(5, "小斧头", 1, 1, TargetType::None)], vec![]);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Actions { slot: 5, index: 0 })));
    assert!(world(&a).nav.is(&Layer::Action), "动作层压在背包页上面");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).bag_step.is_none(), "Esc 退回背包页");
    assert!(world(&a).nav.is(&Layer::Page(BAG_MENU)), "导航栈回到背包页");
}

#[test]
fn esc_inside_a_sub_step_returns_to_the_action_list_not_the_bag() {
    let mut a = app(vec![item(5, "石头", 5, 1, TargetType::None)], vec![]);
    key(&mut a, KeyCode::Enter); // 材料的第一项是「移动」
    key(&mut a, KeyCode::Enter); // 进目的格
    assert!(matches!(world(&a).bag_step, Some(BagStep::MoveTo { .. })));
    key(&mut a, KeyCode::Esc);
    assert!(
        matches!(world(&a).bag_step, Some(BagStep::Actions { .. })),
        "先退回动作列表"
    );
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).bag_step.is_none(), "再退才回背包页");
}

// ---- 动作列表 ----

#[test]
fn disabled_action_stays_visible_with_a_reason() {
    // 三把堆在一起的斧头：能看到的「装备」是灰的，按下去解释原因，不提交。
    let mut a = app(vec![item(5, "小斧头", 1, 3, TargetType::None)], vec![]);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "数量不是 1"), "解释为什么不能装备");
    assert!(matches!(world(&a).bag_step, Some(BagStep::Actions { .. })), "留在动作列表");
}

#[test]
fn equipment_slot_offers_unequip_only() {
    let mut a = app(vec![item(2, "铁斧", 1, 1, TargetType::None)], vec![]);
    // 物品栏是空的：焦点自动落在装备栏，选中手上的铁斧。
    key(&mut a, KeyCode::Enter);
    let step = world(&a).bag_step.as_ref().expect("打开动作");
    assert!(matches!(step, BagStep::Actions { slot: 2, .. }));
    assert_eq!(world(&a).bag_step_rows(), 2, "装备格只有「卸下」和「说明」");
}

// ---- 使用目标 ----

#[test]
fn ally_item_asks_for_a_target() {
    let mut a = app(
        vec![item(5, "回复药", 30, 3, TargetType::Ally)],
        vec![pet(0, "乌力", 12)],
    );
    key(&mut a, KeyCode::Enter); // 打开动作，第一项是「使用」
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Target { slot: 5, index: 0 })), "进目标层");
    assert_eq!(world(&a).bag_step_rows(), 2, "自己 + 一只宠物");
}

#[test]
fn ally_down_item_lists_only_down_pets() {
    let mut a = app(
        vec![item(5, "复活草", 30, 1, TargetType::AllyDown)],
        vec![pet(0, "乌力", 20), pet(1, "贝鲁卡", 0)],
    );
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Target { .. })));
    assert_eq!(world(&a).bag_step_rows(), 2, "自己 + 唯一倒下的宠物");
}

// ---- 世界中的使用（STAGE-326：战斗外才能用的忠诚道具）----

fn field_item(slot: u8, name: &str, kind: u32, count: u32, field: TargetType) -> InventorySlot {
    // 战斗中不能用：target_type 是 none，世界中的目标看 field_target_type。
    InventorySlot { field_target_type: Some(field), ..item(slot, name, kind, count, TargetType::None) }
}

#[test]
fn loyalty_item_offers_use_in_the_world() {
    let mut a = app(
        vec![field_item(5, "安抚之素", 16, 1, TargetType::Ally)],
        vec![pet(0, "乌力", 12), pet(1, "贝鲁卡", 20)],
    );
    key(&mut a, KeyCode::Enter); // 动作列表：第一项是「使用」
    assert_eq!(world(&a).bag_step_rows(), 4, "使用、移动、丢弃、说明");
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Target { slot: 5, index: 0 })), "进目标层");
    assert_eq!(world(&a).bag_target_pets(false), vec![0, 1], "两只宠物都能选");
}

#[test]
fn battle_only_item_has_no_use_in_the_world() {
    // field 为 none（只能在战斗中用）：动作列表没有「使用」，只剩移动、丢弃、说明。
    let mut a = app(vec![field_item(5, "复活草", 16, 1, TargetType::None)], vec![pet(0, "乌力", 0)]);
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).bag_step_rows(), 3, "移动、丢弃、说明");
}

#[test]
fn ordinary_meat_still_has_no_use() {
    // 普通的肉：target 和 field 都是 none，动作只有移动、丢弃、说明。
    let mut a = app(vec![item(5, "肉", 5, 1, TargetType::None)], vec![pet(0, "乌力", 12)]);
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).bag_step_rows(), 3, "移动、丢弃、说明");
}

// ---- 移动目的格 ----

#[test]
fn move_step_lists_every_bag_cell_and_starts_off_the_current_one() {
    let mut a = app(vec![item(5, "石头", 5, 1, TargetType::None)], vec![]);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    let Some(BagStep::MoveTo { slot, index }) = world(&a).bag_step.as_ref() else {
        panic!("应当进入移动目的格");
    };
    assert_eq!(*slot, 5);
    assert_eq!(*index, 1, "当前格不可选，初始停在 6 号");
    assert_eq!(world(&a).bag_step_rows(), 15, "5–19 共 15 格");
}

#[test]
fn move_onto_the_current_cell_is_refused() {
    let mut a = app(vec![item(5, "石头", 5, 1, TargetType::None)], vec![]);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Up); // 回到 5 号（当前格）
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "选的是当前这一格"), "解释为什么不选它");
    assert!(matches!(world(&a).bag_step, Some(BagStep::MoveTo { .. })), "留在目的格选择");
}

// ---- 丢弃数量 ----

#[test]
fn drop_count_form_checks_the_range() {
    let mut a = app(vec![item(5, "回复药", 30, 3, TargetType::Ally)], vec![]);
    key(&mut a, KeyCode::Enter); // 动作
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter); // 丢弃
    assert!(matches!(world(&a).bag_step, Some(BagStep::DropCount { slot: 5, .. })));

    key(&mut a, KeyCode::Char('0'));
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "数量要填 1–3 之间的整数"), "0 不合法");
    assert!(matches!(world(&a).bag_step, Some(BagStep::DropCount { .. })), "错误留在表单");

    for c in ['5'] {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "数量要填 1–3 之间的整数"), "超过持有数也不合法");
}

#[test]
fn esc_in_the_count_form_asks_nothing() {
    let mut a = app(vec![item(5, "回复药", 30, 3, TargetType::Ally)], vec![]);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Char('2'));
    key(&mut a, KeyCode::Esc);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Actions { .. })), "退回动作列表");
    assert!(!a.log.all().iter().any(|m| m.text.contains("丢弃后不能恢复")), "取消不发确认");
}
