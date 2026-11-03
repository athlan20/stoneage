//! 任务页和百科页的页面选择（STAGE-115，`docs/interaction.md`「页面选择」「按键表」、`docs/ui.md` 任务页、百科页）：
//! `↑↓` 移动列表的选中项，`Tab` 换到攻略或详情栏后 `↑↓` `PgUp` `PgDn` 滚动它，`Enter` 打开，`Esc` 退一层，
//! 窄屏时列表被详情替换、焦点只在详情栏上；命令（`quest 编号`、`wiki 分类`）执行后选中项跟过去。
//! 数据是编译时嵌入的真实百科快照和对照表；不连服务端。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, MAP_MENU, Options, QUEST_MENU, Screen, WIKI_MENU, WorldState, page_cursor};
use stoneage_client::proto::{Appearance, Character, Location, Quest, Quests, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

/// 进入世界（没有任何弹窗），菜单停在第 1 页；任务和攻略的测试再切过去。
fn world_app(quests: Vec<Quest>) -> App {
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
    a.world = Some(world);
    a.screen = Screen::World;
    a
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

/// 输入行（倒数第二行，提示画在这一行右边）。
fn input_row(a: &mut App) -> String {
    draw(a, 120, 40).lines().rev().nth(1).unwrap_or_default().to_string()
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

fn command(a: &mut App, line: &str) {
    if !a.world.as_ref().unwrap().nav.is(&stoneage_client::app::Layer::Command) { key(a, KeyCode::Char('/')); }
    text(a, line);
    key(a, KeyCode::Enter);
}

fn quest(no: u32, name: &str, state: &str) -> Quest {
    Quest { no, name: name.into(), state: state.into() }
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().unwrap()
}

/// 任务页：三条进行中的任务（6、7、36 有攻略），进入后切到第 5 页。
fn quest_app() -> App {
    let mut a = world_app(vec![
        quest(6, "恐龙博士 抓鲁尼帖斯", "doing"),
        quest(7, "恐龙博士 抓贝鲁卡", "doing"),
        quest(36, "琉璃洞窟", "doing"),
    ]);
    key(&mut a, KeyCode::Char('5'));
    assert_eq!(world(&a).menu, QUEST_MENU);
    a
}

/// 百科页：从第 9 页进入，分类目录。
fn wiki_app() -> App {
    let mut a = world_app(vec![]);
    key(&mut a, KeyCode::Char('9'));
    assert_eq!(world(&a).menu, WIKI_MENU);
    a
}

fn quest_cursor(a: &App) -> Option<(usize, bool)> {
    page_cursor(world(a), "任务")
}

fn wiki_cursor(a: &App, column: &str) -> Option<(usize, bool)> {
    page_cursor(world(a), column)
}

/// 数字键是兼容入口（docs/interaction.md 功能页）：任务页、百科页上按 1 要切回地图页，
/// 不能被页面自己的按键处理吞掉（这两页原来的 browse 处理会把数字键吞掉，见 STAGE-165）。
#[test]
fn digits_still_switch_pages_from_the_quest_and_wiki_pages() {
    let mut a = quest_app();
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(world(&a).menu, MAP_MENU, "任务页上按 1 切到地图页");

    let mut a = wiki_app();
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(world(&a).menu, MAP_MENU, "百科页上按 1 切到地图页");
}

// ---------- 任务页 ----------

#[test]
fn arrows_move_the_list_and_enter_opens_the_selected_guide() {
    let mut a = quest_app();
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).quest_guide.is_none(), "先打开任务动作");
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).quest_guide.as_ref().map(|g| g.no), Some(7));
    assert_eq!(quest_cursor(&a), Some((1, false)), "详情激活，列表保持原项");
}

#[test]
fn wide_guide_follows_the_list_selection_without_enter() {
    let mut a = quest_app();
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&a).quest_guide.as_ref().map(|g| g.no), Some(7), "阅读层只滚动，不切任务");
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).quest_guide.as_ref().map(|g| g.no), Some(36));
}

