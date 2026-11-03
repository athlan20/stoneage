//! Backspace 备用返回键（STAGE-172，`docs/interaction.md`「返回键」）：能返回的层里 Backspace 等同 Esc；
//! 有文字输入焦点时只删字；战斗、确认层、NPC 窗口的按键归它们自己；场景根页不动。
//! 不连服务端：只看本地导航栈、输入行和提示行。战斗的那一条在 `keys_battle.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{
    App, BAG_MENU, BagStep, BrowseStep, Confirm, Layer, Options, SYSTEM_MENU, Screen, TextInput, WIKI_MENU, WorldState,
    key_hint, system::SystemStep,
};
use stoneage_client::proto::{NpcOption, NpcWindow, WorldEntry};

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    })
}

/// 进入世界：真实入口是场景层。
fn world_app() -> App {
    let mut a = app();
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.screen = Screen::World;
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn text(a: &mut App, s: &str) {
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn world_mut(a: &mut App) -> &mut WorldState {
    a.world.as_mut().expect("已进入世界")
}

fn top(a: &App) -> Layer {
    world(a).nav.top().clone()
}

fn window(options: Vec<NpcOption>) -> NpcWindow {
    NpcWindow {
        npc: 225,
        name: "村长家的厨师".into(),
        options,
        ..NpcWindow::default()
    }
}

/// 进入系统页（栈顶是功能页，和真实的菜单进入一致）。
fn system_app() -> App {
    let mut a = world_app();
    world_mut(&mut a).menu = SYSTEM_MENU;
    world_mut(&mut a).nav.show_page(SYSTEM_MENU);
    a
}

// ---- 场景与导航：能返回的层 ----

#[test]
fn backspace_on_the_scene_root_does_nothing() {
    let mut a = world_app();
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Scene, "场景根页的 Backspace 不打开游戏菜单");
    assert_eq!(world(&a).nav.depth(), 1);
}

#[test]
fn backspace_on_the_scene_root_still_leaves_the_details_open_as_esc_does() {
    // 场景的详情面板开着时 Esc 会关它，Backspace 也要关（它是能返回的层）。
    let mut a = world_app();
    world_mut(&mut a).scene.details = true;
    key(&mut a, KeyCode::Backspace);
    assert!(!world(&a).scene.details, "详情开着时 Backspace 关掉它，和 Esc 一样");
    assert_eq!(top(&a), Layer::Scene);
}

#[test]
fn backspace_in_the_menu_returns_to_the_scene() {
    let mut a = world_app();
    key(&mut a, KeyCode::Esc); // 场景 → 菜单
    assert_eq!(top(&a), Layer::Menu);
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Scene, "游戏菜单里 Backspace 回场景");
}

#[test]
fn backspace_walks_back_one_layer_at_a_time_like_esc() {
    let mut a = world_app();
    key(&mut a, KeyCode::Esc); // 场景 → 菜单
    key(&mut a, KeyCode::Down); // 第一项「角色」→「宠物」
    key(&mut a, KeyCode::Enter); // 进功能页
    assert!(matches!(top(&a), Layer::Page(_)));
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Menu, "功能页 Backspace 退一层回菜单");
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Scene, "再按一次回场景");
    assert_eq!(world(&a).nav.depth(), 1, "场景是栈底，退不掉");
}

#[test]
fn backspace_on_a_page_opened_straight_from_the_scene_goes_to_the_scene() {
    let mut a = world_app();
    world_mut(&mut a).nav.push(Layer::Page(0));
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Scene, "没有菜单层时一层回到场景");
}

// ---- 文字输入焦点：Backspace 只删字 ----

#[test]
fn command_bar_backspace_deletes_text_and_the_bar_stays_open() {
    let mut a = world_app();
    key(&mut a, KeyCode::Char('/'));
    text(&mut a, "help");
    key(&mut a, KeyCode::Backspace);
    assert_eq!(world(&a).input.text(), "hel", "命令栏里 Backspace 删一个字");
    assert_eq!(top(&a), Layer::Command);
    for _ in 0..3 {
        key(&mut a, KeyCode::Backspace);
    }
    assert_eq!(world(&a).input.text(), "", "字删光");
    assert_eq!(top(&a), Layer::Command, "字删光后命令栏仍然开着");
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Command, "空删也不关命令栏");
}

#[test]
fn private_chat_form_backspace_deletes_text_and_does_not_leave_the_page() {
    let mut a = system_app();
    let mut chat = TextInput::new(200);
    chat.set("ab");
    world_mut(&mut a).system.step = Some(SystemStep::Chat(chat));
    key(&mut a, KeyCode::Backspace);
    let w = world(&a);
    assert!(
        matches!(&w.system.step, Some(SystemStep::Chat(t)) if t.text() == "a"),
        "私聊里 Backspace 只删字"
    );
    assert_eq!(top(&a), Layer::Page(SYSTEM_MENU), "没有离开系统页");
}

