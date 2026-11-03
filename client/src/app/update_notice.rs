//! 有新版本时在消息栏提示一行（STAGE-99）。来源有两个：启动后台检查，和握手响应里的
//! `latest_client`（STAGE-100）。同一次运行只提示一次。

use std::sync::mpsc::TryRecvError;

use semver::Version;

use super::App;
use crate::update::version::{current, is_upgrade, parse_tag};

impl App {
    /// 取后台版本检查的结果：有新版本就提示，之后不再查这个通道。
    pub(crate) fn update_tick(&mut self) {
        let Some(rx) = &self.update_rx else { return };
        match rx.try_recv() {
            Ok(latest) => {
                self.announce_new_version(&latest);
                self.update_rx = None;
            }
            Err(TryRecvError::Empty) => {}
            Err(TryRecvError::Disconnected) => self.update_rx = None,
        }
    }

    /// 握手响应里的 `latest_client`：比当前版本新就提示；没有或格式不对时什么也不做。
    pub(crate) fn on_latest_client(&mut self, latest: Option<&str>) {
        let Some(latest) = latest.and_then(parse_tag) else {
            return;
        };
        if is_upgrade(&current(), &latest) {
            self.announce_new_version(&latest);
        }
    }

    fn announce_new_version(&mut self, latest: &Version) {
        if self.update_announced {
            return;
        }
        self.update_announced = true;
        self.log
            .system(format!("有新版本 {latest}，运行 stoneage update 升级"));
    }
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use semver::Version;

    use super::super::{App, Options};

    fn app() -> App {
        App::new(Options {
            server: "127.0.0.1:7065".into(),
            ..Options::default()
        })
    }

    #[test]
    fn new_version_is_announced_once() {
        let mut app = app();
        let (tx, rx) = mpsc::channel();
        app.update_rx = Some(rx);
        app.update_tick();
        assert!(app.log.is_empty(), "没有结果时不提示");

        tx.send(Version::parse("9.1.0").unwrap()).unwrap();
        app.update_tick();
        let last = app.log.all().back().unwrap();
        assert_eq!(last.text, "有新版本 9.1.0，运行 stoneage update 升级");
        assert!(app.update_rx.is_none());

        let before = app.log.len();
        app.update_tick();
        assert_eq!(app.log.len(), before, "只提示一次");
    }

    #[test]
    fn closed_channel_is_dropped_silently() {
        let mut app = app();
        let (tx, rx) = mpsc::channel::<Version>();
        app.update_rx = Some(rx);
        drop(tx);
        app.update_tick();
        assert!(app.log.is_empty());
        assert!(app.update_rx.is_none());
    }

    #[test]
    fn disabled_check_has_no_channel() {
        assert!(app().update_rx.is_none(), "Options 默认不查新版本");
    }

    #[test]
    fn handshake_hint_is_shown_once_and_shared_with_background_check() {
        let mut app = app();
        app.on_latest_client(Some("9.1.0"));
        assert_eq!(app.log.len(), 1);
        assert_eq!(
            app.log.all().back().unwrap().text,
            "有新版本 9.1.0，运行 stoneage update 升级"
        );

        // 后台检查查到同一个版本，不再重复提示。
        let (tx, rx) = mpsc::channel();
        app.update_rx = Some(rx);
        tx.send(Version::parse("9.1.0").unwrap()).unwrap();
        app.update_tick();
        assert_eq!(app.log.len(), 1, "握手提示和后台检查合并，不重复");
    }

    #[test]
    fn handshake_hint_ignores_old_missing_and_bad_versions() {
        let mut app = app();
        app.on_latest_client(None);
        app.on_latest_client(Some("0.0.1"));
        app.on_latest_client(Some(&crate::update::version::current().to_string()));
        app.on_latest_client(Some("not-a-version"));
        assert!(
            app.log.is_empty(),
            "不比当前版本新的、没有的、格式不对的都不提示"
        );
        assert!(!app.update_announced);
    }
}