#[test]
fn tab_moves_into_the_guide_where_arrows_scroll_it() {
    let mut a = quest_app();
    draw(&mut a, 120, 40);
    command(&mut a, "quest 6");
    key(&mut a, KeyCode::Tab);
    assert_eq!(quest_cursor(&a), Some((0, false)), "Tab 后焦点在攻略栏，列表选中项仍在");
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    draw(&mut a, 120, 40);
    assert_eq!(world(&a).quest_guide.as_ref().unwrap().scroll, 2, "↓ 逐行滚动攻略");
    assert_eq!(world(&a).select[QUEST_MENU].slots[0].index, 0, "滚动攻略不移动列表的选中项");
    key(&mut a, KeyCode::Home);
    assert_eq!(world(&a).quest_guide.as_ref().unwrap().scroll, 0, "Home 滚到最前");
}

#[test]
fn narrow_screen_shows_only_the_guide_and_esc_brings_the_list_back() {
    let mut a = quest_app();
    draw(&mut a, 80, 24);
    command(&mut a, "quest 6");
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains("任务 · 进行中"), "窄屏时攻略替换列表\n{s}");
    assert_eq!(quest_cursor(&a), Some((0, false)), "焦点只能在攻略栏上，列表的选中行仍记着");
    key(&mut a, KeyCode::Tab);
    assert_eq!(quest_cursor(&a), Some((0, false)), "列表没画出来，Tab 不去那里");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).quest_guide.is_none(), "空行 Esc 关攻略");
    assert_eq!(quest_cursor(&a), Some((0, true)), "回到列表，焦点落在列表栏");
}

#[test]
fn esc_with_text_only_clears_the_input_and_keeps_the_guide() {
    let mut a = quest_app();
    command(&mut a, "quest 6");
    key(&mut a, KeyCode::Char('/'));
    text(&mut a, "x");
    key(&mut a, KeyCode::Esc);
    assert_eq!(world(&a).input.text(), "x", "命令栏关闭后保留草稿");
    assert!(world(&a).quest_guide.is_some());
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).quest_guide.is_none(), "下一次 Esc 返回列表");
}

#[test]
fn typed_quest_command_moves_the_list_selection() {
    let mut a = quest_app();
    draw(&mut a, 120, 40);
    command(&mut a, "quest 36");
    assert_eq!(quest_cursor(&a), Some((2, false)), "quest 36 之后选中的是第三行（任务 36）");
    assert!(world(&a).quest_guide.is_some());
}

#[test]
fn hint_lists_the_keys_of_the_open_layer() {
    let mut a = quest_app();
    let s = input_row(&mut a);
    assert!(s.contains("↑↓ 选择") && s.contains("Enter 动作") && s.contains("←→ 分类"), "{s}");
    command(&mut a, "quest 6");
    let s = input_row(&mut a);
    assert!(s.contains("↑↓ 阅读") && s.contains("Esc/Backspace 返回"), "攻略开着：{s}");
}

#[test]
fn page_down_on_the_list_jumps_one_screen_of_rows() {
    let quests: Vec<Quest> = (1..=30).map(|n| quest(n, &format!("任务{n:02}"), "done")).collect();
    let mut a = world_app(quests);
    key(&mut a, KeyCode::Char('5'));
    draw(&mut a, 100, 24);
    key(&mut a, KeyCode::PageDown);
    let rows = world(&a).quest_rows;
    assert!(rows >= 5, "测试终端能放下几行任务：{rows}");
    assert_eq!(quest_cursor(&a), Some((rows, true)), "PgDn 跳过一屏的行数");
    let s = draw(&mut a, 100, 24);
    assert!(s.contains(&format!("▶{}", rows + 1)), "列表跟着选中项，选中行可见\n{s}");
}

// ---------- 百科页 ----------

#[test]
fn enter_on_a_category_opens_it_and_lands_on_its_entries() {
    let mut a = wiki_app();
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Down);
    assert_eq!(wiki_cursor(&a, "分类"), Some((1, true)), "↓ 选中第二类");
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).wiki.category, Some(1), "Enter 进入这一类");
    assert_eq!(wiki_cursor(&a, "词条"), Some((0, true)), "焦点落到词条栏，选中第一条");
    key(&mut a, KeyCode::Down);
    assert_eq!(wiki_cursor(&a, "词条"), Some((1, true)));
}

#[test]
fn typed_category_command_moves_the_category_selection_and_the_focus() {
    let mut a = wiki_app();
    draw(&mut a, 120, 40);
    command(&mut a, "wiki 3");
    assert_eq!(wiki_cursor(&a, "分类"), Some((2, false)), "wiki 3 把分类的选中项定到第三类");
    assert_eq!(wiki_cursor(&a, "词条"), Some((0, true)), "焦点落到词条栏");
}

