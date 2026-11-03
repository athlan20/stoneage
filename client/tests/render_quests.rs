//! 任务页（STAGE-86）的界面测试，`docs/ui.md` 任务页：空提示、表格与标题计数、显示范围的标题和空提示、PgUp/PgDn 翻页。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, QUEST_MENU, QuestFilter, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Location, Quest, Quests, WorldEntry};
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

#[test]
fn empty_list_shows_hint_and_zero_counts() {
    let mut a = quest_page(vec![]);
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("任务 · 进行中 0 · 已完成 0"), "标题计数\n{s}");
    assert!(s.contains("还没有接到任务，去和村里的 NPC 聊聊吧"), "空提示\n{s}");
    assert!(!s.contains("编号"), "没有任务时不画表头\n{s}");
}

#[test]
fn list_shows_table_with_states_and_counts() {
    let mut a = quest_page(vec![quest(3, "猜谜3兄弟 拿蓝暴", "doing"), quest(1, "成人仪式", "done")]);
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("任务 · 进行中 1 · 已完成 1"), "标题计数\n{s}");
    for head in ["编号", "任务", "状态"] {
        assert!(s.contains(head), "表头 {head}\n{s}");
    }
    assert!(s.contains("猜谜3兄弟 拿蓝暴") && s.contains("进行中"), "进行中的任务\n{s}");
    assert!(s.contains("成人仪式") && s.contains("已完成"), "已完成的任务\n{s}");
    // 进行中在前：猜谜3兄弟 拿蓝暴 那一行在 成人仪式 之前。
    let doing = s.find("猜谜3兄弟 拿蓝暴").unwrap();
    let done = s.find("成人仪式").unwrap();
    assert!(doing < done, "进行中排在前面\n{s}");
}

#[test]
fn filter_changes_title_and_empty_hint() {
    let mut a = quest_page(vec![quest(3, "猜谜3兄弟 拿蓝暴", "doing"), quest(1, "成人仪式", "done")]);
    a.world.as_mut().unwrap().quest_filter = QuestFilter::Doing;
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("· 只看进行中"), "标题写明范围\n{s}");
    assert!(s.contains("猜谜3兄弟 拿蓝暴") && !s.contains("成人仪式"), "只列进行中\n{s}");

    // 只有进行中的任务时选“只看已完成”：给出空提示，标题计数不变。
    let mut a = quest_page(vec![quest(3, "猜谜3兄弟 拿蓝暴", "doing")]);
    a.world.as_mut().unwrap().quest_filter = QuestFilter::Done;
    let s = draw(&mut a, 100, 30);
    assert!(s.contains("没有已完成的任务"), "空的范围\n{s}");
    assert!(s.contains("进行中 1 · 已完成 0"), "计数不受范围影响\n{s}");
}

#[test]
fn page_down_scrolls_a_full_page_and_stops_at_the_end() {
    let quests: Vec<Quest> = (1..=30).map(|n| quest(n, &format!("任务{n:02}"), "done")).collect();
    let mut a = quest_page(quests);
    let s = draw(&mut a, 100, 24);
    assert!(s.contains("任务01") && !s.contains("任务10"), "第一页从 1 开始\n{s}");

    key(&mut a, KeyCode::PageDown);
    let s = draw(&mut a, 100, 24);
    assert!(s.contains("任务10") && !s.contains("任务01"), "翻到下一页\n{s}");

    // 一直往下翻，最后一页贴底，不会空白。
    for _ in 0..10 {
        key(&mut a, KeyCode::PageDown);
    }
    let s = draw(&mut a, 100, 24);
    assert!(s.contains("任务30"), "最后一页包含最后一个任务\n{s}");

    // 焦点在列表上：PgUp 选中项跳一屏（9 行），列表跟着选中项走；选中项到了上边缘，它就是第一行。
    // 原来 30 项末尾选中的是任务30，往上一屏是任务21。
    key(&mut a, KeyCode::PageUp);
    let s = draw(&mut a, 100, 24);
    assert!(s.contains("任务21") && !s.contains("任务20") && s.contains("▶21"), "向上翻一屏，选中项在第一行\n{s}");
}
