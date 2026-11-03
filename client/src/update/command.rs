//! `stoneage update` 命令（STAGE-99）：查最新版；有新版时按安装方式自己升级，或提示包管理器的升级命令。

use std::path::Path;
use std::process::{Command, ExitStatus};
use std::time::Duration;

use super::install::{self, InstallKind};
use super::park;
use super::source::{fetch_latest, latest_url};
use super::version;

/// npm 主包名（`client/npm/`，见 docs/client.md「安装」）。
pub const NPM_PACKAGE: &str = "stoneage-cli";

/// Homebrew formula 名（`client/dist.toml` 的 `formula`，tap 是 `athlan20/tap`）。
const BREW_FORMULA: &str = "stoneage";

/// 手动执行命令时等待网络的时间，比启动时的后台检查宽松。
const COMMAND_TIMEOUT: Duration = Duration::from_secs(20);

/// 执行 `stoneage update`。出错时返回给调用方打印（调用方负责退出码）。
pub fn run() -> Result<(), String> {
    println!("正在检查最新版本…");
    let latest = fetch_latest(&latest_url(), COMMAND_TIMEOUT).map_err(|e| format!("无法检查新版本：{e}"))?;
    let current = version::current();
    if !version::is_upgrade(&current, &latest) {
        println!("已是最新版本 {current}");
        return Ok(());
    }
    match install::current() {
        InstallKind::Npm if cfg!(windows) => {
            // STAGE-270：Windows 上运行中的 exe 被占用，npm 替换可能失败，真机未验证前仍只提示命令。
            println!(
                "有新版本 {latest}（当前 {current}）。这是 npm 安装的，请运行：{}",
                npm_install_command(&latest.to_string())
            );
            Ok(())
        }
        InstallKind::Npm => {
            // STAGE-270：自己运行 npm 升级。npm 在原位替换文件，shell 的 PATH 不变，所以不用重启终端。
            println!("有新版本 {latest}（当前 {current}），开始用 npm 升级…");
            run_npm_update("npm", &latest.to_string())?;
            println!("升级完成：已更新到 {latest}，重新运行 stoneage 即可（不需要重启终端）。");
            Ok(())
        }
        InstallKind::Homebrew => {
            // STAGE-271：自己运行 brew。先 `brew update` 刷新 tap 里的配方，否则 `brew upgrade` 看不到刚发的新版本。
            println!("有新版本 {latest}（当前 {current}），开始用 Homebrew 升级…");
            run_brew_upgrade("brew")?;
            println!("升级完成：已更新到 {latest}，重新运行 stoneage 即可（不需要重启终端）。");
            Ok(())
        }
        InstallKind::Installer { updater } => {
            println!("有新版本 {latest}（当前 {current}），开始升级…");
            run_updater(&updater)?;
            println!("升级完成：已更新到 {latest}，重新启动 stoneage 后生效。");
            Ok(())
        }
        InstallKind::Cargo => {
            println!("有新版本 {latest}（当前 {current}）。这是 cargo install 安装的，请重新执行安装命令（见 docs/client.md「安装」）。");
            Ok(())
        }
        InstallKind::Other => {
            println!("有新版本 {latest}（当前 {current}）。无法判断安装方式，请重新运行一键安装脚本（见 docs/client.md「安装」）。");
            Ok(())
        }
    }
}

/// 运行一键安装附带的更新器。Windows 上正在运行的 exe 不能被覆盖，所以先把当前的 `stoneage.exe` 改名；
/// 更新器要在安装目录里原地运行（原因见 `park`）。失败，或者更新器没报错却没装出新 exe 时，把旧 exe 改回来。
fn run_updater(updater: &Path) -> Result<(), String> {
    if !cfg!(windows) {
        return check_updater(Command::new(updater).status(), updater);
    }
    let exe = std::env::current_exe().map_err(|e| format!("无法确定当前程序的位置：{e}"))?;
    let old = park::park(&exe).map_err(|e| format!("无法改名正在运行的 {}：{e}", exe.display()))?;
    let result = check_updater(Command::new(updater).status(), updater).and_then(|()| {
        if exe.exists() {
            Ok(())
        } else {
            Err("更新器没有报错，但没有装出新的 stoneage.exe（已恢复旧版本）".to_string())
        }
    });
    if result.is_err() {
        park::restore(&exe, &old);
    }
    result
}

