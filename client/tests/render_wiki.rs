//! 百科页（STAGE-83）的界面与命令测试：分类目录、进分类、搜索、打开词条、翻页、窄屏降级，
//! `docs/ui.md`「百科页」。数据是编译时嵌入的真实快照。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::WorldEntry;
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() });
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    a
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

#[test]
fn key_9_opens_wiki_with_category_directory() {
    let mut a = app(true);
    key(&mut a, KeyCode::Char('9'));
    let s = draw(&mut a, 120, 40);
    // STAGE-131：顶部常驻标签换成导航路径，切到百科页后路径写「场景 › 百科」。
    for want in ["场景 › 百科", "分类", "任务", "宠物与生物", "装备", "wiki find 关键词", "9874"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    // STAGE-185：左侧目录不列这三类。
    for hidden in ["未放置 NPC 模板", "本服任务验证", "新增与修改指南"] {
        assert!(!s.contains(hidden), "目录里不该有 {hidden}\n{s}");
    }
}

#[test]
fn category_by_number_lists_numbered_rows() {
    let mut a = app(true);
    command(&mut a, "wiki 2");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("宠物与生物 · 共 952 条"), "{s}");
    assert!(s.contains("乌力"), "宠物列表第一页应有乌力\n{s}");
    assert!(s.contains("第 1/"), "{s}");
    // 中文名字也行
    let mut b = app(true);
    command(&mut b, "wiki 装备");
    assert!(draw(&mut b, 120, 40).contains("装备 · 共 9874 条"));
}

#[test]
fn unknown_category_is_reported() {
    let mut a = app(true);
    command(&mut a, "wiki 没有这个");
    assert!(last_message(&a).contains("没有叫“没有这个”的分类"), "{}", last_message(&a));
}

#[test]
fn find_then_open_shows_detail_side_by_side() {
    let mut a = app(true);
    command(&mut a, "wiki pet");
    command(&mut a, "wiki find 乌力");
    assert!(last_message(&a).contains("找到"), "{}", last_message(&a));
    let s = draw(&mut a, 140, 40);
    assert!(s.contains("搜索“乌力”"), "{s}");
    command(&mut a, "wiki open 1");
    let s = draw(&mut a, 140, 40);
    assert!(s.contains("词条 ·"), "宽屏打开词条后右边出现详情\n{s}");
    assert!(s.contains("编号：pet:"), "{s}");
    assert!(s.contains("搜索“乌力”"), "宽屏时列表仍在\n{s}");
}

#[test]
fn narrow_screen_detail_replaces_list_and_back_returns() {
    let mut a = app(true);
    command(&mut a, "wiki pet");
    command(&mut a, "wiki open 1");
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("词条 ·"), "{s}");
    assert!(!s.contains("宠物与生物 · 共"), "窄屏打开词条时不画列表\n{s}");
    command(&mut a, "wiki back");
    let s = draw(&mut a, 80, 24);
    // 提示行里的“Tab 词条 ·”也含“词条”，所以认详情面板的标题。
    assert!(s.contains("宠物与生物 · 共 952 条") && !s.contains("词条 · 乌力"), "{s}");
}

#[test]
fn open_by_key_works_from_any_state() {
    let mut a = app(true);
    command(&mut a, "wiki open pet:1");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("词条 · 乌力"), "{s}");
    command(&mut a, "wiki open pet:999999999");
    assert!(last_message(&a).contains("找不到词条"), "{}", last_message(&a));
    command(&mut a, "wiki open 1");
    assert!(last_message(&a).contains("列表里没有第 1 条"), "还没有列表时按编号打开要报错\n{}", last_message(&a));
}

#[test]
fn paging_by_command_and_keys() {
    let mut a = app(true);
    command(&mut a, "wiki 3"); // 装备 9874 条，肯定多页
    let first = draw(&mut a, 120, 40);
    assert!(first.contains("第 1/"), "{first}");
    command(&mut a, "wiki next");
    let second = draw(&mut a, 120, 40);
    assert!(second.contains("第 2/"), "{second}");
    key(&mut a, KeyCode::PageUp);
    assert!(draw(&mut a, 120, 40).contains("第 1/"));
    command(&mut a, "wiki prev");
    assert_eq!(last_message(&a), "已经是第一页");
}

#[test]
fn detail_scrolls_with_page_keys_and_clamps() {
    let mut a = app(true);
    command(&mut a, "wiki open battle_npc:npc/100/sb_nusu.create@25-0");
    let top = draw(&mut a, 120, 30);
    assert!(top.contains("PgUp/PgDn 滚动"), "长词条要提示滚动\n{top}");
    for _ in 0..20 {
        key(&mut a, KeyCode::PageDown);
        draw(&mut a, 120, 30);
    }
    let end = draw(&mut a, 120, 30);
    assert_ne!(top, end);
    assert!(end.contains("相关词条"), "滚到底应看到相关词条\n{end}");
    for _ in 0..20 {
        key(&mut a, KeyCode::PageUp);
        draw(&mut a, 120, 30);
    }
    assert_eq!(top, draw(&mut a, 120, 30), "滚回顶部和刚打开时一样");
}

#[test]
fn ascii_fallback_has_no_box_drawing_in_detail() {
    let mut a = app(false);
    command(&mut a, "wiki open battle_npc:npc/100/sb_nusu.create@25-0");
    let s = draw(&mut a, 140, 40);
    assert!(s.contains("-- 战斗阵容") && s.contains("敌人 | 敌人ID"), "ASCII 降级的小节和表头\n{s}");
    assert!(!s.contains("敌人 │") && !s.contains("── 战斗阵容"), "详情行在 --unicode false 时不能用画线字符\n{s}");
}

#[test]
fn back_peels_one_layer_at_a_time() {
    let mut a = app(true);
    command(&mut a, "wiki pet");
    command(&mut a, "wiki find 乌力");
    command(&mut a, "wiki open 1");
    command(&mut a, "wiki back"); // 关词条
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("搜索“乌力”") && !s.contains("词条 · 乌力"), "{s}");
    command(&mut a, "wiki back"); // 清搜索
    assert!(draw(&mut a, 120, 40).contains("宠物与生物 · 共 952 条"));
    command(&mut a, "wiki back"); // 回目录
    assert!(draw(&mut a, 120, 40).contains("wiki find 关键词"));
}

#[test]
fn all_categories_reachable_at_minimum_size() {
    let mut a = app(true);
    key(&mut a, KeyCode::Char('9'));
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("共 11 类"), "放不下全部分类时底边提示总数\n{s}");
    command(&mut a, "wiki 11");
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("Agent"), "进了第 11 类，分类栏要滚到它\n{s}");
}

#[test]
fn every_category_renders_at_every_size() {
    let mut a = app(true);
    for c in 1..=14 {
        command(&mut a, &format!("wiki {c}"));
        command(&mut a, "wiki open 1");
        for (w, h) in [(80, 24), (110, 30), (200, 60)] {
            draw(&mut a, w, h);
        }
    }
}
