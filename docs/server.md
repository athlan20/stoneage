# 新服务端（`server/`）开发说明

STAGE-140：断线只读、真实重新登录与完整刷新规则见 [reconnect.md](reconnect.md)。

本文只写服务端内部的约定：配置、本地运行和测试工具。客户端、agent 与服务端之间的接口统一以 `docs/protocol.md` 为准，本文不重复定义。需求见STAGE-2～4、STAGE-6。

## 配置（`.env`）

服务端只有两项配置，都从环境变量读取。服务端启动时会先加载仓库根目录的 `.env` 文件。

| 变量 | 示例 / 默认值 | 说明 |
|---|---|---|
| `STONEAGE_LISTEN` | `127.0.0.1:7065` | 服务端监听地址和端口 |
| `STONEAGE_DATABASE_URL` | `postgres://stoneage:<密码>@127.0.0.1:5432/stoneage?sslmode=disable` | PostgreSQL 连接串，标准 URL 格式 |
| `STONEAGE_CHAT_FILTER_FILE` | 空（只用内置的少量词，`server/gamedata/chat_filter.txt`） | 聊天与名字屏蔽词表的路径（STAGE-287，规则见 `docs/chat.md` 1.3）。文件不提交仓库；配置了但读不出来时服务端启动失败，不静默退回内置词表 |
| `STONEAGE_INLAY_OVERLAY_FILE` | 空（用随程序发布的覆盖表，`server/gamedata/inlay_overlay.tsv`，只有表头、不改任何物品） | 精工覆盖表的路径（STAGE-297，规则见 `docs/inlay.md` 2.4）：每行 物品编号、类别码、初始镶嵌码（TAB 分隔）。写了不存在的物品编号、或文件读不出来，服务端启动失败 |
| `STONEAGE_ADMIN_LISTEN` | 空（不开） | 本地管理口的地址（如 `127.0.0.1:7066`，只应监听本机），供协议用例的 `start_battle` 管理行使用，也可以用 `stoneage-server -admin-listen` 指定 |
| `STONEAGE_ENCOUNTER_INTERVAL_MS` | `500` | 遇敌开关两场战斗之间的间隔（毫秒，STAGE-40），协议用例的 `server_config.encounter_interval_ms` 只影响单个用例 |
| `STONEAGE_WS_LISTEN` | 空（不开） | WebSocket 入口的监听地址（如 `0.0.0.0:7067`，路径 `/ws`），与 TCP 入口同时开，协议一样（STAGE-101，见下文“WebSocket 入口”） |
| `STONEAGE_TRUSTED_PROXIES` | 空（不信任任何代理） | 可信代理的地址段，逗号分隔的 IP 或 CIDR（如 Cloudflare 公布的网段）。只有 WebSocket 的直连地址在其中时，才采信 `CF-Connecting-IP` 作为玩家的真实 IP |
| `STONEAGE_MIN_CLIENT_VERSION` | 空（不检查） | 最低客户端版本 `x.y.z`（STAGE-100）。握手时版本低于它的客户端（玩家和 agent）回复 `CLIENT_TOO_OLD` 并断开；版本取不到或格式不对的客户端放行。协议用例的 `server_config.min_client_version` 只影响单个用例 |
| `STONEAGE_LATEST_CLIENT_VERSION` | 空（不返回） | 最新客户端版本 `x.y.z`（STAGE-100）。配置后 `hello` 响应里返回 `latest_client`，客户端据此在消息栏提示升级；协议用例的 `server_config.latest_client_version` 只影响单个用例 |
| `STONEAGE_AGENT_MAX_ONLINE_MINUTES` | `0`（不限） | agent 连续在线的上限（分钟，STAGE-118）。登录满这么久的 agent 被断开，原因 `AGENT_TOKEN_REVOKED`，控制权转回玩家；每 60 秒检查一次，所以最迟晚一个周期才断开。玩家连接不受限制 |
| `STONEAGE_AUTOSAVE_SECONDS` | `180` | 在线角色定时存盘的间隔（秒，STAGE-290）。等级、经验、HP/MP、金币、位置原本只在离开世界时写库，强杀或崩溃会回档；开着时每隔这么久存一遍，战斗结束也立即存一次。`0` 关闭。重启服务端要优雅退出，见 `maintain/README.md` 第 10 节 |
| `STONEAGE_RATE_LIMIT` | `log` | 按连接限速的模式（STAGE-351，协议见 `docs/protocol.md` 第 4 节）：`off` 不检查；`log` 超限只记 Warn 日志（同一连接每秒最多一条），照常分发；`enforce` 超限回复 `RATE_LIMITED`（带 `retry_after_seconds`），10 秒内被拒绝满 200 次则断开。心跳和握手不计入。写了别的值，服务端启动失败 |
| `STONEAGE_RATE_LIMIT_RATE` | `20` | 按连接限速的令牌桶每秒补充的令牌数（正数，STAGE-351）。写了非正数或不是数字，服务端启动失败 |
| `STONEAGE_RATE_LIMIT_BURST` | `40` | 按连接限速的令牌桶容量（正整数，STAGE-351）：一条连接一次最多连发这么多个计入的请求。写了非正整数，服务端启动失败 |
| `STONEAGE_WIKI_CDN` | 空（不加载外部图片） | 石器百科图片和地图的来源（STAGE-118，`/wiki/` 挂在 WebSocket 端口上时用）。和 `cmd/stoneage-wiki` 的 `-cdn` 一样 |
| `STONEAGE_NPC_EVENT_OPEN` | 空（只看代码白名单） | **仅供测试**，生产环境不设（STAGE-214）。任务脚本的放开开关：`all`（不分大小写）在白名单之外再放开每个块的 `Unsupported` 都为空的脚本，含不支持块的仍不放开；逗号分隔的脚本文件名（如 `king/event69_1,jaruga/event/ruri05`）则只放开这几个，含不支持块的也放开。名字在数据里找不到的，启动时打警告，服务端照常启动。只影响“脚本是否放开”，对话、接取、完成和不支持块的拒绝都不变 |

