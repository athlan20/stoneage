# 客户端（`client/`）开发说明

STAGE-140：断线只读、真实重新登录与完整刷新规则见 [reconnect.md](reconnect.md)。

本文写客户端的安装、构建、运行、模拟服务端、测试和发布。界面与交互见 `docs/login.md`、`docs/ui.md`，接口见 `docs/protocol.md`。需求见云效 STAGE-1（客户端）和 STAGE-98（一键安装）。

## 安装

玩家用发布版：一键脚本或 npm 任选一个，装好后执行 `stoneage`，默认连线上服务端。

| 入口 | 命令 | 说明 |
|---|---|---|
| 一键脚本（macOS、Linux） | `curl --proto '=https' --tlsv1.2 -LsSf https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.sh \| sh` | 装到 `~/.cargo/bin/stoneage` |
| 一键脚本（Windows，PowerShell） | `powershell -ExecutionPolicy Bypass -c "irm https://github.com/athlan20/stoneage/releases/latest/download/stoneage-client-installer.ps1 \| iex"` | 推荐，不需要 Node |
| Homebrew（macOS、Linux） | `brew install athlan20/tap/stoneage` | 装到 Homebrew；之后用 `brew upgrade stoneage` 升级 |
| npm | `npm i -g stoneage-cli`；免安装试玩：`npx stoneage-cli` | 需要 Node 18 或更新版本 |
| cargo（Rust 开发者） | `cargo install --locked --git https://github.com/athlan20/stoneage stoneage-client --bin stoneage` | 从源码编译，默认连本机；要连线上，先设 `STONEAGE_DEFAULT_SERVER=线上地址` 再装 |

