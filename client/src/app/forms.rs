//! 登录页与注册页的表单状态和按键处理。

use crossterm::event::{KeyCode, KeyEvent};

use super::{App, Screen, TextInput, cycle};
use crate::proto::{ACCOUNT_MAX, PASSWORD_MAX, check_account, check_password};
use crate::app::session::Action;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LoginFocus {
    Route,
    Account,
    Password,
    Buttons,
}

const LOGIN_ORDER: [LoginFocus; 4] = [LoginFocus::Route, LoginFocus::Account, LoginFocus::Password, LoginFocus::Buttons];

/// 登录页按钮：登录、注册、退出。
pub const LOGIN_BUTTONS: [&str; 3] = ["登录", "注册", "退出"];

#[derive(Debug)]
pub struct LoginForm {
    pub route: usize,
    pub account: TextInput,
    pub password: TextInput,
    pub focus: LoginFocus,
    pub button: usize,
}

impl Default for LoginForm {
    fn default() -> Self {
        Self {
            route: 0,
            account: TextInput::new(ACCOUNT_MAX),
            password: TextInput::new(PASSWORD_MAX),
            focus: LoginFocus::Account,
            button: 0,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RegisterFocus {
    Route,
    Account,
    Password,
    Confirm,
    Buttons,
}

const REGISTER_ORDER: [RegisterFocus; 5] = [
    RegisterFocus::Route,
    RegisterFocus::Account,
    RegisterFocus::Password,
    RegisterFocus::Confirm,
    RegisterFocus::Buttons,
];

/// 注册页按钮：注册、返回。
pub const REGISTER_BUTTONS: [&str; 2] = ["注册", "返回"];

#[derive(Debug)]
pub struct RegisterForm {
    pub account: TextInput,
    pub password: TextInput,
    pub confirm: TextInput,
    pub focus: RegisterFocus,
    pub button: usize,
}

impl Default for RegisterForm {
    fn default() -> Self {
        Self {
            account: TextInput::new(ACCOUNT_MAX),
            password: TextInput::new(PASSWORD_MAX),
            confirm: TextInput::new(PASSWORD_MAX),
            focus: RegisterFocus::Account,
            button: 0,
        }
    }
}

impl App {
    fn step_route(&mut self, forward: bool) {
        let n = self.routes.len();
        self.login.route = cycle(self.login.route, n, forward);
        let addr = self.current_route().addr.clone();
        self.use_server(&addr);
    }

    pub(super) fn login_key(&mut self, key: KeyEvent) {
        let f = self.login.focus;
        let idx = LOGIN_ORDER.iter().position(|x| *x == f).unwrap_or(0);
        match key.code {
            // 登录页是第一屏，Esc 不起作用；退出用 Ctrl+C 或「退出」按钮（STAGE-103，docs/interaction.md「表单屏幕」）。
            KeyCode::Esc => {self.cancel_reconnect();}
            KeyCode::Tab | KeyCode::Down => self.login.focus = LOGIN_ORDER[cycle(idx, 4, true)],
            KeyCode::BackTab | KeyCode::Up => self.login.focus = LOGIN_ORDER[cycle(idx, 4, false)],
            KeyCode::Enter => match f {
                LoginFocus::Buttons => self.login_button(),
                _ => self.submit_login(),
            },
            KeyCode::Left | KeyCode::Right if f == LoginFocus::Route => self.step_route(key.code == KeyCode::Right),
            KeyCode::Left | KeyCode::Right if f == LoginFocus::Buttons => {
                self.login.button = cycle(self.login.button, LOGIN_BUTTONS.len(), key.code == KeyCode::Right);
            }
            _ => match f {
                LoginFocus::Account => {
                    self.login.account.handle_key(&key);
                }
                LoginFocus::Password => {
                    self.login.password.handle_key(&key);
                }
                _ => {}
            },
        }
    }

    fn login_button(&mut self) {
        match self.login.button {
            0 => self.submit_login(),
            1 => self.open_register(),
            _ => self.request_exit(super::ExitKind::Quit),
        }
    }

    pub(super) fn submit_login(&mut self) {
        if self.session.busy() {
            return;
        }
        let account = self.login.account.text().trim().to_lowercase();
        let password = self.login.password.text().to_string();
        if let Err(e) = check_account(&account).and_then(|()| check_password(&password)) {
            self.log.error(e);
            return;
        }
        self.run_action(Action::Login { account, password });
    }

    fn open_register(&mut self) {
        self.screen = Screen::Register;
        self.register.account.set(self.login.account.text());
        self.register.password.clear();
        self.register.confirm.clear();
        self.register.focus = RegisterFocus::Account;
        self.register.button = 0;
        self.log.system("正在检查服务端是否开放注册…");
        self.run_action(Action::Probe);
    }

    /// 注册按钮是否可用：服务端握手后明确声明开放注册才可用。
    pub fn register_enabled(&self) -> bool {
        self.session.hello.as_ref().is_some_and(|h| h.register_enabled)
    }

    pub(super) fn register_key(&mut self, key: KeyEvent) {
        let f = self.register.focus;
        let idx = REGISTER_ORDER.iter().position(|x| *x == f).unwrap_or(0);
        match key.code {
            KeyCode::Esc => self.leave_register(),
            KeyCode::Tab | KeyCode::Down => self.register.focus = REGISTER_ORDER[cycle(idx, 5, true)],
            KeyCode::BackTab | KeyCode::Up => self.register.focus = REGISTER_ORDER[cycle(idx, 5, false)],
            KeyCode::Enter => match f {
                RegisterFocus::Buttons if self.register.button == 1 => self.leave_register(),
                _ => self.submit_register(),
            },
            KeyCode::Left | KeyCode::Right if f == RegisterFocus::Route => self.step_route(key.code == KeyCode::Right),
            KeyCode::Left | KeyCode::Right if f == RegisterFocus::Buttons => {
                self.register.button = cycle(self.register.button, REGISTER_BUTTONS.len(), key.code == KeyCode::Right);
            }
            _ => match f {
                RegisterFocus::Account => {
                    self.register.account.handle_key(&key);
                }
                RegisterFocus::Password => {
                    self.register.password.handle_key(&key);
                }
                RegisterFocus::Confirm => {
                    self.register.confirm.handle_key(&key);
                }
                _ => {}
            },
        }
    }

    fn leave_register(&mut self) {
        self.screen = Screen::Login;
        self.register.password.clear();
        self.register.confirm.clear();
    }

    fn submit_register(&mut self) {
        if self.session.busy() {
            return;
        }
        if self.session.hello.is_some() && !self.register_enabled() {
            self.log.error("服务端未开放注册");
            return;
        }
        let account = self.register.account.text().trim().to_lowercase();
        let password = self.register.password.text().to_string();
        let check = check_account(&account).and_then(|()| check_password(&password)).and_then(|()| {
            if password == self.register.confirm.text() { Ok(()) } else { Err("两次输入的密码不一致".to_string()) }
        });
        if let Err(e) = check {
            self.log.error(e);
            return;
        }
        self.run_action(Action::Register { account, password });
    }
}
