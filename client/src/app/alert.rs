//! 提示音（STAGE-265）：打输后响一声。这里是**唯一**的播放入口，调用方只认 `play_lose_alert()`。
//!
//! 声音文件是 `client/assets/sounds/lose.wav`（WAV，单声道 11025 Hz 8 位），编译期用 `include_bytes!`
//! 嵌进二进制，运行时写到系统临时目录，再交给系统自带的播放器（不等待、不接管终端）：
//! macOS `afplay`；Linux 依次试 `paplay`、`aplay`；Windows 用 PowerShell 的 `SoundPlayer`。
//! 找不到播放器、写文件失败或 spawn 失败时，退回终端响铃（BEL）；播放失败不报错、不弹窗。
//!
//! 替换声音：换 `assets/sounds/lose.wav` 即可（保持 WAV 格式，`LOSE_WAV` 的测试会卡住非 WAV 文件）；
//! 要换格式（如 mp3/ogg）就改 `candidates()` 里的播放器列表，并同步 `docs/auto-battle-settings.md`。
//! 不引入音频 crate：Linux 上 rodio/cpal 需要 alsa 开发库，会拖累发布构建。

use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};

/// 嵌进二进制的提示音（路径相对本文件：`client/src/app/alert.rs` → `client/assets/sounds/lose.wav`）。
pub const LOSE_WAV: &[u8] = include_bytes!("../../assets/sounds/lose.wav");

/// 临时目录里的文件名（固定名，已存在且大小一致就不重写）。
const TEMP_NAME: &str = "stoneage-client-lose.wav";

/// 打输后响一声。每次调用都记一次（`lose_rings`）；单元测试里和 `set_muted(true)` 之后不真的发声。
pub fn play_lose_alert() {
    LOSE_RINGS.with(|c| c.set(c.get() + 1));
    if cfg!(test) || MUTED.load(Ordering::Relaxed) {
        return;
    }
    play_with(&mut System);
}

static MUTED: AtomicBool = AtomicBool::new(false);

/// 静音开关：集成测试在场景开头调用，免得跑测试时真的响。
pub fn set_muted(on: bool) {
    MUTED.store(on, Ordering::Relaxed);
}

/// 播放的出口：真实环境是系统播放器和终端 BEL，测试里可以换成记录用的实现。
pub trait Sink {
    /// 用外部程序播放文件；能 spawn 出去就返回 true（不等待它结束）。
    fn spawn_player(&mut self, program: &str, args: &[String]) -> bool;
    /// 终端响铃。
    fn bell(&mut self);
}

/// 按顺序尝试：写临时文件 → 系统播放器 → 终端 BEL。
pub fn play_with(sink: &mut dyn Sink) {
    let Some(path) = materialize(&std::env::temp_dir()) else {
        sink.bell();
        return;
    };
    let played = candidates(&path)
        .iter()
        .any(|(program, args)| sink.spawn_player(program, args));
    if !played {
        sink.bell();
    }
}

/// 把嵌入的字节写到 `dir` 下的固定文件名；已存在且大小一致就直接用。失败返回 `None`。
pub fn materialize(dir: &Path) -> Option<PathBuf> {
    let path = dir.join(TEMP_NAME);
    let same = std::fs::metadata(&path).is_ok_and(|m| m.len() == LOSE_WAV.len() as u64);
    if !same && std::fs::write(&path, LOSE_WAV).is_err() {
        return None;
    }
    Some(path)
}

/// 本平台的播放器候选，按优先级排列。
fn candidates(path: &Path) -> Vec<(String, Vec<String>)> {
    let p = path.to_string_lossy().into_owned();
    if cfg!(target_os = "macos") {
        vec![("afplay".into(), vec![p])]
    } else if cfg!(target_os = "linux") {
        vec![
            ("paplay".into(), vec![p.clone()]),
            ("aplay".into(), vec!["-q".into(), p]),
        ]
    } else if cfg!(target_os = "windows") {
        let script = format!("(New-Object Media.SoundPlayer '{p}').PlaySync()");
        vec![(
            "powershell".into(),
            vec!["-NoProfile".into(), "-Command".into(), script],
        )]
    } else {
        Vec::new()
    }
}

