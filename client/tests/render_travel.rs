//! 快捷传送页的渲染（STAGE-161，`docs/ui.md`「快捷传送页」）：各种窗口尺寸、Unicode 降级、
//! 长路径/长名字、目录不可用与空数据。用 TestBackend 画出整屏再找关键字，和 `render.rs` 同一套做法。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{TravelNode, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() })
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

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn node(id: &str, parent: Option<&str>, name: &str, floor: Option<u32>, order: u32) -> TravelNode {
    TravelNode {
        id: id.into(),
        parent_id: parent.map(Into::into),
        name: name.into(),
        floor,
        order,
    }
}

/// 一棵够深的目录，用来测长路径和多层滚动。
fn catalog() -> Vec<TravelNode> {
    let mut nodes = vec![
        node("group:sainasu", None, "萨伊那斯", Some(100), 0),
        node("map:1000", Some("group:sainasu"), "萨姆吉尔村", Some(1000), 0),
        node("map:1006", Some("map:1000"), "萨姆吉尔的村长家", Some(1006), 0),
        node("map:1001", Some("map:1000"), "萨姆吉尔的武器店", Some(1001), 1),
        node("group:dungeons", Some("group:sainasu"), "洞窟与通路", None, 1),
        node("map:10001", Some("group:dungeons"), "阿布的洞窟地下１楼", Some(10001), 0),
        node("group:giiru", None, "吉鲁岛", Some(300), 1),
    ];
    // 一个很长的名字，测窄屏截断。
    nodes.push(node(
        "map:31111",
        Some("group:giiru"),
        "非常非常长的地图名字用来检查窄屏下的截断行为",
        Some(31111),
        0,
    ));
    nodes
}

fn travel_app(unicode: bool, floor: u32, map_name: &str) -> App {
    let mut a = app(unicode);
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = floor;
    w.entry.position.map_name = map_name.into();
    w.travel.install("2026-10-17.1".into(), &catalog());
    w.travel.here_floor = Some(floor);
    w.nav.push(Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

#[test]
fn root_layer_lists_regions_at_every_size() {
    for (w, h) in [(80, 24), (100, 30), (120, 40)] {
        let mut a = travel_app(true, 1006, "萨姆吉尔的村长家");
        let s = draw(&mut a, w, h);
        // 根层末尾还有「1级宠物」（STAGE-251）和「练级传送」（STAGE-279），所以是 2 个地图分组加 2 项。
        for want in ["快捷传送", "萨伊那斯", "吉鲁岛", "（4 项）", "1级宠物", "练级传送"] {
            assert!(s.contains(want), "{w}×{h} 缺少 {want}\n{s}");
        }
        assert!(!s.contains("窗口太小"), "{w}×{h} 不该报窗口太小\n{s}");
    }
}

#[test]
fn wide_window_shows_the_destination_panel() {
    let mut a = travel_app(true, 1006, "萨姆吉尔的村长家");
    let s = draw(&mut a, 120, 40);
    for want in ["目的地", "地图编号", "下级", "当前位置", "目录版本"] {
        assert!(s.contains(want), "宽屏右侧要有详情：缺少 {want}\n{s}");
    }
    assert!(s.contains("萨姆吉尔的村长家 (1006)"), "详情里写当前所在地\n{s}");
}

#[test]
fn narrow_window_only_shows_the_list() {
    let mut a = travel_app(true, 1006, "萨姆吉尔的村长家");
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains("目的地"), "80 列放不下右侧面板\n{s}");
    assert!(s.contains("萨伊那斯"), "列表还在\n{s}");
}

#[test]
fn path_grows_as_you_descend() {
    let mut a = travel_app(true, 1006, "萨姆吉尔的村长家");
    key(&mut a, KeyCode::Enter); // 萨伊那斯
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("快捷传送 › 萨伊那斯"), "标题写当前路径\n{s}");
    key(&mut a, KeyCode::Down); // 萨姆吉尔村
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("快捷传送 › 萨伊那斯 › 萨姆吉尔村"), "{s}");
    assert!(s.contains("传送到本图 · 萨姆吉尔村"), "有下级的地图首项是传送本图\n{s}");
}

#[test]
fn long_names_do_not_break_the_narrow_layout() {
    let mut a = travel_app(true, 31111, "很长很长的地图名字");
    key(&mut a, KeyCode::Down); // 吉鲁岛
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("吉鲁岛") || s.contains("非常"), "长名字要么显示要么截断，不能崩\n{s}");
    assert!(s.contains("> ") || s.contains('│'), "界面框架还在\n{s}");
}

#[test]
fn unicode_off_still_renders_the_page() {
    let mut a = travel_app(false, 1006, "萨姆吉尔的村长家");
    let s = draw(&mut a, 80, 24);
    for want in ["快捷传送", "萨伊那斯", "吉鲁岛"] {
        assert!(s.contains(want), "关掉 Unicode 也要能读：缺少 {want}\n{s}");
    }
}

#[test]
fn unavailable_catalog_explains_instead_of_pretending_empty() {
    let mut a = app(true);
    let mut w = WorldState::new(WorldEntry::default());
    w.travel.install(String::new(), &[]);
    w.nav.push(Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("目录不可用"), "{s}");
    assert!(s.contains("Enter 重新获取"), "要有重试入口\n{s}");
}

#[test]
fn empty_layer_says_so() {
    let mut a = app(true);
    let mut w = WorldState::new(WorldEntry::default());
    // 状态是 Ready，但这一层真的没有项（一个没有子的分组）：不能显示成空白的白板。
    w.travel.install("v0".into(), &[node("group:empty", None, "空分组", None, 0)]);
    w.nav.push(Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Enter); // 进入那个空分组
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("这一层没有可去的目录"), "{s}");
}

#[test]
fn current_map_is_marked_in_the_list() {
    let mut a = travel_app(true, 1000, "萨姆吉尔村");
    key(&mut a, KeyCode::Enter); // 萨伊那斯
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("← 当前"), "列表里要标出当前所在地\n{s}");
}
