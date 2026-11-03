//! 带选项的 NPC 对话窗口正文高度（STAGE-281）：正文按实际行数显示，不再固定 3 行，
//! 宠物店卖宠（5 行）、学技能（4 行）的价格行不被裁掉；小窗口下正文仍可 PgUp/PgDn 翻页。
//! 窗口带 window_id，走 dialog_menu 的渲染路径（服务端驱动的确认页）。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Location, NpcOption, NpcWindow, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
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

/// 打开一个带选项的服务端驱动窗口（window_id 非空，走 dialog_menu）。
fn app_with_window(text: &str, options: Vec<NpcOption>) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance {
            body: 3,
            color: 2,
            eye: 1,
            mouth: 4,
        },
        location: Location {
            floor: 1000,
            map_name: "加加村".into(),
        },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.npc_window = Some(NpcWindow {
        window_id: "shop-test".into(),
        npc: 1419,
        name: "宠物店".into(),
        type_: "卖宠".into(),
        text: text.into(),
        options,
        ..Default::default()
    });
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn option(no: u32, label: &str) -> NpcOption {
    NpcOption {
        no,
        label: label.into(),
    }
}

/// 某个子串所在的行号（画面按行拼接，第 0 行是终端第一行）。
fn row_of(screen: &str, needle: &str) -> Option<usize> {
    screen.lines().position(|l| l.contains(needle))
}

#[test]
fn shop_sell_confirm_shows_price_line_with_five_body_lines() {
    let mut a = app_with_window(
        "是[小白]没错吧？\n\n卖掉之后就不能再找回来了喔。\n\n一般的价格吧。\n\n    买卖价格         1234 STONE",
        vec![option(1, "确定")],
    );
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("买卖价格"), "卖宠确认页应显示价格行:\n{screen}");
    assert!(screen.contains("1234 STONE"), "价格数字应完整显示:\n{screen}");
    assert!(screen.contains("1. 确定"), "选项应显示:\n{screen}");
}

#[test]
fn skill_learn_confirm_shows_price_line_with_four_body_lines() {
    let mut a = app_with_window(
        "学习[冲撞]（攻击）\n给[小白]的 2 号技能栏位（现在是空的）\n\n    学习价格  300 金币",
        vec![option(1, "是"), option(2, "否")],
    );
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("学习价格"), "学技能确认页应显示学习价格:\n{screen}");
    assert!(screen.contains("300 金币"), "价格数字应完整显示:\n{screen}");
    assert!(screen.contains("2. 否"), "选项应全部显示:\n{screen}");
}

#[test]
fn three_line_body_keeps_three_rows_before_options() {
    // 寄放确认（3 行正文）：正文仍占 3 行，选项紧跟其后，不被撑高。
    let mut a = app_with_window(
        "寄放[小白]\n\n    寄放价格            50 stone",
        vec![option(1, "确定"), option(2, "取消")],
    );
    let screen = draw(&mut a, 90, 28);
    let title = row_of(&screen, "寄放[小白]").expect("正文第一行应可见");
    let price = row_of(&screen, "寄放价格").expect("价格行应可见");
    let first_option = row_of(&screen, "1. 确定").expect("选项应可见");
    assert_eq!(price, title + 2, "正文三行应在连续的三行内:\n{screen}");
    assert_eq!(first_option, title + 3, "选项应紧跟正文（正文固定 3 行）:\n{screen}");
}

#[test]
fn short_body_with_options_stays_at_three_rows() {
    let mut a = app_with_window("你好啊", vec![option(1, "好")]);
    let screen = draw(&mut a, 90, 28);
    let text_row = row_of(&screen, "你好啊").expect("正文应可见");
    let option_row = row_of(&screen, "1. 好").expect("选项应可见");
    assert_eq!(option_row, text_row + 3, "1 行正文的选项位置应与原来一致:\n{screen}");
}

#[test]
fn small_window_clips_body_and_keeps_options_and_hint() {
    let mut body: Vec<String> = (1..=12).map(|i| format!("第{i}行台词")).collect();
    body.push("    买卖价格         1234 STONE".into());
    let mut a = app_with_window(&body.join("\n"), vec![option(1, "确定")]);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("1. 确定"), "80x24 下选项不能被挤掉:\n{screen}");
    assert!(screen.contains("PgUp/PgDn"), "80x24 下底部提示行不能被挤掉:\n{screen}");
    assert!(!screen.contains("买卖价格"), "正文超出可用高度时应被裁剪:\n{screen}");

    // PgDn 每次翻 5 行，正文可见 7 行，翻两次才到价格行；选项和提示行仍在。
    for _ in 0..2 {
        a.on_key(KeyEvent::new(KeyCode::PageDown, KeyModifiers::NONE));
    }
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("买卖价格"), "PgDn 后应能翻到价格行:\n{screen}");
    assert!(screen.contains("1. 确定"), "翻页后选项仍应显示:\n{screen}");
    assert!(screen.contains("PgUp/PgDn"), "翻页后底部提示行仍应显示:\n{screen}");
}
