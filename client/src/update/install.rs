//! 判断客户端是怎么装的（STAGE-99）。决定 `stoneage update` 是自己升级，还是只提示用哪个包管理器升级。

use std::path::{Path, PathBuf};

/// cargo-dist 随一键安装脚本附带的更新器（`install-updater = true`）的程序名，和 `stoneage` 放在同一目录。
/// 名字按 cargo-dist 的 app 名（包名 `stoneage-client`）取；首次发布后要核对一次（见 docs/client.md「升级」）。
pub const UPDATER_BIN: &str = "stoneage-client-update";

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum InstallKind {
    /// npm 全局安装：可执行文件在 `node_modules` 里。
    Npm,
    /// Homebrew 安装：可执行文件在 Cellar 里（macOS 和 Linuxbrew）。
    Homebrew,
    /// 一键脚本安装：旁边有 cargo-dist 的更新器，可以直接调用它升级。
    Installer { updater: PathBuf },
    /// `cargo install` 安装：在 `.cargo/bin` 里，但没有更新器。
    Cargo,
    /// 其他情况（手动放的一份二进制等），无法判断。
    Other,
}

/// 按可执行文件路径判断安装方式。`exe` 应是解析过符号链接之后的真实路径。
/// `updater` 是更新器的路径（存在时才传），由调用方检查文件是否存在，这里只做路径判断。
pub fn detect(exe: &Path, updater: Option<PathBuf>) -> InstallKind {
    let parts: Vec<&str> = exe.components().filter_map(|c| c.as_os_str().to_str()).collect();
    let has = |name: &str| parts.contains(&name);
    if has("node_modules") {
        InstallKind::Npm
    } else if has("Cellar") || has("Homebrew") || has("homebrew") {
        InstallKind::Homebrew
    } else if let Some(updater) = updater {
        InstallKind::Installer { updater }
    } else if parts.windows(2).any(|w| w == [".cargo", "bin"]) {
        InstallKind::Cargo
    } else {
        InstallKind::Other
    }
}

/// 和 `exe` 同目录的更新器路径（Windows 下带 `.exe`）。
pub fn updater_path(exe: &Path) -> PathBuf {
    exe.with_file_name(format!("{UPDATER_BIN}{}", std::env::consts::EXE_SUFFIX))
}

/// 检测当前进程的安装方式：先解析符号链接（Homebrew 的 bin 目录是指向 Cellar 的链接），再看路径。
pub fn current() -> InstallKind {
    let exe = std::env::current_exe().ok().map(|p| std::fs::canonicalize(&p).unwrap_or(p));
    match exe {
        Some(exe) => {
            let updater = updater_path(&exe);
            detect(&exe, updater.is_file().then_some(updater))
        }
        None => InstallKind::Other,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn npm_install_is_detected_from_node_modules() {
        let exe = Path::new("/usr/local/lib/node_modules/stoneage-cli/node_modules/@stoneage/darwin-arm64/bin/stoneage");
        assert_eq!(detect(exe, None), InstallKind::Npm);
    }

    #[test]
    fn homebrew_install_is_detected_from_cellar() {
        assert_eq!(
            detect(Path::new("/opt/homebrew/Cellar/stoneage/0.2.0/bin/stoneage"), None),
            InstallKind::Homebrew
        );
        assert_eq!(
            detect(Path::new("/home/linuxbrew/.linuxbrew/Cellar/stoneage/0.2.0/bin/stoneage"), None),
            InstallKind::Homebrew
        );
    }

    #[test]
    fn installer_with_updater_beside_it() {
        let exe = Path::new("/Users/me/.cargo/bin/stoneage");
        let updater = updater_path(exe);
        assert_eq!(detect(exe, Some(updater.clone())), InstallKind::Installer { updater });
    }

    #[test]
    fn cargo_install_without_updater() {
        assert_eq!(detect(Path::new("/Users/me/.cargo/bin/stoneage"), None), InstallKind::Cargo);
    }

    #[test]
    fn unknown_location_is_other() {
        assert_eq!(detect(Path::new("/opt/games/stoneage"), None), InstallKind::Other);
    }

    #[test]
    fn package_manager_wins_over_updater() {
        // 包管理器装的目录里即使有更新器，也不能让它覆盖包管理器管理的文件。
        let exe = Path::new("/opt/homebrew/Cellar/stoneage/0.2.0/bin/stoneage");
        let updater = updater_path(exe);
        assert_eq!(detect(exe, Some(updater)), InstallKind::Homebrew);
    }
}
