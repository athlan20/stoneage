//! 任务 NPC 的暗号（STAGE-92，`docs/ui.md`「服务端驱动的界面」）：闲聊窗口上的“说暗号”选项用数字选；
//! 文字输入窗口（accepts_text）提示 `reply 文本`，数字键是文字而不是关窗或选项。
//! 窗口由测试直接塞进应用状态（服务端的推送形状见 docs/protocol.md 5.34）。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Location, NpcOption, NpcWindow, WorldEntry};
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

/// 进入世界，打开一个 NPC 窗口（正文、选项和是否接受文字由参数给）。
fn world_with_window(text: &str, options: Vec<NpcOption>, accepts_text: bool) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 10204, map_name: "加加村".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.npc_window = Some(NpcWindow {
        npc: 1272,
        name: "检查人员(A-1)".into(),
        type_: "任务".into(),
        text: text.into(),
        options,
        accepts_text,
        ..Default::default()
    });
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn keyword_option() -> Vec<NpcOption> {
    vec![NpcOption { no: 1, label: "说暗号".into() }]
}

#[test]
fn keyword_option_shows_as_numbered_choice() {
    let mut a = world_with_window("加鲁卡全岛巡回活动", keyword_option(), false);
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("1. 说暗号"), "闲聊窗口应列出“说暗号”选项:\n{screen}");
    assert!(screen.contains("数字选择"), "有选项的窗口提示数字选择:\n{screen}");
}

#[test]
fn text_window_hints_reply_and_hides_options() {
    let mut a = world_with_window("你想说什么？", vec![], true);
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("reply 文本"), "文字输入窗口应提示 reply 文本:\n{screen}");
    assert!(!screen.contains("说暗号"), "文字输入窗口不列选项:\n{screen}");
}

#[test]
fn digits_are_text_in_text_window_and_close_plain_window() {
    // 文字输入窗口：数字键是输入行里的字，窗口不关。
    let mut a = world_with_window("你想说什么？", vec![], true);
    key(&mut a, KeyCode::Char('1'));
    let world = a.world.as_ref().unwrap();
    assert_eq!(world.input.text(), "1", "文字窗口里的数字应该进输入行");
    assert!(world.npc_window.is_some(), "文字窗口里按数字不应该关窗");

    // 没有选项、也不接受文字的窗口：按数字关窗（原有行为）。
    let mut a = world_with_window("……", vec![], false);
    key(&mut a, KeyCode::Char('1'));
    let world = a.world.as_ref().unwrap();
    assert!(world.npc_window.is_none(), "普通窗口按数字应关掉");
    assert_eq!(world.input.text(), "", "普通窗口的数字不进输入行");
}
