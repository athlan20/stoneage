<div align="center">

# 石器时代 2.5 · 命令行版

**在终端里玩石器时代。** 装好一个程序，输入 `stoneage`，注册、建角色、进游戏。

[安装](#安装) · [开始游戏](#开始游戏) · [升级](#升级) · [常见问题](#常见问题)

</div>

---

## 安装

任选一种，**装一次就够**。装完后**新开一个终端**，输入 `stoneage` 回车。

| 你的系统 | 推荐方式 | 要先装什么 |
|---|---|---|
| macOS | [Homebrew](#macos--linux) 或 [一键脚本](#macos--linux) | 无 |
| Linux | [一键脚本](#macos--linux) | 无 |
| Windows | [PowerShell 一键脚本](#windows) | 无 |
| 已有 Node.js | [npm](#npm所有系统) | Node 18+ |

支持 macOS（Apple 芯片 / Intel）、Linux x64、Windows x64。

### macOS / Linux

一键脚本（装到 `~/.cargo/bin`）：

```sh
curl --proto '=https' --tlsv1.2 -LsSf https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.sh | sh
```

或用 Homebrew：

```sh
brew install athlan20/tap/stoneage
```

### Windows

打开 PowerShell，粘贴运行：

```powershell
powershell -ExecutionPolicy Bypass -c "irm https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.ps1 | iex"
```

> `-ExecutionPolicy Bypass` 不能省。Windows 默认不让运行脚本，这个参数只对这一次生效，不改系统设置。

<details>
<summary>想装到别的目录，或不改 PATH</summary>

默认装到 `%USERPROFILE%\.cargo\bin`，并加进用户 PATH。运行安装命令**之前**先设置：

```powershell
$env:STONEAGE_CLIENT_INSTALL_DIR = "D:\games\stoneage"   # 程序会在该目录的 bin\ 下
$env:STONEAGE_CLIENT_NO_MODIFY_PATH = "1"                # 不改 PATH，之后用完整路径运行
```

</details>

### npm（所有系统）

```sh
npm i -g stoneage-cli      # 安装
npx stoneage-cli           # 或者免安装，直接试玩
```

### 从源码编译（Rust 开发者）

```sh
cargo install --locked --git https://github.com/athlan20/stoneage stoneage-client --bin stoneage
```

> 这种方式默认连本机服务端。

---

## 开始游戏

```sh
stoneage
```

1. 终端窗口建议开到 **120 × 40** 以上，字体选等宽字体。
2. 第一次进入先注册账号，再创建角色，然后进入世界。
3. 默认连线上服务端，不用配置。

**最常用的几个键**

| 按键 | 作用 |
|---|---|
| `Esc` | 打开游戏菜单；在菜单里逐层退回 |
| `↑` `↓` `←` `→` | 选择对象、切换栏目 |
| `Enter` | 确认 / 打开动作 |
| `/` | 打开命令栏，输入命令 |
| `Ctrl+C` | 退出（会先确认，默认不退出） |

**常用启动参数**

| 参数 | 作用 |
|---|---|
| `--server 地址` | 连别的服务端，如 `--server 127.0.0.1:7065` 或 `--server wss://域名/ws` |
| `--account 账号` | 预先填好账号 |
| `--unicode false` | 终端显示乱码或方块时用，改用兼容性更好的细线和点阵字符 |
| `--reduce-motion` | 减少动画，适合刷新慢的终端 |
| `--no-update-check` | 启动时不检查新版本 |

---

## 升级

```sh
stoneage update
```

启动时如果发现新版本，底部消息栏会提示一行。`stoneage update` 会按你的安装方式处理：

| 安装方式 | 升级方式 |
|---|---|
| 一键脚本 | `stoneage update` 直接升级 |
| Homebrew | `brew upgrade stoneage` |
| npm | `npm update -g stoneage-cli` |
| cargo | 重新执行上面的 `cargo install` 命令 |

---

## 常见问题

**装完提示找不到 `stoneage`？**
已经打开的终端不认新的 PATH，关掉重新开一个。

**Windows 运行安装命令报「禁止运行脚本」？**
用上面带 `-ExecutionPolicy Bypass` 的完整命令，不要只粘贴后半段。

**界面错位、出现乱码或方块？**
先把窗口放大到 120 × 40 以上；还不行就加 `--unicode false`。Windows 推荐使用 Windows Terminal。

**连不上服务端？**
检查网络，或用 `--server` 指定地址。

---

<sub>本仓库只放发布包和发布流程，源码不在这里。维护者请看 [MAINTAINING.md](MAINTAINING.md)。</sub>
