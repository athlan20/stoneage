//! Windows 上的升级文件处理：正在运行的 exe 不能被覆盖，但可以改名。
//!
//! 一键安装的更新器会把新的 `stoneage.exe` 写进安装目录，而它正被 `stoneage update` 自己占用。所以升级前：
//! 1. 把正在运行的 `stoneage.exe` 改名为 `stoneage.exe.old`，让安装器能写入同名的新文件；
//! 2. 更新器必须在安装目录里原地运行：它会先核对“安装回执对应的是不是自己这个 exe”，从别处（如临时目录的
//!    副本）运行会被判成“不是同一份安装”，直接报 “Already up to date” 并以 0 退出、什么都不装。
//!    更新器自己的 exe 由它改名为 `.previous.exe` 再替换，不用我们处理；
//! 3. 安装失败，或成功后 `stoneage.exe` 仍不存在，就把旧文件改回来；成功后，下次启动时删掉 `.old`。
//! 其他平台不需要这些步骤，`stoneage update` 直接运行更新器。

use std::fs;
use std::io;
use std::path::{Path, PathBuf};

/// 正在运行的 exe 改名后的路径：同目录，文件名后面加 `.old`。
pub fn old_path(exe: &Path) -> PathBuf {
    let mut name = exe.file_name().unwrap_or_default().to_os_string();
    name.push(".old");
    exe.with_file_name(name)
}

/// 把正在运行的 exe 改名，返回旧文件的路径。
pub fn park(exe: &Path) -> io::Result<PathBuf> {
    let old = old_path(exe);
    let _ = fs::remove_file(&old);
    fs::rename(exe, &old)?;
    Ok(old)
}

/// 安装失败时调用：新文件没有写进去，就把旧文件改回来；新文件已经在了就保持原样。
pub fn restore(exe: &Path, old: &Path) {
    if !exe.exists() {
        let _ = fs::rename(old, exe);
    }
}

/// 启动时调用：删掉上次升级留下的旧文件。还被占用时删不掉，留到下次。
/// 只有新的 exe 已经在原位时才删：新文件不在，说明上次升级没装成功，`.old` 是仅剩的一份。
pub fn remove_leftovers(exe: &Path) {
    if exe.exists() {
        let _ = fs::remove_file(old_path(exe));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn dir(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("stoneage-park-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    #[test]
    fn old_path_adds_suffix_next_to_exe() {
        assert_eq!(
            old_path(Path::new("bin/stoneage.exe")),
            PathBuf::from("bin/stoneage.exe.old")
        );
    }

    #[test]
    fn park_then_restore_puts_the_old_exe_back() {
        let exe = dir("restore").join("stoneage.exe");
        fs::write(&exe, b"old").unwrap();
        let old = park(&exe).unwrap();
        assert!(!exe.exists());
        restore(&exe, &old);
        assert_eq!(fs::read(&exe).unwrap(), b"old");
        assert!(!old.exists());
    }

    #[test]
    fn restore_keeps_a_new_exe_that_was_already_written() {
        let exe = dir("keep").join("stoneage.exe");
        fs::write(&exe, b"old").unwrap();
        let old = park(&exe).unwrap();
        fs::write(&exe, b"new").unwrap();
        restore(&exe, &old);
        assert_eq!(fs::read(&exe).unwrap(), b"new");
        assert_eq!(fs::read(&old).unwrap(), b"old");
    }

    #[test]
    fn leftovers_are_removed_at_startup() {
        let exe = dir("leftover").join("stoneage.exe");
        fs::write(&exe, b"new").unwrap();
        fs::write(old_path(&exe), b"old").unwrap();
        remove_leftovers(&exe);
        assert!(!old_path(&exe).exists());
        assert_eq!(fs::read(&exe).unwrap(), b"new");
    }

    #[test]
    fn old_exe_is_kept_when_the_new_one_is_missing() {
        let exe = dir("keep-old").join("stoneage.exe");
        fs::write(old_path(&exe), b"old").unwrap();
        remove_leftovers(&exe);
        assert_eq!(fs::read(old_path(&exe)).unwrap(), b"old");
    }
}
