//! 装备耐久（STAGE-225）的界面测试：背包页和装备表底边的耐久行（`docs/ui.md` 背包页 A），
//! 三种等级的文字和颜色，没有耐久的物品不显示这一行，80×24 不越界。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use ratatui::buffer::Buffer;
use ratatui::style::Color;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Inventory, InventorySlot, Location, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

fn buffer(app: &mut App, w: u16, h: u16) -> Buffer {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    term.backend().buffer().clone()
}

/// 把缓冲区转成文字（同 render_inventory.rs 的 draw）。
fn text_of(buf: &Buffer) -> String {
    let (w, h) = (buf.area.width, buf.area.height);
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

fn draw(app: &mut App, w: u16, h: u16) -> String {
    text_of(&buffer(app, w, h))
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

/// 一件小斧头（攻 +9 防 -3 敏 -3），耐久和受损等级由参数给。
fn axe(slot: u8, durability: u32, max: u32, level: u8) -> InventorySlot {
    InventorySlot {
        slot,
        item_id: 0,
        name: "小斧头".into(),
        effect: "攻 +9 防 -3 敏 -3".into(),
        kind: 1,
        count: 1,
        durability,
        max_durability: max,
        crush_level: level,
        ..Default::default()
    }
}

fn bag_with(item: InventorySlot) -> App {
    bag_page(Inventory { gold: 1_000_000, slots: vec![item] })
}

/// 找到第一个含 `marker` 的行里、字符为 `sym` 的格子，返回它的前景色。按可见字符走（宽字符只算一次）。
fn fg_on_row(buf: &Buffer, marker: &str, sym: &str) -> Option<Color> {
    let (w, h) = (buf.area.width, buf.area.height);
    for y in 0..h {
        let mut chars: Vec<(String, Color)> = vec![];
        let mut x = 0;
        while x < w {
            let cell = &buf[(x, y)];
            let ch = cell.symbol().to_string();
            x += UnicodeWidthStr::width(ch.as_str()).max(1) as u16;
            chars.push((ch, cell.fg));
        }
        let row: String = chars.iter().map(|(c, _)| c.as_str()).collect();
        if !row.contains(marker) {
            continue;
        }
        if let Some((_, color)) = chars.iter().find(|(c, _)| c == sym) {
            return Some(*color);
        }
    }
    None
}

#[test]
fn full_durability_shows_percent_and_intact_text() {
    let mut a = bag_with(axe(5, 200000, 200000, 0));
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("耐久 200000/200000（100%） 完好"), "选中的斧头要显示耐久行\n{s}");
    assert!(s.contains("攻 +9 防 -3 敏 -3"), "说明文字还在，接在耐久行后面\n{s}");
}

#[test]
fn each_crush_level_has_its_text_and_colour() {
    // 完好绿、受损黄、毁坏红、不堪使用红（只用 16 色具名颜色）。
    let cases = [
        (0u8, 180u32, "完好", Color::Green),
        (1, 120, "受损", Color::Yellow),
        (2, 60, "毁坏", Color::Red),
        (3, 10, "不堪使用", Color::Red),
    ];
    for (level, cur, text, color) in cases {
        let mut a = bag_with(axe(5, cur, 200, level));
        let buf = buffer(&mut a, 120, 40);
        let s = text_of(&buf);
        let line = format!("耐久 {cur}/200");
        assert!(s.contains(&format!("{line}")) && s.contains(text), "等级 {level} 要显示 {text}\n{s}");
        let first = text.chars().next().unwrap().to_string();
        assert_eq!(fg_on_row(&buf, &line, &first), Some(color), "等级 {level} 的颜色\n{s}");
    }
}

#[test]
fn percent_rounds_down_and_reads_from_current_over_max() {
    let mut a = bag_with(axe(5, 1, 3, 1));
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("耐久 1/3（33%） 受损"), "33%，向下取整\n{s}");
}

#[test]
fn item_without_durability_has_no_durability_line() {
    let plain = InventorySlot {
        slot: 5,
        item_id: 1234,
        name: "小块肉".into(),
        effect: "耐久力20前後回复".into(),
        kind: 20,
        count: 1,
        ..Default::default()
    };
    let mut a = bag_with(plain);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("耐久力20前後回复"), "说明还在\n{s}");
    assert!(!s.contains("耐久 "), "没有耐久的物品不显示耐久行\n{s}");
}

/// 装备表的选中项也显示耐久行：按 Tab 切到装备栏，再往下选到 2 号手上的斧头（空的装备格跳过）。
#[test]
fn equipped_item_shows_durability_in_equip_panel() {
    let meat = InventorySlot { slot: 5, item_id: 1234, name: "小块肉".into(), count: 1, kind: 20, ..Default::default() };
    let mut a = bag_page(Inventory { gold: 0, slots: vec![axe(2, 150000, 200000, 1), meat] });
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("耐久 150000/200000（75%） 受损"), "装备栏的选中项要显示耐久行\n{s}");
}

#[test]
fn durability_line_fits_at_every_size() {
    let mut a = bag_with(axe(5, 200000, 200000, 3));
    for (w, h) in [(1, 1), (40, 10), (80, 24), (100, 30), (120, 40), (200, 60)] {
        draw(&mut a, w, h);
    }
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("耐久 200000/200000（100%）"), "80×24 下耐久数字放得下\n{s}");
}
