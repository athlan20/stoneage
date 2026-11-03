//! 联调用的真实服务端进程：编译 `cmd/stoneage-server`，用 `-temp-schema` 在库里建临时 schema 运行。

use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::{Duration, Instant};

// ---------- 真实服务端 ----------

pub struct Server {
    child: Child,
    pub addr: String,
    pub ws_addr: String,
    stderr: Arc<Mutex<Vec<String>>>,
    protocol_log: PathBuf,
    _dir: PathBuf,
}

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).parent().unwrap().to_path_buf()
}

/// 编译一次服务端，所有测试共用。编译不了（没有 Go）返回 None。
fn server_binary() -> Option<&'static PathBuf> {
    static BIN: OnceLock<Option<PathBuf>> = OnceLock::new();
    BIN.get_or_init(|| {
        let out = Path::new(env!("CARGO_MANIFEST_DIR")).join("target/e2e");
        std::fs::create_dir_all(&out).ok()?;
        let bin = out.join("stoneage-server");
        let status = Command::new("go")
            .args(["build", "-o"])
            .arg(&bin)
            .arg("./cmd/stoneage-server")
            .current_dir(repo_root())
            .status()
            .ok()?;
        status.success().then_some(bin)
    })
    .as_ref()
}

impl Server {
    /// 启动服务端；没有测试数据库或没有 Go 时返回 None（测试跳过）。
    pub fn start() -> Option<Server> {
        let Ok(db) = std::env::var("STONEAGE_TEST_DATABASE_URL") else {
            eprintln!("跳过：没有设置 STONEAGE_TEST_DATABASE_URL");
            return None;
        };
        let Some(bin) = server_binary() else {
            eprintln!("跳过：没有 Go 工具链，编译不了服务端");
            return None;
        };
        static N: std::sync::atomic::AtomicU32 = std::sync::atomic::AtomicU32::new(0);
        let dir = std::env::temp_dir().join(format!("stoneage-e2e-{}-{}", std::process::id(), N.fetch_add(1, std::sync::atomic::Ordering::SeqCst)));
        std::fs::create_dir_all(&dir).unwrap();
        let protocol_log = dir.join("protocol.log");
        let mut child = Command::new(bin)
            .args(["-env", "/nonexistent.env", "-temp-schema", "-shutdown-stdin", "-protocol-log"])
            .arg(&protocol_log)
            .env("STONEAGE_LISTEN", "127.0.0.1:0")
            .env("STONEAGE_WS_LISTEN", "127.0.0.1:0")
            .env("STONEAGE_DATABASE_URL", db)
            .current_dir(repo_root())
            .stdin(Stdio::piped())
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .spawn()
            .expect("无法启动服务端");
        let mut reader = BufReader::new(child.stderr.take().unwrap());
        let lines = Arc::new(Mutex::new(Vec::new()));
        let mut addr = None;
        let mut ws_addr=String::new();
        let mut line = String::new();
        let deadline = Instant::now() + Duration::from_secs(30);
        while addr.is_none() && Instant::now() < deadline {
            line.clear();
            if reader.read_line(&mut line).unwrap() == 0 {
                break;
            }
            lines.lock().unwrap().push(line.clone());
            if line.contains("服务端已启动") {
                ws_addr=line.split("ws_listen=").nth(1).and_then(|r|r.split_whitespace().next()).unwrap_or("").to_string();
                addr = line.split("listen=").nth(1).and_then(|r| r.split_whitespace().next()).map(String::from);
            }
        }
        let Some(addr) = addr else {
            let _ = child.kill();
            panic!("服务端没有启动成功：\n{}", lines.lock().unwrap().join(""));
        };
        // 后台继续读完日志，既防止管道写满，也留给“日志里没有明文密码”的检查。
        let sink = lines.clone();
        std::thread::spawn(move || {
            let mut l = String::new();
            while reader.read_line(&mut l).unwrap_or(0) > 0 {
                sink.lock().unwrap().push(std::mem::take(&mut l));
            }
        });
        Some(Server { child, addr, ws_addr, stderr: lines, protocol_log, _dir: dir })
    }

    /// 发 SIGTERM 让服务端正常关闭（保存数据、通知连接、删临时 schema），等它退出。
    pub fn stop(&mut self) {
        drop(self.child.stdin.take());
        let end = Instant::now() + Duration::from_secs(15);
        while Instant::now() < end {
            if self.child.try_wait().unwrap().is_some() {
                return;
            }
            std::thread::sleep(Duration::from_millis(50));
        }
        let _ = self.child.kill();
        let _ = self.child.wait();
    }

    /// 服务端日志和协议日志里不能出现明文密码。
    pub fn assert_no_plaintext(&self, secrets: &[&str]) {
        let log = self.stderr.lock().unwrap().join("");
        let proto = std::fs::read_to_string(&self.protocol_log).unwrap_or_default();
        assert!(!proto.is_empty(), "协议日志是空的，检查没有意义");
        for s in secrets {
            assert!(!log.contains(s), "服务端日志里出现了明文密码 {s}");
            assert!(!proto.contains(s), "协议日志里出现了明文密码 {s}");
        }
    }
}

impl Drop for Server {
    fn drop(&mut self) {
        if std::thread::panicking(){eprintln!("服务端协议诊断：{}",std::fs::read_to_string(&self.protocol_log).unwrap_or_default());}
        if self.child.try_wait().ok().flatten().is_none() {
            self.stop();
        }
        let _ = std::fs::remove_dir_all(&self._dir);
    }
}

