//! 消息页（STAGE-27）的界面测试：历史消息、翻页锚定、换行、上限与提示，`docs/ui.md`「消息页」。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use ratatui::buffer::Buffer;
use ratatui::style::Color;
use stoneage_client::app::{App, Kind, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Location, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    draw_buf(app, w, h).0
}

fn draw_buf(app: &mut App, w: u16, h: u16) -> (String, Buffer) {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    // 宽字符（中文）占两格，第二格是占位，拼接时要跳过。
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

fn key(app: &mut App, code: KeyCode) {
    app.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

/// 进入世界并切到「7 消息」页；先画一遍让页面记住可用行数（80×24 下消息页 16 行：底部只剩输入行，STAGE-111）。
fn messages_page(n: usize) -> App {
    let mut a = app();
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
    };
    let mut w = WorldState::new(entry);
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    for i in 1..=n {
        a.log.system(format!("第{i}条消息"));
    }
    key(&mut a, KeyCode::Char('7'));
    a
}

#[test]
fn messages_page_lists_history_with_range_title() {
    let mut a = messages_page(30);
    let s = draw(&mut a, 80, 24);
    // 80×24 的中间区域给消息页 16 行（底部只剩输入行）：跟随最新显示第 15–30 条。
    assert!(s.contains("消息 · 第 15–30 / 30 条"), "标题缺范围\n{s}");
    assert!(s.contains("第30条消息"), "最新的在最下面\n{s}");
    assert!(!s.contains("第14条消息"), "一屏只放得下 16 条\n{s}");
}

#[test]
fn messages_page_wraps_long_chinese_lines() {
    let mut a = messages_page(0);
    // 80 列窗口里正文宽 70 列（76 减 6 列标签），40 个汉字 80 列要拆成两行。
    let text = "一二三四五六七八九十壹贰叁肆伍陆柒捌玖零甲乙丙丁戊己庚辛壬癸子丑寅卯辰巳午未申酉";
    assert_eq!(text.width(), 80);
    a.log.agent(text);
    let s = draw(&mut a, 80, 24);
    let first = s.lines().find(|l| l.contains("agent")).expect("第一行带标签");
    assert!(first.contains("一二三四五六七八九十壹贰叁肆伍陆柒捌玖零甲乙丙丁戊己庚辛壬癸子丑寅卯辰巳"), "第一行放下前 35 个字\n{s}");
    let second = s.lines().find(|l| l.contains("午未申酉")).expect("长句要换行");
    assert!(second.starts_with("│      午"), "续行缩进到标签后面（6 列）\n{s}");
    // 两行拼起来是原文，没有丢字（去掉边框、标签和缩进空格）。
    let strip = |l: &str| -> String { l.chars().filter(|c| *c != '│' && !c.is_whitespace()).collect() };
    let first_stripped = strip(first).replace("agent", "");
    assert_eq!(format!("{first_stripped}{}", strip(second)), text);
}

#[test]
fn paging_home_end_and_bounds() {
    let mut a = messages_page(30);
    draw(&mut a, 80, 24); // 更新页面行数
    key(&mut a, KeyCode::PageUp);
    // 16 行一屏，PgUp 翻 15 行：从 15–30 翻到顶（第 1–16 条），和上一屏重叠一行。
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 1–16 / 30 条"), "PgUp 翻一页少一行重叠");
    key(&mut a, KeyCode::PageUp);
    key(&mut a, KeyCode::PageUp);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("消息 · 第 1–16 / 30 条"), "翻到顶\n{s}");
    key(&mut a, KeyCode::PageUp); // 到顶再翻不越界
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 1–16 / 30 条"));
    key(&mut a, KeyCode::Home);
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 1–16 / 30 条"));
    key(&mut a, KeyCode::End);
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 15–30 / 30 条"), "End 回到最新");
    key(&mut a, KeyCode::PageDown); // 已在最新，再翻没有变化
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 15–30 / 30 条"));
}

#[test]
fn anchored_view_keeps_position_and_shows_unseen_hint() {
    let mut a = messages_page(30);
    draw(&mut a, 80, 24);
    key(&mut a, KeyCode::PageUp); // 停在第 1–16 条
    for i in 31..=33 {
        a.log.system(format!("第{i}条消息"));
    }
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("消息 · 第 1–16 / 33 条"), "翻到旧位置后新消息不顶走视图\n{s}");
    assert!(s.contains("有 3 条新消息 ↓"), "要有新消息提示\n{s}");
    key(&mut a, KeyCode::End);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("消息 · 第 18–33 / 33 条"), "End 恢复跟随\n{s}");
    assert!(!s.contains("新消息"), "提示要消失\n{s}");
}

