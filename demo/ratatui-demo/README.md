# Ratatui 经典演示（demo）

来自 [ratatui](https://github.com/ratatui/ratatui) 仓库的 `examples/apps/demo`（上游 commit `cdccc2b`）。源码与上游一致，只改了 `Cargo.toml`，使它能脱离 ratatui 源码树单独编译：

- 对 workspace 的引用（`xxx.workspace = true`）改成了显式版本，包名改为 `ratatui-demo`；
- 去掉了 workspace 的 lint 配置。

许可证为 MIT（同 ratatui）。

## 在 Windows 上运行

1. 安装 Rust：打开 <https://rustup.rs>，下载并运行 `rustup-init.exe`，默认选项即可。安装过程中会提示安装 Visual Studio C++ 生成工具（MSVC），按提示装好。需要 Rust 1.88 或更新版本，rustup 安装的 stable 满足要求。
2. 在本目录打开 PowerShell 或 Windows Terminal，运行：

   ```powershell
   cargo run --release
   ```

   第一次会下载并编译依赖，需要几十秒到几分钟。

## 操作

- 方向键或 `h` `j` `k` `l` 切换、移动。
- `q` 退出。

## 说明

- 默认使用 crossterm 后端，Windows 下开箱即用。
- 建议用 Windows Terminal 运行，颜色和字符显示比老的 cmd 窗口好。
- macOS / Linux 上同样可以 `cargo run --release`。
