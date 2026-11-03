//! 全局按键统一（STAGE-103，`docs/interaction.md`）：Esc 分层、确认层、`page` 切页、Home/End 滚动、
//! 战斗结果窗口的关闭键。不连服务端：发请求失败时界面状态留在本地，所以这里只检查本地的状态和消息栏；
//! 真正发出并回放的流程见 `scenarios_*.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, BAG_MENU, Confirm, Options, Screen, WIKI_MENU, WorldState};
use stoneage_client::proto::{
    BattleAllow, BattleEnd, BattleStart, BattleUnit, Inventory, InventorySlot, NpcWindow, Pet, Pets, WorldEntry,
};

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
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
    if a.world.as_ref().is_some_and(|w|w.npc_window.is_none()&&w.confirm.is_none()&&!w.nav.is(&stoneage_client::app::Layer::Command)){key(a,KeyCode::Char('/'));}
    text(a, s);
    key(a, KeyCode::Enter);
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn input(a: &App) -> String {
    world(a).input.text().to_string()
}

fn menu(a: &App) -> usize {
    world(a).menu
}

fn logged(a: &App, s: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(s))
}

// ---- Esc 分层 ----

#[test]
fn login_esc_does_nothing() {
    let mut a = app();
    a.screen = Screen::Login;
    key(&mut a, KeyCode::Esc);
    assert!(!a.should_quit, "Esc 不退出程序，退出用 Ctrl+C");
    assert_eq!(a.screen, Screen::Login);
}

#[test]
fn npc_window_esc_clears_text_first_then_closes() {
    let mut a = world_app();
    a.world.as_mut().unwrap().npc_window = Some(NpcWindow { npc: 225, name: "村长家的厨师".into(), ..NpcWindow::default() });
    text(&mut a, "ab");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).npc_window.is_some(), "输入行有字时 Esc 只清空，窗口还开着");
    assert_eq!(input(&a), "");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).npc_window.is_none(), "空行时 Esc 关窗");
}

#[test]
fn npc_window_does_not_swallow_typed_letters() {
    let mut a = world_app();
    a.world.as_mut().unwrap().npc_window = Some(NpcWindow { npc: 225, name: "村长家的厨师".into(), ..NpcWindow::default() });
    line(&mut a, "help");
    assert!(world(&a).npc_window.is_some(), "字母是文字，窗口不关");
    assert!(logged(&a, "全部命令"), "打的命令照常执行");
}

// ---- 确认层 ----

fn pet_app() -> App {
    let mut a = world_app();
    a.world.as_mut().unwrap().pets = Pets { pets: vec![Pet { slot: 0, name: "乌力".into(), ..Pet::default() }], ..Pets::default() };
    a
}

fn confirm(a: &App) -> Option<&Confirm> {
    world(a).confirm.as_ref()
}

#[test]
fn pet_drop_asks_and_y_confirms() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0");
    assert!(matches!(confirm(&a), Some(Confirm::PetDrop { slot: 0, .. })));
    assert!(logged(&a, "放生后不能恢复：乌力（0 号）。输入 y 确认，其他内容取消"));
    line(&mut a, "y");
    assert!(confirm(&a).is_none(), "输入 y 确认后问题清掉");
}

#[test]
fn yes_and_shi_also_confirm() {
    for answer in ["yes", "是", "Y"] {
        let mut a = pet_app();
        line(&mut a, "pet drop 0");
        line(&mut a, answer);
        assert!(confirm(&a).is_none(), "{answer} 应当是确认");
    }
}

#[test]
fn other_text_cancels_and_runs_as_a_command() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0");
    line(&mut a, "help");
    assert!(confirm(&a).is_none());
    assert!(logged(&a, "已取消放生"));
    assert!(logged(&a, "全部命令"), "其他内容照常当命令处理");
}

#[test]
fn trailing_yes_skips_the_question() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0 yes");
    assert!(confirm(&a).is_none(), "命令末尾带 yes 直接放生，不问");
    assert!(!logged(&a, "输入 y 确认"));
}

#[test]
fn esc_cancels_a_pending_confirm_when_input_is_empty() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0");
    key(&mut a, KeyCode::Esc);
    assert!(confirm(&a).is_none());
    assert!(logged(&a, "已取消放生"));
}

#[test]
fn esc_clears_typed_text_before_cancelling() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0");
    text(&mut a, "ab");
    key(&mut a, KeyCode::Esc);
    assert!(confirm(&a).is_some(), "输入行有字时 Esc 只清空");
    assert_eq!(input(&a), "");
    key(&mut a, KeyCode::Esc);
    assert!(confirm(&a).is_none());
}

#[test]
fn digits_are_text_while_confirming() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0");
    key(&mut a, KeyCode::Char('3'));
    assert_eq!(menu(&a), 0, "确认中数字是文字，不切菜单");
    assert_eq!(input(&a), "3");
}

fn bag_app() -> App {
    let mut a = world_app();
    a.world.as_mut().unwrap().inventory = Inventory {
        slots: vec![InventorySlot { slot: 5, item_id: 1, name: "小斧头".into(), count: 1, ..InventorySlot::default() }],
        ..Inventory::default()
    };
    a
}

#[test]
fn item_drop_asks_and_y_drops() {
    let mut a = bag_app();
    line(&mut a, "item drop 5");
    assert!(matches!(confirm(&a), Some(Confirm::ItemDrop { slot: 5, count: None, .. })));
    assert!(logged(&a, "丢弃后不能恢复：小斧头（5 号）。输入 y 确认"));
    line(&mut a, "y");
    assert!(confirm(&a).is_none());
}

