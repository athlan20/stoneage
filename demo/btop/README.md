# btop（Windows 版 btop4win）

[btop](https://github.com/aristocratos/btop) 是 Linux / macOS 上的系统监控终端工具，官方没有提供 Windows 版本。Windows 上请用作者维护的移植版 **btop4win**：

- 项目：<https://github.com/aristocratos/btop4win>
- 与 btop 的关系：btop++ 在 Windows 上的移植版，主题文件与 btop 通用。

## 安装（二选一）

### 方案 A：Scoop（推荐）

先安装 [Scoop](https://scoop.sh)，然后在 PowerShell 里运行：

```powershell
scoop install btop          # 标准版，不需要管理员权限
# scoop install btop-lhm    # LHM 版，功能更完整，但必须以管理员身份运行
```

### 方案 B：下载 exe

到 btop4win 仓库的 Releases 页面下载最新版的安装包，解压后运行 `btop4win.exe`。

## 运行前提

- 需要安装最新的 MSVC 运行库（`vc_redist.x64.exe`）。
- 建议用 Windows Terminal。标准 cmd 需要 Windows 10 1607 或更高版本，才能正确显示 ANSI 转义序列。

## 功能限制（标准版）

- 不支持 GPU 监控和 CPU 温度，CPU 频率也不够准确。
- 不以管理员身份运行时，部分进程信息会缺失。
- 页面注明：Windows 11 上启动时的误报问题尚未修复。

## 运行

在 Windows Terminal 里执行 `btop4win`（或直接运行 `btop4win.exe`）。按键与 btop 相同：`h` 查看帮助，`q` 退出。
