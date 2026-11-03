# AGENTS.md

本文件给在本仓库里工作的 AI 编码助手（Claude Code 等）和贡献者一个快速的项目导览。

## 个性化开发

- 读取本地 `AGENTS.local.md`（如果有，已被 gitignore），里面有个人开发的注意事项。

## 项目定位

独立开发的石器时代 2.5 **命令行（TUI）客户端** + **用 Go 重写的服务端**，两者之间使用我们自定义的协议。

- **服务端**（`server/`，Go）：与客户端之间使用自定义协议（`docs/protocol.md`，TCP 上的 JSON Lines，另有 WebSocket 入口），不兼容原版 2.5 客户端；账号与角色数据存 PostgreSQL。Go module 路径为 `github.com/athlan20/stoneage`。
- **客户端**（`client/`，Rust + ratatui）：TCP 直连服务端，不经过 Web/HTTP。
- **游戏规则**（建角校验、出生点、数值公式、战斗、任务等）对照原版 2.5 的行为实现；各处代码注释和 `docs/` 里写明了对应的原版出处。网络协议不沿用原版。
- **石器百科（wiki）**：`internal/gamewiki`（前端 `site/` 与嵌入的数据快照 `snapshot/`）、`internal/{gamecatalog,aiknowledge}`、`cmd/stoneage-wiki-index`、`cmd/stoneage-wiki`（独立 HTTP 入口），说明见 `docs/game-wiki.md`。
- `demo/ratatui-demo/`：ratatui 官方经典 demo（上游 `examples/apps/demo`），**CLI 界面风格的基准**，见下文“界面风格”。

## 原版数据与素材（不随仓库提供）

仓库不包含原版游戏的素材、数据表或源码，使用者自备：

- `assets/original/`：原版 2.5 美术素材（`battle/*.png` 背景、`sprites.json` 动画索引、`bitmaps/`、`maps/`、`pal/`）。demo 默认读取此目录（`--assets DIR` 可覆盖）。
- `original-data/`：原版 2.5 数据表（如 `original-data/gmsv/data`，GBK 编码），只有 `tools/` 下的生成脚本和少数对照测试需要；没有时相关测试自动跳过。也可以用环境变量 `STONEAGE_ORIGINAL_DATA` 指向数据表目录。

两个目录都在 `.gitignore` 里，不要提交。服务端运行时需要的数据已经转成 UTF-8 的 tsv/json，嵌入在 `server/gamedata/` 下，不依赖上述目录。

## 常用命令

Rust 工具链在 `~/.cargo/bin`，非交互 shell 可能不在 `PATH` 中。

```sh
# 服务端（详见 docs/server.md）
scripts/server-restart.sh                       # 一键重启：停掉正在运行的、重新编译、启动（默认开协议日志，--no-protocol-log 关闭，--foreground 不放后台）
scripts/server-start.sh [--protocol-log]       # 编译并后台启动服务端；数据库用 .env 的连接串（本机装 PostgreSQL 即可）
scripts/server-status.sh                        # 查看服务端、数据库连接、表结构版本
scripts/server-stop.sh                          # 停止服务端（数据库不受影响）
go run ./cmd/stoneage-server -protocol-log -    # 前台直接运行，便于调试，协议日志打到终端
go run ./cmd/stoneage-casetest -reset-db        # 协议测试客户端：对运行中的服务端重放 docs/protocol/cases/（会清空本地开发库的业务数据）
go run ./cmd/stoneage-admin accounts            # 账号管理：accounts / disable / enable / unlock
go test -race ./server/...                      # 服务端单元测试 + 协议用例回归（需要数据库的测试设 STONEAGE_TEST_DATABASE_URL）
go run ./cmd/stoneage-mcp -server 127.0.0.1:7065   # MCP 服务（stdio，令牌用环境变量 STONEAGE_AGENT_TOKEN）；接入见 docs/mcp.md
go test -race ./internal/agentmcp/                 # MCP 服务与会话层测试
scripts/smoke.sh [--full]                       # 冒烟测试：真实服务端（临时 schema）+ 真实客户端二进制走完整流程，见 docs/smoke.md

# 石器百科（快照已编译进二进制）
go run ./cmd/stoneage-wiki                      # http://127.0.0.1:8090/wiki/；-listen 改地址，-cdn 指定图片来源
node --test internal/gamewiki/site_test.js internal/gamewiki/search_worker_test.js

# demo
cd demo/ratatui-demo && cargo run --release     # 界面风格基准；方向键/hjkl 切换，t 开关图表，q 退出；--unicode false 看降级效果

# 客户端（详见 docs/client.md）
cd client && cargo build
cargo run --bin mock-server -- --case register_ok   # 模拟服务端，按 docs/protocol/cases/ 回放；--list 看全部场景
cargo run --bin stoneage                            # 客户端，另开终端连模拟服务端
scripts/client.sh [--account alice ...]             # 一键启动客户端：编译并运行，参数原样传给客户端，默认连本机服务端
cargo test --test keys_nav                          # 日常：只跑一个测试文件，按改动范围选（选法见 docs/client.md“测试”）
cargo test --no-fail-fast                           # 全量，较慢（链接占大头）；只在改了公共代码或收尾时跑一次
```

