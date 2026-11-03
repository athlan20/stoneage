//! 导航栈（STAGE-131，`docs/interaction.md`「按键分层」）：进入世界默认落在场景，
//! 场景 Esc 打开游戏菜单，菜单 Enter 进功能页，Esc 逐层退回；`/` 打开命令栏，Esc 收起并保留草稿。
//! 不连服务端：只查本地导航状态、输入行和消息栏。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{App, CatalogScope, Layer, Options, Screen, SceneStep, TextInput, WorldState};
use stoneage_client::proto::{Npc, NpcMatch, WorldEntry};

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

/// 进入世界：真实入口是场景层（不是某个功能页）。
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

fn top(a: &App) -> Layer {
    world(a).nav.top().clone()
}

fn logged(a: &App, s: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(s))
}

// ---- 分层：场景 → 菜单 → 功能页 ----

#[test]
fn world_starts_at_the_scene() {
    let a = world_app();
    assert_eq!(top(&a), Layer::Scene, "进入世界默认落在场景");
}

#[test]
fn esc_opens_the_menu_and_esc_returns_to_the_scene() {
    let mut a = world_app();
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Menu, "场景 Esc 打开游戏菜单");
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Scene, "菜单 Esc 回场景");
}

#[test]
fn menu_enter_opens_a_page_and_esc_walks_back_one_layer_at_a_time() {
    let mut a = world_app();
    key(&mut a, KeyCode::Esc); // 场景 → 菜单
    key(&mut a, KeyCode::Down); // 第一项「角色」→ 第二项「宠物」
    key(&mut a, KeyCode::Enter); // 进入功能页
    assert!(matches!(top(&a), Layer::Page(_)), "菜单 Enter 进入功能页");
    assert_eq!(world(&a).menu, stoneage_client::app::PETS_MENU, "进的是菜单里选中的那一页");
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Menu, "功能页 Esc 退一层回菜单");
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Scene, "再按一次回到场景");
    assert_eq!(world(&a).nav.depth(), 1, "场景是栈底，退不掉");
}

#[test]
fn page_esc_falls_back_to_the_scene_without_a_menu_layer() {
    // 从场景入口直接进页面（栈里没有菜单层）时，Esc 一层就回到场景。
    let mut a = world_app();
    a.world.as_mut().unwrap().nav.push(Layer::Page(0));
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Scene);
}

#[test]
fn page_switching_replaces_the_top_instead_of_stacking() {
    // 功能页里用数字键换页：换掉栈顶，不叠一层（STAGE-131）。
    let mut a = world_app();
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Enter); // 进第一项「角色」
    let depth = world(&a).nav.depth();
    key(&mut a, KeyCode::Char('4')); // 换到背包页
    assert_eq!(world(&a).menu, stoneage_client::app::BAG_MENU);
    assert_eq!(world(&a).nav.depth(), depth, "换页不叠层");
    assert_eq!(top(&a), Layer::Page(stoneage_client::app::BAG_MENU));
}

// ---- 场景层不是文字输入层 ----

#[test]
fn scene_swallows_plain_letters() {
    let mut a = world_app();
    text(&mut a, "help");
    assert!(world(&a).input.is_empty(), "场景层不接文字，也不静默执行命令");
    assert_eq!(top(&a), Layer::Scene);
}

#[test]
fn slash_opens_the_command_bar_and_esc_keeps_the_draft() {
    let mut a = world_app();
    key(&mut a, KeyCode::Char('/'));
    assert_eq!(top(&a), Layer::Command, "`/` 打开命令栏");
    text(&mut a, "help");
    assert_eq!(world(&a).input.text(), "help", "命令栏里可以打字");
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Scene, "Esc 收起命令栏");
    assert_eq!(world(&a).input.text(), "help", "草稿留着");
}

// ---- 命令栏：成功才收起 ----

#[test]
fn command_bar_runs_the_command_and_closes_on_enter() {
    let mut a = world_app();
    key(&mut a, KeyCode::Char('/'));
    text(&mut a, "help");
    key(&mut a, KeyCode::Enter);
    assert_eq!(top(&a), Layer::Scene, "命令成功后收起命令栏");
    assert!(logged(&a, "全部命令"), "命令照常执行");
}

#[test]
fn command_bar_keeps_an_unknown_command_in_the_bar() {
    let mut a = world_app();
    key(&mut a, KeyCode::Char('/'));
    text(&mut a, "nosuchcmd");
    key(&mut a, KeyCode::Enter);
    assert_eq!(top(&a), Layer::Command, "未知命令留在命令栏");
    assert!(logged(&a, "未知命令"), "命令栏里报错");
}

// ---- 场景列表：选 NPC 直接对话 ----

#[test]
fn scene_enter_talks_to_the_selected_npc() {
    let mut a = world_app();
    a.world.as_mut().unwrap().npcs = vec![Npc { id: 225, name: "村长".into(), ..Npc::default() }];
    a.world.as_mut().unwrap().scene.npc_load=stoneage_client::app::LoadState::Ready;
    key(&mut a,KeyCode::Right);
    key(&mut a, KeyCode::Enter);
    // 没连服务端，请求发不出去，但编号要被正确解析（不能报“用法”错误）。
    assert!(!logged(&a, "用法：talk"), "场景 Enter 用选中 NPC 的编号发 talk");
}

#[test]
fn scene_enter_on_the_warp_entry_opens_the_travel_page() {
    // 第一项入口是「快捷传送」（STAGE-161）：打开的是游戏菜单里的同一个目录页，不是另开的 8 个地点列表。
    let mut a = world_app();
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).menu, stoneage_client::app::MAP_MENU, "切到菜单第 1 页");
    assert_eq!(top(&a), Layer::Page(stoneage_client::app::MAP_MENU), "栈顶是快捷传送页");
    assert!(world(&a).scene_step.is_none(), "不再是场景里的地点列表步骤");
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&a), Layer::Scene, "Esc 回到场景");
}
/// 直接改世界状态（测试里造一个特定状态，不经过发请求的入口）。
fn world_mut(a: &mut App) -> &mut WorldState {
    a.world.as_mut().expect("已进入世界")
}

/// 造一个目录项。
fn npc_match(id: u32, name: &str, floor: u32) -> NpcMatch {
    NpcMatch { id, name: name.into(), floor, ..NpcMatch::default() }
}
