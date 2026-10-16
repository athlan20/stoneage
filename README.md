# stoneage

石器时代 2.5 命令行（TUI）客户端的发布页。

这个仓库只放两样东西：

- **GitHub Release**：各平台的压缩包、一键安装脚本（`stoneage-client-installer.sh` / `.ps1`）和校验和。
- **发布流程**：`.github/workflows/`。源码不在这里，在云效（Codeup）的 stoneage 仓库里。

## 安装

```sh
curl --proto '=https' --tlsv1.2 -LsSf https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.sh | sh
brew install athlan20/tap/stoneage
npm i -g stoneage-cli
```

Windows 用 PowerShell 运行下面这条（带 `-ExecutionPolicy Bypass`：Windows 默认的执行策略是 Restricted，安装脚本的自检会拒绝运行，只对这一次生效，不改系统设置）：

```powershell
powershell -ExecutionPolicy Bypass -c "irm https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.ps1 | iex"
```

默认装到 `%USERPROFILE%\.cargo\bin`，并把它加进用户 PATH。要装到别处（如 `D:\games\stoneage`），先设环境变量再运行同一条命令，程序会在该目录的 `bin\` 下：

```powershell
$env:STONEAGE_CLIENT_INSTALL_DIR = "D:\games\stoneage"
```

装好后**新开**一个终端再执行 `stoneage`（已打开的终端不认新的 PATH）。不想改 PATH 就先设 `$env:STONEAGE_CLIENT_NO_MODIFY_PATH = "1"`，之后用完整路径运行。

## 发布

在源码仓库里运行 `scripts/release-github.sh v0.1.0`：它把这里的内容同步过来，并打上同名 tag。tag 推上去以后，GitHub Actions 按这个 tag 从云效拉源码，编译四个平台，创建 Release，再发布 npm 包。流程细节见源码仓库的 `docs/client.md`「发布」。

仓库需要配置（Settings → Secrets and variables → Actions）：

| 名称 | 类型 | 用途 |
|---|---|---|
| `CODEUP_URL` | 变量 | 云效源码仓库的 HTTPS 地址 |
| `CODEUP_TOKEN` | 密钥 | 云效 Codeup 的访问令牌，只需要读源码仓库 |
| `CODEUP_USERNAME` | 变量（可选） | 令牌对应的用户名，默认 `oauth2` |
| `STONEAGE_ONLINE_SERVER` | 变量 | 线上服务端地址，如 `wss://game.example.com/ws`，编译发布版时写进客户端 |
| `HOMEBREW_TAP_TOKEN` | 密钥 | 推送 Homebrew 配方到 `athlan20/homebrew-tap`，需要对该仓库有写权限。没有它 Homebrew 任务失败，Release 已建但 announce 被跳过 |