- **优先级**：真实环境变量 > `.env` 文件 > 代码里的默认值。
- **数据库自己准备**：本机安装 PostgreSQL 即可，不需要 Docker，做法见下文“本地数据库”。脚本和服务端都不负责安装或启动数据库，只按连接串去连。
- **文件管理**：
  - 提交一份 `.env.example`，带上示例值和注释；`.env` 本身加入 `.gitignore`，不提交。
  - 首次启动时如果没有 `.env`，脚本从示例复制一份并停下，提示你先填好连接串。
- **其他默认值**：第 7 节的其他默认值（如登录锁定次数、注册频率）以后如果需要调整，也按 `STONEAGE_` 前缀加到这里。

## WebSocket 入口（STAGE-101）

- **开启**：`.env` 里设 `STONEAGE_WS_LISTEN=127.0.0.1:7067`（部署时监听 `0.0.0.0`）。不设就只有 TCP，行为和之前一样。服务端启动日志的 `ws_listen=` 给出实际地址，冒烟脚本从这里取。
- **路径与格式**：`ws://主机:端口/ws`，一个文本帧一条 JSON（见 `docs/protocol.md` 第 1 节）。`server/gateway/ws.go` 是入口，`wsconn.go` 把 WebSocket 包成 `net.Conn`，会话逻辑与 TCP 共用，所以两种入口的行为一致（`go test ./server/gateway/` 的 `TestWS*` 覆盖）。
- **心跳**：服务端每 30 秒发一个 WebSocket ping，pong 收不到就断开这条连接；应用层的 `ping`（`heartbeat_seconds`）和空闲超时（`idle_timeout_seconds`）不变。
- **接 Cloudflare**：Cloudflare 负责 TLS 和防护，把 `wss://域名/ws` 回源到服务端的 WebSocket 端口。回源地址只能是 Cloudflare 支持代理的端口，具体见 Cloudflare 的端口列表。服务端要设 `STONEAGE_TRUSTED_PROXIES` 为 Cloudflare 的网段（<https://www.cloudflare.com/ips/>），否则所有玩家都是 Cloudflare 的地址，登录限流会全站共用一个桶。网段变化时要同步更新。
- **不做**：Cloudflare Spectrum（非 Enterprise 要额外付费）；WebSocket 入口本身不做 TLS，直连公网时自己加。

## agent 手册、参考脚本与百科（STAGE-118）

WebSocket 端口（`STONEAGE_WS_LISTEN`）同时提供下面这些只读的 HTTP 路径，不需要令牌，内容编译进服务端，和服务端版本一起发布：

| 路径 | 内容 | 来源 |
|---|---|---|
| `/protocol.md` | 协议 | `docs/protocol.md` |
| `/agent.md` | agent 使用手册（行为规则，字段引用协议章节号） | `docs/agent-guide.md` |
| `/agent-connect.py` | agent 参考连接脚本，只用 Python 3 标准库：握手、`agent_login`、心跳、按行收发 JSON | `docs/agent-connect.py` |
| `/install.md` | 客户端安装说明（STAGE-301；`docs/play.md` 是面向玩家的完整版，两份内容相近，改一份要看另一份要不要同步） | `docs/install.md` |
| `/wiki/` | 石器百科 | `internal/gamewiki` |
| `/wiki/q` | 百科的纯文本查询，见 `docs/game-wiki.md` | `internal/gamewiki/query*.go` |

- **挂载**：`server/webpages/webpages.go` 的 `Install` 登记路径，`server/gateway` 的 `HandleHTTP` 把它们和 `/ws` 放进同一个 mux；`docs/embed.go` 负责内嵌文件。
- **改手册**：`docs/agent-guide.md` 第 6 节的三级表必须包含全部操作类请求（`go test ./server/webpages` 检查）；手册里“第 X.Y 节”必须在 `docs/protocol.md` 里有对应标题，手册第 9 节的错误码必须在协议里定义。
- **脚本的版本**：脚本发送的客户端版本是 `stoneage-agent/0.1.0`，服务端配置了最低版本时要不低于它（否则握手回复 `CLIENT_TOO_OLD`）；改版本时改 `docs/agent-connect.py` 里的 `CLIENT`。
- **冒烟**：`scripts/smoke.sh` 用 TCP 那一遍生成的第二个令牌，经 WebSocket 跑一遍参考脚本（登录、进入世界、查状态、离开），并检查输出里没有令牌原文。

