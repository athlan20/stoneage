//! STAGE-276 社交开关的客户端（`docs/ui.md`「社交页的开关」，`docs/protocol.md` 5.69）：设置栏渲染与 80×24、
//! Tab 换到设置栏、TARGET_REFUSES 的提示；经模拟服务端的切换（Enter 与空格、响应更新、失败保持原值）。
//! 模拟服务端自动应答 `social_settings`，值变了先推 `social_changed` 再回响应。

#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{self, BackTab, Char, Down, Enter, Esc, Tab};
use crossterm::event::{KeyEvent, KeyModifiers};
use ratatui::buffer::Buffer;
use ratatui::style::Color;
use ratatui::{Terminal, backend::TestBackend};
use stoneage_client::app::social_settings::{self, SocialField};
use stoneage_client::app::{App, Kind, LoadState, Options, SOCIAL_MENU, Screen, WorldState, party_players};
use stoneage_client::proto::WorldEntry;
use stoneage_client::ui;
use support::{Harness, opts};
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    w.entry.character.name = "我".into();
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    a.world = Some(w);
    a
}

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

/// `/party` 打开社交页，Tab 两次落到第三栏「设置」。
fn open_settings(a: &mut App) {
    key(a, Char('/'));
    for c in "party".chars() {
        key(a, Char(c));
    }
    key(a, Enter);
    assert_eq!(a.world.as_ref().unwrap().menu, SOCIAL_MENU);
    key(a, Tab);
    key(a, Tab);
    assert!(social_settings::on_settings_column(a.world.as_ref().unwrap()));
}

fn draw(a: &mut App, width: u16, height: u16) -> Buffer {
    let mut t = Terminal::new(TestBackend::new(width, height)).unwrap();
    t.draw(|f| ui::render(f, a)).unwrap();
    t.backend().buffer().clone()
}

/// 一行的文字（宽字符占两列时跳过补位格）。
fn row_text(b: &Buffer, y: u16) -> String {
    let (mut line, mut x) = (String::new(), 0);
    while x < b.area.width {
        let sym = b[(x, y)].symbol();
        line.push_str(sym);
        x += UnicodeWidthStr::width(sym).max(1) as u16;
    }
    line
}

/// 一屏的文字，每行一个字符串。
fn text(b: &Buffer) -> String {
    (0..b.area.height)
        .map(|y| row_text(b, y))
        .collect::<Vec<_>>()
        .join("\n")
}

/// 第一行含 `label` 的那一行（开关行在说明文字之前，所以取第一次出现）上「开/关」的文字与颜色。
fn status_of(b: &Buffer, label: &str) -> (String, Color) {
    let area = b.area;
    for y in 0..area.height {
        if !row_text(b, y).contains(label) {
            continue;
        }
        for x in 0..area.width {
            let sym = b[(x, y)].symbol();
            if sym == "开" || sym == "关" {
                return (sym.to_string(), b[(x, y)].fg);
            }
        }
    }
    panic!("没有找到 {label} 这一行\n{}", text(b));
}

fn has_log(a: &App, kind: Kind, text: &str) -> bool {
    a.log.all().iter().any(|m| m.kind == kind && m.text.contains(text))
}

