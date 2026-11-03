//! 快捷传送、角色、消息三页的方向键逛页面（`docs/ui.md`）：
//! 快捷传送页（STAGE-161）`↑↓` 选、`Enter` 逐级下钻或传送、`Esc` 逐级返回；
//! 角色页 `↑↓` 在精灵清单和能力之间选，能力有点数时 `Enter` 填 `addpoint`；
//! 消息页 `↑↓` 逐行滚动，输入行有字时仍是历史。不连服务端：只看本地状态和输入行的文字。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, CharStep, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{AbilityPoints, MagicView, Npc, Status, TargetType, TravelNode, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

/// 画一帧并返回文字。画的时候会记下中间页面的宽高，页面选择判断栏显示与否用的就是它。
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

/// 输入行：提示和输入文字就在这一行（倒数第二行）。按给定的尺寸画一帧，窗口尺寸会改变栏的显示。
fn input_row_at(a: &mut App, w: u16, h: u16) -> String {
    draw(a, w, h).lines().rev().nth(1).unwrap_or_default().to_string()
}

fn input_row(a: &mut App) -> String {
    input_row_at(a, 120, 40)
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().unwrap()
}

fn input(a: &App) -> String {
    world(a).input.text().to_string()
}

fn top(a: &App) -> Layer {
    world(a).nav.top().clone()
}

/// 进入世界，切到指定的菜单页（数字键）。`edit` 给世界状态填数据。
fn page(menu: char, edit: impl FnOnce(&mut WorldState)) -> App {
    let mut a = app();
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    edit(&mut w);
    a.world = Some(w);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char(menu));
    a
}

fn npc(id: u32, name: &str) -> Npc {
    Npc { id, name: name.into(), type_: "对话".into(), x: 1, y: 1 }
}

/// 清空输入行（有字时 Esc 只清空）。
fn clear_input(a: &mut App) {
    key(a, KeyCode::Esc);
}

// ---- 快捷传送页（STAGE-161） ----

/// 一小棵目录：萨伊那斯 → 萨姆吉尔村 → 村长家，外加一个纯分组。
fn travel_nodes() -> Vec<TravelNode> {
    let n = |id: &str, parent: Option<&str>, name: &str, floor: Option<u32>, order: u32| TravelNode {
        id: id.into(),
        parent_id: parent.map(Into::into),
        name: name.into(),
        floor,
        order,
    };
    vec![
        n("group:sainasu", None, "萨伊那斯", Some(100), 0),
        n("map:1000", Some("group:sainasu"), "萨姆吉尔村", Some(1000), 0),
        n("map:1006", Some("map:1000"), "萨姆吉尔的村长家", Some(1006), 0),
        n("group:sainasu.dungeons", Some("group:sainasu"), "洞窟与通路", None, 1),
        n("map:10001", Some("group:sainasu.dungeons"), "阿布的洞窟地下１楼", Some(10001), 0),
        n("group:seimu", None, "沙姆岛", Some(400), 1),
    ]
}

fn travel_world(w: &mut WorldState) {
    w.travel.install("test-1".into(), &travel_nodes());
    w.travel.here_floor = Some(1006);
    w.entry.position.floor = 1006;
    w.entry.position.map_name = "萨姆吉尔的村长家".into();
}

#[test]
fn travel_page_enter_descends_and_esc_walks_back_one_layer() {
    let mut a = page('1', travel_world);
    assert!(input_row(&mut a).contains("↑↓/翻页 选择"), "{}", input_row(&mut a));
    key(&mut a, KeyCode::Enter); // 进入萨伊那斯
    assert_eq!(world(&a).travel.path(), vec!["萨伊那斯".to_string()]);
    key(&mut a, KeyCode::Esc); // 退回根层
    assert!(world(&a).travel.path().is_empty());
    key(&mut a, KeyCode::Esc); // 根层再退就离开这一页
    assert_eq!(top(&a), Layer::Scene);
}

#[test]
fn travel_page_shows_the_path_counts_and_the_current_marker() {
    let mut a = page('1', travel_world);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("快捷传送"), "{s}");
    assert!(s.contains("萨伊那斯"), "根层列出主区域\n{s}");
    assert!(s.contains("（4 项）"), "标题写当前层的项数\n{s}");
    assert!(s.contains("地图编号"), "宽屏右侧是目的地详情\n{s}");
    assert!(s.contains("当前位置"), "{s}");
}

#[test]
fn travel_page_marks_the_current_map_and_does_not_send_again() {
    let mut a = page('1', travel_world);
    key(&mut a, KeyCode::Enter); // 萨伊那斯
    key(&mut a, KeyCode::Down); // 萨姆吉尔村
    key(&mut a, KeyCode::Enter); // 进入村庄
    // 村庄这一层：首项是「传送到本图」；第二项是村长家，正好是当前所在地
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("传送到本图 · 萨姆吉尔村"), "有下级的地图首项要能传送本图\n{s}");
    assert!(s.contains("← 当前"), "当前所在地要标出来\n{s}");
}

#[test]
fn travel_page_grouping_nodes_have_no_floor() {
    let mut a = page('1', travel_world);
    key(&mut a, KeyCode::Enter); // 萨伊那斯
    key(&mut a, KeyCode::End); // 洞窟与通路
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("洞窟与通路"), "{s}");
    assert!(s.contains("分组（不能直接传送）"), "纯分组不能传送\n{s}");
}

#[test]
fn travel_page_unavailable_catalog_says_so() {
    let mut a = page('1', |_| {});
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("正在取快捷传送目录") || s.contains("目录不可用"), "{s}");
}

// ---- 角色页 ----