#[test]
fn token_form_backspace_deletes_the_label_and_keeps_the_form() {
    let mut a = system_app();
    let mut label = TextInput::new(40);
    label.set("我的号");
    world_mut(&mut a).system.step = Some(SystemStep::Create {
        label,
        days: TextInput::new(4),
        focus: 0,
        confirm: false,
    });
    key(&mut a, KeyCode::Backspace);
    let w = world(&a);
    assert!(
        matches!(&w.system.step, Some(SystemStep::Create { label, .. }) if label.text() == "我的"),
        "令牌备注里 Backspace 删掉最后一个字，表单还在"
    );
}

#[test]
fn wiki_search_backspace_deletes_the_query_and_does_not_go_back() {
    let mut a = world_app();
    world_mut(&mut a).menu = WIKI_MENU;
    world_mut(&mut a).nav.show_page(WIKI_MENU);
    let mut input = TextInput::new(40);
    input.set("史莱");
    world_mut(&mut a).browse.step = Some(BrowseStep::Search { input });
    key(&mut a, KeyCode::Backspace);
    let w = world(&a);
    assert!(
        matches!(&w.browse.step, Some(BrowseStep::Search { input }) if input.text() == "史"),
        "百科搜索框里 Backspace 只删字"
    );
    assert_eq!(top(&a), Layer::Page(WIKI_MENU), "搜索框没有退出");
}

#[test]
fn drop_count_form_backspace_deletes_a_digit_and_keeps_the_form() {
    let mut a = world_app();
    world_mut(&mut a).menu = BAG_MENU;
    world_mut(&mut a).nav.show_page(BAG_MENU);
    let mut text = TextInput::new(4);
    text.set("12");
    world_mut(&mut a).bag_step = Some(BagStep::DropCount { slot: 5, text });
    key(&mut a, KeyCode::Backspace);
    let w = world(&a);
    assert!(
        matches!(&w.bag_step, Some(BagStep::DropCount { text, .. }) if text.text() == "1"),
        "丢弃数量表单里 Backspace 删一位数字"
    );
}

// ---- 战斗、确认层、NPC 窗口：Backspace 不返回、不取消 ----

#[test]
fn backspace_does_not_cancel_a_pending_confirm() {
    let mut a = world_app();
    world_mut(&mut a).confirm = Some(Confirm::PetDrop {
        slot: 0,
        name: "乌力".into(),
    });
    key(&mut a, KeyCode::Backspace);
    assert!(world(&a).confirm.is_some(), "确认层的 Backspace 不取消确认");
}

#[test]
fn backspace_does_not_close_an_npc_window() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![NpcOption {
        no: 1,
        label: "买东西".into(),
    }]));
    key(&mut a, KeyCode::Backspace);
    assert!(world(&a).npc_window.is_some(), "NPC 对话窗口的 Backspace 不关窗");
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).npc_window.is_none(), "Esc 照旧关窗");
}

#[test]
fn backspace_does_not_leave_a_page_while_an_npc_window_is_open() {
    let mut a = world_app();
    world_mut(&mut a).nav.push(Layer::Page(0));
    world_mut(&mut a).npc_window = Some(window(vec![NpcOption {
        no: 1,
        label: "买东西".into(),
    }]));
    key(&mut a, KeyCode::Backspace);
    assert_eq!(top(&a), Layer::Page(0), "弹窗开着时功能页不退层");
}

// ---- 提示行 ----

#[test]
fn key_hint_writes_esc_backspace_where_the_layer_can_return() {
    let mut a = world_app();
    key(&mut a, KeyCode::Esc); // 游戏菜单可以回场景
    let hint = key_hint(world(&a)).join(" · ");
    assert!(
        hint.contains("Esc/Backspace 场景"),
        "菜单层的提示写出 Backspace：{hint}"
    );
}

#[test]
fn key_hint_keeps_esc_alone_on_text_focus_and_scene_root() {
    let mut a = world_app();
    let scene = key_hint(world(&a)).join(" · ");
    assert!(!scene.contains("Backspace"), "场景根页不写返回键：{scene}");
    key(&mut a, KeyCode::Char('/'));
    let command = key_hint(world(&a)).join(" · ");
    assert!(
        !command.contains("Backspace"),
        "命令栏是文字焦点，不写 Backspace：{command}"
    );
}

// ---- 其他文字输入：登录页账号、NPC 暗号窗口 ----

#[test]
fn login_account_backspace_deletes_text_on_the_login_screen() {
    let mut a = app();
    a.screen = Screen::Login;
    a.login.account.set("ab");
    key(&mut a, KeyCode::Backspace);
    assert_eq!(a.login.account.text(), "a", "登录页账号里 Backspace 只删字");
    assert_eq!(a.screen, Screen::Login, "没有离开登录页");
}

#[test]
fn npc_keyword_window_backspace_deletes_the_password_text() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(NpcWindow {
        window_id: String::new(),
        accepts_text: true,
        ..window(vec![])
    });
    world_mut(&mut a).input.set("暗号");
    key(&mut a, KeyCode::Backspace);
    assert_eq!(world(&a).input.text(), "暗", "暗号窗口里 Backspace 只删字");
    assert!(world(&a).npc_window.is_some(), "暗号窗口没有关");
}