## 架构

```text
CLI 客户端 ──JSON Lines / TCP（docs/protocol.md）──▶ Go 服务端（server/）──▶ PostgreSQL
```

- **协议**：`docs/protocol.md` 是客户端与服务端之间的唯一约定（消息、状态、错误码、默认值），前后端各自照它开发；改协议先改文档和 `docs/protocol/cases/` 用例，再改代码。双方用同一套用例做契约测试：服务端有协议测试客户端，客户端有模拟服务端。
- **文本编码**：协议全程 UTF-8，文本长度一律按 UTF-8 字符数计算。原版数据表是 CP936/GBK，只在生成脚本读取时转成 UTF-8。

## 文档导览

- `docs/ui.md`：主界面（上菜单 / 中内容 / 下消息与命令输入）、菜单页内容、命令与按键，界面开发以它为准。
- `docs/interaction.md`：全局交互规范——按键分层、每个键的统一含义、确认方式、提示行、快捷键与命令的对应、鼠标与终端兼容；所有页面的按键都要符合它。
- `docs/protocol.md`：客户端（以及 agent）与服务端之间的协议约定，是接口的唯一依据。
- `docs/client.md`：客户端的构建、模拟服务端、测试和代码结构。
- `docs/smoke.md`：前后端联调的冒烟测试。
- `docs/server.md`：服务端内部的约定：`.env` 配置、本地数据库、协议测试客户端。
- `docs/login.md`：登录、注册、角色选择与创建的界面和交互。
- `docs/agent.md`、`docs/agent-guide.md`：agent 协同游玩（令牌登录、控制权、私聊）的规则、界面和使用手册；消息定义在 `docs/protocol.md`。
- `docs/mcp.md`：stoneage-mcp（MCP 接入）的安装、配置、工具清单与限制。
- `docs/map.md`：世界地图数据与快捷传送目录。
- `docs/ride.md`、`docs/encounter.md`、`docs/pk.md` 等：各玩法的设计与原版对照。
- `docs/game-wiki.md`：石器百科。

## 界面风格

CLI 界面风格**完全参考 `demo/ratatui-demo/`**（`src/ui.rs` 是样板）。写客户端界面前先跑一遍 demo、对照 `ui.rs` 的写法，不要另起视觉方案。具体包括：

- **整体布局**：上中下三块——顶部带边框的导航路径与状态行，中间是当前那一层（场景 / 游戏菜单 / 功能页），底部是消息加命令栏；页内用 `Layout` 按 `Length`/`Min`/`Percentage` 切分成若干面板。具体见 `docs/ui.md`。
- **面板**：每块内容都用 `Block::bordered().title(..)` 包起来，标题简短；需要强调的面板标题加色并加粗。
- **控件对照**（游戏数据 → demo 控件）：HP/MP/经验等进度用 `Gauge`/`LineGauge`；物品、宠物、技能等列表用带编号的 `List`/`Table`，编号与命令参数一致；系统消息/战斗记录/聊天用滚动 `List`，按级别用固定颜色的定宽前缀；属性、队伍、线路选择用带黄色表头的 `Table`；主界面是一整张世界地图，用 `Canvas` + Braille 标记绘制（设计见 `docs/map.md`）；趋势类数据用 `Sparkline`/`BarChart`；说明文字用 `Paragraph` + `Wrap`。
- **配色**：只用 ratatui 的 16 色具名颜色（`Color::Green`、`Color::Yellow` 等），不用 RGB/256 色，保证各平台终端效果一致。正常/异常状态沿用 demo 的绿/红区分。
- **字符与降级**：照搬 demo 的 `enhanced_graphics` 开关（`--unicode`），开启时用 Braille、九级块、粗线，关闭时退回 Dot、三级块、细线。
- **运行方式**：crossterm 后端，raw mode + 备用屏幕，固定 tick（默认 250ms）驱动刷新，退出时恢复终端。
- **按键**：进入世界落在场景页，`Esc` 打开游戏菜单、选中一项 `Enter` 进功能页、`Esc` 逐层退回；场景与游戏菜单里普通字符不接收，要打命令按 `/` 打开命令栏。功能页用“对象 → 动作 → 参数”：`↑↓` 选对象，`←→` 换本页栏目/任务分类，`Enter` 打开动作，`Esc` 逐层返回；普通字符只在专用表单或 `/` 命令栏接收。`Ctrl+C` 打开默认取消的退出确认；按键规则见 `docs/interaction.md`，主界面按键表见 `docs/ui.md`。

