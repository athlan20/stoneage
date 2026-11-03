//! 任务攻略（STAGE-89）的界面与命令测试，`docs/ui.md` 任务页：`quest 编号` 打开攻略、宽窄两种布局、
//! 没有攻略的灰色提示、多篇词条、进度标注、`quest 编号 wiki` 跳到百科、`quest back` 和 Esc 回列表。
//! 数据是编译时嵌入的真实百科快照和对照表。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, QUEST_MENU, Screen, WIKI_MENU, WorldState};
use stoneage_client::proto::{Appearance, Character, Location, Quest, Quests, WorldEntry};
use stoneage_client::ui;
use stoneage_client::wiki::wiki;
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

fn command(app: &mut App, line: &str) {
    if app.world.as_ref().is_some_and(|w|!w.nav.is(&stoneage_client::app::Layer::Command)){app.on_key(crossterm::event::KeyEvent::new(crossterm::event::KeyCode::Char('/'),crossterm::event::KeyModifiers::NONE));}
    if !app.world.as_ref().unwrap().nav.is(&stoneage_client::app::Layer::Command) { key(app, KeyCode::Char('/')); }
    for c in line.chars() {
        key(app, KeyCode::Char(c));
    }
    key(app, KeyCode::Enter);
}

fn last_message(app: &App) -> String {
    app.log.recent(1).next().map(|m| m.text.clone()).unwrap_or_default()
}

fn quest(no: u32, name: &str, state: &str) -> Quest {
    Quest { no, name: name.into(), state: state.into() }
}

/// 进入世界并切到「5 任务」页，列表是 quests。
fn quest_page(quests: Vec<Quest>) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1006, map_name: "萨姆吉尔村长之家".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.quests = Quests { quests, changed: vec![] };
    world.menu = QUEST_MENU;
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn doing_pair() -> App {
    quest_page(vec![quest(6, "恐龙博士 抓鲁尼帖斯", "doing"), quest(7, "恐龙博士 抓贝鲁卡", "doing")])
}

#[test]
fn wide_screen_shows_list_and_guide_side_by_side() {
    let mut a = doing_pair();
    command(&mut a, "quest 6");
    let s = draw(&mut a, 120, 30);
    assert!(s.contains("任务 · 进行中 2"), "列表还在\n{s}");
    assert!(s.contains("攻略 · 恐龙博士 抓鲁尼帖斯"), "攻略标题\n{s}");
    assert!(s.contains("当前进度：第 1 步 / 共 2 步"), "同一篇攻略的第几步\n{s}");
    assert!(s.contains("为恐龙博士培养指定宠物"), "词条说明\n{s}");
}

#[test]
fn narrow_screen_replaces_list_with_guide() {
    let mut a = doing_pair();
    command(&mut a, "quest 7");
    let s = draw(&mut a, 100, 30);
    assert!(!s.contains("任务 · 进行中"), "窄屏时攻略替换列表\n{s}");
    assert!(s.contains("攻略 · 恐龙博士 抓贝鲁卡") && s.contains("当前进度：第 2 步 / 共 2 步"), "攻略内容\n{s}");
}

#[test]
fn task_without_guide_shows_hint_and_wiki_find_suggestion() {
    let mut a = quest_page(vec![quest(9, "水田迷宫", "doing")]);
    command(&mut a, "quest 9");
    let s = draw(&mut a, 120, 30);
    assert!(s.contains("百科里暂时没有这个任务的攻略"), "灰色提示\n{s}");
    assert!(s.contains("可以试试：wiki find 水田迷宫"), "建议命令\n{s}");
}

#[test]
fn task_with_two_entries_numbers_each_guide() {
    let mut a = quest_page(vec![quest(36, "琉璃洞窟", "doing")]);
    command(&mut a, "quest 36");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("攻略 1/2"), "第一篇标号\n{s}");
    assert!(!s.contains("当前进度"), "只有一步的攻略不标进度\n{s}");
    // 两篇词条的详情很长，第二篇要 PgDn 翻过去才看得到。
    // 宽屏打开攻略后焦点留在列表栏，Tab 进攻略栏才翻攻略（docs/ui.md 任务页）。
    key(&mut a, KeyCode::Tab);
    for _ in 0..40 {
        if draw(&mut a, 120, 40).contains("攻略 2/2") {
            break;
        }
        key(&mut a, KeyCode::PageDown);
    }
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("攻略 2/2") && s.contains("完成伊布母亲支线"), "第二篇标号和它的说明\n{s}");
}

#[test]
fn quest_back_and_esc_return_to_the_list() {
    let mut a = doing_pair();
    command(&mut a, "quest 6");
    command(&mut a, "quest back");
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("任务 · 进行中 2") && !s.contains("攻略 · "), "quest back 回到列表\n{s}");

    command(&mut a, "quest 7");
    key(&mut a, KeyCode::Esc);
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("任务 · 进行中 2") && !s.contains("攻略 · "), "Esc 回到列表\n{s}");
}

#[test]
fn filter_command_closes_the_guide() {
    let mut a = doing_pair();
    command(&mut a, "quest 6");
    command(&mut a, "quest doing");
    let s = draw(&mut a, 100, 30);
    assert!(!s.contains("攻略 · "), "切换范围时回到列表\n{s}");
}

#[test]
fn quest_number_jumps_to_wiki_entry() {
    let mut a = doing_pair();
    command(&mut a, "quest 6 wiki");
    let world = a.world.as_ref().unwrap();
    assert_eq!(world.menu, WIKI_MENU, "跳到百科页");
    let open = world.wiki.open.as_ref().expect("打开了词条");
    assert_eq!(wiki().row(open.hit).key, "quest:b2", "第一篇攻略是恐龙博士");
}

#[test]
fn wiki_jump_without_guide_reports_it() {
    let mut a = quest_page(vec![quest(9, "水田迷宫", "doing")]);
    command(&mut a, "quest 9 wiki");
    assert_eq!(a.world.as_ref().unwrap().menu, QUEST_MENU, "没有攻略时留在任务页");
    assert!(last_message(&a).contains("任务【水田迷宫】在百科里没有攻略"), "{}", last_message(&a));
}

#[test]
fn unknown_task_number_is_an_error() {
    let mut a = doing_pair();
    command(&mut a, "quest 99");
    assert!(last_message(&a).contains("任务列表里没有编号 99 的任务"), "{}", last_message(&a));
    assert!(a.world.as_ref().unwrap().quest_guide.is_none());
}

#[test]
fn bad_usage_shows_the_full_usage_line() {
    let mut a = doing_pair();
    command(&mut a, "quest 6 乱来");
    assert!(last_message(&a).contains("quest [doing|done|all] | 编号 [wiki] | back"), "{}", last_message(&a));
}