## 协议测试客户端（STAGE-2 交付）

用法：`go run ./cmd/stoneage-casetest [-addr 地址] [-stages STAGE-2,STAGE-3] [-run 正则] [-reset-db] [-v]`。
`-reset-db` 用 `.env` 里的连接串在每个用例前清空业务数据，**只允许本机的库**（`localhost`、`127.0.0.1`、`::1`），指向别的主机时拒绝执行，确实要清空再加 `-force`；`-v` 打印每条收发的消息（密码和令牌显示为 `***`）。
需要特殊服务端配置（`meta.server_config`，如空闲时限 2 秒）或人工触发（`meta.manual`）的用例，命令行默认跳过；
`go test ./server/...` 会为这类用例按需启动一个进程内的服务端来运行。


- 读取 `docs/protocol/cases/*.jsonl`，按文件中的顺序处理每一行：
  - 遇到 `c2s` 行，按 `conn` 选中对应的连接，把消息发给服务端；
  - 遇到 `s2c` 行，等待该连接上的下一条消息，逐字段比对。期望值是 `"*"` 的字段只检查是否存在。
- 每个用例都在空数据库上运行，彼此互不影响。
- 服务端的 STAGE-2、3、4 都用它验收，并且接入 `go test`，作为回归测试。

## 战斗用例与本地管理口（STAGE-43 起）

- 战斗用例（`docs/protocol/cases/battle_*.jsonl`）里的 `admin` 行 `start_battle` 要让**正在运行的服务端**立刻开一场战斗，不能直接写库。服务端因此可以另开一个只监听本机的管理口：`STONEAGE_ADMIN_LISTEN` 或 `stoneage-server -admin-listen 127.0.0.1:7066`。管理口没有鉴权，**只允许回环地址**（`127.0.0.1` / `::1` / `localhost`）：配置成其他地址（含 `0.0.0.0`、留空主机的 `:端口`）服务端会启动失败，它仅用于本地联调（STAGE-55）。它说 JSON Lines：一行 `{"do":"start_battle","character":"勇者小明","scenario":"win"}`，回一行 `{"ok":true}` 或 `{"ok":false,"error":"说明"}`；`set_setting`（改全服设置，STAGE-59）也在这个口上：`{"do":"set_setting","key":"exp_rate","value":2}`，写库并同步重读后回 ok。`stoneage-casetest` 会按 `.env` 的配置自动连它。
- 剧本（`win`、`lose`、`escape`、`no_escape`、`swap`、`capture`、`showcase`、`drop`：与 `win` 同一场，怪物出生时各掉一把小棍棒，STAGE-60；`fly`：两只攻击放大的怪物，一下打飞人物，STAGE-63；`npc_win`：里昂蛙王的一场，怪物同 `win`，NPC 发起的战斗，STAGE-96；`combo`：与 `win` 同样的怪物、打开合击，STAGE-126——别的剧本默认关合击，真实遇敌与 NPC 战斗开）定义在 `server/battle/scenario.go`：怪物的组合、等级和随机数种子固定，重放结果可复现；`give_pet` 管理行的宠物资质随机也按用例名固定了种子（`casetest.SeedForCase`，命令行重放与 `go test` 回归同源，STAGE-58）。
- **固定战斗走向的种子**（STAGE-291）：`scenario` 可以写成 `名字@种子`（如 `"beaten@40"`、`"capture@3"`），用这个种子替换剧本自带的种子，其余不变。战斗的随机数调用一增减，同一个种子的走向就会变；这时给用例换一个能还原原走向的种子，比改剧本表或加回合更省事。找种子的办法：对每个候选种子在记录模式（`STONEAGE_UPDATE_CASES=用例名`）下跑一遍，比对录出来的消息序列、倒下的单位和关键事件（捕捉、魔法的 result/status_add）是否与原用例一致。找到后再按下面的方法记录并还原通配符。

- **对常驻服务端黑盒重放战斗用例**：用例里录制的 `battle_id` 从 `b_1` 起编号，而战斗计数挂在服务端进程内存上，所以每个战斗用例前要重启服务端（计数归零），并让 casetest 与服务端都能看到管理口配置（`.env` 的 `STONEAGE_ADMIN_LISTEN`，或两边都设环境变量）：

  ```sh
  STONEAGE_ADMIN_LISTEN=127.0.0.1:7066 scripts/server-restart.sh
  STONEAGE_ADMIN_LISTEN=127.0.0.1:7066 go run ./cmd/stoneage-casetest -run '^battle_win$' -reset-db
  ```

