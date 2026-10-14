# stoneage

石器时代 2.5 命令行（TUI）客户端的发布页。

这个仓库只放两样东西：

- **GitHub Release**：各平台的压缩包、一键安装脚本（`stoneage-client-installer.sh` / `.ps1`）和校验和。
- **发布流程**：`.github/workflows/`。源码不在这里，在云效（Codeup）的 stoneage 仓库里。

## 安装

```sh
curl --proto '=https' --tlsv1.2 -LsSf https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.sh | sh
npm i -g stoneage-cli
```

Windows 用 PowerShell 运行 `irm https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.ps1 | iex`。

## 发布

在源码仓库里运行 `scripts/release-github.sh v0.1.0`：它把这里的内容同步过来，并打上同名 tag。tag 推上去以后，GitHub Actions 按这个 tag 从云效拉源码，编译四个平台，创建 Release，再发布 npm 包。流程细节见源码仓库的 `docs/client.md`「发布」。

仓库需要配置（Settings → Secrets and variables → Actions）：

| 名称 | 类型 | 用途 |
|---|---|---|
| `CODEUP_URL` | 变量 | 云效源码仓库的 HTTPS 地址 |
| `CODEUP_TOKEN` | 密钥 | 云效 Codeup 的访问令牌，只需要读源码仓库 |
| `CODEUP_USERNAME` | 变量（可选） | 令牌对应的用户名，默认 `oauth2` |
| `STONEAGE_ONLINE_SERVER` | 变量 | 线上服务端地址（`host:port`），编译发布版时写进客户端 |
| `NPM_TOKEN` | 密钥（可选） | npm 访问令牌，对 `stoneage-cli` 有发布权限。不设置就跳过 npm 发布 |
