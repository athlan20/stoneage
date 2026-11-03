//! 宠物页、背包页的方向键逛页面（STAGE-114，`docs/ui.md`「页面选择」3 宠物、4 背包）：
//! `↑↓` 移动选中项（空栏位、空格子跳过），`Tab` 换栏，`Enter` 只把常用命令填进输入行、不执行；
//! 选中项同时决定详情（宠物）和底边说明（背包），`pet status N` 与方向键共用同一个选中项。
//! 不连服务端：只看本地状态和输入行的文字。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, BagStep, Options, PetStep, Screen, WorldState};
use stoneage_client::proto::{Inventory, InventorySlot, Pet, PetGrowth, PetRates, Pets, TargetType, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    })
}

fn draw(a: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..h)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < w {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 输入行：提示和输入文字就在这一行（和 keys_page.rs 一样取倒数第二行）。
fn input_row(a: &mut App) -> String {
    draw(a, 120, 40).lines().rev().nth(1).unwrap_or_default().to_string()
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn text(a: &mut App, s: &str) {
    if a.screen==Screen::World && a.world.as_ref().is_some_and(|w|!w.text_entry_active()){key(a,KeyCode::Char('/'));}
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().unwrap()
}

/// 进入世界，给出宠物或背包，再切到对应的菜单页。
fn page(menu: char, pets: Option<Pets>, inv: Option<Inventory>) -> App {
    let mut a = app();
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    if let Some(pets) = pets {
        w.pets = pets;
    }
    if let Some(inv) = inv {
        w.inventory = inv;
    }
    a.world = Some(w);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char(menu));
    a
}

fn pet(slot: u8, state: &str, hp: u32) -> Pet {
    Pet {
        slot,
        name: format!("宠{slot}"),
        state: state.into(),
        hp,
        max_hp: 10,
        ..Pet::default()
    }
}

fn pets(list: Vec<Pet>) -> Pets {
    Pets {
        max_slots: 5,
        pets: list,
    }
}

fn slot(n: u8, name: &str, kind: u32, count: u32, target: TargetType) -> InventorySlot {
    InventorySlot {
        slot: n,
        name: name.into(),
        kind,
        count,
        target_type: target,
        effect: format!("{name}的说明"),
        ..Default::default()
    }
}

fn inventory(list: Vec<InventorySlot>) -> Inventory {
    Inventory { gold: 100, slots: list }
}

// ---- 宠物页 ----

#[test]
fn pets_arrows_skip_empty_slots_and_detail_follows() {
    let mut a = page(
        '3',
        Some(pets(vec![
            pet(0, "rest", 10),
            pet(2, "battle", 10),
            pet(4, "standby", 10),
        ])),
        None,
    );
    assert!(draw(&mut a, 120, 40).contains("宠2 · 2 号 · 出战"), "默认是出战的那只");
    key(&mut a, KeyCode::Down);
    assert!(draw(&mut a, 120, 40).contains("宠4 · 4 号 · 待命"), "空栏位 3 跳过");
    key(&mut a, KeyCode::Up);
    key(&mut a, KeyCode::Up);
    assert!(draw(&mut a, 120, 40).contains("宠0 · 0 号 · 休息"), "往上到 0 号");
    key(&mut a, KeyCode::Up);
    assert!(draw(&mut a, 120, 40).contains("宠0 · 0 号 · 休息"), "到头停住");
}

/// STAGE-217：`↑↓` 换到另一只宠物，右侧详情的表头和成长率跟着换（同一个选中项）。
#[test]
fn pets_arrows_switch_the_growth_shown() {
    let mut raised = pet(0, "rest", 35);
    raised.growth = Some(PetGrowth {
        basis: "raised".into(),
        from_level: Some(1),
        rates: PetRates { hp: Some(7.25), ..PetRates::default() },
    });
    let mut caught = pet(2, "battle", 10);
    caught.growth = Some(PetGrowth {
        basis: "caught".into(),
        from_level: Some(5),
        rates: PetRates { hp: Some(3.5), ..PetRates::default() },
    });
    let mut a = page('3', Some(pets(vec![raised, caught])), None);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("自 Lv5 起") && s.contains("(3.50)"), "默认是出战的 2 号，捕获后升级\n{s}");
    key(&mut a, KeyCode::Up);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("基础属性（成长/级）") && s.contains("(7.25)"), "切到 0 号，1 级起养\n{s}");
    assert!(!s.contains("(3.50)"), "切走后不再显示 2 号的成长率\n{s}");
}