- **校准用例**：服务端行为改了以后，用记录模式把服务端的真实回复写回用例文件，再人工 review diff：

  ```sh
  export STONEAGE_TEST_DATABASE_URL=$(grep -o 'postgres[^ ]*' .env | head -1)
  STONEAGE_UPDATE_CASES=battle_win go test ./server/casetest/ -run TestProtocolCases
  ```

  `STONEAGE_UPDATE_CASES` 的值是用例名前缀（`all` 是全部）。写回后要把 `deadline_ms` / `server_time_ms` 换回 `"*=样例"`（时间戳每次不同），模拟服务端回放时会把它们换成发送时的真实时间。

- **遇敌用例**（`encounter_*.jsonl`，STAGE-40）：野外战斗的怪物组成按遇敌表抽取，随机源按用例名固定种子（`casetest.SeedForCase` 的第二个种子 43），重放的敌人、伤害完全可复现；两个连场用例把 `server_config.encounter_interval_ms` 调小，不用等默认的 2 秒。校准这类用例时可以先用 `go run ./build/simencounter <用例名> <额外腕力> <场数>` 离线打出每条推送的确切内容再手工比对（该工具在 `build/` 下，不提交）。

## 脚本与日志

| 脚本 | 作用 |
|---|---|
| `scripts/server-start.sh [--protocol-log]` | 没有 `.env` 时从 `.env.example` 生成并停下，等你填好连接串；编译并在后台启动服务端，启动时自动建表、升级表结构 |
| `scripts/server-status.sh` | 服务端是否在运行、数据库连接信息、表结构版本（需要本机有 `psql`） |
| `scripts/server-stop.sh` | 停止服务端（先给所有连接推送 `SERVER_SHUTDOWN`）；数据库不受影响 |

- **改代码或配置**：直接 `scripts/server-restart.sh`（等于先 `server-stop.sh` 再 `server-start.sh --protocol-log`，没有在运行也不报错，最后显示状态；`--no-protocol-log` 关掉协议日志；`--foreground` 不放到后台，服务端留在当前终端运行、日志直接显示，Ctrl+C 停止，别的终端的 `server-stop.sh` 也能停它。`server-start.sh` 同样支持 `--foreground`）。环境变量优先于 `.env`，如 `STONEAGE_LISTEN=127.0.0.1:7067 scripts/server-start.sh`。
- **临时 schema**：`stoneage-server -temp-schema` 在数据库里新建一个临时 schema 来运行，退出（正常关闭）时删除，用于联调和冒烟测试（`docs/smoke.md`），不碰开发库里的数据。
- **直接调试**：不用脚本也可以，`go run ./cmd/stoneage-server`（在仓库根目录运行，自动读取 `.env`；加 `-protocol-log -` 把协议日志打到终端），适合下断点或用 IDE 调试。
- **日志**：服务端日志 `runtime/logs/server.log`；协议日志 `runtime/logs/protocol.log`，用 `--protocol-log`（或 `STONEAGE_PROTOCOL_LOG=1`）打开，每条收发的消息一行，`password`、`token` 字段一律写成 `***`，无法解析的行只写长度。行末 `(N 字节)` 是这一行脱敏前的字节数（不含换行），用来统计最大的消息；超长的请求行写 `(超过上限)`。
- **断线与大消息日志**（STAGE-246）：服务端写一条消息失败（超时、对端已关闭或其他错误）时记 Warn「写消息失败，断开连接」，带会话 id（`session=`）、远端地址、账号（已登录时）、消息 `type`、字节数 `bytes=`、`reason=`（超时 / 对端关闭 / 其他）和原始错误 `err=`。发送队列满时的「发送队列已满，断开连接」也带 `type` 和 `bytes`。服务端发出单条超过 10 KiB 的消息时记 Info「单条消息超过 10 KiB」，带 `type`、`bytes`，用来找出最大的消息类型。这些日志只写类型和字节数，不写消息内容。压缩过的消息 `bytes` 是原文字节数，另带 `wire_bytes` 是线上字节数；协议日志的 `(N 字节)` 同样是原文字节数。
- **本地数据库**：连接串就是 `.env` 里的 `STONEAGE_DATABASE_URL`，`psql "$STONEAGE_DATABASE_URL"` 或任何数据库客户端都能直接连。第一次准备（以 macOS Homebrew 为例，其他系统同理）：

  ```sh
  brew install postgresql@18 && brew services start postgresql@18
  createuser --pwprompt stoneage          # 输入一个密码
  createdb --owner stoneage stoneage
  # .env: STONEAGE_DATABASE_URL=postgres://stoneage:<密码>@127.0.0.1:5432/stoneage?sslmode=disable
  ```

  库里可以有别的表，服务端只管自己建的表；但协议测试工具的 `-reset-db` 会清空当前 schema 下除 `schema_migrations`、`server_meta` 以外的**所有表**，库里有别的数据时不要对它用 `-reset-db`，或者先在库里 `CREATE SCHEMA game`，再在连接串后加 `&search_path=game`，让服务端的表单独放在这个 schema 里。

## 代码结构