## 约定

- 生成物、运行时数据和编译产物写入 `build/`、`runtime/`、`target/`，均不提交。
- 新增或修改请求、推送时，同时改 `docs/protocol.md`、协议用例，以及 `docs/agent-guide.md` 第 6 节的先问玩家分级表（`go test ./server/webpages` 会检查，新增的操作类请求没分级就失败）。
- 提交信息、代码注释可以用需求号（如 `STAGE-29`）开头，方便对照 `docs/` 里的设计说明。

## 代码规范

- 单个手写代码文件达到 300 行时，检查是否存在可独立拆分的职责；300 行是评审提醒，不是强制拆分线。
- 单个手写代码文件不得超过 500 行；按职责拆分模块，避免通过压缩格式、挪动空行或无意义拆分来满足行数限制。
- 自动生成的文件不受上述行数限制；不要手工修改生成文件以规避限制。

## 开发流程（分支与 worktree）

一个云效需求或缺陷对应一个分支、一个 worktree；`test` 只用来测试，**不往 develop 合**，测试通过的是需求分支本身合进 develop。

```text
develop ──stage-start──▶ STAGE-29-背包穿脱整理（worktree ../stoneage-open-STAGE-29）
                              │ stage-to-test                │ stage-finish（测试通过后）
                              ▼                              ▼
                         test（worktree ../stoneage-open-test）   develop ──（发布时）──▶ main
```

| 步骤 | 命令 | 云效状态 |
|---|---|---|
| 1. 从云效领一个“待处理”的需求，从 develop 开分支和 worktree | `scripts/stage-start.sh STAGE-29 背包穿脱整理` | 开发中 / 处理中 |
| 2. 在 worktree 里开发、自测、提交（提交信息以需求号开头） | `cd ../stoneage-open-STAGE-29` | |
| 3. 先在需求分支里同步 develop（见下“合并前先同步 develop”），再合进 test，在 `../stoneage-open-test` 里测试 | `scripts/stage-to-test.sh STAGE-29` | 测试中（技术类需求：已完成）；缺陷：已修复 |
| 4a. 测试不通过：回需求分支修改，再执行第 3 步 | | 回到开发中 |
| 4b. 测试通过：需求分支合进 develop，删 worktree 和分支 | `scripts/stage-finish.sh STAGE-29` | 已完成；缺陷：已关闭 |

- **分支名**：`需求号-短标题`，如 `STAGE-29-背包穿脱整理`；缺陷用缺陷号。短标题只用汉字、字母、数字、`-`、`_`，不超过 20 个字，不带空格、【】和标点（脚本会检查）。worktree 目录 = 主仓库目录名 + 需求号（主仓库在 `stoneage-open`，所以是 `../stoneage-open-STAGE-29`），保持纯 ASCII 路径。
- **合并**：一律 `--no-ff`，合并提交写成 `STAGE-29-背包穿脱整理：合入 test/develop`，保留“这批提交属于哪个需求”。冲突时脚本停下，解决并提交后再运行一次即可。合进 develop 时如果有冲突，最好先在需求分支上合一次 develop 解决，test 上也就不会再冲突。
- **合并前先同步 develop**（第 3 步和 `stage-finish` 之前都做）：在需求分支的 worktree 里执行 `git merge develop`，把冲突在需求分支上解决掉，再跑 `stage-to-test.sh` / `stage-finish.sh`。已知每次必冲突的两处：
  - `server/casetest/cases_test.go` 的 `enabledStages` 一整行：两边的 `"STAGE-N": true` 取并集。
  - `docs/agent-changes.md`：新条目都写在最上面，两边的条目都保留；`protocol.md` / `agent-guide.md` 合并后 `docs_rev` 会变，`go test ./server/webpages` 会报“当前 docs_rev 没登记”，在最上面补一条“合并记录”（标题写它报出的修订号，正文写明无新增内容、指向下面已登记的条目）。
  同步后跑 `go build ./...` 和改动涉及的包测试（至少 `./server/webpages ./server/casetest/`），再提交合并。