/// 真实的出口：子进程的输入输出全部接到 null，不往终端写任何东西（否则会破坏 ratatui 的渲染）。
struct System;

impl Sink for System {
    fn spawn_player(&mut self, program: &str, args: &[String]) -> bool {
        let child = Command::new(program)
            .args(args)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn();
        match child {
            Ok(mut child) => {
                // 不等播放结束，但交给一个线程回收，免得退出前留下僵尸进程。
                std::thread::spawn(move || {
                    let _ = child.wait();
                });
                true
            }
            Err(_) => false,
        }
    }

    fn bell(&mut self) {
        let mut out = std::io::stdout();
        let _ = out.write_all(b"\x07");
        let _ = out.flush();
    }
}

thread_local! {
    static LOSE_RINGS: std::cell::Cell<usize> = const { std::cell::Cell::new(0) };
}

/// 当前线程里 `play_lose_alert` 被调用了几次（测试断言“响了几次”用）。
pub fn lose_rings() -> usize {
    LOSE_RINGS.with(|c| c.get())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 记录每次尝试的播放器；`accept` 为真的那个播放器 spawn 成功。
    #[derive(Default)]
    struct Fake {
        tried: Vec<String>,
        accept: Option<String>,
        bells: usize,
    }

    impl Sink for Fake {
        fn spawn_player(&mut self, program: &str, _args: &[String]) -> bool {
            self.tried.push(program.to_string());
            self.accept.as_deref() == Some(program)
        }
        fn bell(&mut self) {
            self.bells += 1;
        }
    }

    #[test]
    fn embedded_sound_is_a_wav_file() {
        assert!(LOSE_WAV.len() > 1000, "长度 {}", LOSE_WAV.len());
        assert_eq!(&LOSE_WAV[0..4], b"RIFF");
        assert_eq!(&LOSE_WAV[8..12], b"WAVE");
    }

    #[test]
    fn a_working_player_stops_the_search_and_no_bell_rings() {
        let all = candidates(Path::new("x"));
        let Some((first, _)) = all.first().cloned() else { return };
        let mut f = Fake {
            accept: Some(first.clone()),
            ..Fake::default()
        };
        play_with(&mut f);
        assert_eq!(f.bells, 0, "播放器起来了就不响铃");
        assert_eq!(f.tried, vec![first], "第一个能起来的就停");
    }

    #[test]
    fn every_player_failing_falls_back_to_bell() {
        let mut f = Fake::default();
        play_with(&mut f);
        assert_eq!(f.bells, 1, "全部失败才退回 BEL");
        assert_eq!(f.tried.len(), candidates(Path::new("x")).len(), "依次试完全部候选");
    }

    #[test]
    fn writes_the_file_once_and_reuses_it() {
        let dir = std::env::temp_dir().join(format!("stoneage-alert-test-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let p = materialize(&dir).expect("可写的临时目录");
        assert_eq!(std::fs::read(&p).unwrap(), LOSE_WAV);
        let before = std::fs::metadata(&p).unwrap().modified().unwrap();
        let again = materialize(&dir).unwrap();
        assert_eq!(again, p);
        assert_eq!(
            std::fs::metadata(&p).unwrap().modified().unwrap(),
            before,
            "大小一致就不重写"
        );
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn unwritable_dir_gives_no_file() {
        assert!(materialize(Path::new("/nonexistent-stoneage-dir/xyz")).is_none());
    }

    #[test]
    fn the_test_hook_counts_rings() {
        let before = lose_rings();
        play_lose_alert();
        play_lose_alert();
        assert_eq!(lose_rings(), before + 2);
    }
}
