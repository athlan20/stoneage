# stoneage

石器时代 2.5 命令行（TUI）客户端的发布页。

这个仓库只放两样东西：

- **GitHub Release**：各平台的压缩包、一键安装脚本（`stoneage-client-installer.sh` / `.ps1`）和校验和。
- **发布流程**：`.github/workflows/`，源码就在这个仓库里。

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

## 让 AI 玩石器（MCP）

让 Claude Code、Claude Desktop 等 AI 工具替你在游戏里操作角色。

1. 在游戏里输入 `agent` 生成令牌。令牌以 `sat_` 开头，只显示一次，丢了就重新生成。
2. 从本页的 Release 下载对应平台的 `stoneage-mcp` 包，解压，把 `stoneage-mcp` 放到 PATH 里的目录（Windows 也可以不放，用完整路径运行）：
   - macOS（Apple 芯片）`stoneage-mcp-<版本>-darwin-arm64.tar.gz`，macOS（Intel）`stoneage-mcp-<版本>-darwin-amd64.tar.gz`，Linux x64 `stoneage-mcp-<版本>-linux-amd64.tar.gz`，Windows x64 `stoneage-mcp-<版本>-windows-amd64.zip`。
   - macOS / Linux：`tar -xzf stoneage-mcp-<版本>-<系统>-<架构>.tar.gz && mv stoneage-mcp ~/.local/bin/`（先确认 `~/.local/bin` 在 PATH 里）。
   - Windows（PowerShell）：`Expand-Archive stoneage-mcp-<版本>-windows-amd64.zip -DestinationPath $env:USERPROFILE\stoneage-mcp`，之后用完整路径 `%USERPROFILE%\stoneage-mcp\stoneage-mcp.exe`。
   - macOS 首次运行可能被系统拦下，到「系统设置 → 隐私与安全性」点「仍要打开」。
3. 接入：
   - Claude Code：

     ```sh
     claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_xxx -- stoneage-mcp --server <线上地址>
     ```

   - Claude Desktop：在 `claude_desktop_config.json` 的 `mcpServers` 里加一项：

     ```json
     "stoneage": {
       "command": "stoneage-mcp",
       "args": ["--server", "<线上地址>"],
       "env": { "STONEAGE_AGENT_TOKEN": "sat_xxx" }
     }
     ```

   `<线上地址>` 这里不写死，游戏里 `agent` 命令显示的接入命令里已经带好，照抄即可；`sat_xxx` 换成第 1 步的令牌。
4. 完整说明（工具、资源、排错）见游戏服务端的 `/mcp.md`，和 `/install.md` 在同一主机上，如 `https://主机/mcp.md`。

## 发布

运行 `scripts/release-github.sh v0.1.0`：它校验版本号后打上同名 tag 并推上去。tag 推上去以后，GitHub Actions 按这个 tag 检出源码，编译四个平台，创建 Release，再发布 npm 包。流程细节见 `docs/client.md`「发布」。

仓库需要配置（Settings → Secrets and variables → Actions）：

| 名称 | 类型 | 用途 |
|---|---|---|
| `STONEAGE_ONLINE_SERVER` | 变量 | 线上服务端地址，如 `wss://game.example.com/ws`，编译发布版时写进客户端 |
| `HOMEBREW_TAP_TOKEN` | 密钥 | 推送 Homebrew 配方到 `athlan20/homebrew-tap`，需要对该仓库有写权限。没有它 Homebrew 任务失败，Release 已建但 announce 被跳过 |
