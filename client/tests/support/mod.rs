//! 集成测试的公共部分：启动模拟服务端，驱动 `App` 的按键和网络事件。

use std::io::{BufRead, BufReader, Read};
use std::process::{Child, ChildStderr, Command, Stdio};
use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{App, Options, Screen};

pub struct Harness {
    pub app: App,
    pub mock: Child,
    log: BufReader<ChildStderr>,
    last_tick: Instant,
}

fn cases_dir() -> String {
    format!("{}/../docs/protocol/cases", env!("CARGO_MANIFEST_DIR"))
}

impl Harness {
    /// 启动模拟服务端（严格模式，用例走完且客户端断开后退出），等它开始监听后再创建 `App`。
    /// 不能用“连一下探测端口”的办法：探测连接会消耗模拟服务端的一次 accept。
    pub fn new(case: &str, opts: impl FnOnce(String) -> Options) -> Self {
        // 让系统分配端口（--listen 127.0.0.1:0），再从模拟服务端的日志里读出实际地址，避免并行测试抢同一个端口。
        let mut mock = Command::new(env!("CARGO_BIN_EXE_mock-server"))
            .args(["--case", case, "--listen", "127.0.0.1:0", "--cases-dir", &cases_dir(), "--strict", "--once"])
            .stderr(Stdio::piped())
            .spawn()
            .expect("无法启动 mock-server");
        let mut log = BufReader::new(mock.stderr.take().unwrap());
        let mut line = String::new();
        let addr = loop {
            line.clear();
            assert!(log.read_line(&mut line).unwrap() > 0, "模拟服务端启动失败");
            if let Some(rest) = line.split("监听 ").nth(1) {
                break rest.split('，').next().unwrap().to_string();
            }
        };
        let app = App::new(opts(addr));
        Self { app, mock, log, last_tick: Instant::now() }
    }

    pub fn key(&mut self, code: KeyCode) {
        self.app.on_key(KeyEvent::new(code, KeyModifiers::NONE));
    }

    /// STAGE-131：进入世界的落点是场景层（Esc 打开游戏菜单、Enter 进选中的功能页）。
    /// 用例继续测功能页里的行为，进入世界后先走这一步落到功能页。
    pub fn enter_page(&mut self) {
        self.key(KeyCode::Esc);
        self.key(KeyCode::Enter);
    }

    pub fn keys(&mut self, codes: &[KeyCode]) {
        for c in codes {
            self.key(*c);
        }
    }

    pub fn repeat(&mut self, code: KeyCode, n: usize) {
        for _ in 0..n {
            self.key(code);
        }
    }

    pub fn text(&mut self, s: &str) {
        if self.app.screen==Screen::World && self.app.world.as_ref().is_some_and(|w|!w.text_entry_active()){self.key(KeyCode::Char('/'));}
        if self.app.world.as_ref().is_some_and(|w|w.battle.is_some()&&!w.nav.is(&stoneage_client::app::Layer::Command)){self.key(KeyCode::Char('/'));}
        for c in s.chars() {
            self.key(KeyCode::Char(c));
        }
    }

    /// 敲一条命令并回车。场景和游戏菜单层不接普通打字（STAGE-131），先按 `/` 打开命令栏再打。
    /// 有模态层（战斗、对话窗口）时字母照旧直接进输入行。
    pub fn command(&mut self, line: &str) {
        let open_bar = self.app.world.as_ref().is_some_and(|w| {
            (w.at_scene() || w.at_menu()) && w.battle.is_none() && w.npc_window.is_none()
        });
        if open_bar {
            self.key(KeyCode::Char('/'));
        }
        self.text(line);
        self.key(KeyCode::Enter);
    }

    /// 反复处理网络事件和 tick，直到条件成立；超时则 panic，并带上最近的消息。
    pub fn wait(&mut self, what: &str, secs: u64, cond: impl Fn(&App) -> bool) {
        let end = Instant::now() + Duration::from_secs(secs);
        while Instant::now() < end {
            self.pump();
            if cond(&self.app) {
                return;
            }
            std::thread::sleep(Duration::from_millis(10));
        }
        let msgs: Vec<_> = self.app.log.all().iter().map(|m| m.text.clone()).collect();
        panic!("等待“{what}”超时；界面 {:?}；消息 {msgs:?}", self.app.screen);
    }

    pub fn pump(&mut self) {
        self.app.pump_net();
        if self.last_tick.elapsed() >= Duration::from_millis(250) {
            self.app.on_tick(Instant::now());
            self.last_tick = Instant::now();
        }
    }

    pub fn run_for(&mut self, ms: u64) {
        let end = Instant::now() + Duration::from_millis(ms);
        while Instant::now() < end {
            self.pump();
            std::thread::sleep(Duration::from_millis(10));
        }
    }

    pub fn wait_screen(&mut self, screen: Screen) {
        self.wait(&format!("{screen:?}"), 5, |a| a.screen == screen);
    }

    pub fn wait_ready(&mut self) {
        self.wait("握手完成", 5, |a| a.session.state == stoneage_client::app::ConnState::Ready);
    }

    pub fn last_error(&self) -> String {
        self.app
            .log
            .all()
            .iter()
            .rev()
            .find(|m| m.kind == stoneage_client::app::Kind::Error)
            .map(|m| m.text.clone())
            .unwrap_or_default()
    }

    /// 中途结束：不要求用例走完，直接结束模拟服务端（只验证中间状态的场景用）。
    #[allow(dead_code)]
    pub fn abort(self) {
        let Harness { app, mut mock, .. } = self;
        drop(app);
        let _ = mock.kill();
        let _ = mock.wait();
    }

    /// 关闭客户端，等模拟服务端退出，断言它判定“全部符合”。
    pub fn finish(self) {
        self.finish_with_log();
    }

    /// 同 `finish`，再把模拟服务端的日志返回给调用方：日志里有它收到的每一条请求
    /// （`[mock] ← {...}`），用来断言“某个请求根本没有发出去”（STAGE-104 观察模式的快捷键拦截）。
    #[allow(dead_code)]
    pub fn finish_with_log(self) -> String {
        let (status, err) = self.close_and_wait();
        assert!(status.success(), "模拟服务端报告不符：\n{err}");
        err
    }

    /// 客户端在用例中途有意停下（用例里有一步界面发不出去，比如选己方倒下的单位以外的目标会被本地挡住）：
    /// 断开后模拟服务端只能报“没有走完”，之前的每一步都要逐字段一致，有“不符”就失败。
    #[allow(dead_code)]
    pub fn finish_stopped_early(self) {
        let (_, err) = self.close_and_wait();
        assert!(
            err.contains("用例没有走完") && !err.contains("处不符"),
            "停下之前的步骤应当全部一致，模拟服务端报告：\n{err}"
        );
    }

    /// 断开客户端，等模拟服务端退出，返回它的退出状态和日志。
    fn close_and_wait(self) -> (std::process::ExitStatus, String) {
        let Harness { app, mut mock, mut log, .. } = self;
        drop(app);
        let end = Instant::now() + Duration::from_secs(5);
        let status = loop {
            if let Some(s) = mock.try_wait().unwrap() {
                break s;
            }
            if Instant::now() > end {
                let _ = mock.kill();
                panic!("模拟服务端没有退出（客户端断开后应当退出）");
            }
            std::thread::sleep(Duration::from_millis(20));
        };
        let mut err = String::new();
        log.read_to_string(&mut err).ok();
        (status, err)
    }
}

pub fn opts(addr: String) -> Options {
    Options { server: addr, ..Options::default() }
}