#[test]
fn settings_column_shows_both_switches_with_colored_status_and_help_at_80x24() {
    let mut a = app();
    open_settings(&mut a);
    let b = draw(&mut a, 80, 24);
    let s = text(&b);
    for want in ["设置", "对附近玩家可见", "接收组队邀请", "隐身", "组队邀请"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    assert_eq!(status_of(&b, "对附近玩家可见"), ("开".into(), Color::Green), "默认都开");
    assert_eq!(status_of(&b, "接收组队邀请"), ("开".into(), Color::Green), "默认都开");

    a.world.as_mut().unwrap().entry.social.visible = false;
    a.world.as_mut().unwrap().entry.social.invite = false;
    let b = draw(&mut a, 80, 24);
    assert_eq!(status_of(&b, "对附近玩家可见"), ("关".into(), Color::Red));
    assert_eq!(status_of(&b, "接收组队邀请"), ("关".into(), Color::Red));
}

#[test]
fn arrows_move_between_columns_and_rows_inside_the_settings_column() {
    let mut a = app();
    open_settings(&mut a);
    let w = a.world.as_ref().unwrap();
    assert_eq!(social_settings::selected_field(w), Some(SocialField::Visible));
    key(&mut a, Down);
    assert_eq!(
        social_settings::selected_field(a.world.as_ref().unwrap()),
        Some(SocialField::Invite)
    );
    // 从设置栏往回切到请求栏，设置栏的选中项记着。
    key(&mut a, BackTab);
    let w = a.world.as_ref().unwrap();
    assert!(!social_settings::on_settings_column(w));
    assert!(party_players::on_request_column(w));
}

#[test]
fn target_refuses_says_the_target_closed_invites() {
    assert_eq!(
        party_players::party_error_hint("TARGET_REFUSES"),
        Some("对方关闭了组队邀请，不接收别人的邀请")
    );
    assert_eq!(
        party_players::party_error_hint("PLAYER_NOT_FOUND"),
        Some("没有这个在线角色")
    );
}

/// 登录、选第一个角色、进入世界（`ui_party` 用例；social 由模拟服务端补成两个都开）。
fn enter_world() -> Harness {
    let mut h = Harness::new("ui_party", opts);
    h.text("alice");
    h.key(Tab);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h
}

/// 退回场景，Esc 打开游戏菜单，选「社交」，再 Tab 两次到设置栏。
fn open_social_settings(h: &mut Harness) {
    while !h.app.world.as_ref().unwrap().at_scene() {
        h.key(Esc);
    }
    h.key(Esc);
    for _ in 0..5 {
        h.key(Down);
    }
    h.key(Enter);
    h.keys(&[Tab, Tab]);
    assert!(social_settings::on_settings_column(h.app.world.as_ref().unwrap()));
}

/// 经模拟服务端：Enter 切“隐身”、空格切“拒绝邀请”，响应（前有 social_changed）更新显示并写消息；
/// 再设置模拟失败（环境变量在模拟服务端启动时读取，所以失败用例放在同一个测试里、成功之后），
/// 失败时显示保持原值、解除处理中，原因写进消息栏。两段串行，避免别的测试启动的模拟服务端继承失败开关。
#[test]
fn switches_round_trip_through_the_server_and_a_failure_keeps_the_value() {
    let mut h = enter_world();
    open_social_settings(&mut h);
    h.key(Enter);
    h.wait("隐身生效", 5, |a| !a.world.as_ref().unwrap().entry.social.visible);
    h.key(Down);
    h.key(Char(' '));
    h.wait("拒绝邀请生效", 5, |a| {
        !a.world.as_ref().unwrap().entry.social.invite
    });
    assert!(h.app.world.as_ref().unwrap().social_busy.is_none());
    assert!(has_log(&h.app, Kind::Party, "你已隐身，别人看不到你"));
    assert!(has_log(&h.app, Kind::Party, "已拒绝接收组队邀请"));
    h.finish_stopped_early();

    // SAFETY: 测试进程内只有这一个测试会设置该变量，且在启动模拟服务端之前设好；其余测试不发 social_settings。
    unsafe { std::env::set_var("STONEAGE_MOCK_SOCIAL_FAIL", "1") };
    let mut h = enter_world();
    // SAFETY: 同上，子进程已经启动，之后不再需要这个变量。
    unsafe { std::env::remove_var("STONEAGE_MOCK_SOCIAL_FAIL") };
    open_social_settings(&mut h);
    h.key(Enter);
    h.wait("失败应答", 5, |a| {
        a.world.as_ref().unwrap().social_busy.is_none() && has_log(a, Kind::Error, "模拟失败")
    });
    let w = h.app.world.as_ref().unwrap();
    assert!(w.entry.social.visible, "失败后显示保持原值");
    assert!(w.social_busy.is_none());
    h.finish_stopped_early();
}
