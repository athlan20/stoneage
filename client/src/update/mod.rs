//! 客户端自升级（STAGE-99）：启动时后台查一次新版本，`stoneage update` 手动升级。
//!
//! 版本比较见 `version`，查询见 `source`，安装方式判断见 `install`，Windows 上的文件替换见 `park`，命令见 `command`。

pub mod command;
pub mod install;
pub mod park;
pub mod source;
pub mod version;

use std::sync::mpsc::{self, Receiver};
use std::thread;
use std::time::Duration;

use semver::Version;

/// 启动时后台检查的超时；超时或失败就静默跳过，不影响进游戏。
const BACKGROUND_TIMEOUT: Duration = Duration::from_secs(3);

/// 关闭启动时检查的环境变量（离线和内网使用），设为非空即关闭。`--no-update-check` 同效。
pub const DISABLE_ENV: &str = "STONEAGE_NO_UPDATE_CHECK";

pub fn disabled_by_env() -> bool {
    std::env::var_os(DISABLE_ENV).is_some_and(|v| !v.is_empty())
}

/// 启动时清理上次升级留下的文件（Windows 上旧的 exe 要等进程退出后才能删，见 `park`）。
pub fn cleanup_after_upgrade() {
    if let Ok(exe) = std::env::current_exe() {
        park::remove_leftovers(&exe);
    }
}

/// 在后台线程查一次最新版本。只有比当前版本新的稳定版才会通过返回的通道发出来；
/// 失败或不需要升级时通道直接关闭，调用方不用处理错误。
pub fn spawn_check() -> Receiver<Version> {
    spawn_check_from(source::latest_url(), BACKGROUND_TIMEOUT)
}

fn spawn_check_from(url: String, timeout: Duration) -> Receiver<Version> {
    let (tx, rx) = mpsc::channel();
    thread::spawn(move || {
        if let Ok(latest) = source::fetch_latest(&url, timeout) {
            if version::is_upgrade(&version::current(), &latest) {
                let _ = tx.send(latest);
            }
        }
    });
    rx
}

#[cfg(test)]
pub(crate) mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::thread;
    use std::time::Duration;

    /// 起一个只回一次固定响应的本地 HTTP 服务，返回发布接口的地址。
    pub(crate) fn serve_once(status: &str, body: &'static str) -> String {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        let status = status.to_string();
        thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            let mut buf = [0u8; 2048];
            let _ = stream.read(&mut buf);
            let resp = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
            let _ = stream.write_all(resp.as_bytes());
        });
        format!("http://{addr}/releases/latest")
    }

    #[test]
    fn newer_release_is_sent_to_channel() {
        let rx = super::spawn_check_from(serve_once("200 OK", r#"{"tag_name":"v9.0.0"}"#), Duration::from_secs(5));
        assert_eq!(rx.recv_timeout(Duration::from_secs(5)).unwrap().to_string(), "9.0.0");
    }

    #[test]
    fn same_or_failed_check_sends_nothing() {
        let same = super::spawn_check_from(serve_once("200 OK", r#"{"tag_name":"v0.0.1"}"#), Duration::from_secs(5));
        assert!(same.recv_timeout(Duration::from_secs(5)).is_err());
        let failed = super::spawn_check_from(serve_once("500 Internal Server Error", "{}"), Duration::from_secs(5));
        assert!(failed.recv_timeout(Duration::from_secs(5)).is_err());
    }
}