#[test]
fn home_end_need_empty_input() {
    let mut a = messages_page(30);
    draw(&mut a, 80, 24);
    key(&mut a, KeyCode::Char('/'));
    for c in "ab".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Home); // 输入行有内容：移动光标，不滚动
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 15–30 / 30 条"), "有内容时 Home 不滚动\n");
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Home);
    assert!(draw(&mut a, 80, 24).contains("消息 · 第 1–16 / 30 条"), "输入行清空后 Home 回到最早\n");
}

#[test]
fn leaving_and_returning_keeps_the_history_anchor() {
    let mut a = messages_page(30);
    draw(&mut a, 80, 24);
    key(&mut a, KeyCode::PageUp);
    let anchor=a.world.as_ref().unwrap().msg_view.anchor;
    for command in ["page 地图", "page 消息"] {
        key(&mut a, KeyCode::Char('/'));
        for c in command.chars(){key(&mut a, KeyCode::Char(c));}
        key(&mut a, KeyCode::Enter);
    }
    draw(&mut a, 80, 24);
    assert_eq!(a.world.as_ref().unwrap().msg_view.anchor,anchor,"返回后保留原阅读位置");
}

#[test]
fn empty_log_shows_hint() {
    let mut a = messages_page(0);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("消息 · 暂无"), "标题写暂无\n{s}");
    assert!(s.contains("本次登录以来还没有消息"), "中间要有灰色提示\n{s}");
}

#[test]
fn cap_at_1000_drops_oldest() {
    let mut a = messages_page(1200);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("消息 · 第 1185–1200 / 1200 条"), "只保留最近 1000 条\n{s}");
    key(&mut a, KeyCode::Home);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("消息 · 第 201–216 / 1200 条"), "最早的现存消息是第 201 条\n{s}");
    assert!(!s.contains("第1条消息"), "第 1 条已被丢弃\n{s}");
}

/// 验收 1：消息页与底部消息栏是同一套标签配色。消息页上底部消息栏已隐藏（STAGE-111），所以在地图页上比对。
#[test]
fn colors_match_bottom_bar() {
    let mut a = messages_page(0);
    a.log.error("出问题了"); // 正文避开“错”字，好按标签数
    let (_, buf) = draw_buf(&mut a, 80, 24);
    // 消息页上只有一处（消息页自己）。
    let page: Vec<_> = buf.content().iter().filter(|c| c.symbol() == "错").collect();
    assert_eq!(page.len(), 1, "消息页上错误消息只有一处");
    assert!(page.iter().all(|c| c.style().fg == Some(Color::Red)), "消息页上的标签是红色");
    // 切到地图页：底部消息栏里的同一条消息，标签也是红色。
    key(&mut a, KeyCode::Char('1'));
    let (_, buf) = draw_buf(&mut a, 80, 24);
    let cells: Vec<_> = buf.content().iter().filter(|c| c.symbol() == "错").collect();
    assert_eq!(cells.len(), 1, "错误消息出现在底部消息栏");
    assert!(cells.iter().all(|c| c.style().fg == Some(Color::Red)), "底部消息栏的标签是红色");
    key(&mut a, KeyCode::Char('7'));
    // agent 标签是洋红色。
    a.log.agent("agent 说");
    let (_, buf) = draw_buf(&mut a, 80, 24); // 回到消息页
    let agent_cells: Vec<_> = buf.content().iter().filter(|c| c.symbol() == "a" && c.style().fg == Some(Color::Magenta)).collect();
    assert!(!agent_cells.is_empty(), "agent 标签是洋红色");
    let _ = Kind::Command; // 命令标签青色由同一套 Kind::color 保证
}

#[test]
fn survives_every_size() {
    let mut a = messages_page(30);
    a.log.agent("一二三四五六七八九十壹贰叁肆伍陆柒捌玖零甲乙丙丁戊己庚辛壬癸子丑寅卯辰巳午未申酉");
    draw(&mut a, 80, 24);
    key(&mut a, KeyCode::PageUp);
    for (w, h) in [(1, 1), (79, 23), (80, 24), (120, 40), (200, 60)] {
        draw(&mut a, w, h);
    }
    a.world.as_mut().unwrap().msg_view.rows = 0; // 还没画过就翻页也不怕
    key(&mut a, KeyCode::PageUp);
    draw(&mut a, 80, 24);
}
