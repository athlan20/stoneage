//! 角色页（STAGE-23）的界面测试：用 TestBackend 画出菜单 2，检查能力、衍生数值和属性的显示，以及各种窗口大小和符号集下不出错。

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{AbilityPoints, Appearance, Character, DerivedStats, ElementPoints, Location, MagicView, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    // 宽字符（中文）占两格，第二格是占位，拼接时要跳过。
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

fn character() -> Character {
    Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1006, map_name: "萨姆吉尔村长之家".into() },
    }
}

/// STAGE-23：角色页的数值，与用例 `world_character_panel` 里服务端返回的一致（能力 5/5/5/5，属性 地 8 水 2）。
fn world_with_stats() -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = character();
    entry.position.floor = 1006;
    entry.position.map_name = "萨姆吉尔村长之家".into();
    entry.position.x = 15;
    entry.position.y = 22;
    let s = &mut entry.status;
    s.level = 1;
    s.hp = 35;
    s.max_hp = 35;
    s.mp = 100;
    s.max_mp = 100;
    s.next_exp = 2;
    s.gold = 1_000_000;
    s.stats = AbilityPoints { vital: 5, strength: 5, toughness: 5, dexterity: 5 };
    s.derived = DerivedStats { attack: 6, defense: 6, agility: 5, charm: 60, luck: 0 };
    s.elements = ElementPoints { earth: 8, water: 2, fire: 0, wind: 0 };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.menu = 1;
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

/// STAGE-25：角色带两个精灵（0、1 号栏位，与用例 `battle_magic` 的 `status.magic` 一致）。
fn world_with_magic() -> App {
    let mut a = world_with_stats();
    a.world.as_mut().unwrap().entry.status.magic = vec![
        MagicView { slot: 0, id: 301, name: "岩石撞击".into(), mp: 10, ..Default::default() },
        MagicView { slot: 1, id: 0, name: "治愈的精灵 Lv1".into(), mp: 6, ..Default::default() },
    ];
    a
}

/// 包含 `key` 的那一行（表格里的一行），找不到时返回空串。
fn line_with<'a>(s: &'a str, key: &str) -> &'a str {
    s.lines().find(|l| l.contains(key)).unwrap_or("")
}

#[test]
fn character_page_shows_stats_and_derived() {
    let mut a = world_with_stats();
    let s = draw(&mut a, 120, 40);
    for want in ["基本信息", "能力", "地水火风", "勇者小明", "称号", "无", "转生", "35/35", "100/100", "未分配", "魅力", "60", "运气"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    assert!(line_with(&s, "攻击").contains('6') && line_with(&s, "防御").contains('6'), "攻击、防御应为 6\n{s}");
    assert!(line_with(&s, "敏捷").contains('5') && line_with(&s, "HP").contains("35/35"), "敏捷 5、HP 35/35\n{s}");
    let bar = |name: char, v: char| s.lines().any(|l| l.contains(name) && l.contains(v));
    assert!(bar('地', '8') && bar('水', '2') && bar('火', '0'), "属性 地 8 水 2 火 0\n{s}");
    assert!(line_with(&s, "体力").contains('5'), "体力 5\n{s}");
}

#[test]
fn character_page_lists_learned_magic() {
    let mut a = world_with_magic();
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("精灵（魔法）"), "缺精灵清单\n{s}");
    assert!(line_with(&s, "岩石撞击").contains("MP 10"), "0 号栏位：{}\n{s}", line_with(&s, "岩石撞击"));
    assert!(line_with(&s, "治愈的精灵 Lv1").contains("MP 6"), "1 号栏位：{}\n{s}", line_with(&s, "治愈的精灵 Lv1"));
    assert_eq!(s.lines().filter(|l| l.contains("—")).count(), 3, "2–4 号空栏位各一行 —\n{s}");
}