- **test 分支**：第一次运行 `stage-to-test.sh` 时从 develop 建出来。test 上积累的未通过需求太多或太乱时，用 `scripts/stage-to-test.sh --reset STAGE-29` 把它重置成 develop 再合（其他还在测试的需求要重新合一次）。
- **跳过测试**：`stage-finish.sh` 默认要求需求分支的最新提交已经在 test 里；纯文档、脚本这类不需要测试的改动可以加 `--force` 直接合进 develop。只合并、不清理 worktree 用 `--keep`。
- **并行开发**：多个 worktree 共用本机 PostgreSQL 和服务端端口（7065），表结构版本也可能不同。同一时间只在一个 worktree 里跑 `scripts/server-*.sh`（测试阶段就是 `../stoneage-open-test`）；需求分支自测优先用 `scripts/smoke.sh`，它用临时 schema，互不影响。`stage-start.sh` 会把主仓库的 `.env` 复制进新 worktree。
- **推送与发布**：脚本只做本地合并，不推送；develop 合进 main 是发布动作，由用户决定什么时候做。

## 云效协作平台

- 本项目所在云效的项目名是stoneage
- 提到云效，需求，任务，缺陷，优先使用yunxiao技能去查询
- 提的需求和缺陷优先以本游戏顶部menu里的功能开头，比如 需求【背包】XXXXXX功能
- 云效里的需求或者缺陷，开头要以”说人话“方式描述一下，然后再具体铺开
- 云效的需求，只开发“待处理”的，一旦开始某个需求开发，把状态改成（开发中 或 处理中），开发完成后 状态优先改成 ”待测试“ ，如果技术类需求则改成 ”已完成“
- 如果某个需求无法落地开发，则改成”设计中“ 并且评论上需要设计的理由
- 云效的缺陷，只需要测试”已修复“的缺陷， 如果测试通过，则改成”已关闭“，如果不通过，则改成”再次打开“并添加评论

### 需求完成定义（DoD）

一个需求或缺陷**算做完**，除了代码本身，还要满足下面几条；缺任何一条都不算完成：

- [ ] 代码已提交，提交信息以需求号/缺陷号开头
- [ ] 测试按改动范围通过（单元测试 / 协议用例 / 界面验收）。**开发过程中不要反复跑全量**：客户端用 `cargo test --test <文件>`，服务端用 `go test ./server/<改到的包>/`；全量只在改了公共代码（`tests/support/mod.rs`、`App` 主循环、协议解析、`server/casetest`）或交付前跑一次
- [ ] 相关文档同步（改协议先改 `docs/protocol.md` 和 `docs/protocol/cases/`）
- [ ] **云效工作项状态已更新**，改完再用 `get_work_item` 复核一次 `status`
- [ ] 需要别人验收的，把验收要点写进工作项评论

### 状态名对照（规范用词 ≠ 云效实际状态名）

| 规范里的说法 | 云效实际状态 | statusId |
| --- | --- | --- |
| 待处理 / 开发中 / 设计中 | 同名 | `100005` / `142838` / `156603` |
| **待测试** | **「测试中」**（Testing）—— 云效没有“待测试”这个状态名 | `100012` |
| 已完成 / 已取消 | 同名 | `100014` / `141230` |
| 缺陷：待确认 / 已修复 / 已关闭 | 同名 | `28` / `29` / `100085` |

改状态用 `update_work_item`，把 statusId 放进 `updateWorkItemFields.status`。

### ⚠️ 状态不会自己变

`scripts/stage-*.sh` **只做本地 Git 合并**，不推送、也不调云效接口；它们只在结尾打印一行提示
（如「1. 云效里把 STAGE-29 改成「开发中」」）。所以“改状态”这一步**全靠执行的人记得**，
是最容易漏的收尾项 —— STAGE-131 做完后状态就一直停在「待处理」。
走非 worktree 流程（按需求直接在某个分支上开发）时，连那行提示都不会出现，必须自己盯。
