//! 背包页（STAGE-28）的界面测试：空格子、石币、物品显示与滚动，`docs/ui.md`「背包页 A」。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Inventory, InventorySlot, Location, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
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

fn key(app: &mut App, code: KeyCode) {
    app.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

/// 进入世界并切到「4 背包」页。
fn bag_page(inv: Inventory) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.inventory = inv;
    a.world = Some(world);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('4'));
    a
}

fn empty() -> Inventory {
    Inventory { gold: 1_000_000, slots: vec![] }
}

#[test]
fn empty_bag_shows_all_slots_and_gold() {
    let mut a = bag_page(empty());
    let s = draw(&mut a, 120, 40);
    for want in ["装备", "物品", "石币 1000000", "编号", "部位", "数量", "头", "身", "手", "饰品"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    // 装备 5 格加物品 15 格，空格子写灰色的 —，编号固定（验收 1）。
    assert_eq!(s.matches('—').count(), 20, "20 个空格子都要有占位符\n{s}");
    for slot in [0, 1, 2, 3, 4, 5, 10, 19] {
        assert!(s.lines().any(|l| l.contains(&format!(" {slot} ")) || l.contains(&format!("│{slot} "))), "缺少格子 {slot}\n{s}");
    }
}

#[test]
fn item_shows_in_first_free_bag_slot() {
    // 管理命令发了小斧头（item_id 0），编号是 5（验收 2 的界面侧）。
    let inv = Inventory {
        gold: 1_000_000,
        slots: vec![InventorySlot {
            slot: 5,
            item_id: 0,
            name: "小斧头".into(),
            effect: "攻 +9 防 -3 敏 -3".into(),
            kind: 1,
            count: 1,
            ..Default::default()
        }],
    };
    let mut a = bag_page(inv);
    let s = draw(&mut a, 120, 40);
    let row = s.lines().find(|l| l.contains("小斧头")).expect("要显示小斧头");
    assert!(row.contains('5'), "小斧头在 5 号格子\n{row}");
    assert!(row.contains("攻 +9 防 -3 敏 -3"), "要显示效果文字\n{row}");
    assert!(row.contains('1'), "数量是 1\n{row}");
    assert_eq!(s.matches('—').count(), 19, "只剩 19 个空格子\n{s}");
}

/// 装备位上也有物品时，装备表显示部位对应的格子。
#[test]
fn equipped_item_shows_in_equip_table() {
    let inv = Inventory {
        gold: 0,
        slots: vec![InventorySlot {
            slot: 2,
            item_id: 0,
            name: "小斧头".into(),
            effect: "攻 +9 防 -3 敏 -3".into(),
            kind: 1,
            count: 1,
            ..Default::default()
        }],
    };
    let mut a = bag_page(inv);
    let s = draw(&mut a, 120, 40);
    let row = s.lines().find(|l| l.contains("手") && l.contains("小斧头")).expect("2 号装备位是手");
    assert!(row.contains('2'), "装备表的编号是 2\n{row}");
}

/// 高度不够时物品表滚动：80×24 的中间区域 10 行，物品表显示 9 行数据（验收 3）。
#[test]
fn bag_table_scrolls_when_short() {
    let mut a = bag_page(empty());
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("5–13 / 19"), "标题写当前范围\n{s}");
    assert!(!s.lines().any(|l| l.contains(" 14 ")), "只显示到 13 号格\n{s}");
    key(&mut a, KeyCode::PageDown);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("11–19 / 19"), "翻页后最后一页贴底，不越界\n{s}");
    key(&mut a, KeyCode::PageDown);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("11–19 / 19"), "到底再翻不动\n{s}");
    key(&mut a, KeyCode::PageUp);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("5–13 / 19"), "往回翻\n{s}");
}

/// 高度够时物品表全部显示，没有滚动范围（验收 3：120×40 不越界）。
#[test]
fn tall_window_shows_everything() {
    let mut a = bag_page(empty());
    let s = draw(&mut a, 120, 40);
    assert!(!s.contains("/ 19"), "高度够时不写滚动范围\n{s}");
    assert!(s.lines().any(|l| l.contains("19") && l.contains('—')), "19 号格可见\n{s}");
}

#[test]
fn survives_every_size() {
    let inv = Inventory {
        gold: 1_000_000,
        slots: vec![
            InventorySlot { slot: 0, item_id: 1, name: "皮帽".into(), effect: "防 +2".into(), kind: 0, count: 1, ..Default::default() },
            InventorySlot { slot: 5, item_id: 0, name: "小斧头".into(), effect: "攻 +9 防 -3 敏 -3".into(), kind: 1, count: 1, ..Default::default() },
            InventorySlot { slot: 19, item_id: 2, name: "气力之杖".into(), effect: "攻 +3".into(), kind: 1, count: 3, ..Default::default() },
        ],
    };
    let mut a = bag_page(inv);
    for (w, h) in [(1, 1), (79, 23), (80, 24), (100, 30), (120, 40), (200, 60)] {
        draw(&mut a, w, h);
    }
    // 滚动到中间再画一遍也不怕。
    key(&mut a, KeyCode::PageDown);
    draw(&mut a, 80, 24);
}