#[test]
fn enter_opens_the_entry_and_tab_moves_into_the_detail() {
    let mut a = wiki_app();
    command(&mut a, "wiki pet");
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).wiki.open.is_some(), "Enter 打开选中的词条");
    assert_eq!(wiki_cursor(&a, "词条"), Some((0, false)), "打开词条后详情接收按键");
    key(&mut a, KeyCode::Tab);
    assert_eq!(wiki_cursor(&a, "词条"), Some((0, false)), "Tab 换到详情栏");
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&a).wiki.open.as_ref().unwrap().scroll, 1, "详情栏 ↓ 逐行滚动");
}

#[test]
fn wide_detail_follows_the_entry_selection_without_enter() {
    let mut a = wiki_app();
    command(&mut a, "wiki 2");
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&a).wiki.open.as_ref().unwrap().hit.index, 0, "详情层只滚动");
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).wiki.open.as_ref().unwrap().hit.index, 1);
}

#[test]
fn arrows_scroll_a_long_entry_when_it_is_the_only_column() {
    let mut a = wiki_app();
    command(&mut a, "wiki open battle_npc:npc/100/sb_nusu.create@25-0");
    let top = draw(&mut a, 120, 30);
    for _ in 0..3 {
        key(&mut a, KeyCode::Down);
    }
    let moved = draw(&mut a, 120, 30);
    assert_ne!(top, moved, "没有列表时 ↓ 直接滚动详情");
    key(&mut a, KeyCode::Home);
    assert_eq!(draw(&mut a, 120, 30), top, "Home 滚回顶部");
}

#[test]
fn esc_peels_one_layer_at_a_time_and_lands_the_focus() {
    let mut a = wiki_app();
    command(&mut a, "wiki pet");
    command(&mut a, "wiki find 乌力");
    command(&mut a, "wiki open 1");
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).wiki.open.is_none(), "第一下 Esc 关词条");
    assert_eq!(wiki_cursor(&a, "词条"), Some((0, true)), "焦点回到词条栏");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).wiki.query.is_empty(), "第二下清掉搜索");
    key(&mut a, KeyCode::Esc);
    assert_eq!(world(&a).wiki.category, None, "第三下回分类目录");
    assert_eq!(wiki_cursor(&a, "分类"), Some((1, true)), "焦点回到分类栏");
}

#[test]
fn narrow_screen_detail_replaces_the_list_and_takes_the_focus() {
    let mut a = wiki_app();
    draw(&mut a, 80, 24);
    command(&mut a, "wiki pet");
    command(&mut a, "wiki open 1");
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("词条 · 乌力") && !s.contains("宠物与生物 · 共"), "窄屏详情替换列表\n{s}");
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&a).wiki.open.as_ref().unwrap().scroll, 1, "焦点在详情栏，↓ 滚动详情");
    assert_eq!(wiki_cursor(&a, "词条"), Some((0, false)), "列表没画出来，不接收焦点");
}

#[test]
fn wiki_next_moves_the_selection_to_the_first_row_of_the_new_page() {
    let mut a = wiki_app();
    command(&mut a, "wiki 3");
    draw(&mut a, 120, 40);
    let rows = world(&a).wiki.list_rows;
    command(&mut a, "wiki next");
    assert_eq!(wiki_cursor(&a, "词条"), Some((rows, true)), "wiki next 之后选中第二页的第一条");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("第 2/"), "{s}");
}

#[test]
fn page_down_on_the_entries_moves_one_page_and_the_page_follows() {
    let mut a = wiki_app();
    command(&mut a, "wiki 3");
    draw(&mut a, 120, 40);
    let rows = world(&a).wiki.list_rows;
    key(&mut a, KeyCode::PageDown);
    assert_eq!(wiki_cursor(&a, "词条"), Some((rows, true)));
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("第 2/"), "选中项到了第二页，页码跟着变\n{s}");
    key(&mut a, KeyCode::PageUp);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("第 1/"), "{s}");
}

#[test]
fn hint_shows_back_only_when_there_is_a_layer_to_go_back_to() {
    let mut a = wiki_app();
    let s = input_row(&mut a);
    assert!(s.contains("Tab 搜索") && s.contains("Esc/Backspace 返回"), "分类目录可回游戏菜单：{s}");
    command(&mut a, "wiki pet");
    let s = input_row(&mut a);
    assert!(s.contains("Esc/Backspace 返回"), "进了分类就能退回去：{s}");
}