/// STAGE-272：npm 升级用的手动命令。不用 `npm update`：registry 元数据缓存没过期时它认为已是最新，
/// 刚发布的版本装不上；指定版本号并加 `--prefer-online`，强制重新向 registry 查询，装的版本也和提示一致。
fn npm_install_command(latest: &str) -> String {
    format!("npm install -g {NPM_PACKAGE}@{latest} --prefer-online")
}

/// STAGE-270：运行 `<npm> install -g stoneage-cli@<latest> --prefer-online`，输出原样透传给用户。失败时带上手动命令，方便用户自己处理
/// （常见原因：npm 不在 PATH 里，或全局目录没有写权限）。
fn run_npm_update(npm: &str, latest: &str) -> Result<(), String> {
    let manual = npm_install_command(latest);
    let package = format!("{NPM_PACKAGE}@{latest}");
    run_tool(npm, &["install", "-g", &package, "--prefer-online"], "npm", &manual)
}

/// STAGE-271：依次运行 `<brew> update` 和 `<brew> upgrade stoneage`，输出原样透传。任何一步失败都停下，并给出手动命令。
fn run_brew_upgrade(brew: &str) -> Result<(), String> {
    let manual = format!("brew update && brew upgrade {BREW_FORMULA}");
    run_tool(brew, &["update"], "brew update", &manual)?;
    run_tool(brew, &["upgrade", BREW_FORMULA], "brew upgrade", &manual)
}

/// 运行一个包管理器命令，输出透传；`what` 是出错提示里的名字，`manual` 是手动命令。
fn run_tool(program: &str, args: &[&str], what: &str, manual: &str) -> Result<(), String> {
    match Command::new(program).args(args).status() {
        Ok(status) if status.success() => Ok(()),
        Ok(status) => Err(format!(
            "{what} 失败（退出码 {}）。可以手动运行：{manual}",
            status.code().map_or("未知".into(), |c| c.to_string())
        )),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            Err(format!("找不到 {program}（不在 PATH 里）。可以手动运行：{manual}"))
        }
        Err(e) => Err(format!("无法运行 {program}：{e}。可以手动运行：{manual}")),
    }
}

fn check_updater(status: std::io::Result<ExitStatus>, updater: &Path) -> Result<(), String> {
    let status = status.map_err(|e| format!("无法启动更新器 {}：{e}", updater.display()))?;
    if !status.success() {
        return Err(format!(
            "升级失败（更新器退出码 {}）",
            status.code().map_or("未知".into(), |c| c.to_string())
        ));
    }
    Ok(())
}

#[cfg(all(test, unix))]
mod tests {
    use super::*;

    #[test]
    fn npm_update_succeeds_when_the_command_exits_zero() {
        assert!(run_npm_update("true", "9.9.9").is_ok());
    }

    #[test]
    fn npm_update_failure_reports_exit_code_and_manual_command() {
        let err = run_npm_update("false", "9.9.9").unwrap_err();
        assert!(err.contains("退出码 1"), "{err}");
        assert!(
            err.contains("npm install -g stoneage-cli@9.9.9 --prefer-online"),
            "{err}"
        );
    }

    #[test]
    fn missing_npm_is_reported() {
        let err = run_npm_update("stoneage-no-such-npm", "9.9.9").unwrap_err();
        assert!(
            err.contains("找不到 stoneage-no-such-npm") && err.contains("npm install -g stoneage-cli@9.9.9"),
            "{err}"
        );
    }

    #[test]
    fn brew_upgrade_runs_both_steps_and_succeeds() {
        assert!(run_brew_upgrade("true").is_ok());
    }

    #[test]
    fn brew_failure_reports_step_and_manual_command() {
        let err = run_brew_upgrade("false").unwrap_err();
        assert!(
            err.contains("brew update 失败") && err.contains("brew update && brew upgrade stoneage"),
            "{err}"
        );
    }

    #[test]
    fn missing_brew_is_reported() {
        let err = run_brew_upgrade("stoneage-no-such-brew").unwrap_err();
        assert!(
            err.contains("找不到 stoneage-no-such-brew") && err.contains("brew upgrade stoneage"),
            "{err}"
        );
    }
}
