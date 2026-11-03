# MCP 接入：stoneage-mcp

`stoneage-mcp` 是一个本地运行的 [MCP](https://modelcontextprotocol.io) 服务（stdio）。Claude Code、Claude Desktop 等 AI 工具通过它调用工具来玩石器时代：AI 直接调用「连接」「查看状态」「传送」「出招」「私聊玩家」这类工具，不用自己处理连接、命名管道和推送。

它只是一个新的 agent 客户端：用玩家给的 agent 令牌连游戏服务端，令牌、控制权、观察方被拒、审计这些规则全部沿用服务端现有的实现（`docs/protocol.md` 第 9 节，`docs/agent-guide.md`）。它不改协议，也不改服务端规则。

> 本期只有本地 stdio 版：玩家在自己机器上运行 `stoneage-mcp`。服务端托管的远程 MCP（玩家只填 URL）以后再说。

## 怎么装

**从 Release 下载（玩家，不需要 Go）**

发布页（GitHub Release，见 `docs/install.md`）每个版本都带四个平台的包，文件名是 `stoneage-mcp-<版本>-<系统>-<架构>`：

| 平台 | 包 |
|---|---|
| macOS（Apple 芯片） | `darwin-arm64.tar.gz` |
| macOS（Intel） | `darwin-amd64.tar.gz` |
| Linux x64 | `linux-amd64.tar.gz` |
| Windows x64 | `windows-amd64.zip` |

包里只有二进制（`stoneage-mcp`，Windows 为 `stoneage-mcp.exe`）和本说明（`MCP.md`），解开就能用。校验和见同一 Release 的 `stoneage-mcp-<版本>-SHA256SUMS`。把二进制放进 PATH 的目录，或者配置时写完整路径。macOS 首次运行可能被系统拦下，到「系统设置 → 隐私与安全性」点「仍要打开」。

**从源码编译（开发者）**

在仓库里编译（需要 Go，版本见 `go.mod`）：

```sh
go build -o stoneage-mcp ./cmd/stoneage-mcp
# 或者安装到 $GOPATH/bin
go install ./cmd/stoneage-mcp
```

令牌在游戏里生成（「Agent」页，或协议 9.1 `create_agent_token`）。令牌以 `sat_` 开头，只显示一次，丢了就重新生成。

## 怎么配置

启动参数：

| 参数 | 说明 |
|---|---|
| `--server` | 游戏服务端地址。`host:port` 走 TCP；`ws://`、`wss://` 走 WebSocket（和客户端的约定一样，见 `docs/protocol.md` 第 1 节）。必填。 |
| `--token-file` | 令牌所在的文件。不写则读环境变量 `STONEAGE_AGENT_TOKEN`。 |
| `--wiki` | 石器百科的 `http(s)` 地址，只给 TCP 连接的 `wiki_search` 用。WebSocket 连接时用同一台主机，不用写。 |
| `--log` | 协议日志写到这个文件（可选）。令牌和密码都会脱敏。 |

令牌不能写在命令行参数里（会留在进程列表和 shell 历史里），用环境变量或 `--token-file`。

**Claude Code**

```sh
claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_xxx -- stoneage-mcp --server wss://主机/ws
# 本机开发
claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_xxx -- stoneage-mcp --server 127.0.0.1:7065
```

装好后在 Claude Code 里输入 `/mcp`，能看到 `stoneage` 的工具和资源。

**Claude Desktop**（`claude_desktop_config.json` 的 `mcpServers` 里加一项）

```json
"stoneage": {
  "command": "stoneage-mcp",
  "args": ["--server", "wss://主机/ws"],
  "env": { "STONEAGE_AGENT_TOKEN": "sat_xxx" }
}
```

`command` 写 `stoneage-mcp` 的完整路径更稳妥。

## 第一次使用

1. AI 先读资源 `stoneage://docs/agent-guide`（服务端的 agent 手册，规则和「先问玩家」的分级都在里面）。`stoneage-mcp` 的 `instructions` 也会提示它这样做。
2. 调用 `connect`：连接、登录、取第一个角色（也可以传 `slot`）并进入世界。进程启动时**不连接**，第一次调用工具时才连。
3. 之后用 `status`、`battle`、`battle_command`、`wait_events`、`say_to_player`、`report_plan` 等工具玩。

## 工具

每个工具的结果都是一段 JSON 文本：

```jsonc
{"ok": true, "data": {...}}                       // 或 {"ok": false, "error": {"code": "...", "message": "..."}}
 "events": [...],                                 // 上次调用以来积压的推送（取出后清空），没有就省略
 "dropped_events": 3,                             // 推送队列满了丢掉的条数，有丢弃时才出现（队列上限 500 条）
 "docs_updated": "文档已更新：docs_rev … → …"      // 文档修订号变了时才出现，提示去读 stoneage://docs/changes
```

| 工具 | 作用 | 对应协议 |
|---|---|---|
| `connect` | 连接、登录，取角色进入世界；可选 `slot`。已经在世界里时返回当前状态。断线后再调用会重连并重新进入世界。 | 5.1、9.2、5.5、5.8 |
| `request` | 通用：`type` + `data`，发任意请求并等响应。`hello`、`agent_login`、`ping`、`battle_ack` 由 MCP 服务自己管，不能用它发；其他类型（包括 MCP 不认识的新类型）一律转给服务端，由服务端判断权限和状态，错误原样返回。分级不写在工具描述里，见 `stoneage://docs/agent-guide` 第 6 节，第三级操作要先用 `say_to_player` 问玩家。 | 全部 |
| `status` | 当前状态 | 5.12 |
| `inventory` | 背包 | 5.17 |
| `pets` | 宠物 | 5.19 |
| `warp` | 传送，`place` 或 `floor` 二选一 | 5.16 |
| `battle` | 查看当前战斗：回合、阶段、需要下的指令、单位 | 10.11 |
| `battle_command` | 出招：`turn`、`actor`、`action` 必填，其余参数按协议 10.5 原样传 | 10.5 |
| `wait_events` | 等推送：`types`（可选）、`timeout_seconds`（默认 30，最大 120）。等到匹配的推送或超时，返回期间的全部推送。要等的推送已经在上一次工具结果的 `events` 里时立刻返回，`already_delivered` 列出类型。 | — |
| `say_to_player` | 私聊玩家（汇报、问确认） | 9.5 |
| `report_plan` | 上报计划（整份替换） | 9.7 |
| `wiki_search` | 查石器百科：`q`、`kind`、`limit`，或 `key` 查详情 | `docs/game-wiki.md` |

打一场战斗的常见循环：`battle` 看回合和 `needs` → `battle_command` 出招 → `wait_events`（`types`: `battle_turn`、`battle_end`）等下一回合或结束。回放确认由 MCP 服务自动发，不用管。

## 资源

| URI | 内容 |
|---|---|
| `stoneage://docs/agent-guide` | agent 手册（`docs/agent-guide.md`） |
| `stoneage://docs/protocol` | 协议（`docs/protocol.md`） |
| `stoneage://docs/changes` | agent 变更记录（`docs/agent-changes.md`） |
| `stoneage://docs/leveling` | 练级路线（`docs/leveling.md`） |

WebSocket 连接时从服务端取（和服务端版本一致）；TCP 连接时读 `stoneage-mcp` 里内嵌的副本。

## 行为要点

- **断线**：服务端断开、网络断了，下一次工具调用时自动重连（重新登录）。连续 3 次连接失败后不再重连，工具返回 `RECONNECT_LIMIT`，重启进程才会再试。
- **不自动重连的原因**：`KICKED_DUPLICATE_LOGIN`、`KICKED_BY_PLAYER`、`AGENT_TOKEN_REVOKED`、`ACCOUNT_DISABLED`、`INVALID_TOKEN`。之后的工具调用直接返回这个原因和应对说明（停手，等玩家指示），重启进程才会再试。
- **断线推送**：连接断了、而服务端没有推送 `disconnect` 时，`events` 里会补一条 `mcp_closed`（`data.reason` 写明原因）。服务端推了 `disconnect` 的，就是它本身。
- **控制权**：玩家收回控制权后，操作类工具返回 `CONTROLLED_BY_PLAYER`，AI 应该停手。`control_changed` 推送出现在 `events` 里。
- **心跳**：按服务端 `hello` 给的间隔自动发 `ping`。
- **超时**：单个请求等 15 秒。
- **令牌**：只放在进程内存里，不写日志、不进工具结果。

## 和 agent-connect.py 的关系

`docs/agent-connect.py` 是参考脚本，靠命名管道和 `tail` 工作。`stoneage-mcp` 做的是同一件事，区别是：

- 请求和响应由 MCP 服务按 id 配对，AI 不用自己对；
- 推送先缓存，随下一次工具结果返回，或用 `wait_events` 等；
- 战斗回放确认自动发送；
- docs_rev 记录共用 `~/.stoneage-agent/docs-rev.json`（环境变量 `STONEAGE_AGENT_STATE` 可改），两个工具之间不会重复提醒。

规则（何时做什么、先问谁、断线怎么办）两者一致，都以 `docs/agent-guide.md` 为准。

## 已知限制

- 只有本地 stdio 版，不能远程托管。
- 只支持 agent 令牌，不能用玩家账号登录，也不能建角、删角、管理令牌。
- 推送队列上限 500 条，积压太多时最旧的会被丢掉，结果里的 `dropped_events` 会告诉你丢了几条。
- 第三级操作（`docs/agent-guide.md` 第 6 节）靠 AI 自觉先问玩家，服务端和 MCP 服务都不拦。
- **不需要升级 `stoneage-mcp` 的情况**（STAGE-303）：服务端新增请求类型、新推送、手册内容（含第 6 节分级表）的变化，都不用升级 MCP。`request` 不认识的类型也原样转给服务端；工具说明只引用手册。注意 TCP 连接时手册读的是 MCP 内嵌的副本，要拿服务端最新手册需用 WebSocket 连接。只有协议版本号、握手/登录/心跳/战斗回放确认的规则，或需要 MCP 自动回应的新推送变化，才需要升级 MCP。
- 百科的 `wiki_search` 在 TCP 连接时需要 `--wiki`。
- 其他工具不会替你进入世界。断线后要先调 `connect`。

## 排错

- 工具返回 `CONNECT_FAILED`：地址或网络不对，或服务端没开。检查 `--server`。
- 返回 `INVALID_TOKEN`、`AGENT_TOKEN_REVOKED`：令牌无效，让玩家重新生成。
- 返回 `CLIENT_TOO_OLD`：`stoneage-mcp` 版本低于服务端要求，更新它。
- 需要看发了什么、收到什么：加 `--log /某个/文件`，内容已脱敏。
