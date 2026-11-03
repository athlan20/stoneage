//! STAGE-251 渲染：快捷传送页「1级宠物」目录的画面（`docs/ui.md`「快捷传送页」）。
//! 三种尺寸（80×24 / 100×30 / 120×40）下断言：标题命中数、光标行可见、窄屏没有详情、宽屏有详情、
//! 长名字截断后不撑破边框；另外是地图目录不可用时根层仍有「1级宠物」。用 TestBackend 画整屏，和 `render_travel.rs` 同一套做法。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use ratatui::buffer::Buffer;
use stoneage_client::app::pet_dir::PetDir;
use stoneage_client::app::{App, Layer, MAP_MENU, Options, Screen, TravelNav, WorldState};
use stoneage_client::proto::{PetAtoms, PetCatalog, PetEntry, PetPoint, PetRect, PetSpot, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

const SIZES: [(u16, u16); 3] = [(80, 24), (100, 30), (120, 40)];

fn app(unicode: bool) -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode,
        ..Options::default()
    })
}

/// 整屏文字（宽字符只取一次），和 `render_travel.rs` 的 `draw` 相同。
fn draw(a: &mut App, w: u16, h: u16) -> String {
    draw_buf(a, w, h).0
}

/// 整屏文字加缓冲区，边框要逐格看时用缓冲区。
fn draw_buf(a: &mut App, w: u16, h: u16) -> (String, Buffer) {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    let buf = term.backend().buffer().clone();
    let text = (0..h)
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
        .join("\n");
    (text, buf)
}

fn pet(id: i64, name: &str, aliases: &[&str], series: &str, wind: i64) -> PetEntry {
    PetEntry {
        pet_id: id,
        name: name.into(),
        aliases: aliases.iter().map(|s| s.to_string()).collect(),
        series: series.into(),
        atoms: PetAtoms {
            wind,
            ..PetAtoms::default()
        },
        size: 1,
        catch_difficulty: 2,
        spot_count: 1,
    }
}

fn spot(pet_id: i64, number: i64, floor: u32, map: &str, anchor: (i32, i32), others: &[&str]) -> PetSpot {
    PetSpot {
        pet_id,
        area_id: format!("{floor}-{number}"),
        number,
        floor,
        map_name: map.into(),
        anchor: PetPoint {
            x: anchor.0,
            y: anchor.1,
        },
        rect: PetRect {
            x1: 547,
            y1: 115,
            x2: 699,
            y2: 241,
        },
        max_enemies: 3,
        steps: 26,
        also_level1: others.iter().map(|s| s.to_string()).collect(),
    }
}

/// 几只有代表性的宠物：俗称、系列、属性为空与不为空各一；最后一只名字很长。
fn sample() -> PetCatalog {
    let long = "非常非常长的宠物名字用来检查窄屏下的截断行为一二三四五六七八九十";
    PetCatalog {
        catalog_version: "test".into(),
        pets: vec![
            pet(1, "扬奇洛斯", &["蓝人龙"], "人龙系", 100),
            pet(2, "利则诺顿", &[], "", 0),
            pet(3, "布比", &[], "", 100),
            pet(4, "人龙王", &[], "人龙系", 0),
            pet(5, long, &[], "人龙系", 0),
        ],
        spots: vec![
            spot(1, 38, 100, "萨伊那斯", (547, 115), &["布比"]),
            spot(1, 12, 101, "萨伊那斯", (300, 200), &[]),
        ],
    }
}

/// 多到一屏放不下的列表，用来测光标行可见。
fn many(n: usize) -> PetCatalog {
    PetCatalog {
        catalog_version: "test".into(),
        pets: (0..n)
            .map(|i| pet(100 + i as i64, &format!("测试宠物{i:02}"), &[], "", 0))
            .collect(),
        spots: vec![],
    }
}