#[test]
fn item_drop_count_is_part_of_the_question() {
    let mut a = bag_app();
    line(&mut a, "item drop 5 1");
    assert!(matches!(confirm(&a), Some(Confirm::ItemDrop { slot: 5, count: Some(1), .. })));
    assert!(logged(&a, "小斧头 ×1（5 号）"));
}

#[test]
fn item_drop_with_trailing_yes_skips_the_question() {
    let mut a = bag_app();
    line(&mut a, "item drop 5 yes");
    assert!(confirm(&a).is_none());
    assert!(!logged(&a, "输入 y 确认"));
}

// ---- page 切页 ----

#[test]
fn page_switches_by_number_or_name() {
    let mut a = world_app();
    line(&mut a, "page 4");
    assert_eq!(menu(&a), BAG_MENU);
    line(&mut a, "page 百科");
    assert_eq!(menu(&a), WIKI_MENU);
    line(&mut a, "页 1");
    assert_eq!(menu(&a), 0, "别名“页”");
    line(&mut a, "page 0");
    assert_eq!(menu(&a), 0, "没有 0 页");
    assert!(logged(&a, "没有叫“0”的页面"));
    line(&mut a, "page");
    assert!(logged(&a, "用法：page"));
}

#[test]
fn page_works_under_a_dialog_and_keeps_the_dialog_open() {
    let mut a = world_app();
    a.world.as_mut().unwrap().npc_window = Some(NpcWindow { npc: 225, name: "村长家的厨师".into(), ..NpcWindow::default() });
    line(&mut a, "page 4");
    assert_eq!(menu(&a), BAG_MENU, "弹窗底下的页面换了");
    assert!(world(&a).npc_window.is_some(), "弹窗不关");
}

fn battle_app(with_result: bool) -> App {
    let mut a = world_app();
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![BattleUnit { no: 0, kind: "player".into(), name: "勇者小明".into(), level: 3, hp_pct: 100, ..BattleUnit::default() }],
        ..BattleStart::default()
    };
    let mut battle = BattleState::from_start(start);
    if with_result {
        battle.show_result(BattleEnd { duel_point: None, result: "win".into(), turn: 2, ..BattleEnd::default() });
    }
    a.world.as_mut().unwrap().battle = Some(battle);
    a
}

#[test]
fn page_is_refused_in_battle() {
    let mut a = battle_app(false);
    line(&mut a, "page 4");
    assert_eq!(menu(&a), 0);
    assert!(logged(&a, "战斗中不能切页"));
}

// ---- 战斗结果窗口 ----

fn battle_at(a: &App) -> bool {
    world(a).battle.is_some()
}

#[test]
fn result_window_closes_on_enter_space_or_esc_when_input_is_empty() {
    for code in [KeyCode::Enter, KeyCode::Char(' '), KeyCode::Esc] {
        let mut a = battle_app(true);
        key(&mut a, code);
        assert!(!battle_at(&a), "{code:?} 关闭结果窗口");
    }
}

#[test]
fn result_window_typing_closes_and_keeps_the_letter() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Char('x'));
    assert!(battle_at(&a));
    assert_eq!(input(&a), "", "普通字母不关闭结果，也不进入隐藏输入");
}

#[test]
fn result_window_digit_is_text_not_a_menu_switch() {
    let mut a = battle_app(true);
    key(&mut a, KeyCode::Char('3'));
    assert!(battle_at(&a));
    assert_eq!(menu(&a), 0, "关窗后这个数字是文字，不切菜单");
    assert_eq!(input(&a), "");
}

#[test]
fn result_window_esc_with_text_only_clears() {
    let mut a = battle_app(true);
    // 结果出来之前已经输入到一半的字：直接放进输入行，模拟这种状态（窗口打开时键入会关窗）。
    a.world.as_mut().unwrap().input.set("ab");
    key(&mut a, KeyCode::Esc);
    assert!(!battle_at(&a), "隐藏草稿不改变继续键的含义");
    assert_eq!(input(&a), "ab");
}

#[test]
fn result_window_enter_with_text_runs_the_command() {
    let mut a = battle_app(true);
    key(&mut a,KeyCode::Char('/'));
    a.world.as_mut().unwrap().input.set("help");
    key(&mut a, KeyCode::Enter);
    assert!(battle_at(&a), "输入行有字时 Enter 执行命令，窗口不关");
    assert!(logged(&a, "全部命令"));
}

// ---- Home / End ----

#[test]
fn home_and_end_scroll_the_bag_page_when_input_is_empty() {
    let mut a = world_app();
    {
        let w = a.world.as_mut().unwrap();
        w.menu = BAG_MENU;
        w.bag_rows = 5;
    }
    key(&mut a, KeyCode::End);
    assert_eq!(world(&a).bag_scroll, 15, "最后一页贴底：15–19");
    key(&mut a, KeyCode::Home);
    assert_eq!(world(&a).bag_scroll, 5, "最前面是第一格");
}

#[test]
fn home_and_end_move_the_cursor_when_input_has_text() {
    let mut a = world_app();
    {
        let w = a.world.as_mut().unwrap();
        w.menu = BAG_MENU;
        w.bag_rows = 5;
    }
    text(&mut a, "ab");
    key(&mut a, KeyCode::End);
    assert_eq!(world(&a).bag_scroll, 5, "有字时 End 只移动光标，不滚动");
    assert_eq!(input(&a), "ab");
}
