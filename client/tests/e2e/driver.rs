//! 联调用的客户端驱动：用按键驱动真实的 `App`，把界面画成文本供检查。

use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyCode::*, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, ConnState, CreateStep, Kind, Options, Screen};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

// ---------- 驱动客户端 ----------

pub struct Client {
    pub app: App,
    last_tick: Instant,
}

impl Client {
    pub fn new(opts: Options) -> Self {
        Client { app: App::new(opts), last_tick: Instant::now() }
    }

    pub fn connect(addr: &str) -> Self {
        // 客户端停在登录页，按下登录或进入注册页时才连接服务端。
        Self::new(Options { server: addr.to_string(), ..Options::default() })
    }

    pub fn key(&mut self, code: KeyCode) {
        self.app.on_key(KeyEvent::new(code, KeyModifiers::NONE));
    }
    pub fn keys(&mut self, codes: &[KeyCode]) {
        codes.iter().for_each(|c| self.key(*c));
    }
    pub fn repeat(&mut self, code: KeyCode, n: usize) {
        (0..n).for_each(|_| self.key(code));
    }
    pub fn text(&mut self, s: &str) {
        if self.app.screen==Screen::World && self.app.world.as_ref().is_some_and(|w|!w.text_entry_active()){self.key(KeyCode::Char('/'));}
        s.chars().for_each(|c| self.key(Char(c)));
    }

    pub fn pump(&mut self) {
        self.app.pump_net();
        if self.last_tick.elapsed() >= Duration::from_millis(250) {
            self.app.on_tick(Instant::now());
            self.last_tick = Instant::now();
        }
    }

    pub fn wait(&mut self, what: &str, secs: u64, cond: impl Fn(&App) -> bool) {
        let end = Instant::now() + Duration::from_secs(secs);
        while Instant::now() < end {
            self.pump();
            if cond(&self.app) {
                return;
            }
            std::thread::sleep(Duration::from_millis(10));
        }
        panic!("等待“{what}”超时；界面 {:?}；消息 {:?}", self.app.screen, self.messages());
    }

    pub fn wait_screen(&mut self, screen: Screen) {
        self.wait(&format!("{screen:?}"), 15, |a| a.screen == screen);
    }

    pub fn messages(&self) -> Vec<String> {
        self.app.log.all().iter().map(|m| m.text.clone()).collect()
    }

    pub fn last_error(&self) -> String {
        self.app.log.all().iter().rev().find(|m| m.kind == Kind::Error).map(|m| m.text.clone()).unwrap_or_default()
    }

    /// 把当前界面画成文本（120×40），宽字符只算一格。
    pub fn screen_text(&mut self) -> String {
        let (w, h) = (120u16, 40u16);
        let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
        term.draw(|f| ui::render(f, &mut self.app)).unwrap();
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

    // ----- 界面操作 -----

    /// 登录页 → 注册页 → 注册成功后回到登录页。
    pub fn register(&mut self, account: &str, password: &str) {
        self.keys(&[Tab, Tab, Right, Enter]);
        self.wait_screen(Screen::Register);
        self.wait("握手完成", 15, |a| a.session.state == ConnState::Ready);
        self.text(account);
        self.keys(&[Tab]);
        self.text(password);
        self.keys(&[Tab]);
        self.text(password);
        self.keys(&[Tab, Enter]);
        self.wait_screen(Screen::Login);
    }

    /// 注册成功或登录失败后，账号已经填好、焦点在密码框：只需要输入密码并回车。
    pub fn submit_password(&mut self, password: &str) {
        self.text(password);
        self.key(Enter);
    }

    pub fn login(&mut self, account: &str, password: &str) {
        self.text(account);
        self.key(Tab);
        self.text(password);
        self.key(Enter);
    }

    /// 在空栏位上走完建角三步：外形 3/颜色 2（眼型、嘴型固定默认值），能力点 8/4/4/4，属性点 地 6 水 4，出生地 0 萨姆吉尔。
    pub fn create_character(&mut self, name: &str) {
        self.key(Enter);
        assert_eq!(self.app.screen, Screen::Create);
        self.repeat(Right, 3);
        self.key(Down);
        self.repeat(Right, 2);
        self.key(Enter);
        assert_eq!(self.app.create.step, CreateStep::Stats);
        self.text(name);
        self.keys(&[Down, Down, Left, Down, Left, Down, Left]);
        self.repeat(Up, 3);
        self.repeat(Right, 3);
        self.repeat(Down, 4);
        self.key(Right);
        self.key(Enter);
        assert_eq!(self.app.create.step, CreateStep::Home);
        self.key(Enter);
    }

    pub fn world_position(&self) -> (u32, i32, i32, String) {
        let e = &self.app.world.as_ref().expect("应当在世界中").entry;
        (e.position.floor, e.position.x, e.position.y, e.position.map_name.clone())
    }
}

pub fn unique(prefix: &str) -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let n = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().subsec_nanos() % 100_000;
    format!("{prefix}{n}")
}

impl Client {
    pub fn run_ms(&mut self, ms: u64) {
        let end = Instant::now() + Duration::from_millis(ms);
        while Instant::now() < end {
            self.pump();
            std::thread::sleep(Duration::from_millis(10));
        }
    }
}