| 位置 | 内容 |
|---|---|
| `cmd/stoneage-server` | 服务端入口：读配置、连库并迁移、监听、优雅退出 |
| `cmd/stoneage-casetest` | 协议测试客户端 |
| `server/config` | `.env` 与环境变量 |
| `server/protocol` | 消息结构、错误码、请求登记表（类别、允许的状态和身份）、日志脱敏 |
| `server/gateway` | 网络层：每条连接一个读 goroutine 加一个写 goroutine，握手、心跳、空闲超时、权限检查、分发；`Hub` 记录在线连接并支持向“观看同一角色的多条连接”推送 |
| `server/store` | pgx 连接池、`migrations/*.sql` 带版本记录的迁移 |
| `server/casetest` | 协议用例的读取、比对与重放 |

**加一种请求**（STAGE-3、4、6 照此做）：
1. 在 `docs/protocol.md` 和 `docs/protocol/cases/` 里先写好消息和用例（`meta.stage` 写负责的需求）；
2. 在 `server/protocol/meta.go` 的 `Requests` 里登记它的类别、允许的状态和身份（已登记的请求也在这里，没实现的会回复 `UNKNOWN_TYPE`）；
3. 用 `gateway.Server.Handle(type, handler)` 注册处理函数；处理函数返回 `*protocol.Error` 即失败响应，登录、进入世界等状态变化调用 `Session.SetState`，同账号的连接用 `Hub().ByAccount`；
4. 在 `server/casetest/cases_test.go` 的 `enabledStages` 里加上这个阶段，它的用例就成为回归测试。

**加一个表结构变更**：在 `server/store/migrations/` 新增 `NNNN_说明.sql`（版本号连续递增），服务端启动时自动升级并写入 `schema_migrations`；数据库版本比服务端新时拒绝启动。

## 账号与登录（STAGE-3）

