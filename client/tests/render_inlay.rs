//! STAGE-299 精工的界面（不连服务端）：材料选择面板在 80×24 下完整可见，`--unicode false` 下一样；
//! 现有数据（物品没有 `inlay` 字段）的背包动作里没有「精工」入口。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Inlay, Inventory, InventorySlot, Location, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode,
        ..Options::default()
    })
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

fn item(slot: u8, name: &str, count: u32, inlay: Option<Inlay>) -> InventorySlot {
    InventorySlot {
        slot,
        name: name.into(),
        count,
        kind: 1,
        inlay,
        ..Default::default()
    }
}

fn gear(free: u8) -> Inlay {
    Inlay {
        role: "gear".into(),
        free,
        codes: vec![None, None, None],
    }
}

fn material() -> Inlay {
    Inlay {
        role: "material".into(),
        ..Default::default()
    }
}

/// 进入世界并切到「4 背包」页。
fn bag_page(slots: Vec<InventorySlot>, unicode: bool) -> App {
    let mut a = app(unicode);
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance {
            body: 3,
            color: 2,
            eye: 1,
            mouth: 4,
        },
        location: Location {
            floor: 1000,
            map_name: "萨姆吉尔村".into(),
        },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(Layer::Page(0));
    world.inventory = Inventory { gold: 1_000_000, slots };
    a.world = Some(world);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('4'));
    a
}

fn inlay_slots() -> Vec<InventorySlot> {
    vec![
        item(5, "轻的斧头", 1, Some(gear(2))),
        item(6, "魅力梨子", 1, Some(material())),
        item(7, "红宝石", 2, Some(material())),
    ]
}

#[test]
fn picker_fits_80x24_in_both_unicode_modes() {
    for unicode in [true, false] {
        let mut a = bag_page(inlay_slots(), unicode);
        key(&mut a, KeyCode::Enter); // 动作列表：装备、精工、移动、丢弃、说明
        key(&mut a, KeyCode::Down);
        key(&mut a, KeyCode::Enter); // 材料选择
        key(&mut a, KeyCode::Char(' '));
        let screen = draw(&mut a, 80, 24);
        assert!(
            screen.contains("精工 · 轻的斧头 · 勾选材料 · 已选 1/2"),
            "标题写已选件数（unicode={unicode}）\n{screen}"
        );
        assert!(
            screen.contains("[x]") && screen.contains("[ ]"),
            "勾选框（unicode={unicode}）\n{screen}"
        );
        assert!(
            screen.contains("魅力梨子") && screen.contains("红宝石"),
            "材料列表（unicode={unicode}）\n{screen}"
        );
        assert!(
            screen.contains("Space 勾选") && screen.contains("Enter 确认"),
            "提示行（unicode={unicode}）\n{screen}"
        );
    }
}

#[test]
fn bag_without_inlay_data_has_no_entry() {
    let mut a = bag_page(vec![item(5, "小斧头", 1, None)], true);
    key(&mut a, KeyCode::Enter);
    let screen = draw(&mut a, 80, 24);
    assert!(
        screen.contains("丢弃") && screen.contains("说明"),
        "动作列表照常显示\n{screen}"
    );
    assert!(!screen.contains("精工"), "没有 inlay 字段时没有精工入口\n{screen}");
}