#[test]
fn pets_enter_opens_the_action_menu() {
    let mut a = page('3', Some(pets(vec![pet(0, "rest", 10), pet(2, "battle", 10)])), None);
    // 默认选中出战的那只（2 号）。STAGE-133：Enter 打开动作菜单，不填命令。
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { slot: 2, .. })), "打开 2 号的动作菜单");
    assert!(
        world(&a).pets.pets.iter().any(|p| p.slot == 2 && p.is_battle()),
        "只开菜单，没有执行"
    );
    assert!(draw(&mut a, 120, 40).contains("动作 · 宠2"), "面板标题写宠物名");
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Up);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { slot: 0, .. })), "换到 0 号");
}

#[test]
fn pets_fainted_opens_the_menu_with_a_reason() {
    let mut a = page('3', Some(pets(vec![pet(0, "battle", 0)])), None);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { slot: 0, .. })));
    key(&mut a, KeyCode::Enter); // 第一项「出战」
    assert!(
        a.log.all().iter().any(|m| m.text.contains("倒下的宠物不能出战")),
        "禁用项要解释原因"
    );
}

#[test]
fn pets_hint_names_the_enter_action() {
    let mut a = page('3', Some(pets(vec![pet(0, "battle", 10)])), None);
    assert!(
        input_row(&mut a).contains("↑↓ 选择 · Enter 动作"),
        "{}",
        input_row(&mut a)
    );
}

#[test]
fn pet_status_moves_the_arrow_selection_too() {
    let mut a = page('3', Some(pets(vec![pet(0, "rest", 10), pet(2, "battle", 10)])), None);
    text(&mut a, "pet status 0");
    key(&mut a, KeyCode::Enter);
    assert!(draw(&mut a, 120, 40).contains("宠0 · 0 号 · 休息"));
    key(&mut a, KeyCode::Down);
    assert!(
        draw(&mut a, 120, 40).contains("宠2 · 2 号 · 出战"),
        "方向键接着从 0 号往下走"
    );
}

#[test]
fn pets_selection_follows_the_row_marker() {
    let mut a = page('3', Some(pets(vec![pet(0, "rest", 10), pet(2, "battle", 10)])), None);
    key(&mut a, KeyCode::Up);
    let rows = draw(&mut a, 120, 40);
    let marked: Vec<&str> = rows.lines().filter(|l| l.contains('▶')).collect();
    assert_eq!(marked.len(), 1, "只有一行有 ▶");
    assert!(marked[0].contains("宠0"), "{}", marked[0]);
}

// ---- 背包页 ----

#[test]
fn bag_default_is_first_item_and_enter_opens_actions() {
    let inv = inventory(vec![
        slot(5, "小斧头", 1, 1, TargetType::None),
        slot(6, "回复药", 30, 3, TargetType::Ally),
        slot(7, "石头", 30, 2, TargetType::None),
    ]);
    let mut a = page('4', None, Some(inv));
    assert!(
        draw(&mut a, 120, 40).contains("小斧头的说明"),
        "默认选物品栏第一件，说明写在底边"
    );
    // STAGE-132：Enter 打开对象动作菜单，不是填命令。
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Actions { slot: 5, .. })), "能穿的打开动作菜单");
    let screen = draw(&mut a, 120, 40);
    assert!(screen.contains("动作 · 小斧头") && screen.contains("装备"), "{screen}");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).bag_step.is_none(), "Esc 退回背包页");
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Actions { slot: 6, .. })));
    assert!(draw(&mut a, 120, 40).contains("使用"), "能用的有使用动作");
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    let screen = draw(&mut a, 120, 40);
    assert!(screen.contains("移动") && screen.contains("丢弃"), "{screen}");
    assert!(!screen.contains("使用"), "材料没有使用动作：{screen}");
}

#[test]
fn bag_arrows_skip_empty_cells() {
    let inv = inventory(vec![
        slot(5, "小斧头", 1, 1, TargetType::None),
        slot(9, "回复药", 30, 1, TargetType::Ally),
    ]);
    let mut a = page('4', None, Some(inv));
    key(&mut a, KeyCode::Down);
    assert!(draw(&mut a, 120, 40).contains("回复药的说明"), "6–8 空格跳过");
    key(&mut a, KeyCode::Down);
    assert!(draw(&mut a, 120, 40).contains("回复药的说明"), "到头停住");
}

