//! STAGE-100 验收（场景，`docs/login.md`「客户端版本门槛」）：版本低于服务端最低版本时，
//! 登录页提示升级并停在登录页、不再重试；握手响应带上最新版本时，消息栏只提示一次升级。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Enter, Right, Tab};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, ConnState, Screen};
use stoneage_client::ui;
use support::{Harness, opts};
use unicode_width::UnicodeWidthStr;

/// 把当前界面画一遍还原成文字（和 `tests/render.rs` 一样）：检查玩家真正看到的内容。
fn screen_text(app: &mut App, w: u16, h: u16) -> String {
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

#[test]
fn too_old_client_is_refused_on_login_page() {
    let mut h = Harness::new("hello_client_too_old", opts);
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait("提示升级", 5, |a| {
        a.log
            .all()
            .iter()
            .any(|m| m.text.contains("需要升级到 0.3.0 以上"))
    });
    assert!(
        h.last_error().contains("运行 stoneage update"),
        "{}",
        h.last_error()
    );
    // 消息记录里有不算数：要能在登录页的屏幕上看到升级命令（STAGE-100 验收口径 1）。
    let s = screen_text(&mut h.app, 120, 40);
    assert!(s.contains("需要升级到 0.3.0 以上"), "提示被截断：\n{s}");
    assert!(s.contains("stoneage update"), "屏幕上应看得到升级命令：\n{s}");
    assert_eq!(h.app.screen, Screen::Login, "停在登录页");
    assert_eq!(
        h.app.session.state,
        ConnState::Idle,
        "连接已断开，不自动重试"
    );
    h.finish();
}

#[test]
fn newer_client_version_is_hinted_once() {
    let mut h = Harness::new("hello_client_latest", opts);
    h.keys(&[Tab, Tab, Right, Enter]);
    h.wait_screen(Screen::Register);
    h.wait_ready();
    let hints = h
        .app
        .log
        .all()
        .iter()
        .filter(|m| m.text.contains("有新版本 0.3.0"))
        .count();
    assert_eq!(hints, 1, "握手响应带来的升级提示只出现一次");
    h.finish();
}
