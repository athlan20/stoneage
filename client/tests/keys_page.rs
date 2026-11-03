//! 方向键逛页面的公共部分（STAGE-113，`docs/interaction.md`「页面选择」「历史命令」「提示行」）：
//! 空行 `←→` 切菜单页（循环），有字时 `←→` 移动光标；`↑↓` 没有可选的列表时翻历史，有字时按前缀翻；
//! 弹窗、确认开着时方向键归它们；提示行写 `←→ 栏目` 加当前页的键。各页的栏在 114–116 接入，这里的页都还没有栏。
//! 不连服务端：只看本地状态和输入行的文字。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use unicode_width::UnicodeWidthStr;
use stoneage_client::app::{App, Confirm, Options, Screen, WorldState};
use stoneage_client::proto::{NpcOption, NpcWindow, WorldEntry};
use stoneage_client::ui;

const WIDTH: u16 = 120;
const HEIGHT: u16 = 40;

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    })
}

/// 进入世界、没有任何弹窗的应用。
fn world_app() -> App {
    let mut a = app();
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().unwrap()
}

fn world_mut(a: &mut App) -> &mut WorldState {
    a.world.as_mut().unwrap()
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

/// 输入一整行并回车（和手敲命令一样）。
fn line(a: &mut App, s: &str) {
    text(a, s);
    key(a, KeyCode::Enter);
}

fn draw(a: &mut App) -> String {
    let mut term = Terminal::new(TestBackend::new(WIDTH, HEIGHT)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..HEIGHT)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < WIDTH {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 输入行：提示就画在这一行右边（和 keys_hint.rs 一样取倒数第二行）。
fn input_row(a: &mut App) -> String {
    draw(a).lines().rev().nth(1).unwrap_or_default().to_string()
}

fn window(options: Vec<NpcOption>) -> NpcWindow {
    NpcWindow {
        npc: 225,
        name: "村长家的厨师".into(),
        options,
        ..NpcWindow::default()
    }
}

/// 有几条历史：`wa1`、`help`、`wa2`（`help` 是本地命令，`wa*` 是未知命令，都只进历史）。
fn with_history() -> App {
    let mut a = world_app();
    line(&mut a, "wa1");
    line(&mut a, "help");
    line(&mut a, "wa2");
    a
}

#[test]
fn left_and_right_cycle_the_menu_pages_when_the_input_is_empty() {
    let mut a = world_app();
    key(&mut a, KeyCode::Right);
    assert_eq!(world(&a).menu, 0, "左右只切本页栏目");
    key(&mut a, KeyCode::Left);
    key(&mut a, KeyCode::Left);
    assert_eq!(world(&a).menu, 0, "左右不会离开地图页");
    key(&mut a, KeyCode::Right);
    assert_eq!(world(&a).menu, 0);
}

#[test]
fn left_and_right_move_the_cursor_when_text_is_typed() {
    let mut a = world_app();
    text(&mut a, "ab");
    key(&mut a, KeyCode::Left);
    assert_eq!(world(&a).menu, 0, "有字时 ←→ 不切页");
    text(&mut a, "x");
    assert_eq!(world(&a).input.text(), "axb", "光标往左移了一格，打的字插在中间");
}

#[test]
fn page_switching_is_not_blocked_by_the_agent_observing() {
    let mut a = world_app();
    world_mut(&mut a).entry.control.you = "observer".into();
    key(&mut a, KeyCode::Right);
    assert_eq!(world(&a).menu, 0, "观察模式下栏目浏览仍可用，左右不离开本页");
}

#[test]
fn an_open_dialog_keeps_the_arrows_from_switching_pages() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![NpcOption { no: 1, label: "买东西".into() }]));
    key(&mut a, KeyCode::Right);
    assert_eq!(world(&a).menu, 0, "弹窗开着时 ←→ 归弹窗，不切页");
}

#[test]
fn a_pending_confirm_keeps_the_arrows_from_switching_pages() {
    let mut a = world_app();
    world_mut(&mut a).confirm = Some(Confirm::PetDrop {
        slot: 0,
        name: "乌力".into(),
    });
    key(&mut a, KeyCode::Right);
    assert_eq!(world(&a).menu, 0);
}

#[test]
fn up_on_an_empty_line_recalls_the_latest_command_when_the_page_has_no_list() {
    let mut a = with_history();
    key(&mut a, KeyCode::Up);
    assert_eq!(world(&a).input.text(), "wa2", "没有列表的页空行 ↑ 照旧翻全部历史");
    key(&mut a, KeyCode::Up);
    assert_eq!(world(&a).input.text(), "help");
}

#[test]
fn up_with_text_recalls_only_commands_starting_with_it() {
    let mut a = with_history();
    text(&mut a, "wa");
    key(&mut a, KeyCode::Up);
    assert_eq!(world(&a).input.text(), "wa2");
    key(&mut a, KeyCode::Up);
    assert_eq!(world(&a).input.text(), "wa1", "跳过中间的 help");
    key(&mut a, KeyCode::Up);
    assert_eq!(world(&a).input.text(), "wa1", "前面没有以 wa 开头的了，不动");
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&a).input.text(), "wa", "越过最新一条，还原成开始翻时打的字");
}

#[test]
fn zero_does_nothing_when_the_input_is_empty() {
    let mut a = world_app();
    key(&mut a, KeyCode::Char('0'));
    assert!(world(&a).input.is_empty(), "空行的 0 不写进输入行");
    assert_eq!(world(&a).menu, 0, "0 不切页");
    text(&mut a, "x0");
    assert_eq!(world(&a).input.text(), "x0", "有字时 0 照常是文字");
}

#[test]
fn tab_and_enter_do_nothing_on_a_page_without_a_list() {
    let mut a = world_app();
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).input.is_empty());
    assert_eq!(world(&a).menu, 0);
    key(&mut a, KeyCode::Up);
    assert!(world(&a).input.is_empty(), "空行的 Enter 没有记入历史，↑ 也翻不出东西");
}

#[test]
fn the_hint_names_clone_creation_on_social_and_scrolling_on_messages() {
    let mut a = world_app();
    // STAGE-162 社交已开放：没有队伍时提示组成分身队，消息页仍提示滚动。
    key(&mut a, KeyCode::Char('6'));
    assert!(input_row(&mut a).contains("Enter 组成分身队 · / 命令"), "{}", input_row(&mut a));
    assert!(!input_row(&mut a).contains("1-9"), "提示只写当前页面可用的操作");
    key(&mut a, KeyCode::Char('7'));
    assert!(input_row(&mut a).contains("←→ 栏目 · ↑↓ 滚动 · End 最新"), "消息页的 ↑↓ 逐行滚动（STAGE-116）");
}

#[test]
fn the_hint_follows_the_page_and_the_layer_it_is_in() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![NpcOption { no: 1, label: "买东西".into() }]));
    assert!(input_row(&mut a).contains("1-1 选择"), "弹窗层的提示不变");
    world_mut(&mut a).npc_window = None;
    text(&mut a, "wa");
    assert!(!input_row(&mut a).contains("←→"), "有字时写的是命令的用法");
}