#[test]
fn bag_tab_moves_to_equipment_and_enter_opens_unequip() {
    let inv = inventory(vec![
        slot(0, "铁盔", 6, 1, TargetType::None),
        slot(5, "小斧头", 1, 1, TargetType::None),
    ]);
    let mut a = page('4', None, Some(inv));
    assert!(input_row(&mut a).contains("Tab 装备"), "{}", input_row(&mut a));
    key(&mut a, KeyCode::Tab);
    assert!(draw(&mut a, 120, 40).contains("铁盔的说明"), "Tab 到装备栏，选中 0 号");
    // STAGE-132：装备格的动作是「卸下」和「说明」。
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).bag_step, Some(BagStep::Actions { slot: 0, .. })));
    let screen = draw(&mut a, 120, 40);
    assert!(screen.contains("卸下"), "{screen}");
    key(&mut a, KeyCode::Esc);
    assert!(input_row(&mut a).contains("Tab 物品"), "{}", input_row(&mut a));
}

#[test]
fn bag_pgdn_jumps_the_selection_and_the_table_follows() {
    // 窗口是最小的 80×24 级别，物品表只看得见一部分；PgDn 跳到的格子要滚进可见范围。
    let inv = inventory(vec![
        slot(5, "小斧头", 1, 1, TargetType::None),
        slot(19, "尾巴", 30, 1, TargetType::Ally),
    ]);
    let mut a = page('4', None, Some(inv));
    key(&mut a, KeyCode::PageDown);
    let rows = draw(&mut a, 120, 24);
    assert!(rows.contains("尾巴"), "选中的格子滚进了可见范围");
    assert!(rows.contains("尾巴的说明"), "底边写完整说明");
}

#[test]
fn bag_hint_has_no_page_keys_when_the_list_is_selectable() {
    let inv = inventory(vec![slot(5, "小斧头", 1, 1, TargetType::None)]);
    let mut a = page('4', None, Some(inv));
    let hint = input_row(&mut a);
    assert!(hint.contains("↑↓ 选择 · Enter 动作"), "{hint}");
    assert!(!hint.contains("PgUp/PgDn"), "PgUp/PgDn 移动选中项，不再写翻页：{hint}");
}

#[test]
fn bag_without_items_keeps_the_old_hint() {
    let mut a = page('4', None, Some(inventory(vec![])));
    let hint = input_row(&mut a);
    assert!(hint.contains("↑↓ 历史"), "{hint}");
}

#[test]
fn bag_tab_onto_an_empty_equipment_column_moves_the_focus() {
    // 装备栏一件都没有：Tab 也要能停在它上面（它是画得出来的栏），那里没有选中项，↑↓ 不翻历史、不动物品栏。
    let inv = inventory(vec![
        slot(5, "小斧头", 1, 1, TargetType::None),
        slot(6, "回复药", 30, 1, TargetType::Ally),
    ]);
    let mut a = page('4', None, Some(inv));
    key(&mut a, KeyCode::Tab);
    assert!(input_row(&mut a).contains("Tab 物品"), "{}", input_row(&mut a));
    assert!(!input_row(&mut a).contains("↑↓ 选择"), "空栏里没有选中项，不写 ↑↓ 选择");
    let rows = draw(&mut a, 120, 24);
    assert!(
        rows.lines().any(|l| l.contains('▶') && l.contains("小斧头")),
        "物品栏的选中行还留着 ▶"
    );
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&a).input.text(), "", "↑↓ 没有翻历史");
    assert!(
        draw(&mut a, 120, 24)
            .lines()
            .any(|l| l.contains('▶') && l.contains("小斧头")),
        "选中项没有动"
    );
    key(&mut a, KeyCode::Tab);
    assert!(
        draw(&mut a, 120, 24).contains("小斧头的说明"),
        "Tab 回到物品栏，底边说明跟着回来"
    );
}

#[test]
fn bag_unfocused_column_keeps_its_selected_row_marked() {
    // 两栏都有东西：Tab 到装备栏后，物品栏的选中行仍有 ▶（只是不反色），两栏各一个 ▶。
    let inv = inventory(vec![
        slot(0, "铁盔", 6, 1, TargetType::None),
        slot(5, "小斧头", 1, 1, TargetType::None),
    ]);
    let mut a = page('4', None, Some(inv));
    key(&mut a, KeyCode::Tab);
    // 两个面板并排，两个 ▶ 可能在同一屏行上，所以数的是 ▶ 本身。
    let rows = draw(&mut a, 120, 24);
    assert_eq!(rows.matches('▶').count(), 2, "两栏各一个选中行");
    assert!(
        rows.lines().any(|l| l.contains("▶ 0") && l.contains("铁盔")),
        "焦点栏的选中行"
    );
    assert!(
        rows.lines().any(|l| l.contains("▶ 5") && l.contains("小斧头")),
        "非焦点栏的选中行也有 ▶"
    );
}
