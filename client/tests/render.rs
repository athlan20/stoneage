//! 界面绘制测试：用 TestBackend 画出各页，检查关键内容，并确认各种窗口大小下不会出错。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, CreateStep, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, CharacterList, Location, NpcOption, NpcWindow, Place, WorldEntry};
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

fn type_text(app: &mut App, s: &str) {
    for c in s.chars() {
        app.on_key(KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE));
    }
}

fn character() -> Character {
    Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1006, map_name: "萨姆吉尔村长之家".into() },
    }
}

#[test]
fn too_small_window_only_shows_hint() {
    let mut a = app();
    let s = draw(&mut a, 60, 20);
    assert!(s.contains("窗口太小"));
    assert!(!s.contains("账号"));
}

#[test]
fn login_page_masks_password() {
    let mut a = app();
    type_text(&mut a, "alice");
    a.on_key(KeyEvent::new(KeyCode::Tab, KeyModifiers::NONE));
    type_text(&mut a, "secret1");
    for (w, h) in [(80, 24), (120, 40)] {
        let s = draw(&mut a, w, h);
        for want in ["石器时代", "线路", "本地 127.0.0.1:7065", "alice", "[ 登录 ]", "[ 注册 ]", "[ 退出 ]", "•••••••"] {
            assert!(s.contains(want), "{w}x{h} 缺少 {want}\n{s}");
        }
        assert!(!s.contains("secret1"), "密码不能明文显示");
    }
}

#[test]
fn form_message_bar_wraps_long_server_message() {
    // STAGE-100：服务端拒绝旧客户端时的提示比消息面板宽，表单页要换行显示完整（验收口径 1）。
    let mut a = app();
    a.log.error("当前版本 0.1.0 过旧，需要升级到 0.2.0 以上，运行 stoneage update");
    for (w, h) in [(80, 24), (120, 40), (200, 50)] {
        let s = draw(&mut a, w, h);
        assert!(s.contains("需要升级到 0.2.0 以上"), "{w}x{h} 提示被截断\n{s}");
        assert!(s.contains("stoneage update"), "{w}x{h} 升级命令应完整显示\n{s}");
        // 续行要缩进到标签后（LABEL_WIDTH = 6），首行仍然是「错误  」开头。
        let cont = s.lines().find(|l| l.contains("stoneage update")).unwrap().split('│').nth(1).unwrap();
        assert!(cont.starts_with("      "), "{w}x{h} 续行应缩进：{cont:?}");
    }
}

#[test]
fn ascii_fallback_when_unicode_off() {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: false, ..Options::default() });
    a.on_key(KeyEvent::new(KeyCode::Tab, KeyModifiers::NONE));
    type_text(&mut a, "secret1");
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("*******") && !s.contains('•'), "{s}");
}

