//! 登录页与注册页：居中的表单加下方的消息面板。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph};

use super::widgets::{button_line, focus_style, form_and_messages, input_text, pad, render_messages};
use crate::app::{App, LoginFocus, RegisterFocus};

const LABEL_W: usize = 10;
const FORM_W: u16 = 56;

fn route_line(app: &App, focused: bool) -> Line<'static> {
    let r = app.current_route();
    let arrows = if app.unicode { "◂▸" } else { "<>" };
    Line::from(vec![
        Span::styled(format!(" {}", pad("线路", LABEL_W)), focus_style(focused)),
        Span::raw(format!("{} {}  {arrows}", r.name, r.addr)),
    ])
}

fn field_line(label: &str, text: String, focused: bool) -> Line<'static> {
    Line::from(vec![
        Span::styled(format!(" {}", pad(label, LABEL_W)), focus_style(focused)),
        Span::raw(text),
    ])
}

pub fn render_login(frame: &mut Frame, app: &mut App, area: Rect) {
    let [form, msgs] = form_and_messages(area, 9, 6, FORM_W);
    let f = app.login.focus;
    let title = Span::styled("石器时代", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD));
    let block = Block::bordered().title(title).title_bottom(if app.reconnect.is_some(){"重新登录 · Esc 返回只读页面"}else{""});
    let inner = block.inner(form);
    let lines = vec![
        Line::raw(""),
        route_line(app, f == LoginFocus::Route),
        field_line("账号", app.login.account.text().to_string(), f == LoginFocus::Account),
        field_line("密码", input_text(&app.login.password, true, app.unicode), f == LoginFocus::Password),
        Line::raw(""),
        button_line(&["登录", "注册", "退出"], app.login.button, f == LoginFocus::Buttons, &[]),
        Line::raw(""),
    ];
    frame.render_widget(Paragraph::new(lines).block(block), form);
    let mask = Some(super::widgets::mask_char(app.unicode));
    let x0 = inner.x + 1 + LABEL_W as u16;
    match f {
        LoginFocus::Account => frame.set_cursor_position((x0 + app.login.account.cursor_col(None), inner.y + 2)),
        LoginFocus::Password => frame.set_cursor_position((x0 + app.login.password.cursor_col(mask), inner.y + 3)),
        _ => {}
    }
    render_messages(frame, app, msgs);
}

pub fn render_register(frame: &mut Frame, app: &mut App, area: Rect) {
    let [form, msgs] = form_and_messages(area, 10, 6, FORM_W);
    let f = app.register.focus;
    let enabled = app.register_enabled();
    let title = Span::styled("注册账号", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD));
    let block = Block::bordered().title(title);
    let inner = block.inner(form);
    let r = &app.register;
    let lines = vec![
        Line::raw(""),
        route_line(app, f == RegisterFocus::Route),
        field_line("账号", r.account.text().to_string(), f == RegisterFocus::Account),
        field_line("密码", input_text(&r.password, true, app.unicode), f == RegisterFocus::Password),
        field_line("确认密码", input_text(&r.confirm, true, app.unicode), f == RegisterFocus::Confirm),
        Line::raw(""),
        button_line(&["注册", "返回"], r.button, f == RegisterFocus::Buttons, &[!enabled && app.session.hello.is_some()]),
        Line::raw(""),
    ];
    frame.render_widget(Paragraph::new(lines).block(block), form);
    let mask = Some(super::widgets::mask_char(app.unicode));
    let x0 = inner.x + 1 + LABEL_W as u16;
    match f {
        RegisterFocus::Account => frame.set_cursor_position((x0 + r.account.cursor_col(None), inner.y + 2)),
        RegisterFocus::Password => frame.set_cursor_position((x0 + r.password.cursor_col(mask), inner.y + 3)),
        RegisterFocus::Confirm => frame.set_cursor_position((x0 + r.confirm.cursor_col(mask), inner.y + 4)),
        _ => {}
    }
    render_messages(frame, app, msgs);
}