Windows 安装的几点说明（脚本是 cargo-dist 生成的）：
- 命令里的 `-ExecutionPolicy Bypass` 不能省：Windows 默认执行策略是 Restricted，脚本自检会直接报错退出；只对这一个进程生效。
- 默认装到 `%USERPROFILE%\.cargo\bin`；脚本不吃目录参数，要换位置先设环境变量 `$env:STONEAGE_CLIENT_INSTALL_DIR`（程序在该目录的 `bin\` 下）；不改 PATH 设 `$env:STONEAGE_CLIENT_NO_MODIFY_PATH=1`。装完要新开终端。
- 换了安装目录后 `stoneage update` 照常可用（更新器按安装回执找到目录）。

支持的平台：macOS（Apple 芯片、Intel）、Linux x64（glibc）、Windows x64。发布流程见下面的「发布」。

从源码构建时，`STONEAGE_DEFAULT_SERVER` 没设置就默认连本机 `127.0.0.1:7065`；任何构建都能用 `--server 地址` 换服务端。

## 构建与运行

Rust 工具链在 `~/.cargo/bin`，非交互 shell 要先 `export PATH=$HOME/.cargo/bin:$PATH`。

一键启动（编译并运行，参数原样传给客户端，在项目根目录执行）：`scripts/client.sh`、`scripts/client.sh --account alice`。下面是等价的手工命令：

```sh
cd client
cargo build
cargo run --bin stoneage -- --server 127.0.0.1:7065            # 先停在登录页
cargo run --bin stoneage -- --server ws://127.0.0.1:7067/ws    # 经 WebSocket 连（STAGE-101；线上用 wss://）
STONEAGE_PASSWORD=secret1 cargo run --bin stoneage -- --account alice --character 1   # 直接进入游戏
```

启动参数和环境变量见 `docs/login.md`。客户端不写日志，也不把密码存到磁盘。

## 升级

STAGE-99。`stoneage update` 查 GitHub Releases（`athlan20/stoneage`）的最新稳定版：有新版就升级，已是最新就提示 `已是最新版本 x.y.z`。命令不进游戏界面，失败时退出码为 1。

升级方式按安装方式分开处理，不替包管理器覆盖它管理的文件：

| 安装方式 | 判断依据 | `stoneage update` 的行为 |
|---|---|---|
| 一键脚本 | 同目录有 cargo-dist 的更新器 `stoneage-client-update` | 调用更新器替换自己，完成后提示 |
| npm | 可执行文件在 `node_modules` 里 | 自己运行 `npm install -g stoneage-cli@<最新版本> --prefer-online`（STAGE-270；STAGE-272 起不用 `npm update`，避免 registry 缓存没过期时装不到刚发的版本），输出透传，完成后提示重新运行 `stoneage`，不用重启终端；找不到 npm 或 npm 失败（如没有全局目录写权限）时报原因并给出手动命令，退出码 1。Windows 上运行中的 exe 会被占用，暂仍只提示命令 |
| Homebrew | 可执行文件（解析链接后）在 Cellar 里 | 自己依次运行 `brew update`、`brew upgrade stoneage`（STAGE-271），输出透传，完成后提示重新运行 `stoneage`，不用重启终端；找不到 brew 或某一步失败时报原因并给出手动命令，退出码 1 |
| `cargo install` | 在 `.cargo/bin` 里，没有更新器 | 提示重新执行安装命令 |
| 其他 | 无法判断 | 提示重新运行一键安装脚本 |

启动时在后台线程查一次最新版本，超时 3 秒，失败或超时静默跳过，不影响进游戏。有新版时底部消息栏写一行「有新版本 x.y.z，运行 stoneage update 升级」，同一次运行只写一次。预发布版本（如 `0.2.0-rc.1`）不提示。

- 关闭启动时的检查：`--no-update-check`，或环境变量 `STONEAGE_NO_UPDATE_CHECK=1`（非空即关闭）。
- 测试用：环境变量 `STONEAGE_UPDATE_URL` 代替 GitHub 的最新发布接口，指向一个返回 `{"tag_name":"v9.0.0"}` 的本地地址即可模拟新版本。
- Windows：正在运行的 `stoneage.exe` 不能被覆盖。`stoneage update` 先把它改名为 `stoneage.exe.old`，再在安装目录里原地运行更新器；失败，或更新器没报错却没装出新 exe 时改回来，成功后下次启动时删掉 `.old`（实现见 `src/update/park.rs`）。更新器不能从临时目录的副本运行：它会核对安装回执对应的是不是自己这个 exe，副本通不过，就报 “Already up to date” 并以 0 退出、什么都不装（0.1.3 到 0.1.6 的 Windows 版本因此升级后 `stoneage` 命令消失）。修复之前的版本在 Windows 上 `stoneage update` 会报「文件正由另一进程使用」或装不出新 exe，需要用一键安装脚本覆盖安装一次；已经丢了命令的，先把安装目录（通常是 `%USERPROFILE%\.cargo\bin`）里的 `stoneage.exe.old` 改名回 `stoneage.exe`，或直接重新运行安装脚本。
- 依赖：一键脚本的更新器由 cargo-dist 生成，需要在 `dist-workspace.toml` 的 `[dist]` 里设 `install-updater = true`（STAGE-98 的发布配置）。更新器的程序名 `stoneage-client-update` 按 cargo-dist 的 app 名（包名）推断，首次发布后要核对一次，名字不对就改 `src/update/install.rs` 的 `UPDATER_BIN`。

## 模拟服务端

`mock-server` 按 `docs/protocol/cases/*.jsonl` 回放一个场景，客户端的开发和验收都连它，不依赖真实服务端。

```sh
cargo run --bin mock-server -- --list                  # 列出所有场景
cargo run --bin mock-server -- --case login_locked     # 默认监听 127.0.0.1:7065
cargo run --bin stoneage                               # 另开一个终端，连上去操作
```

- **宽松模式（默认）**：客户端发来的请求只检查字段结构和协议规则（账号密码格式、建角的点数规则等），不要求值与用例一致，所以手工操作时填什么都行；服务端的回复按用例走。不符的地方打印在模拟服务端的终端里。
- **严格模式 `--strict`**：每个值都要与用例一致，用于自动化测试。
- **`--once`**：用例走完、客户端断开后退出；有不符或没走完时退出码为 1。
- 日志里的 `password` 一律显示成 `***`。
- 用例格式（`setup` 连接、`*=样例`、`delay_ms`）见 `docs/protocol.md` 第 6 节。

## 测试

```sh
cargo test --no-fail-fast            # 全量：约 8 分钟，其中 86% 是链接 50 多个测试二进制
cargo test --test keys_nav           # 只跑一个测试文件（tests/keys_nav.rs）
cargo test --lib                     # 只跑 src/ 里的单元测试
```

**按改动范围选测试**（`tests/*.rs` 每个都是独立二进制，要各自链接一次，所以全量很慢；DoD 要的是“按改动范围通过”，不是每次全量）：

| 改动 | 跑什么 |
|---|---|
| 单个页面的渲染 | `--test render_<页>`（如 `render_pets`） |
| 按键 / 交互 | `--test keys_<域>`，改了全局按键分层则 `--test keys_global --test keys_nav --test keys_hint` |
| 某个功能的场景验收 | `--test scenarios_<功能>` |
| `tests/support/mod.rs`、`App` 主循环、协议解析、跨页状态 | 全量 |
| 纯文档 / 脚本 | 不跑 |

注意：让 cargo 重编某个文件时别对不存在的路径 `touch`（会创建空文件，如 `src/app.rs` 与 `src/app/mod.rs` 冲突导致整个 lib 编不过）；改源码本身就会触发重编。

- `tests/scenarios.rs`：STAGE-1 验收。每个测试起一个严格模式的模拟服务端，用按键驱动 `App`，覆盖注册、登录、锁定、建角、删除、进入世界、被挤下线、超时、心跳和开发快捷方式。
- `tests/render.rs`：用 `TestBackend` 画出各页，检查关键内容、密码圆点、`--unicode false` 降级，以及各种窗口大小下不出错。
- `tests/render_character.rs`：STAGE-23 角色页，检查能力、衍生数值和属性的显示、80×24 下三块都在、进度条符号随 `--unicode` 切换。
- `tests/render_quests.rs`：STAGE-86 任务页，空提示、表格与标题计数、显示范围的标题和空提示、PgUp/PgDn 翻页。
- `tests/scenarios_quests.rs`：STAGE-86 验收，回放 `ui_quests` 用例：`quests_changed` 推送不用刷新就更新列表、消息栏提示进行中和已完成（清除不提示）、`quest doing/done/all` 过滤、离开再进入进度还在。
- `tests/scenarios_points.rs`：STAGE-24 加点，回放 `ui_addpoint` 用例：`addpoint` 的消息反馈、管理命令发的点数在线推送、点数不够时报错且数值不变、能力名本地校验、离开再进入后点数还在。
- `tests/agent.rs`：STAGE-6 验收，回放 `ui_agent_*` 用例：令牌管理、观察模式（“agent 操控中”、操作命令被拦下）、`take` / `release` / `kick-agent`、私聊、吊销令牌；`ui_agent_dialog_control`（STAGE-104）覆盖观察模式下按数字选对话选项：商店窗口开着时 agent 接管，观察方按数字被拦下、模拟服务端的日志里收不到 `npc_choose`，`take` 收回控制权后同一个键才发出。
- `tests/render_battle.rs`：STAGE-37 战斗画面，直接搭 `BattleState` 画图：80×24（紧凑档）、120×40 和 200×60（完整档，敌左我右及下方日志）、`--unicode false` 降级、倒下的单位、倒计时取整、战斗记录、三种结果窗口。
- `tests/keys_global.rs`：STAGE-103 全局按键统一，不连服务端：Esc 分层（登录页、对话窗口）、确认层（放生、丢弃，是 y 才执行，Esc 取消，数字键是文字）、`page` 切页（弹窗开着也能切，战斗中不切）、Home/End 滚动（输入行为空时）、战斗结果窗口的关闭键和打字键写入输入行。
- `tests/keys_hint.rs`：STAGE-104 输入行提示与观察模式拦截，不连服务端：空行提示按当前层显示（世界、对话、确认、战斗选指令/选目标/列表/回放/结果各状态，能滚动的页面加翻页键，观察模式下换成“agent 操控中 · take 收回”）、数字选项和 y 的回显（`choose N`、带 yes 的原命令）、观察模式下对话数字、确认 y、战斗出招和文字命令被拦、方向键不拦。
- `tests/keys_page.rs`：STAGE-113 方向键逛页面的公共部分，不连服务端：空行 `←→` 循环切页、有字时 `←→` 移动光标，`↑↓` 没有可选列表时空行翻历史、有字时按前缀翻，弹窗和确认开着时方向键归它们，观察模式下照常切页，提示行写 `←→ 切页 · ↑↓ 历史 · help 命令`。各页的栏在 114–116 接入后再补页面级的用例；选择、移动、切栏、Enter 的规则见 `app/select.rs` 的单元测试。
- `tests/keys_pet_bag.rs`：STAGE-114 宠物页、背包页的方向键，不连服务端：`↑↓` 跳过空栏位和空格子，详情（宠物）和底边说明（背包）跟着选中项，`Tab` 在物品栏和装备栏之间切换，`Enter` 只填命令（宠物：出战或已倒下填 `pet rest N`，其他填 `pet battle N`；背包：能穿填 `item equip N`，能用填 `item use N`，其他填 `item move N `；装备格填 `item unequip N`）且不执行，`pet status N` 与方向键共用选中项，背包表跟着选中格滚动，提示行的翻页键只在没有选择栏的页面上出现。
- `tests/keys_map_char_msg.rs`：STAGE-116 地图页、角色页、消息页的方向键，不连服务端：地图页 `↑↓` 在本地图 NPC 和地点之间选（地点按编号排，窄屏只剩地点栏），`Tab` 换栏，`Enter` 只填 `talk N` / `warp N`，选中的地点在地图上反色、底边写名字；角色页 `↑↓` 在精灵清单和能力之间选（有精灵的栏位能选，能力矮到画不下时 `Tab` 跳过），能力有未分配点数时 `Enter` 填 `addpoint`，精灵底边写效果；消息页 `↑↓` 逐行滚动（输入行有字时仍是历史），提示行写 `↑↓ 滚动 · End 最新`。
- `tests/keys_battle.rs`：STAGE-38 战斗按键的选择流程，不连服务端：道具用 ↑↓ + Enter 选、数字键在道具列表里不直选、Esc 逐级退回、能选的一方由目标类型决定（不用 Tab 换边）、不用选目标的直接出招、方向键跳过不能用的动作、目标为空时的提示、输入行有字时数字只是文字、文字命令的本地检查。
- `tests/keys_battle_list.rs`：STAGE-107 列表面板的按键，不连服务端：道具默认高亮本场上次用过的、PgUp/PgDn 跨 5 行、灰色道具留在列表里并说明原因、换宠的数字键按栏位号、收回宠物只能用方向键到达、技能按栏位号直选（有空栏位时也不错位）。
- `tests/render_battle_list.rs`：STAGE-107 列表面板的界面：80×24 下 15 个道具逐个都能被高亮看到（能用的 14 个，不能用的小斧头灰色列出），120×40 下整栏换成列表、Esc 后恢复战斗记录，换宠列表的收回行号是 `-`。
- `tests/render_social.rs`：STAGE-162 社交页「队伍」栏，不连服务端：没有队伍的入口和规则说明、倒下时入口灰显并写原因、成员表（队长/分身标记、HP 迷你条、出战宠物、顶部“队伍 5/5”）、宽屏并排详情和窄屏不并排、`--unicode false` 退回 ASCII、动作菜单、详情层（`Esc` 回动作菜单）、解散确认默认取消不发请求、`/party` 的未知参数和各类被拦的原因、观察模式下解散被拦但能看详情、游戏菜单不再写“未开放”。
- `tests/keys_battle_clone.rs`、`tests/scenarios_party_clone.rs`：STAGE-186 分身指定攻击目标。前者不连服务端：`f` 面板的进出和导航、没有分身时说明原因、只能选敌方活着的单位、没列出的分身被拒、观察模式被拦、文字命令 `clone` 的本地检查、80×24 的入口和面板；后者回放 `ui_party_battle_clone`（严格模式）：数字键给分身 1、文字命令给分身 2 的宠物指定目标，发出 `clone:1` / `clone_pet:2` 的 `battle_command`，卡片写“自→2”，队长再交两条指令就结算。
- `tests/scenarios_party.rs`：STAGE-162 验收，回放 `ui_party`、`ui_party_battle`、`ui_party_agent` 用例（严格模式）：社交页 Enter 组队、传送后成员位置同步、动作菜单和 `/party` 命令解散（取消一次再确认）、带分身开战（战场 12 个单位、8 个 `auto`、分身卡片标记、队长交两条指令就结算、胜利窗口注明分身不领奖励、战后队伍还在）、agent 操控期间解散被客户端拦下（不发请求）而看详情照常、`take` 之后才能解散；重连后队伍随断线消失的提示在 `app/reconnect_tests.rs`。
- `tests/pet_dir_keys.rs`：STAGE-251 验收（界面），回放 `docs/protocol/cases/ui_pet_dir.jsonl`（严格模式）：根层「1级宠物」进入目录、目录只取一次、筛选、L2 遇敌点、L3 动作；选中的遇敌点不对时留在 L3，换一项后成功回场景。
- `tests/render_pet_dir.rs`：STAGE-251 渲染，不连服务端：80×24 / 100×30 / 120×40 下的标题命中数、光标行可见、窄屏没有详情、宽屏有详情、长名字截断不撑破边框，地图目录不可用时根层仍有「1级宠物」。
- `tests/pet_merge.rs`：STAGE-208 合成界面，不连服务端：技能页入口、勾选材料、食物与道具混合和件数的拦截、默认取消的确认、结果页、80×24 渲染。
- `tests/scenarios_pet_merge.rs`：STAGE-208 验收，回放 `ui_pet_merge`（严格模式）：界面勾选 5、6 号合成出合成枪1、结果页退回、命令 `pet skill 0 3 7 8` 冷却还料、界面再选 9、10 号得到冷却结果。
- `tests/scenarios_battle.rs`：STAGE-37 至 39 验收，回放 `battle_win`、`battle_events`、`battle_lose`、`battle_magic`、`battle_target_pick` 用例（严格模式，客户端发出的指令要与用例逐字段一致）：数字键、方向键加回车、文字命令出招，空格跳过、`f` 加速，结果窗口，战斗结束后的 `battle` 查询和 `warp`；精灵攻击选敌方、道具（己方一个）默认落在自己身上（STAGE-78）。
- `tests/e2e/`：STAGE-5、6 联调，连**真实的**服务端（需要数据库，没有时跳过），其中 `agent.rs` 用一条原始 TCP 连接扮演 agent，见 `docs/smoke.md`。
- `tests/cases/`：**客户端专用**的模拟服务端用例（真实服务端走不到的状态，比如角色在 floor 100 或吉鲁岛上），不参与服务端的协议用例回归；`tests/scenarios.rs` 的 `map_page_follows_position_changes`（STAGE-20）和 `warp_command_teleports`（STAGE-22，用例 `warp_commands.jsonl`）用它。 角色页的场景 `character_page_shows_status_from_server`（STAGE-23）回放的是协议用例 `world_character_panel`，不是客户端专用用例。模拟服务端对用例里没写的 `places` 请求自动应答一期地点表（客户端进入世界后会自动取一次）。
- 单元测试：输入框（中文光标宽度）、账号/密码/角色名校验、属性点互斥规则、地图比例、玩家 `@` 的定位（`ui/worldmap_locate.rs`）、加点命令的参数（`app/points.rs`）。
- 单元测试（战斗）：阶段与回合（`app/battle/tests.rs`）、站位编号（`app/battle/slots.rs`）、文字命令的解析（`app/battle/command.rs`）、回放节拍与跳过（`app/battle/playback.rs`）、战斗记录的文字（`app/battle/record.rs`）、卡片的血条与状态标记（`ui/battle/card.rs`、`ui/battle/gauge.rs`）、字幕状态兼容（`app/battle/caption.rs`）及界面不再播报的回归、闪色和残影的时间（`ui/battle/fx.rs`）。回放界面的场景在 `tests/render_battle_replay.rs`。
- 想直接看某一页的样子：`cargo run --example snapshot -- create3 110 34`（页名见文件头注释）。战斗画面用 `battle`：`cargo run --example snapshot -- battle 120 40`。

## 世界地图数据

出生地选择页和主界面「地图」页共用 `src/ui/worldmap_data.rs`（海岸线）；地图页还用 `src/ui/worldmap_floors.rs`（floor → 岛上入口，STAGE-20）。两个都是生成文件，不要手改。游戏数据变了就重新生成：

```sh
python3 tools/gen-worldmap.py
```

## 代码结构

| 目录 | 内容 |
|---|---|
| `src/proto/` | 协议类型、本地格式检查、外形/颜色/出生地名称表 |
| `src/net/` | 服务端连接（STAGE-101 起支持 `ws://`、`wss://`）。`endpoint.rs` 解析地址并建连，`transport.rs` 做 TCP 按行、WebSocket 按帧的收发；每条连接一个网络线程独占套接字，主线程的发送经 channel 投递，收到的消息经 channel 交给主循环 |
| `src/app/` | 应用状态：`session`（协议流程）、`pending`（已发出待响应的请求 `Pending` 与握手后的动作 `Action`）、`encounter`（遇敌开关）、`forms`（登录/注册）、`characters`、`create`、`world`（主界面命令）、`wiki`（百科页的浏览状态与 `wiki` 命令）、`agent`（令牌、控制权命令、私聊）、`travel`（快捷传送目录的父子索引、路径栈、每层光标与滚动，STAGE-161）、`travel_keys`（这一页的按键与传送请求）、`pet_dir`（快捷传送里「1级宠物」目录的三层状态、筛选、光标与滚动，STAGE-251）、`pet_dir_app`（目录的协议请求与按键分发，STAGE-251） |
| `src/app/battle/` | 战斗（STAGE-37 至 39）：`mod`（状态与阶段）、`keys`（按键选择）、`issue`（发出指令、目标检查）、`command`（指令与文字命令）、`playback`（结算回放）、`record`（战斗记录文字）、`menu`（哪些动作能选）、`net`（推送与回复的接入）、`slots`（站位编号）；表现层（STAGE-167）：`motion`（武器形态、位移与飞行）、`spell`（精灵属性/治疗/复活/净化/异常形态）、`swap`（换宠先收回主人再放出） |
| `src/wiki/` | 石器百科的本地数据层（STAGE-83）：`build.rs` 在编译期把 `internal/gamewiki/snapshot/` 当前版本嵌进来，`mod`（分类、列表行、搜索、按 key 定位、读详情）、`entry`（词条详情转纯文本行） |
| `src/ui/` | 界面绘制，每页一个文件（快捷传送在 `travelpage.rs`，STAGE-161；1级宠物目录在 `pet_dir_view.rs`（列表与标题）和 `pet_dir_detail.rs`（右侧详情），STAGE-251）；战斗画面在 `battle/`（左右战场、角色身体、指令条、下方日志、结果窗口；动作层的投射物/接触/精灵标记在 `effect.rs`，STAGE-167） |
| `src/app/players_*`、`src/proto/players.rs`、`src/ui/scene_players.rs` | 场景页附近玩家列（STAGE-273）：`players_state`（整表替换与 `players_changed` 增量、三种状态）、`players_net`（推送入口）、`players_keys`（动作面板与 `/players`）；协议解析在 `proto/players.rs`，列的绘制在 `scene_players.rs` |
| `src/update/` | 客户端自升级（STAGE-99）：`version`（语义化版本比较）、`source`（查最新发布）、`install`（判断安装方式）、`command`（`stoneage update`）；启动时的后台检查和握手响应里的最新版本提示都在 `src/app/update_notice.rs`（STAGE-100） |
| `src/bin/mock_server/` | 模拟服务端 |

连接复用：登录页和注册页共用同一条已握手的连接（注册成功、登录失败都不重连）；切换线路、返回登录页（Esc）、被服务端断开时才关闭。

## 发布

源码和发布都在 GitHub 的 `athlan20/stoneage`（公开）：源码、`dist-workspace.toml`、`client/dist.toml`、`scripts/release-github.sh`、发布流程（`.github/workflows/`）都在同一个仓库里，Release（各平台压缩包、一键安装脚本）也发在这里。

流程是这样的：

1. 把版本号改好（`client/Cargo.toml` 的 `version`），并在仓库根目录的 `CHANGELOG.md`（cargo-dist 只在根目录找，放在 `client/` 里不生效）最上面加一节 `## vX.Y.Z`，用玩家看得懂的话写这版新增和修复了什么（不写需求号、模块名、协议、服务端内部改动等）。cargo-dist 按版本号取这一节，放到 GitHub Release 说明的最前面；没有这一节，Release 说明就只剩安装方式。一起提交并推到 `origin`。tag 格式是 `vX.Y.Z`，预发布可以带后缀，如 `v0.2.0-rc.1`。
2. 运行 `scripts/release-github.sh v0.1.0`。它检查 tag 和 `client/Cargo.toml` 的版本一致、`CHANGELOG.md` 有这一节、要发布的提交已经在 `origin` 上，然后打 tag 并推到 GitHub。
3. tag 推到 GitHub 后，`release.yml` 被触发：`plan` 任务按 tag 检出源码并打成 `source-pack` 传给后面的任务，各平台构建、全局产物和 `host` 都只解包；在 macOS（Apple 芯片、Intel）、Linux x64、Windows x64 上编译 `stoneage`（`mock-server` 不随发布包分发），生成两个一键脚本，创建 GitHub Release；再由 `publish-npm.yml` 发布 npm 包。运行记录在 `https://github.com/athlan20/stoneage/actions`。
4. 发布后按「安装」表各装一次，执行 `stoneage` 能进登录页，并连上线上服务端。

编译发布版时，线上服务端地址取自仓库变量 `STONEAGE_ONLINE_SERVER`（填 `wss://域名/ws`，也可以是 `host:port` 的 TCP 地址），编译期写进 `DEFAULT_SERVER`。变量没设置就直接失败，不出包，免得发布版默认连到本机。

GitHub 仓库要配置（Settings → Secrets and variables → Actions）：

| 名称 | 类型 | 用途 |
|---|---|---|
| `STONEAGE_ONLINE_SERVER` | 变量 | 线上服务端地址，如 `wss://game.example.com/ws` |
| `HOMEBREW_TAP_TOKEN` | 密钥 | 推送 Homebrew 配方到 `athlan20/homebrew-tap`，需要对该仓库有写权限。没有它 Homebrew 任务失败，Release 已建但 announce 被跳过 |

- tag 必须和 `client/Cargo.toml` 的 `version` 一致，不一致时 `dist plan` 会拒绝，脚本也会先检查。
- npm 用 Trusted Publishing（OIDC）发布，不需要 `NPM_TOKEN`（npm 要取消可直接发布的绕过 2FA 令牌）。`stoneage-cli` 和四个子包（`stoneage-cli-darwin-arm64`、`-darwin-x64`、`-linux-x64`、`-windows-x64`）都要在 npmjs.com 的包设置里加可信发布者：GitHub Actions，用户 `athlan20`，仓库 `stoneage`，Workflow filename 填 `release.yml`（外层，不是被调用的 `publish-npm.yml`）。新配置 2 天内要完成一次成功发布，否则失效。发布需要 npm 11.5.1 以上、Node 22.14 以上，外层和内层 workflow 都要 `id-token: write`。Windows 子包不叫 `win32-x64`，那个名字被 npm 垃圾包检测拒绝过。
- 带 `-` 的预发布 tag 只发 GitHub Release 和一键脚本，npm 默认跳过。
- **MCP 包**（STAGE-304）：`stoneage-mcp`（让 AI 玩石器，见 `docs/mcp.md`）随同一个 Release 发布，预发布 tag 也出。由 `release.yml` 的 `build-mcp` 任务交叉编译 `darwin/arm64`、`darwin/amd64`、`linux/amd64`、`windows/amd64`（`CGO_ENABLED=0`，`-trimpath -ldflags "-s -w"`，Go 版本取 `go.mod`）。产物名 `stoneage-mcp-<tag>-<os>-<arch>.tar.gz`（Windows 为 `.zip`），包内只有二进制（Windows 为 `stoneage-mcp.exe`）和 `MCP.md`（即 `docs/mcp.md`）；另有 `stoneage-mcp-<tag>-SHA256SUMS`。产物名为 `mcp-release`（不以 `artifacts-` 开头，不进 dist 和 Homebrew 的下载），由 `host` 任务单独下载到 `artifacts/` 后一起上传。
- Homebrew：tap 仓库是 `athlan20/homebrew-tap`。每个正式版发布时，`publish-homebrew-formula` 任务会把 `Formula/stoneage.rb` 推到这个仓库；预发布版不推，brew 只保留最新版。formula 名在 `client/dist.toml` 的 `formula` 里设。
- `release.yml` 不再由 `dist generate` 生成（`dist-workspace.toml` 里 `allow-dirty = ["ci"]`）。改流程直接改 `.github/workflows/`。
- 本流程不做 macOS 公证和 Windows 签名。

## STAGE-167 离线角色动画验收

在仓库根目录运行（先创建 `build/`）：

```powershell
cargo run --locked --manifest-path client/Cargo.toml --example battle_preview -- --export build/battle-actors.html
```

输出使用正式 TUI 渲染器的逐帧 HTML，可播放、暂停、重播、拖进度，包含武器、精灵、命中结果、换宠、反击、合击、打飞与满员站位。附加 `--compact` 检查 80×24，`--ascii` 检查符号降级，`--reduce-motion` 检查减少动态效果。默认尺寸 120×40，不连接服务端，不读写游戏存档。

实际游戏仍启动 `client/target/debug/stoneage.exe`；本例只提供渲染验收入口，不另建一套游戏画面。

验收页性能修正：固定 Canvas 按需绘制，不再每 33ms 重建整屏文字 DOM。默认暂停，播放一遍停止，切到后台暂停；帧数据省略空白段，保留原始终端坐标和颜色。播放调度回归使用 Node 执行：

node --test client/examples/battle_preview/player.test.cjs

直接打开 client/examples/battle_preview.html 会跳转到 build/battle-actors.html；真正的生成模板是 battle_preview.html.in，不作为验收入口。重新生成请使用本节的导出命令。导出占位符与入口回归：cargo test --locked --manifest-path client/Cargo.toml --example battle_preview。

STAGE-167 左右布局补充：敌方始终在左、我方在右（含 `you=10`），角色朝中场；细血条最多六格，中场/指令条不重复整句战报。日志置于战场下方，选列表时仍保留。上下选站位、左右换前后列；回旋镖仍只扫服务端指定的逻辑同排最多五个目标，换宠仍回到事件指定的主人。