- **表**（迁移 `0002_accounts.sql`）：`accounts`（账号名恒为小写，只存 Argon2id 哈希 `m=65536,t=3,p=2`，带禁用标记、创建/最后登录的时间和 IP）；`auth_audit`（每次注册和登录，无论成功失败，都记账号、来源 IP、结果、时间；频率限制也从这里统计）；`login_locks`（被锁定的账号名和到期时间）。用数据库客户端能直接查看，看不到明文密码。
- **默认限制**在 `server/auth/service.go` 的 `DefaultLimits`：账号 10 分钟失败 5 次锁 15 分钟，IP 10 分钟失败 20 次限制登录，每个 IP 每小时注册 50 次。要调整就改这里（配置项仍然只有两个，不加环境变量）。精确语义见 `docs/protocol.md` 5.3、5.4。
- **账号管理** `go run ./cmd/stoneage-admin <命令>`（直接操作 `.env` 里的数据库，不需要服务端在运行）：`accounts` 列出账号，`disable <账号> [原因]` / `enable <账号>` 禁用或启用，`unlock <账号>` 解除登录锁定。禁用立即生效：这个账号已在线的玩家和 agent 连接会收到 `ACCOUNT_DISABLED` 推送并被断开（`docs/protocol.md` 5.4）。`stoneage-admin` 和服务端是两个进程，禁用时由 `stoneage-admin` 在同一个事务里执行 `NOTIFY account_disabled, '<账号>'`，服务端启动时 `LISTEN account_disabled`，收到后断开该账号的所有连接（断开前同样调用 `BeforeKick` 保存数据）；服务端没在运行时，下次登录自然被拒。
- **批量发放（STAGE-241）** `go run ./cmd/stoneage-admin grant --kind item|pet|magic|point --batch <批次号> <人群参数> [--yes]`：人群三选一（`--names-file` 角色名清单、`--accounts-file` 账号清单、`--all` 全部角色，可加 `--registered-before` / `--min-level` 过滤）。不加 `--yes` 只预演。逐个角色各自处理（复用上面几条单发函数，规则不变），每个角色的结果追加写进 `runtime/grants/<批次号>.jsonl`；同一批次号重跑时已成功的跳过，`--retry` 只重试记录里失败的。不新增迁移、不入库。运维步骤和样例见 `maintain/README.md`。
- **发物品（STAGE-28）** `go run ./cmd/stoneage-admin give <角色名> <物品编号> [数量]`：把物品放进角色第一个空的背包格（编号 5–19，满了报错）。物品模板来自原版服务端 `gmsv/data/itemset.txt`（`tools/gen-itemset.py` 生成 `server/gamedata/itemset.tsv` 编译进服务端，UTF-8），物品编号从 0 开始（0 是小斧头）。同样走数据库通知：同一个事务里 `NOTIFY character_items_changed`（payload 是角色 id），服务端启动时 `LISTEN` 这个频道，在线时收到后重读该角色的物品并给世界中的连接推送 `inventory_changed`（`docs/protocol.md` 5.18），不在线时只写库、下次进入世界可见。
- **发未分配点数（STAGE-24）** `go run ./cmd/stoneage-admin givepoint <角色名> <点数>`：给角色加未分配的能力点（直接写库，并通过 `character_points_changed` 通知在线的服务端，在线的角色页立刻更新）。玩家用 `addpoint` 把点数加到能力上（`docs/protocol.md` 5.21）。
- **设骑乘等级（STAGE-122）** `go run ./cmd/stoneage-admin ridelevel <角色名> <0|40|80|120|200>`：直接设骑乘等级（写库，并通过 `character_ride_changed` 通知在线的服务端，在线的角色立刻收到 `status_changed`）。正常玩家用骑乘训练师上课（`docs/protocol.md` 5.49）。
- **设宠物转生次数（STAGE-196）** `go run ./cmd/stoneage-admin setpettrans <角色名> <栏位> <次数>`：直接设随身宠物（栏位 0–19，STAGE-346）的转生次数，次数不能是负数。写库并通过 `character_pets_changed` 通知在线的服务端，在线的连接立刻收到 `pets_changed`（宠物详情的“转生”一项马上变）。转生次数超过 2 次的宠物不能骑（`RIDE_TRANS_LIMIT`，`docs/ride.md` 第 9 条）。宠物转生本身还没做，这里只是测试用。
- **公开聊天禁言（STAGE-278）** `go run ./cmd/stoneage-admin mute <角色名> --minutes N [--reason 文字]`：写角色表的 `chat_muted_until`（需要新迁移字段，迁移号合并前看 test 分支是否撞号），每条发言前读一次该字段，所以对在线角色立即生效（不走 PostgreSQL 通知）；禁言挡本图/世界/队伍频道，不影响私聊；`unmute <角色名>` 立即解除；`mutes` 列出仍在禁言的角色和到期时间。只影响 `map`、`world`、`party` 公开频道，不影响 `agent_say` 私聊。
- **屏蔽词表（STAGE-287）** `go run ./cmd/stoneage-admin chatfilter test <文字>` 用 `.env` 指定的词表（没配则内置词表）检查这段文字：打印词表统计、聊天里会显示成什么、名字会不会被拒绝，只输出到终端，不发给任何人。`chatfilter reload` 先校验 `STONEAGE_CHAT_FILTER_FILE` 指向的文件，通过后才往数据库发 `chat_filter_reload` 通知，运行中的服务端收到后重读文件；校验不过则不发通知。服务端重载失败（文件读不了等）保留旧词表、只记警告日志，不影响在线玩家。
- **设人物等级（STAGE-156）** `go run ./cmd/stoneage-admin level <角色名> <1–160>`：直接写库，不通知在线的服务端，所以角色要先离开世界，下次进入世界才生效（同 `move`）。协议用例的 `set_level` 行用的是同一个函数。
- **给精灵（STAGE-48）** `go run ./cmd/stoneage-admin givemagic <角色名> <精灵编号>`：给人物一个精灵（magic.tsv 里的编号，如 301 岩石撞击），放进第一个空的精灵栏（0–4），通过 `character_magics_changed` 通知在线的服务端重读并推送 `status_changed`（status 里多了 `magic` 字段）；角色不在线时只写库。
- **发宠物（STAGE-31）** `go run ./cmd/stoneage-admin givepet <角色名> <宠物编号> [等级]`：按原版服务端 `ENEMY_createPetFromEnemyIndex` 的规则随机资质，放进角色第一个空的宠物栏位（编号 0–19，满了报错；STAGE-346 起 20 格）。宠物模板来自原版服务端 `gmsv/data/enemybase.txt`（`tools/gen-petbase.py` 生成 `server/gamedata/petbase.tsv`，UTF-8），只有 PETFLG 不为 0 的模板能当宠物（编号 1 是乌力）；不写等级时为 1，最高 160。同样走数据库通知：`NOTIFY character_pets_changed`，在线时重读该角色的宠物并推送 `pets_changed`（`docs/protocol.md` 5.20），不在线时只写库。 宠物进库（到手）时的等级和能力记为起始值（`character_pets` 的 `origin_*` 五列，STAGE-215，迁移 `0021_pet_origin.sql`；历史宠物为空，不倒推）；升级、战斗、存取宠物店、改名、改技能、忠诚与转生次数的变化都不改它。
- **任务进度（STAGE-86）** `go run ./cmd/stoneage-admin quest <角色名> <任务编号> doing|done|clear`：把角色的任务设成进行中（`doing`）、已完成（`done`），或清除（`clear`）。任务表 `server/gamedata/quests.tsv` 由 `tools/gen-quests.py` 从原版服务端 `jobdaily.txt` 生成（109 个任务，第 61 号因事件号为 0 未收录）；任务编号不在表里时报错。表在迁移 `0013_character_quests.sql`（每个任务编号一行）。同一个事务里 `NOTIFY character_quests_changed`，运行中的服务端收到后重读并给世界中的连接推送 `quests_changed`（`docs/protocol.md` 5.45），没有实际变化时不推送；角色不在线时只写库，下次进入世界时可见。协议用例的 admin 行 `{"do": "give_quest", "character": "…", "quest_no": 1, "state": "doing"}` 走同一条路径。
- **任务 NPC（STAGE-90）**：`server/gamedata/npc-event.json` 与 `npcs.tsv` 由 `python3 tools/gen-npcs.py` 一起生成（脚本解析见 `tools/npcevent.py`）。放开哪些脚本由 `server/gamedata/npcevent.go` 的白名单决定（第一批「成人仪式」）。事件的完成和进行中写进上面的任务进度（事件号经任务表映射到任务编号），交易的背包和任务进度在同一个事务里提交；规则、分批和与原版的差异见 `docs/quest.md`。
- **全服设置（STAGE-59）** 表在迁移 `0009_game_settings.sql`：一个设置一行（`key`、`value numeric(8,2)`、`note`、`updated_at`、`updated_by`），第一项是战斗经验倍数 `exp_rate`，初始 1（和原版节奏一致；原私服 `setup.cf` 配过 `battleexp=300`，对应原版 `battle.c` 的 `addexp *= getBattleexp()`）。管理命令：
  - `go run ./cmd/stoneage-admin settings`：列出全部设置和当前值；
  - `go run ./cmd/stoneage-admin set exp_rate 2`：改成 2 倍。值允许小数、最多两位（如 1.5），范围 0.1–100，非法直接拒绝、数据库不变；未知设置名也拒绝。
  生效范围只有战斗胜利后人物和出战宠物的经验（`gain` 乘倍数、小数舍去、基础经验大于 0 时至少 1 点；金币不受倍数影响（按人物等级 × 10 算，STAGE-60），掉落、捕捉率不变）。运行中的服务端立即生效：`set` 在同一个事务里 `NOTIFY game_settings_changed`，服务端 `LISTEN` 收到后把整张表重读进内存（断线重连后也会重读一遍），战斗在结算那一刻读内存里的值。数据库里的值坏了或读不到时按 1 倍处理并记警告日志。协议用例的 admin 行 `{"dir": "admin", "do": "set_setting", "key": "exp_rate", "value": 2}` 走服务端本地管理口（同样写库并同步重读，见 `battle_exp_rate.jsonl`）。