/// 快捷传送页上打开「1级宠物」目录（目录已取到），站在萨伊那斯。
fn pets_app(unicode: bool, cat: PetCatalog) -> App {
    let mut a = app(unicode);
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    w.menu = MAP_MENU;
    w.nav.push(Layer::Page(MAP_MENU));
    w.travel.pets.open();
    w.travel.pets.install(cat);
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn pets(a: &mut App) -> &mut PetDir {
    &mut a.world.as_mut().unwrap().travel.pets
}

/// 「1级宠物」面板的上下边框行号：内容行是 top+1 .. bottom。快捷传送页外面还有顶部状态栏和消息栏，所以不能按整屏查边框。
fn pet_block_rows(s: &str) -> (u16, u16) {
    let lines: Vec<&str> = s.lines().collect();
    let top = lines
        .iter()
        .position(|l| l.starts_with("┌ 1级宠物"))
        .expect("目录面板的上边框");
    let bottom = top
        + 1
        + lines[top + 1..]
            .iter()
            .position(|l| l.starts_with('└'))
            .expect("下边框");
    (top as u16, bottom as u16)
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn visible_count(a: &mut App) -> usize {
    pets(a).visible_pets().len()
}

#[test]
fn title_shows_the_hit_count_at_every_size() {
    for (w, h) in SIZES {
        let mut a = pets_app(true, sample());
        let s = draw(&mut a, w, h);
        assert!(s.contains("1级宠物 · 5 只"), "{w}×{h} 无筛选时标题写总数\n{s}");
        pets(&mut a).filter = "人龙".into();
        let n = visible_count(&mut a);
        assert_eq!(
            n, 3,
            "人龙：扬奇洛斯（俗称、系列都带人龙）、人龙王、长名字那只（系列人龙系）"
        );
        let s = draw(&mut a, w, h);
        assert!(s.contains(&format!("筛选“人龙” · {n} 只")), "{w}×{h} 标题写命中数\n{s}");
    }
}

#[test]
fn no_match_says_how_to_clear_instead_of_an_empty_list() {
    for (w, h) in SIZES {
        let mut a = pets_app(true, sample());
        pets(&mut a).filter = "不存在的宠物".into();
        let s = draw(&mut a, w, h);
        assert!(s.contains("没有匹配的宠物 · Esc 清空筛选"), "{w}×{h}\n{s}");
        assert!(s.contains("· 0 只"), "{w}×{h} 标题写 0 只\n{s}");
    }
}

#[test]
fn filter_box_shows_a_caret_while_typing() {
    let mut a = pets_app(true, sample());
    pets(&mut a).start_typing();
    pets(&mut a).type_char('布');
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("筛选：布▌"), "输入时框里有光标\n{s}");
    let mut a = pets_app(false, sample());
    pets(&mut a).start_typing();
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("筛选：_"), "--unicode 关闭时光标退回 ASCII\n{s}");
}

#[test]
fn list_row_shows_name_alias_series_attribute_and_spot_count() {
    let mut a = pets_app(true, sample());
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("扬奇洛斯（蓝人龙 · 人龙系）"), "行内有俗称与系列\n{s}");
    assert!(s.contains("风 100 · 1处"), "行右侧有属性与可遇点数\n{s}");
    assert!(s.contains("— · 1处"), "没有属性的写 —\n{s}");
}

#[test]
fn cursor_row_stays_visible_in_a_short_list() {
    for (w, h) in SIZES {
        let mut a = pets_app(true, many(40));
        for _ in 0..39 {
            pets(&mut a).navigate(TravelNav::Down);
        }
        let s = draw(&mut a, w, h);
        assert!(s.contains("▶ 测试宠物39"), "{w}×{h} 光标行可见\n{s}");
        assert!(!s.contains("测试宠物00"), "{w}×{h} 列表跟着光标滚动，顶部移出\n{s}");
    }
}

#[test]
fn wide_window_has_a_detail_panel_and_narrow_does_not() {
    let mut a = pets_app(true, sample());
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains("详情"), "80 列只有列表\n{s}");
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("详情"), "100 列（内容区 ≥ 88）有详情\n{s}");
    assert!(s.contains("属性 地 0  水 0  火 0  风 100"), "选中的宠物：四维属性\n{s}");
    assert!(s.contains("体型 1  捕捉难度 2"), "体型与捕捉难度\n{s}");
    assert!(s.contains("俗称 蓝人龙"), "俗称\n{s}");
    assert!(s.contains("系列 人龙系"), "系列\n{s}");
    assert!(s.contains("可遇点 1 处"), "可遇点\n{s}");
}