#[test]
fn character_page_magic_empty_slots() {
    // 没有精灵时五个栏位都是空栏位，不报错（验收口径 2）。
    let mut a = world_with_stats();
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("精灵（魔法）"), "缺精灵清单\n{s}");
    assert_eq!(s.lines().filter(|l| l.contains("—")).count(), 5, "五个空栏位\n{s}");
    let magic_block: Vec<&str> = s.lines().skip_while(|l| !l.contains("精灵（魔法）")).take(6).collect();
    assert!(!magic_block.join("\n").contains("MP"), "没有精灵时清单里不显示 MP\n{}", magic_block.join("\n"));
}

#[test]
fn character_page_fits_minimum_window() {
    // 80×24 时中间区域只有 10 行：上面三栏压缩到只剩标题边框，底部精灵清单保持完整（STAGE-25）。
    let mut a = world_with_stats();
    let s = draw(&mut a, 80, 24);
    for want in ["基本信息", "能力", "地水火风", "精灵（魔法）"] {
        assert!(s.contains(want), "80×24 缺少 {want}\n{s}");
    }
    assert_eq!(s.lines().filter(|l| l.contains("—")).count(), 5, "80×24 下精灵清单仍是五个栏位\n{s}");
}

#[test]
fn character_page_survives_every_size_and_symbol_set() {
    let mut a = world_with_stats();
    for unicode in [true, false] {
        a.unicode = unicode;
        for (w, h) in [(1, 1), (80, 24), (100, 30), (120, 40), (200, 60)] {
            let s = draw(&mut a, w, h);
            assert!(w < 80 || h < 24 || s.contains("地水火风"), "{w}x{h}\n{s}");
        }
    }
}

#[test]
fn character_page_line_gauge_symbol_follows_unicode() {
    let mut a = world_with_stats();
    assert!(draw(&mut a, 120, 40).contains('━'), "unicode 开启时进度条用粗线");
    a.unicode = false;
    assert!(!draw(&mut a, 120, 40).contains('━'), "unicode 关闭时不用粗线");
}

/// STAGE-154：骑乘等级最高 200，「骑乘 200」要在基本信息的 28 列内宽里完整显示（120×40 与 200×60 都要看到；
/// 80×24 时三栏只剩边框，见 `character_page_fits_minimum_window`）。
#[test]
fn character_page_shows_full_ride_level() {
    let mut a = world_with_stats();
    a.world.as_mut().unwrap().entry.status.ride_level = 200;
    for (w, h) in [(120, 40), (200, 60)] {
        let s = draw(&mut a, w, h);
        assert!(line_with(&s, "等级").contains("骑乘 200"), "{w}x{h} 骑乘等级应完整显示\n{s}");
    }
}

#[test]
fn character_page_title_none_when_empty() {
    let mut a = world_with_stats();
    a.world.as_mut().unwrap().entry.status.title = "勇者".into();
    assert!(draw(&mut a, 120, 40).contains("称号  勇者"));
}

/// STAGE-203：变身中角色页基本信息多一行“变身：宠物（剩 N 秒）”，没有变身时没有这一行。
#[test]
fn character_page_shows_metamo_row_only_while_transformed() {
    use stoneage_client::proto::MetamoView;
    let mut a = world_with_stats();
    let s = draw(&mut a, 120, 40);
    assert!(!s.contains("变身："), "没有变身时不显示变身行\n{s}");
    // 到期时间设成很久以后，剩余秒数只要是正数就行（按本地时钟算，测试不依赖具体时刻）。
    let until = std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_millis() as i64 + 53_000;
    a.world.as_mut().unwrap().entry.status.metamo = Some(MetamoView { pet: "乌力".into(), image: 100250, until });
    let s = draw(&mut a, 120, 40);
    let row = line_with(&s, "变身：");
    assert!(row.contains("乌力") && row.contains("剩 5") && row.contains("秒"), "变身行：{row}\n{s}");
    assert!(s.contains("HP") && s.contains("35/35"), "变身行不挤掉 HP 行\n{s}");
}