- **公开聊天设置（STAGE-278）**：三项是全服设置（`game_settings` 表，同 `exp_rate`），用 `stoneage-admin set <设置名> <值>` 修改，运行中的服务端立即生效；表里没有这一行时按默认值，不需要迁移。`chat.world_min_level`：世界频道最低等级，默认 `5`，范围 1–160；`chat.world_cooldown_seconds`：世界频道每人冷却秒数，默认 `30`，范围 0–3600；`chat.map_interval_ms`：本图每人最短间隔毫秒数，默认 `1000`，范围 0–60000。超出范围或不是数字的值被拒绝。协议用例用 `set_setting` 行改，用完改回默认值。
- **重复登录**：同一账号、同一身份（玩家或 agent）再次登录成功时，新连接成功，旧连接先调用 `auth.Options.BeforeKick`（STAGE-4 在这里保存角色数据），再收到 `KICKED_DUPLICATE_LOGIN` 推送并被断开；玩家连接和 agent 连接互不挤占。
- **测试**：`go test ./server/...` 里依赖数据库的测试（`server/auth`、`server/store`、STAGE-3 的协议用例）需要设置 `STONEAGE_TEST_DATABASE_URL`，例如 `STONEAGE_TEST_DATABASE_URL=$(grep ^STONEAGE_DATABASE_URL= .env | cut -d= -f2-) go test ./server/...`；每个测试在独立的 schema 里运行，结束后删除，不影响正在使用的数据。没设置时这些测试被跳过。
- 对运行中的服务端跑 STAGE-3 用例：`go run ./cmd/stoneage-casetest -stages STAGE-3 -reset-db`（会清空账号等业务数据，只用于本地开发库）。需要禁用账号这类管理员操作的用例，必须带 `-reset-db`。


## 角色与进入游戏（STAGE-4）

