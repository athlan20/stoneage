//! 任务 NPC 的分页台词（STAGE-97，`docs/ui.md`「服务端驱动的界面」）：窗口正文是当前页，选项按页列出——
//! 非最后一页是“下一页”（第 2 页起加“上一页”），最后一页的问话才是“是/否”（同样可加“上一页”）。
//! 窗口由测试直接塞进应用状态（服务端的推送形状见 docs/protocol.md 5.34、5.35）。

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

/// 进入世界，打开一个 NPC 窗口（当前页的正文和选项由参数给）。
fn world_with_page(text: &str, options: Vec<NpcOption>) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "加加村".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.npc_window = Some(NpcWindow {
        npc: 1419,
        name: "库伊老爷爷".into(),
        type_: "任务".into(),
        text: text.into(),
        options,
        accepts_text: false,
        ..Default::default()
    });
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn option(no: u32, label: &str) -> NpcOption {
    NpcOption { no, label: label.into() }
}

#[test]
fn first_page_offers_only_next() {
    let mut a = world_with_page("好像有个叫新藏的年轻人", vec![option(1, "下一页")]);
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("好像有个叫新藏的年轻人"), "窗口应显示当前页的正文:\n{screen}");
    assert!(screen.contains("1. 下一页"), "第 1 页应列出“下一页”:\n{screen}");
    assert!(!screen.contains("上一页"), "第 1 页不应有“上一页”:\n{screen}");
}

#[test]
fn middle_page_offers_next_and_prev() {
    let mut a = world_with_page("什么，有只卡卡金宝挡住洞口了吗？", vec![option(1, "下一页"), option(2, "上一页")]);
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("1. 下一页"), "中间页应列出“下一页”:\n{screen}");
    assert!(screen.contains("2. 上一页"), "中间页应列出“上一页”:\n{screen}");
}

#[test]
fn last_question_page_offers_yes_no_then_prev() {
    let mut a = world_with_page(
        "咦，我刚刚是不是已经给过你护身符了啊？",
        vec![option(1, "是"), option(2, "否"), option(3, "上一页")],
    );
    let screen = draw(&mut a, 90, 28);
    assert!(screen.contains("1. 是"), "最后一页的问话应列出“是”:\n{screen}");
    assert!(screen.contains("2. 否"), "最后一页的问话应列出“否”:\n{screen}");
    assert!(screen.contains("3. 上一页"), "最后一页的问话应在最后列出“上一页”:\n{screen}");
    assert!(!screen.contains("下一页"), "最后一页不应有“下一页”:\n{screen}");
}