#[test]
fn spot_level_lists_map_with_floor_suffix_for_duplicates_and_shows_detail_when_wide() {
    let mut a = pets_app(true, sample());
    key(&mut a, KeyCode::Enter); // 扬奇洛斯 → L2
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("萨伊那斯 · 100 (547,115)"), "同名地图补 · floor\n{s}");
    assert!(s.contains("萨伊那斯 · 101 (300,200)"), "同名地图补 · floor\n{s}");
    assert!(
        s.contains("区域 38 · 上限 3 · 同点 布比"),
        "右边：区域、上限、同点其他 1 级宠\n{s}"
    );
    assert!(s.contains("区域 38"), "右侧详情：区域\n{s}");
    assert!(s.contains("矩形 (547,115)–(699,241)"), "右侧详情：矩形\n{s}");
    assert!(s.contains("约 26 格一场"), "右侧详情：节奏\n{s}");
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains("详情"), "窄屏 L2 没有详情\n{s}");
    assert!(s.contains("萨伊那斯 · 100 (547,115)"), "窄屏 L2 仍列出遇敌点\n{s}");
}

#[test]
fn action_menu_has_four_items_at_every_size() {
    for (w, h) in SIZES {
        let mut a = pets_app(true, sample());
        key(&mut a, KeyCode::Enter); // → L2
        key(&mut a, KeyCode::Enter); // → L3
        let s = draw(&mut a, w, h);
        for want in ["直接开战", "连续遇敌", "只传送不遇敌", "取消"] {
            assert!(s.contains(want), "{w}×{h} 动作菜单缺少 {want}\n{s}");
        }
    }
}

#[test]
fn long_names_are_clipped_and_never_break_the_border() {
    let long_prefix = "非常非常长的宠物名字";
    for (w, h) in [(80u16, 24u16), (100, 30)] {
        let mut a = pets_app(true, sample());
        for _ in 0..4 {
            pets(&mut a).navigate(TravelNav::Down); // 长名字那只
        }
        let (s, buf) = draw_buf(&mut a, w, h);
        assert!(s.contains(long_prefix), "{w}×{h} 截断后仍能认出名字\n{s}");
        assert!(!s.contains("十（人龙系）"), "{w}×{h} 超出的部分（系列）被截断\n{s}");
        assert!(s.contains('…'), "{w}×{h} 截断用省略号\n{s}");
        // 每一行的左右两条边框都还在，说明文字没有越过面板。
        let (top, bottom) = pet_block_rows(&s);
        for y in top + 1..bottom {
            assert_eq!(buf[(0, y)].symbol(), "│", "{w}×{h} 第 {y} 行左边框被撑破\n{s}");
            assert_eq!(buf[(w - 1, y)].symbol(), "│", "{w}×{h} 第 {y} 行右边框被撑破\n{s}");
        }
    }
}

#[test]
fn unicode_off_uses_ascii_cursor_and_ellipsis() {
    let mut a = pets_app(false, sample());
    let (s, buf) = draw_buf(&mut a, 80, 24);
    assert!(s.contains("> 扬奇洛斯"), "ASCII 光标\n{s}");
    let (top, bottom) = pet_block_rows(&s);
    for y in top + 1..bottom {
        assert_eq!(buf[(79, y)].symbol(), "│", "第 {y} 行右边框\n{s}");
    }
    let mut long = pets_app(false, sample());
    for _ in 0..4 {
        pets(&mut long).navigate(TravelNav::Down);
    }
    let s = draw(&mut long, 80, 24);
    assert!(s.contains(".."), "ASCII 省略号\n{s}");
}

#[test]
fn map_tree_not_ready_still_shows_the_1级宠物_entry() {
    // STAGE-251：地图目录加载中或不可用时，根层也有「1级宠物」（不依赖地图目录是否就绪）。
    for (w, h) in SIZES {
        let mut a = app(true);
        let mut state = WorldState::new(WorldEntry::default());
        state.menu = MAP_MENU;
        state.nav.push(Layer::Page(MAP_MENU));
        a.world = Some(state);
        a.screen = Screen::World;
        let s = draw(&mut a, w, h);
        assert!(s.contains("正在取快捷传送目录"), "{w}×{h} 加载中的说明\n{s}");
        assert!(s.contains("1级宠物"), "{w}×{h} 加载中也有「1级宠物」\n{s}");
    }
    let mut a = app(true);
    let mut state = WorldState::new(WorldEntry::default());
    state.menu = MAP_MENU;
    state.nav.push(Layer::Page(MAP_MENU));
    state.travel.install(String::new(), &[]);
    a.world = Some(state);
    a.screen = Screen::World;
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("快捷传送目录不可用"), "不可用的说明还在\n{s}");
    assert!(s.contains("1级宠物"), "不可用时「1级宠物」也在\n{s}");
}
