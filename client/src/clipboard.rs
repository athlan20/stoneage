//! STAGE-171：把文本写进系统剪贴板。两条路同时走：
//! - OSC 52 终端转义序列：cmux、Ghostty、iTerm2、kitty、WezTerm、Windows Terminal 支持，SSH 远程也能写进本机剪贴板；
//! - macOS 本机直接调 `pbcopy`（自带 Terminal.app 不支持 OSC 52）。SSH 登录时跳过，免得写进远端机器的剪贴板。
//!
//! 复制是尽力而为：两条路都失败也不报错，调用方在消息栏里同时保留了逐行说明，玩家还能手动选中。

use std::io::{self, Write};
use std::process::{Command, Stdio};

use crossterm::clipboard::CopyToClipboard;
use crossterm::execute;

pub fn copy(text: &str) {
    let _ = execute!(io::stdout(), CopyToClipboard::to_clipboard_from(text));
    if cfg!(target_os = "macos") && std::env::var_os("SSH_CONNECTION").is_none() {
        let _ = pbcopy(text);
    }
}

fn pbcopy(text: &str) -> io::Result<()> {
    let mut child = Command::new("pbcopy")
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()?;
    if let Some(mut stdin) = child.stdin.take() {
        stdin.write_all(text.as_bytes())?;
    }
    child.wait().map(|_| ())
}