fn magic(slot: u8, name: &str, target: TargetType, effect: &str) -> MagicView {
    MagicView { slot, id: 301, name: name.into(), mp: 10, target_type: target, effect: effect.into() }
}

/// 角色页：精灵在 1、3 号栏位（0 号空着），能力点数由参数决定；画在给定尺寸的窗口里。
fn char_page(points: u32, w: u16, h: u16) -> App {
    let mut a = page('2', |world| {
        let mut status = Status { skill_points: points, ..Status::default() };
        status.stats = AbilityPoints { vital: 5, strength: 5, toughness: 5, dexterity: 5 };
        status.magic = vec![
            magic(1, "岩石撞击", TargetType::Enemy, "攻击敌方一个，威力 100，地属性"),
            magic(3, "治愈的精灵 Lv1", TargetType::SelfOnly, "回复自己 HP 80"),
        ];
        world.entry.status = status;
    });
    draw(&mut a, w, h);
    a
}

#[test]
fn char_focus_starts_on_the_first_magic_and_enter_opens_its_actions() {
    let mut a = char_page(0, 120, 40);
    assert!(input_row(&mut a).contains("↑↓ 选择 · Enter 动作 · Tab 能力"), "{}", input_row(&mut a));
    // STAGE-231：有精灵的栏位 Enter 打开「使用」（`tests/char_magic_use.rs`），不填命令。
    key(&mut a, KeyCode::Enter);
    assert_eq!(input(&a), "", "Enter 打开动作列表，不进输入行");
    assert!(matches!(a.world.as_ref().unwrap().char_step, Some(CharStep::MagicAction { slot: 1 })));
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Down);
    assert!(draw(&mut a, 120, 40).contains("▶3"), "↓ 从 1 号栏位移到下一个有精灵的 3 号栏位");
}

#[test]
fn char_ability_enter_opens_the_addpoint_form() {
    let mut a = char_page(2, 120, 40);
    key(&mut a, KeyCode::Tab);
    assert!(input_row(&mut a).contains("Enter 动作 · Tab 精灵"), "{}", input_row(&mut a));
    // STAGE-133：能力项 Enter 打开加点表单，不再填命令。
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).char_step, Some(CharStep::AddPoint { stat: 0, .. })), "体力");
    let screen = draw(&mut a, 120, 40);
    assert!(screen.contains("加点") && screen.contains("1–2"), "{screen}");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).char_step.is_none(), "Esc 取消，不加点");
}

#[test]
fn char_ability_without_points_explains_instead_of_opening() {
    let mut a = char_page(0, 120, 40);
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).char_step.is_none(), "没有未分配点数时不开表单");
    assert!(
        a.log.all().iter().any(|m| m.text.contains("没有未分配点数")),
        "要解释原因：{:?}",
        a.log.all().iter().map(|m| m.text.clone()).collect::<Vec<_>>()
    );
}

#[test]
fn char_ability_is_skipped_when_the_panel_is_too_short() {
    // 80×24：中间区域的三栏只剩边框，能力表画不下，`Tab` 跳过它。
    let mut a = char_page(2, 80, 24);
    assert!(input_row_at(&mut a, 80, 24).contains("↑↓ 选择"), "{}", input_row_at(&mut a, 80, 24));
    assert!(!input_row_at(&mut a, 80, 24).contains("Tab"), "{}", input_row_at(&mut a, 80, 24));
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&a).select[1].column, 0, "Tab 没有去能力栏");
}

#[test]
fn char_magic_panel_shows_target_and_effect_of_the_selected_one() {
    let mut a = char_page(0, 120, 40);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("▶1") && s.contains("敌方一个"), "选中行行首 ▶，目标类型写在行末\n{s}");
    assert!(s.contains("攻击敌方一个，威力 100，地属性"), "焦点在精灵栏时底边写效果\n{s}");
    key(&mut a, KeyCode::Tab);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("▶体力"), "焦点在能力栏时，选中的能力行首 ▶\n{s}");
    assert!(!s.contains("攻击敌方一个，威力 100，地属性"), "能力栏是焦点栏时底边不写效果");
}

// ---- 消息页 ----

#[test]
fn messages_page_up_down_scroll_lines_instead_of_history() {
    let mut a = app();
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    w.menu = 6;
    a.world = Some(w);
    a.screen = Screen::World;
    // 消息要比一屏多，才有能往上滚的地方。
    for i in 1..=40 {
        a.log.system(format!("第{i}条消息"));
    }
    draw(&mut a, 120, 40);
    // 执行一条命令，它留在历史里；消息页的 ↑ 不翻历史，而是逐行往上滚。
    for c in "abc".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Enter);
    draw(&mut a, 120, 40);
    assert!(input_row(&mut a).contains("←→ 栏目 · ↑↓ 滚动 · End 最新"), "{}", input_row(&mut a));
    key(&mut a, KeyCode::Up);
    assert_eq!(input(&a), "", "消息页 ↑ 不翻历史");
    assert!(world(&a).msg_view.anchor.is_some(), "往上滚了一行，离开了跟随最新");
    key(&mut a, KeyCode::End);
    assert!(world(&a).msg_view.anchor.is_none(), "End 回到最新");
}

#[test]
fn messages_page_command_bar_keeps_up_for_the_input() {
    let mut a = page('7', |_| {});
    a.log.system("第一条消息");
    draw(&mut a, 120, 40);
    key(&mut a, KeyCode::Char('/'));
    for c in "he".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Up);
    assert_eq!(input(&a), "he", "输入行有字时 ↑ 是历史，不滚动消息");
    assert!(world(&a).msg_view.anchor.is_none());
}