- **表**（迁移 `0003_characters.sql`）：`characters`，名字、等级、地图（`floor`）、坐标（`x`、`y`）、HP、MP、石币等都是普通列，数据库客户端里直接能看；`name_key` 是折叠大小写和全角半角之后的名字，带唯一约束，保证全服不重名。`(account_id, slot)` 也有唯一约束，防止并发建出第 3 个角色。以后的排行、名片表引用 `characters(id)` 并写 `ON DELETE CASCADE`，删除角色时就会一并清理。
- **游戏数据** `server/gamedata`：出生点、新手配置、最大 HP 公式、经验表、外形图像编号换算，出处都写在注释里。地图名表 `mapnames_gen.go` 是生成的：`python3 tools/gen-mapnames.py` 读旧地图文件头（GBK）生成，游戏数据变了就重新生成。
- **世界中的角色** `character.Live`：进入世界时读一次数据库，之后的改动都在内存里，离开世界、被踢、断开或关服时保存。读写角色数据必须通过 `Live.With`，它会加锁并检查角色是否已经离开，所以“别的 goroutine 里的保存”和“这条连接正在处理的请求”排队执行，离开之后的请求被丢弃（返回 `ErrLeft`）。后面做移动、战斗等玩法时都按这个方式改角色数据。
- **什么时候保存**：`leave_world`；`auth.Options.BeforeKick`（重复登录挤下线、账号被禁用，在推送 `disconnect` 之前）；`gateway.Server.OnClose`（连接结束，包括直接断开、空闲超时、关服；关服时 `Shutdown` 等保存完才返回）。
- **锁的约定**（STAGE-12/13）：保存角色要访问数据库，可能很慢，所以**不能在全服共用的锁里做，也不能持着全服共用的锁去等某个角色的锁**。`character.Service.mu` 只做内存操作，需要同时持有时先取 `Presence.mu` 再取它；`auth.Service.kickMu` 只保护“找出旧连接并登记新连接”一小步，保存和断开都在锁外。新增代码遵守同样的规则，否则一个人的数据库慢会拖慢全服。
- **删除与进入互斥**（STAGE-16）：`delete_character` 先标记“正在删除”，期间进入这个角色回复 `CHARACTER_NOT_FOUND`；读到角色之后才发生的删除，由进入时记录进入时间那条 `UPDATE` 更新 0 行发现并撤销进入。保存更新 0 行会返回错误并记日志。
- **装配** `server/app`：服务端主程序和协议用例的回归测试用同一个 `app.Install` 装配账号和角色服务。
- **测试**：`go test ./server/character`（需要 `STONEAGE_TEST_DATABASE_URL`）覆盖离开/断开/挤下线/禁用/关服后的保存、保存与请求并发、重名（含大小写和全角）与并发创建、删除、`CHARACTER_IN_USE`；规则校验（名字、外形、能力点、属性点）不需要数据库。协议用例：`go run ./cmd/stoneage-casetest -stages STAGE-4 -reset-db`。

## agent 协同（STAGE-6）

- **社交开关（STAGE-276）**：`characters` 表加 `social_visible`、`social_invite` 两列（`boolean not null default true`，迁移 `0029_character_social.sql`，编号以合并时为准），随角色一起保存和删除；协议见 `protocol.md` 5.69。
- **表**（迁移 `0004_agent.sql`）：`agent_tokens`（只存令牌的 SHA-256；令牌是 256 位随机数，所以不需要像密码那样用慢哈希；`token_id` 是给玩家看的编号）和 `agent_audit`（见下）。
- **令牌与 agent 登录** `server/auth/token*.go`：生成时锁住账号行再数有效令牌，并发生成不会超过 5 个；`agent_login` 的失败和密码登录共用 IP 限制，同一 IP 的尝试同样串行处理；吊销令牌时先保存（`BeforeKick`）再给正在用它的 agent 连接推送 `AGENT_TOKEN_REVOKED`；在线 agent 的令牌到期由 `auth.Service.Start` 起的后台循环检查（`Options.TokenCheckInterval`，默认 60 秒），走同一条路径；同一账号的第二条 agent 连接挤掉第一条。
- **控制权** `server/character`：一个账号在世界中的角色是一个 `Presence`，玩家连接和 agent 连接各持有一个 `Live`，共用同一份角色数据和“谁在操控”。`enter_world`、`leave_world`、断线、被踢、令牌被吊销都在 `register` / `finish` 里统一处理控制权的转移，并给世界中的每条连接推送 `control_changed`。
- **观察方不能发“操作”类请求**：`gateway.Server.SetGuard` 设置的守卫在状态和身份检查之后、调用处理函数之前统一拒绝（`CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`），`enter_world` 和 `leave_world` 除外。处理函数里改角色数据时用 `Live.Operate`，在持有角色锁的情况下再确认一次（控制权可能在两次检查之间变化）。以后加新的“操作”类请求：在 `protocol.Requests` 里登记类别，处理函数用 `Live.Operate` 改数据即可，不需要自己判断控制权。
- **审计** `agent_audit`：agent 连接发起的每个“操作”类请求（含被拒绝的，由 `gateway.Server.OnDone` 回调写入）和每次控制权变化（含 `kick_agent`）。查看：`select at, role, token_id, kind, request, result from agent_audit order by id`。
- **私聊** `agent_say`：只在同一账号的玩家连接和 agent 连接之间传递，不存库。
- **测试**：`go test ./server/character`（控制权流程、玩家以观察方进入、操控方离开/断线的转移、`kick_agent`、吊销、私聊、审计、并发压力）和 `go test ./server/auth`（令牌的生成/列出/吊销、上限含并发、过期、IP 限制、令牌不落库）需要 `STONEAGE_TEST_DATABASE_URL`；协议用例见 `docs/protocol/cases/` 里 `agent_*`、`control_*`、`world_status_turn`。
- **地点与传送**（STAGE-22）：`places`（查看类）和 `warp`（操作类）在 `server/character/places.go`，地点表在 `server/gamedata/places.go`。追加地点只改这张表（编号顺延），客户端重新取列表即可，但要在世界图上画得出来（落在 100/200 两座岛上，或有入口表可查）。用例 `world_places`、`world_warp`、`control_warp`（`meta.stage` 是 STAGE-22，要在 `server/casetest/cases_test.go` 的 `enabledStages` 里启用）。