#[test]
fn register_page_draws() {
    let mut a = app();
    a.screen = Screen::Register;
    let s = draw(&mut a, 80, 24);
    for want in ["注册账号", "确认密码", "[ 注册 ]", "[ 返回 ]"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
}

#[test]
fn character_list_shows_slots() {
    let mut a = app();
    a.screen = Screen::Characters;
    a.chars.list = CharacterList { max_slots: 2, characters: vec![character()] };
    let s = draw(&mut a, 100, 30);
    for want in ["选择角色", "栏位", "勇者小明", "12", "萨姆吉尔村长之家", "(空)", "d 删除"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
}

/// 联调发现：名字最长 16 个字符（汉字占两格），角色列表的名字列原来只有 20 格，长名字被截断。
/// 16 个汉字和带 emoji 的名字在最小窗口（80×24）下也要完整显示。
#[test]
fn long_names_fit_in_character_list() {
    let mut a = app();
    a.screen = Screen::Characters;
    let mut long = character();
    long.name = "一二三四五六七八九十一二三四五六".into();
    let mut emoji = character();
    emoji.slot = 1;
    emoji.name = "石器😀勇者小明石器😀勇者小".into();
    emoji.location.map_name = "卡鲁它那的村长家".into();
    a.chars.list = CharacterList { max_slots: 2, characters: vec![long.clone(), emoji.clone()] };
    for (w, h) in [(80, 24), (100, 30), (120, 40)] {
        let s = draw(&mut a, w, h);
        for want in [long.name.as_str(), emoji.name.as_str(), "卡鲁它那的村长家", "萨姆吉尔村长之家"] {
            assert!(s.contains(want), "{w}×{h} 缺少 {want}\n{s}");
        }
    }
}

#[test]
fn create_steps_draw() {
    let mut a = app();
    a.screen = Screen::Create;
    let s = draw(&mut a, 100, 30);
    for want in ["1外形", "2名字与属性", "3出生地", "小矮子", "辫子男孩"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    // 眼型、嘴型、图像编号不显示（默认值即可）；左侧外形列选中项只高亮、不画箭头。
    for gone in ["眼型", "嘴型", "图像编号"] {
        assert!(!s.contains(gone), "不该显示 {gone}\n{s}");
    }
    a.create.body = 1;
    let s = draw(&mut a, 100, 30);
    let row = s.lines().find(|l| l.contains("赛亚人") && l.contains(" 2 ")).expect("左侧列表有“2 赛亚人”");
    let left = row.split('│').nth(1).unwrap_or_default();
    assert!(!left.contains('>'), "左侧外形列不该有箭头: {left:?}\n{s}");
    a.create.step = CreateStep::Stats;
    a.create.name.set("勇者小明");
    let s = draw(&mut a, 100, 30);
    for want in ["能力点 剩余 0", "属性点 剩余 0", "体力", "耐久力", "与地互斥", "与水互斥"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    a.create.step = CreateStep::Home;
    for size in [(80, 24), (100, 30), (160, 50)] {
        let s = draw(&mut a, size.0, size.1);
        for want in ["出生地 · 萨姆吉尔", "X 萨姆吉尔", "X 玛丽娜丝", "X 加加村", "X 卡鲁它那"] {
            assert!(s.contains(want), "{size:?} 缺少 {want}\n{s}");
        }
    }
}


#[test]
fn every_page_survives_every_size() {
    let mut a = app();
    a.chars.list = CharacterList { max_slots: 2, characters: vec![character()] };
    let mut entry = WorldEntry::default();
    entry.character = character();
    let mut w = WorldState::new(entry);
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    for screen in [Screen::Login, Screen::Register, Screen::Characters, Screen::Create, Screen::World] {
        a.screen = screen;
        for step in [CreateStep::Look, CreateStep::Stats, CreateStep::Home] {
            a.create.step = step;
            for (w, h) in [(1, 1), (79, 23), (80, 24), (81, 25), (200, 60)] {
                draw(&mut a, w, h);
            }
        }
    }
}

/// 联调发现：底部消息栏原来只留了 8 行，扣掉边框、分隔线和输入行后只能显示 4 条消息，设计是 5 条（STAGE-11）。
#[test]
fn world_message_bar_shows_five_messages() {
    use stoneage_client::proto::WorldEntry;
    let mut a = app();
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    for i in 1..=7 {
        a.log.system(format!("第{i}条消息"));
    }
    for (w, h) in [(80, 24), (120, 40)] {
        let s = draw(&mut a, w, h);
        for i in 3..=7 {
            assert!(s.contains(&format!("第{i}条消息")), "{w}×{h} 缺少第{i}条\n{s}");
        }
        assert!(!s.contains("第2条消息"), "{w}×{h} 只应显示最近 5 条\n{s}");
    }
}

/// STAGE-20：地图页——世界地图、当前位置面板、玩家 `@` 的定位。
fn world_at(floor: u32, map_name: &str, x: i32, y: i32) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = character();
    entry.position.floor = floor;
    entry.position.map_name = map_name.into();
    entry.position.x = x;
    entry.position.y = y;
    entry.position.direction = 3;
    entry.status.gold = 1_000_000;
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.places = places();
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

/// 服务端一期的地点表（`docs/protocol.md` 5.15）。
fn places() -> Vec<Place> {
    let p = |id, name: &str, floor, x, y| Place { id, name: name.into(), floor, x, y };
    vec![
        p(1, "萨姆吉尔村", 1000, 50, 116),
        p(2, "玛丽娜丝渔村", 2000, 72, 33),
        p(3, "加加村", 3000, 74, 54),
        p(4, "卡鲁它那村", 4000, 100, 96),
        p(5, "萨姆吉尔近郊", 100, 600, 550),
        p(6, "玛丽娜丝近郊", 100, 30, 580),
        p(7, "加加村近郊", 200, 760, 445),
        p(8, "卡鲁它那近郊", 200, 275, 575),
    ]
}






#[test]
fn map_page_survives_every_size_and_floor() {
    for floor in [100, 200, 300, 1006, 99999] {
        let mut a = world_at(floor, "某地", -5, 100_000);
        for (w, h) in [(1, 1), (79, 23), (80, 24), (99, 24), (100, 24), (200, 60)] {
            draw(&mut a, w, h);
        }
    }
}




/// NPC 对话窗口（STAGE-69，docs/protocol.md 5.34）：居中弹窗显示名字、正文（\n 换行）
/// 和按键提示；正文与选项按服务端推送画。任何窗口尺寸都不能画崩。
#[test]
fn dialog_window_renders_npc_text() {
    let mut a = world_at(1006, "萨姆吉尔村长之家", 15, 22);
    let w = a.world.as_mut().unwrap();
    w.npc_window = Some(NpcWindow {
        npc: 225,
        name: "村长家的厨师".into(),
        type_: "对话".into(),
        text: "今天想吃点什么？\n今天的特餐是乌力烤肉。".into(),
        options: vec![],
        accepts_text: false,
        ..Default::default()
    });
    for (w, h) in [(1, 1), (80, 24), (120, 40)] {
        draw(&mut a, w, h); // 任何尺寸都不能画崩
    }
    let s = draw(&mut a, 120, 40);
    for want in ["村长家的厨师", "今天想吃点什么？", "今天的特餐是乌力烤肉。", "按数字或 Esc 关闭"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
}

/// 带选项的窗口（商店等以后的功能）：选项带编号，提示变成数字选择。
#[test]
fn dialog_window_renders_options() {
    let mut a = world_at(1000, "萨姆吉尔村", 50, 116);
    let w = a.world.as_mut().unwrap();
    w.npc_window = Some(NpcWindow {
        npc: 198,
        name: "特产品贩卖员".into(),
        type_: "商店".into(),
        text: "要买点什么？".into(),
        options: vec![NpcOption { no: 1, label: "买东西".into() }, NpcOption { no: 2, label: "卖东西".into() }],
        accepts_text: false,
        ..Default::default()
    });
    let s = draw(&mut a, 120, 40);
    for want in ["特产品贩卖员", "1. 买东西", "2. 卖东西", "数字选择 · Esc 关闭"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
}


