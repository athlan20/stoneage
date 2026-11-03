# 客户端 ↔ 服务端协议（v1）

STAGE-139 在线状态扩展与菜单约定见 [system-menu.md](system-menu.md)。

STAGE-136：新增查看请求 `quest_npcs {no}`（in_world、玩家/agent 均可）。响应 `{no,state,npcs:[{id,name,type,floor,map_name,x,y,here,relation}],reason}`，relation 为 `current`（当前实际命中的、关联该任务的开放脚本块）或 `reference`（脚本相关，非下一步保证）。关系来自任务事件号与正式 changeevent 脚本；不暴露暗号和奖励条件。没有可靠关系时返回空数组和原因。最终对话仍由 npc_open 按即时任务条件执行；客户端收到任务变化必须使已打开候选失效。

STAGE-135 新增显式协商的 [NPC 菜单 v2](npc-menu-v2.md)：`npc_open` 启用修订号、结构化输入与报价确认；旧 `talk` 买卖语义保留。

本文是命令行客户端（`client/`）与 Go 新服务端（`server/`）之间的**唯一约定**。前后端各自照本文开发、各自测试，不依赖对方的进度。本协议是我们自己定义的，**不兼容**原版 2.5 客户端，也不兼容旧的 LSSPROTO（命名方言和数字方言）。游戏规则（建角校验、出生点、数值公式等）仍以原版服务端源码 `original-data/gmsv` 为准，本文注明了每条规则的出处。

本文覆盖**登录阶段**（连接、心跳、注册、登录、角色、进入和离开世界）、进入世界后已经做出来的功能（第 5 章后半、第 9 章 agent 协同、第 10 章战斗）。以后的游戏功能（聊天、交易等）按功能追加章节，协议版本号不变，只追加新的消息类型。

## 1. 传输与格式

- **连接**：两种入口，消息内容、状态和错误码完全一样，只是底层传输不同：
  - **TCP**（默认）：`127.0.0.1:7065`，一行一条 JSON。
  - **WebSocket**（STAGE-101）：`ws://主机:端口/ws`；经 Cloudflare 等前置代理上公网时用 `wss://`（TLS 由前置代理终止）。**一个文本帧就是一条 JSON**，帧内不带换行；二进制帧会被断开。服务端每隔一段时间（默认 30 秒，小于 Cloudflare 的 100 秒空闲上限）主动发一个 WebSocket ping，让代理看到连接还活着。应用层的心跳和空闲超时（第 5.2 节）两种入口相同。
  - 服务端的监听地址见 `docs/server.md`；客户端用 `--server` 指定地址，`host:port` 按 TCP 连，`ws://`、`wss://` 按 WebSocket 连。
  - 一期 TCP 不加密；公网上的加密由前置代理（如 Cloudflare）提供，直连 TCP 只用于本机和内网。
- **文档**：服务端的 WebSocket 端口同时提供 `/protocol.md`（本文）、`/agent.md`（agent 使用手册）、`/agent-connect.py`（参考脚本）和石器百科（`/wiki/`），见 `docs/server.md`。
- **格式**：每行一条 JSON 对象，编码 UTF-8，以 `\n` 结尾（JSON Lines）；WebSocket 上则是每帧一条。单行最长 64 KiB（线上字节数，见下面的“压缩”），超长时对方回复 `BAD_REQUEST` 并断开。
- **压缩（STAGE-246）**：`hello` 请求里声明 `"compress": ["gzip"]` 的连接，服务端发出的一条消息序列化后超过 10 KiB（10240 字节，不含换行）时，改发成一行压缩行：`{"z":"gzip","d":"<base64 of gzip(原文)>"}`，结尾同样是 `\n`（WebSocket 上一帧一行）。接收方看到 `z` 字段就先解压：`d` 解码后的 gzip 内容，就是原来那一行 JSON（不含换行）。解压后的原文上限 1 MiB；压缩行损坏、`z` 不认识、`d` 不是 base64、解压失败或超限，服务端回复 `id` 为 0 的 `BAD_REQUEST` 后断开，客户端按协议错误断开。没有声明压缩的连接永远收到原样 JSON，所以线上超过 64 KiB 的响应对它不可用（见下一条）。参考脚本 `agent-connect.py` 默认声明 gzip 并自动解压，自己写的客户端必须声明。客户端发给服务端的请求本期不压缩，但服务端也认得压缩行。WebSocket 的 permessage-deflate 扩展不用。协议日志记解压后的原文，不记压缩行。
- **64 KiB 上限的口径**：单行上限指**线上的字节数**（不含换行），**双向都按这个口径**；压缩行按压缩后的字节数算，原文超过 64 KiB 但压缩后不超的照常发送。解压后的原文另有 1 MiB 上限。
  - **收**：服务端读入的行超过上限，回复 `BAD_REQUEST` 并断开。
  - **发**：生产经 CDN（Cloudflare）接入，CDN 限制单帧大小，所以服务端发出的线上字节数也不能超过 64 KiB。超过时服务端**不发原消息、连接不断**：请求的响应改发同 `id`、同 `type` 的 `RESPONSE_TOO_LARGE` 错误（第 4 节）；推送（无 `id`）直接丢弃；两种情况服务端都记一条日志。
  - 所以大响应要么在 `hello` 里声明 `compress` 为 `gzip`，要么用参数缩小范围：`maps` 的 `parts`、`keyword`（5.42），`encounter_areas` 的 `offset`、`limit`（5.50）。声明 gzip 后超过 10 KiB 的消息都压缩，`maps` 完整目录（原文约 100 KB）和大岛的 `encounter_areas`（约 100 KB）压缩后都在上限以内。
  - 客户端接收的行长不做限制（以服务端的上限为准）。
- **玩家 IP**：限流（注册、登录）按玩家的 IP 计算。经 WebSocket 进来且直连地址在服务端配置的可信代理里时，IP 取 `CF-Connecting-IP` 头；其余情况一律取直连地址（见 `docs/server.md`）。
- **字段约定**：
  - 键名用 `snake_case`。
  - 整数用 JSON 数字，文本一律 UTF-8。游戏数据里的 GBK 文本由服务端在读取时转成 UTF-8，协议里不出现 GBK。
  - **收到不认识的字段一律忽略**，不当作错误，方便以后加字段。
- **日志**：任何一方的协议日志都**不得记录 `password` 字段的值**，打印前必须替换成 `"***"`。

## 2. 消息结构

客户端只发**请求**，服务端回**响应**，服务端还可以主动发**推送**。

```jsonc
// 请求：客户端 → 服务端
{"id": 3, "type": "login", "data": {"account": "alice", "password": "secret1"}}

// 成功响应
{"id": 3, "type": "login", "ok": true, "data": {"account": "alice"}}

// 失败响应
{"id": 3, "type": "login", "ok": false, "error": {"code": "WRONG_CREDENTIALS", "message": "账号或密码错误"}}

// 推送：服务端 → 客户端，没有 id
{"type": "disconnect", "data": {"reason": "KICKED_DUPLICATE_LOGIN", "message": "账号在别处登录，你已下线"}}
```

- **`id`**：由客户端生成的正整数，在同一条连接内递增、不重复，响应会原样带回。
- **`type`**：消息类型，响应的 `type` 和请求相同。
- **处理顺序**：服务端按请求到达的顺序逐个处理，响应也按同样的顺序返回。客户端可以连续发出多个请求，不必等前一个响应回来。
- **失败响应**：
  - `error.code` 是固定的英文错误码，客户端根据它决定怎么处理。
  - `error.message` 是给玩家看的中文说明，客户端可以直接显示。
  - 部分错误会带 `error.data` 补充信息，例如锁定剩余秒数。

## 3. 连接状态

```text
已连接 ──hello──▶ 已握手 ──login──▶ 已登录 ──enter_world──▶ 世界中
                    │ ▲                │ ▲                     │
                    │ └─register───────┘ └────leave_world──────┘
                    └─ 握手之后任何状态都可以发 ping；断开连接等于登出，服务端负责保存数据
```

| 状态 | 允许的请求 |
|---|---|
| 已连接 | `hello` |
| 已握手 | `register`、`login`、`agent_login`、`ping` |
| 已登录 | `characters`、`create_character`、`delete_character`、`enter_world`、`ping`；agent 相关请求见第 9 节 |
| 世界中 | `leave_world`、`status`、`turn`、`places`、`maps`、`warp`、`npcs`、`players`、`party_invite`、`party_join`、`party_reply`、`party_cancel`、`party_leave`、`party_kick`、`talk`、`npc_choose`、`npc_reply`、`npc_close`、`inventory`、`pets`、`addpoint`、`pet_state`、`pet_rename`、`pet_drop`、`pet_ride`、`pet_unride`、`item_equip`、`item_unequip`、`item_move`、`item_drop`、`gold_drop`、`item_use`、`item_inlay`、`battle`、`battle_command`、`battle_ack`、`ping`，以及以后追加的游戏请求；agent 相关请求见第 9 节；战斗中的限制见第 10 章 |

在不允许的状态下发请求，服务端回复 `WRONG_STATE`，连接保持不断。

**连接身份**：每条连接在 `hello` 时声明自己是**玩家**（`player`）还是 **agent**（`agent`）。同一账号最多同时有 1 条玩家连接和 1 条 agent 连接，规则见第 9 节。

**请求分类**：每种请求都属于以下一类，服务端据此判断权限。以后追加游戏请求时，必须在定义里注明类别。

| 类别 | 含义 | 例子 |
|---|---|---|
| 连接 | 连接本身的维护 | `hello`、`ping` |
| 账号 | 账号与角色管理，**只允许玩家连接**发起 | `register`、`login`、`create_character`、`delete_character`、令牌管理 |
| 查看 | 只读取数据，不改变角色 | `characters`、`status`、`places`、`maps`、`npcs`、`players`、`inventory`、`pets` |
| 聊天 | 发言 | 以后的公开发言、第 9 节的 `agent_say` |
| 操作 | 改变角色或世界 | `enter_world`、`party_invite`、`party_join`、`party_reply`、`party_cancel`、`party_leave`、`party_kick`、`leave_world`、`turn`、`warp`、`addpoint`、`pet_state`、`pet_rename`、`pet_drop`、`pet_ride`、`pet_unride`、`item_equip`、`item_unequip`、`item_move`、`item_drop`、`gold_drop`、`item_use`、`item_inlay`，以后的移动、战斗、交易、NPC 对话等 |

agent 在线并且掌握控制权时，玩家连接发起的“操作”类请求一律回复 `CONTROLLED_BY_AGENT`。`enter_world` 和 `leave_world` 不受这条限制，见第 9 节。

## 4. 通用错误码

| 错误码 | 含义 |
|---|---|
| `BAD_REQUEST` | JSON 解析失败、缺少必填字段、字段类型不对。如果整行都无法解析，服务端回复后断开连接 |
| `UNKNOWN_TYPE` | 服务端不认识这个 `type` |
| `WRONG_STATE` | 当前状态不允许这个请求 |
| `PET_RIDING` | 骑乘中的宠物不能放生、寄放、卖掉、改状态（5.22、5.24、5.41），先 `pet_unride`（5.48，STAGE-123） |
| `ALREADY_RIDING` / `PET_NOT_REST` / `RIDE_NOT_LEARNED` / `RIDE_LOYALTY_LOW` / `RIDE_LEVEL_GAP` / `RIDE_TRANS_LIMIT` / `RIDE_NOT_MATCH` | 上骑（5.47）的各项条件，见 5.47（STAGE-123） |
| `ALREADY_IN_PARTY` / `NOT_IN_PARTY` | 已有队伍不能再组、没有队伍可解散，见 5.57（STAGE-162） |
| 真人组队（STAGE-274）的 14 个错误码与 `PARTY_FOLLOWER`，见 5.67 |
| 精工（STAGE-298）的 `INLAY_TOO_MANY`、`INLAY_NOT_SUITABLE`、`INLAY_MULTI_TARGET`、`INLAY_NO_TARGET`、`INLAY_FULL`，见 5.73 |
| 交易（STAGE-294）新增 8 个错误码（`IN_TRADE`、`TARGET_IN_TRADE`、`TRADE_NOT_FOUND`、`TRADE_STALE`、`ITEM_UNTRADABLE`、`PET_UNTRADABLE`、`PET_FULL`、`PET_TOO_STRONG`），见 5.72 |
| `WRONG_ROLE` | 当前连接身份不允许这个请求，例如 agent 连接请求删除角色 |
| `RATE_LIMITED` | 请求过于频繁；`error.data.retry_after_seconds` 表示多少秒后可以重试 |
| `CLIENT_TOO_OLD` | 客户端版本低于服务端的最低版本，只在 `hello` 时出现（5.1），之后断开，客户端不应重试 |
| `RESPONSE_TOO_LARGE` | 服务端要发的响应线上超过 64 KiB（CDN 限制），没有发出原响应，连接不断（STAGE-258，第 1 节）。`id`、`type` 与原请求相同；`error.message` 写明办法：在 `hello` 里声明 `compress` 为 `gzip`，或用 `maps` 的 `parts`/`keyword`、`encounter_areas` 的 `offset`/`limit` 缩小范围。不要原样重试 |
| 决斗（STAGE-306）新增 `TARGET_DEAD`，见 5.74 |
| `INTERNAL` | 服务端内部错误，玩家看到的说明统一为“服务器内部错误，请稍后再试” |

**错误响应的 `id` 和 `type`**：尽量沿用请求里的值。请求里取不到时，`id` 写 `0`，`type` 写 `"error"`：例如整行无法解析，或超过 64 KiB 的超长行。处理顺序是先看请求类型是否认识，再看连接身份（`WRONG_ROLE`），再看连接状态（`WRONG_STATE`）；握手之前发 `hello` 以外的任何请求，一律回复 `WRONG_STATE` 并断开。

## 5. 消息定义

### 5.1 `hello`：握手

连接建立后，客户端发的**第一条消息必须是 `hello`**。在这之前发其他请求，服务端回复 `WRONG_STATE` 并断开。

```jsonc
// 请求
{"id": 1, "type": "hello", "data": {"protocol": 1, "client": "stoneage-cli/0.1.0", "role": "player", "compress": ["gzip"]}}
// 响应
{"id": 1, "type": "hello", "ok": true, "data": {
  "protocol": 1, "server": "stoneage-server/0.1.0", "server_version": "0.1.0",
  "heartbeat_seconds": 20, "idle_timeout_seconds": 60,
  "register_enabled": true, "compress": "gzip",
  "min_client": "0.1.0", "latest_client": "0.2.0",
  "docs_rev": "3f2a9c1b0d4e"
}}
```

- **版本不一致**：协议版本对不上时，回复 `PROTOCOL_VERSION`，`error.data.supported` 列出服务端支持的版本，然后断开。
- **字段不对**：缺少 `protocol`、`protocol` 不是整数、`role` 不是 `player` 或 `agent` 时回复 `BAD_REQUEST`，连接保持，客户端可以重新握手。握手成功后再发 `hello` 回复 `WRONG_STATE`。
- **`role`**：`player` 或 `agent`，不填时视为 `player`。身份一经声明，这条连接就不能再改。玩家连接只能用 `login`，agent 连接只能用 `agent_login`；用错时回复 `WRONG_ROLE`。
- **`register_enabled`**：为 `false` 时，客户端应把“注册”按钮置灰。
- **压缩协商（STAGE-246）**：`compress` 是客户端能接收的压缩方式数组，可省；目前只认 `"gzip"`，不认识的值忽略。服务端认得 `gzip` 时，在响应里返回 `"compress": "gzip"`，之后超过 10 KiB 的消息才会压缩（见第 1 节）；响应里没有 `compress` 就表示这条连接不压缩。`compress` 不是数组（如写成字符串）时回复 `BAD_REQUEST`。hello 响应本身不压缩。
- **客户端版本（`client`，STAGE-100）**：格式是「名字/版本号」，版本号是 `x.y.z`，如 `stoneage-cli/0.1.0`。服务端取最后一个 `/` 后面的部分比较。取不到版本号（没有 `/`，或版本不是 `x.y.z` 三段数字，如 `dev`）时**放行**，不做检查，避免误伤开发中的客户端。玩家和 agent 连接都适用，协议版本不一致的先报 `PROTOCOL_VERSION`。
- **最低版本**：服务端配置了最低客户端版本时，客户端版本低于它，回复 `CLIENT_TOO_OLD`，`error.message` 写明“当前版本 x.y.z 过旧，需要升级到 a.b.c 以上，运行 stoneage update”，`error.data.min_client` 是最低版本，然后断开。客户端应在登录界面显示这句话，并且不再重试。等于最低版本时放行。
- **推荐版本**：响应里的 `min_client`、`latest_client` 是服务端配置的最低版本和最新版本，只在配置了时才出现（配置为空时两个字段都不返回）。客户端版本低于 `latest_client` 时，客户端在消息栏提示一次“有新版本 a.b.c，运行 stoneage update 升级”，与自升级的后台检查提示合并，同一次运行只提示一次。
- **服务端版本和文档修订号（STAGE-188）**：`server_version` 是服务端版本（与 `server` 里的版本号一致）。`docs_rev` 是随服务端发布的 `protocol.md` 与 `agent-guide.md` 内容的哈希前 12 位十六进制，服务端升级且这两份文档有改动时才会变。同一个服务端的每次握手返回的值相同。agent 用它判断文档有没有更新：不一致时先读 `/changes.md` 里新增的条目（每条的 `docs_rev` 标在标题里），再读相关章节；`protocol` 版本号仍是 1，这两个字段是追加的，客户端忽略不认识的字段即可。上面示例里的 `docs_rev` 只是格式示例，不是真实值。

### 5.2 `ping`：心跳

```jsonc
{"id": 9, "type": "ping", "data": {}}
{"id": 9, "type": "ping", "ok": true, "data": {"server_time_ms": 1760000000000}}
```

- 客户端每隔 `heartbeat_seconds` 秒发一次。
- 服务端如果超过 `idle_timeout_seconds` 秒没有收到任何消息，就推送 `disconnect`（原因 `IDLE_TIMEOUT`），然后断开连接。

### 5.3 `register`：注册

```jsonc
{"id": 2, "type": "register", "data": {"account": "alice", "password": "secret1"}}
{"id": 2, "type": "register", "ok": true, "data": {"account": "alice"}}
```

- **注册后不会自动登录**，连接仍处于“已握手”状态，客户端可以接着发 `login`。
- **账号**：3–15 个字符，只能用小写字母、数字和下划线，必须以字母开头。服务端先把大写字母转成小写，再保存和比较。
- **密码**：6–64 个字符，不能有首尾空格。服务端只保存 Argon2id 哈希值。

| 错误码 | 含义 |
|---|---|
| `REGISTER_DISABLED` | 未开放注册 |
| `INVALID_ACCOUNT` | 账号格式不合法 |
| `INVALID_PASSWORD` | 密码格式不合法 |
| `ACCOUNT_EXISTS` | 账号已存在 |
| `RATE_LIMITED` | 同一 IP 注册太频繁，默认每小时 50 次 |

- **次数统计**：成功注册，以及因账号已存在而失败的注册，都计入该 IP 的次数；因格式不合法、被限制而失败的不计入。`error.data.retry_after_seconds` 是最早一次计入的注册滑出时间窗口还要多久。
- **检查顺序**：未开放注册 → 账号格式 → 密码格式 → 注册频率 → 账号是否已存在。
- 缺少 `account` 或 `password` 字段，或类型不对时回复 `BAD_REQUEST`。

### 5.4 `login`：登录

```jsonc
{"id": 3, "type": "login", "data": {"account": "alice", "password": "secret1"}}
{"id": 3, "type": "login", "ok": true, "data": {"account": "alice"}}
```

| 错误码 | 含义 |
|---|---|
| `WRONG_CREDENTIALS` | 账号不存在或密码错误。两种情况**故意不区分**，避免别人借此探测哪些账号存在 |
| `ACCOUNT_LOCKED` | 连续登录失败次数过多，暂时锁定；`error.data.retry_after_seconds` 是剩余秒数 |
| `ACCOUNT_DISABLED` | 账号已被禁用 |
| `RATE_LIMITED` | 同一 IP 失败次数过多 |

- **锁定规则**（默认值）：
  - 同一账号 10 分钟内失败 5 次，锁定 15 分钟；锁定期间即使密码正确也回复 `ACCOUNT_LOCKED`。
  - 同一 IP 10 分钟内失败 20 次，回复 `RATE_LIMITED`。
- **统计细节**：
  - 账号锁定按**账号名**统计，不论这个账号存不存在，所以外人无法借“能否被锁定”判断账号是否存在；
  - 同一账号 10 分钟内的密码错误次数，只统计上一次成功登录之后的；成功登录后清零；
  - 锁定期间的登录尝试不再计入失败次数；
  - 同一 IP 的次数只统计密码错误（`WRONG_CREDENTIALS`），`retry_after_seconds` 是最早一次失败滑出窗口还要多久；
  - 账号格式不合法的 `login` 按 `WRONG_CREDENTIALS` 处理，不回复 `INVALID_ACCOUNT`。
- **检查顺序**：IP 限制 → 账号锁定 → 校验密码 → 账号是否被禁用。**密码正确之后**才告知 `ACCOUNT_DISABLED`，避免外人借此探测账号；账号不存在时也会做同样耗时的密码计算，响应时间看不出区别。
- 缺少 `account` 或 `password` 字段，或类型不对时回复 `BAD_REQUEST`。
- **重复登录**：同一账号**以相同身份**在另一条连接登录成功时，**新连接成功，旧连接被挤下线**。服务端先保存旧连接的数据，再给旧连接推送 `disconnect`（原因 `KICKED_DUPLICATE_LOGIN`），然后断开旧连接。玩家连接和 agent 连接可以同时在线，互不挤占，见第 9 节。
- **审计**：每次登录，无论成功还是失败，都写入审计记录：账号、来源 IP、结果、时间。
- **禁用立即生效**：管理员禁用账号时，这个账号已经在线的所有连接（玩家和 agent）立即收到 `disconnect`（原因 `ACCOUNT_DISABLED`）并被断开，断开前先保存数据；之后再登录回复 `ACCOUNT_DISABLED`。

### 5.5 `characters`：角色列表

```jsonc
{"id": 4, "type": "characters", "data": {}}
{"id": 4, "type": "characters", "ok": true, "data": {
  "max_slots": 2,
  "characters": [
    {"slot": 0, "name": "勇者小明", "level": 1,
     "appearance": {"body": 0, "color": 0, "eye": 0, "mouth": 0},
     "location": {"floor": 1006, "map_name": "萨姆吉尔的村长家"}}
  ]
}}
```

- `characters` 按 `slot` 从小到大排列，只列出已有角色，空栏位不出现在列表里。
- `slot` 取值为 0 到 `max_slots - 1`。

### 5.6 `create_character`：创建角色

```jsonc
{"id": 5, "type": "create_character", "data": {
  "slot": 1, "name": "勇者小明",
  "appearance": {"body": 3, "color": 2, "eye": 1, "mouth": 4},
  "stats": {"vital": 8, "strength": 4, "toughness": 4, "dexterity": 4},
  "elements": {"earth": 6, "water": 4, "fire": 0, "wind": 0},
  "hometown": 0
}}
// 成功时 data.character 的结构与 5.5 中的一项相同
{"id": 5, "type": "create_character", "ok": true, "data": {"character": { ... }}}
```

**字段规则**（出处是原版服务端 `gmsv/callfromcli.c` 的 `lssproto_CreateNewChar_recv`，以及 `gmsv/char/char.c` 的 `CHAR_createNewChar` 和 `CHAR_makeCharFromOptionAtCreate`）：

| 字段 | 规则 | 违反时的错误码 |
|---|---|---|
| `slot` | 0 到 `max_slots - 1`，且该栏位为空 | `INVALID_SLOT` / `SLOT_OCCUPIED` |
| `name` | 去掉首尾空白后为 1–16 个字符（按 Unicode 字符计，中英文都算 1 个）；不能含空白或控制字符；不能含“华义”“神秘人物”，也不能含 `gm`（不区分大小写和全角半角） | `INVALID_NAME`，`message` 写明具体原因；名字命中屏蔽词（词表同 `docs/chat.md` 1.3）也回复此码，`message` 不回显词 |
| `name` | 全服唯一 | `NAME_TAKEN` |
| `appearance.body` | 0–11，对应 `docs/login.md` 外形表的序号 | `INVALID_APPEARANCE` |
| `appearance.color` | 0–3 | `INVALID_APPEARANCE` |
| `appearance.eye`、`appearance.mouth` | 0–4 | `INVALID_APPEARANCE` |
| `stats` | 4 项都在 0–20 之间，总和**不超过** 20 | `INVALID_STATS` |
| `elements` | 4 项都在 0–10 之间，总和**正好** 10，最多 2 项不为 0；`earth` 和 `fire` 不能同时大于 0，`water` 和 `wind` 也不能 | `INVALID_ELEMENTS` |
| `hometown` | 0 萨姆吉尔、1 玛丽娜丝、2 加加村、3 卡鲁它那 | `INVALID_HOMETOWN` |

- **检查顺序**：缺字段或类型不对（`BAD_REQUEST`）→ `slot` 超范围（`INVALID_SLOT`）→ 栏位已有角色（`SLOT_OCCUPIED`）→ 名字 → 外形 → 能力点 → 属性点 → 出生地 → 名字全服唯一（`NAME_TAKEN`）。`create_character`、`delete_character` 只能在“已登录”状态发，在世界中回复 `WRONG_STATE`。
- **名字细节**：首尾空白会被去掉，保存和回复里用去掉之后的名字；“不能含空白”指中间的空格、制表符、全角空格等，另外控制字符、零宽字符这类不可见字符也不行。比较敏感词和判断重名时，英文不分大小写、全角半角视为相同，所以 `Hero`、`hero`、`Ｈｅｒｏ` 是同一个名字。
- **每个账号的角色数**：`max_slots` 为 2，两个栏位都有角色时再创建，回复 `SLOT_OCCUPIED`（栏位 0 或 1）或 `INVALID_SLOT`（栏位 2 及以上）。
- **能力点上限**：原版服务端开启了 `_NEW_PLAYER_CF`，所以能力点的总和只要不超过 20 就行，不要求正好 20。客户端界面仍建议引导玩家把点数分完。
- **外形换算**：服务端把外形、颜色、眼型、嘴型换算成旧的图像编号后保存：
  - 身体 `100000 + body×20 + color×5`
  - 头像 `30000 + body×100 + color×25 + eye×5 + mouth`

  这个换算满足原版服务端 `CHAR_checkFaceImageNumber` 的校验。
- **出生点**：出自 `gmsv/char/char_data.c` 的 `elders` 表：

  | 出生地 | 地图 | 坐标 |
  |---|---|---|
  | 0 萨姆吉尔 | 1006 | (15,22) |
  | 1 玛丽娜丝 | 2006 | (20,16) |
  | 2 加加村 | 3006 | (21,16) |
  | 3 卡鲁它那 | 4006 | (14,20) |

- **初始数值**：按原版服务端的新手配置（`gmsv/setup.cf`）和公式（`gmsv/char/char.c`）计算：等级 1、转生 0、经验 0、石币 1000000、MP 100；最大 HP = 体力×4 + 腕力 + 耐久力 + 速度（按点数计），初始 HP 满；升到下一级所需经验取自 `gmsv/char/char_data.c` 的经验表（1 级升 2 级需要 2 点）。出生时朝向 4（面向南）。
- **出生送的宠物**（STAGE-62）：建角色时按原版规则（`gmsv/char/char.c` 的 `CHAR_createNewChar`、`gmsv/char/char_data.c` 的 `CHAR_PlayerRide`）在**同一个事务里**送 3 只宠物，进游戏后用 `pets`（5.19）就能看到：0 号栏是出生地宠物（萨姆吉尔送乌力乌力、玛丽娜丝送凯比、加加村送克克尔、卡鲁它那送威伯），1 号栏是配套虎、2 号栏是配套雷龙。配套虎和雷龙按角色的身体图像（外形 × 颜色）查原版对照表：虎看落在哪一行（红行 309 格鲁西斯、绿行 307 贝鲁卡、黄行 310 金格萨贝鲁、灰行 308 贝鲁伊卡），雷龙按外形列（布洛多斯、布林帖斯、布拉奇多斯、斯天多斯、邦恩多斯）。三只都是**等级 1、随机资质**（与捕捉、管理命令 `givepet` 同一套生成规则）、**HP 满**、`rest` 状态；原版配置的 `PET1`/`PET2` 在 `enemy.txt` 里查不到、实际不发，所以只有这 3 只。骑乘（配套虎、雷龙）另见后续需求，现在还不能骑。原版出生不送物品（`ITEM1`–`ITEM15` 都为空），这里也不送；只对新建的角色生效，已有角色不补发。
- **出生礼包**：建角色时在同一个事务里往背包第一格（5 号格）放 1 把马祖的棍棒（物品 2690，武器，自带恩惠的精灵 Lv3；本项目自己加的福利，原版不送）。该物品的精灵耗气已改为 0、可装备等级改为 1，所以新角色 1 级就能穿。礼包清单在 `server/gamedata/newplayeritems.go`，只有生产入口（`cmd/stoneage-server`）开启，协议用例和单元测试的背包仍是空的；已有角色不补发，要补用 `stoneage-admin give`。

### 5.7 `delete_character`：删除角色

```jsonc
{"id": 6, "type": "delete_character", "data": {"slot": 1, "name": "勇者小明"}}
{"id": 6, "type": "delete_character", "ok": true, "data": {}}
```

- `name` 是玩家为确认删除而再输入一次的角色名，服务端也要校验它，不能只靠客户端。
- 错误码：
  - `CHARACTER_NOT_FOUND`：该栏位没有角色；
  - `NAME_MISMATCH`：确认名和角色名不一致。
- 删除角色时，一并清理这个角色的排行和名片数据（这些数据的表用外键引用角色并设置级联删除）。
- 只能删除自己账号下的角色：服务端按“账号 + 栏位”查找，别的账号的角色找不到；确认名首尾的空白会被忽略。`slot` 超范围回复 `INVALID_SLOT`。
- 角色正在世界中（例如被 agent 带入世界）时不能删除，回复 `CHARACTER_IN_USE`。

### 5.8 `enter_world`：进入世界

```jsonc
{"id": 7, "type": "enter_world", "data": {"slot": 0}}
{"id": 7, "type": "enter_world", "ok": true, "data": {
  "character": { ... },
  "position": {"floor": 1006, "map_name": "萨姆吉尔的村长家", "x": 15, "y": 22, "direction": 4},
  "status": {"level": 1, "hp": 35, "max_hp": 35, "mp": 100, "max_mp": 100,
             "exp": 0, "next_exp": 2, "gold": 0, "transmigration": 0, "title": "",
             "skill_points": 0, "stats": {"vital": 5, "strength": 5, "toughness": 5, "dexterity": 5},
             "derived": {"attack": 6, "defense": 6, "agility": 5, "charm": 60, "luck": 2},
             "elements": {"earth": 8, "water": 2, "fire": 0, "wind": 0},
             "magic": [{"slot": 0, "id": 301, "name": "岩石撞击", "mp": 10, "target_type": "enemy", "effect": "攻击敌方一个，威力 100，地属性"}]}},
  "savepoint": {"floor": 1000, "x": 61, "y": 111, "map_name": "萨姆吉尔村"}
}}
```

- `character` 的结构与 5.5 中的一项相同。
- `status` 对象（角色页用，STAGE-23）：
  - `transmigration`：转生次数，新角色为 0；
  - `title`：称号，没有称号时为空字符串 `""`，客户端显示“无”；
  - `skill_points`：未分配的能力点。来源有两个：升级（每级 3 点，见 10.9），以及管理命令 `stoneage-admin givepoint`（测试用，见第 6 节）；用 `addpoint` 加到能力上，见 5.21；
  - `exp` 与 `next_exp`：本级内已攒的经验和升到下一级所需的经验。经验按级计：每升一级扣掉这一级所需的量、从剩下的继续攒（原版 `CHAR_HandleExp`），不跨级累计（STAGE-57）；已是最高级时 `next_exp` 为 -1；
  - `stats`：四项能力点，键名与 `create_character` 的 `stats` 相同；
  - `elements`：四项属性点，键名与 `create_character` 的 `elements` 相同；
  - `derived`：衍生数值，由服务端算好，客户端只显示。出处 `gmsv/char/char.c` 的 `CHAR_initcharWorkInt`，结果舍去小数：
    - `attack`（攻击）= 腕力 + 耐久力×0.1 + 体力×0.1 + 速度×0.05；
    - `defense`（防御）= 耐久力 + 腕力×0.1 + 体力×0.1 + 速度×0.05；
    - `agility`（敏捷）= 速度；
    - `charm`（魅力）：新角色 60，0–100。战斗中增减（升级 +2，被打倒 -2，被打飞 -4，10 级以下减半；见 10.9）；数值是基础值加穿着的装备的魅力修正，再夹在 0–100 以内（`gmsv/char/char.c` 的 `CHAR_AddCharm`，`item/item.c` 的 `ITEM_equipEffect`）；
    - `luck`（运气）：1–5 档，每个游戏日第一次进入世界时随机（40% 为 1 档、30% 为 2 档、20% 为 3 档、7% 为 4 档、3% 为 5 档，同 `CHAR_setLuck` 的 `lucktbl`），同一天内重登不变，跨过本地午夜后下次进入时重掷。数值是随机到的档位加穿着的装备的运气修正，夹在 1–5 以内。
  - `magic`：精灵（魔法）栏位（STAGE-48），数组按 `slot` 从小到大，最多 5 项；没有学过精灵时是空数组。每一项：
    - `slot`：栏位 0–4，也是战斗 `magic` 指令的精灵编号（10.5）和角色页清单的编号（`docs/ui.md` 角色页 C）；
    - `id`：魔法表编号（旧 `gmsv/data/magic.txt`）；
    - `name`：精灵名（服务端已转成 UTF-8）；
    - `mp`：使用一次所需的 MP；
    - `target_type`：目标类型（STAGE-78，见 10.5 的“目标类型”）。攻击和状态精灵的单体是 `enemy`（要选敌方一个），全体是 `enemy_all`；攻击精灵的 `enemy` 只表示要选一个中心目标，实际命中范围可能不止一个（按原版掩码，见 `docs/magic-range.md`），客户端按事件里的 `effects` 显示打了哪些目标；自愈是 `self`；治疗他人和解状态的单体是 `ally`（要选己方一个），全体是 `ally_all`；复活是 `ally_down`（要选己方一个倒下的单位）；场地属性是 `none`（整场生效，不用选）；属性反转（彩虹、极光）是 `any`（任一方一个活着的单位）；防御精灵（光、镜子、守护）是 `ally`（缺省落在自己身上），复活防御精灵是 `ally_down`。召唤龙阵（STAGE-202）：奔龙阵是 `enemy`（要选敌方一个），强恩阵、奔雷阵、GM 烟火是 `enemy_all`（不用选）。精灵版宠物技能（`attskill`）的目标类型取这个技能自己的：破除防御、突击、一击必杀、背水之战是 `enemy`，不防守战法是 `none`。
    - `effect`：效果文字（STAGE-116），服务端从精灵模板的函数、威力、属性、状态和目标算好，客户端只显示，和物品的 `effect` 一样。写法如下（`N` 是数字）：
      - 攻击：「攻击敌方一个，威力 N」，有属性时再加「，地属性」（地、水、火、风）；范围按原版掩码写「敌方一排」「敌方十字范围」或「敌方全体」（STAGE-218，见 `docs/magic-range.md`）；
      - 自愈：「回复自己 HP N」；治疗他人：「回复己方一个 HP N」，全体写「回复己方全体 HP N」；
      - 状态：「使敌方一个中毒，持续 N 回合」，状态名取中毒、麻痹、睡眠、石化、醉酒、混乱、虚弱、魔障、沉默（对应 10.2 的 `poison`、`paralysis`、`sleep`、`stone`、`drunk`、`confusion`、`weaken`、`barrier`、`nocast`），全体写「使敌方全体…」；
      - 解状态：「解除己方一个的中毒」（状态名同上）；每次只解一种，“全”或表里没有对应名字的写「解除己方一个的一种异常状态」，全体写「解除己方全体各一种异常状态」；
      - 复活：「复活己方一个倒下的单位，HP 约 N」，N 是威力（实际回复量是 RAND(威力×0.9, 威力×1.1)，见 10.5）；
      - 魔抗系（STAGE-228）：「使己方一个（全体）魔抗提升 N%，持续 M 回合」「使己方一个火抗（电抗、冰抗），持续 M 回合（原版未生效，只占状态槽）」；
      - 防御（STAGE-199）：「己方一个挨下的 N 次物理攻击改为吸收（回复伤害值的血量）」「己方一个接下的 N 次物理攻击反弹给攻击者」「己方一个接下的 N 次物理攻击无效」，N 是防御次数（吸收、反射、无效分别对应 10.2 的 `absorb`、`reflect`、`vanish`）；
      - 复活防御：「复活己方一个倒下的单位，HP 约 N，」后面接上面的防御文字。
      - 召唤龙阵（STAGE-202）：奔龙阵写「召唤龙攻击敌方一个，威力 N」，强恩阵、奔雷阵、GM 烟火写「召唤龙攻击敌方全体，威力 N」（N 是威力，即这一下的攻击力）；
      - 精灵版宠物技能（STAGE-202，`attskill`）：「使用宠物技能 XX」，XX 是 `petskill.txt` 的技能名（破除防御、突击、一击必杀、背水之战其之１、不防守战法）。
    来源有两个，共用同样的 0–4 栏位（STAGE-190）：
      - **装备自带的精灵**：身上穿的装备带精灵编号（`itemset.txt` 的 `ITEM_MAGICID`，效果文字里的「滋润的精灵 Lv1」只是它的显示）时，同号装备位（0–4）就是一个精灵栏位，`slot` 等于装备位编号，同原版战斗指令 `J` 的“装备位号即精灵号”；`mp` 取装备的 `ITEM_MAGICUSEMP`，不是魔法表推算的值（有些装备写 0，就是不耗 MP）。穿上、卸下装备（5.25、5.26）和管理命令改了装备位时，服务端用 `status_changed` 推送新的 `magic`；魔法表里没有的精灵编号（原版服务端的少数特殊精灵）当作没有，不列出。
      - 人物自己的精灵（栏位存在角色上）：战斗中的精灵学习（未做）、管理命令 `stoneage-admin givemagic`（直接写库并发出 PostgreSQL 通知，服务端收到后重读这个角色的精灵栏位并通过 `status_changed`（10.10）推送；角色不在线时只写库，等下次进入世界时可见）。同一个栏位上人物自己的精灵优先，装备自带的精灵被占住时不显示。
  - `metamo`（STAGE-203，变幻的精灵）：变身中的状态，形如 `{"pet": "乌力", "image": 100250, "until": 1792412345678}`，`pet` 是宠物名，`image` 是宠物模板的图像编号（只记录，一期终端不显示），`until` 是到期的毫秒时间戳。没有变身时**省略**这个字段。变身只在内存里，登出、服务端重启、重新进入世界都回到自己的外观（5.51）。
  - 以上字段都是新增的，老客户端忽略它们即可；`max_hp` 仍按 5.6 的公式计算。
- `savepoint`：存档点落点（STAGE-134），客户端的「回城」入口用它，结构是
  `{"floor": 1000, "x": 61, "y": 111, "map_name": "萨姆吉尔村"}`。记录过存档点的角色给存档点，
  没记录过的给出生地（`SavepointRespawn`）；`status` 响应同样带这个字段，位置或存档点变化后会跟着更新。
- 错误码：
  - `CHARACTER_NOT_FOUND`：该栏位没有角色；
  - `CHARACTER_IN_USE`：同账号的另一条连接已经在世界中，而且操控的是**另一个**角色。同一账号同一时间只能有一个角色在世界中。
- 成功后状态变为“世界中”。`slot` 超范围回复 `INVALID_SLOT`；已经在世界中再发 `enter_world` 回复 `WRONG_STATE`。
- 刚断开的同账号连接还在保存数据时，新的 `enter_world` 会等它保存完再读取角色，所以断线马上重连也能读到最新位置。
- **控制权**：响应的 `data.control` 说明当前由谁操控、本连接是不是操控方，例如 `{"controller": "agent", "you": "observer"}`。同账号的另一条连接已经在世界中操控同一个角色时，以第 9 节的规则决定由谁操控。
- 角色此刻在战斗中时（例如 agent 在战斗中进入世界），响应多一个 `battle` 字段，内容见 10.11；不在战斗中时没有这个字段。
- `party`（STAGE-162）：当前队伍，结构见 5.57；没有队伍时为 `null`。`status` 的响应也带这个字段。
- `social`（STAGE-276）：社交开关的当前值，`{"visible": bool, "invite": bool, "trade": bool}`（`trade` 为 STAGE-296 加入），结构见 5.69。`status` 的响应也带这个字段。
- `map_name` 由服务端从旧地图文件头里读取，去掉 `|` 及其后面的内容。
- `direction` 取值 0–7，0 为北，按顺时针每次加 45°。服务端负责和旧数据的方向值互相换算。
- 示例里 `status` 的数值仅作示意，实际以服务端计算结果为准。

### 5.9 `leave_world`：离开世界

```jsonc
{"id": 8, "type": "leave_world", "data": {}}
{"id": 8, "type": "leave_world", "ok": true, "data": {}}
```

服务端保存位置和状态后，连接回到“已登录”状态，客户端可以重新选择角色。直接断开连接，效果也等同于离开世界并登出。保存失败时 `leave_world` 回复 `INTERNAL`，角色仍在世界中，可以重试。

以下情况服务端同样会先保存再断开：同账号同身份重复登录挤下线、账号被禁用、服务器关闭、空闲超时。被断开的连接上已读入但还没处理的请求一律丢弃，不会改动角色。

### 5.10 推送 `disconnect`：服务端即将断开

```jsonc
{"type": "disconnect", "data": {"reason": "IDLE_TIMEOUT", "message": "长时间没有响应，连接已断开"}}
```

服务端发出这条推送后会立即关闭连接。`reason` 的取值：

| 取值 | 含义 |
|---|---|
| `KICKED_DUPLICATE_LOGIN` | 账号在别处登录 |
| `IDLE_TIMEOUT` | 超时没有收到任何消息 |
| `SERVER_SHUTDOWN` | 服务器维护 |
| `PROTOCOL_ERROR` | 收到无法处理的数据 |
| `AGENT_TOKEN_REVOKED` | agent 所用的令牌被吊销或已过期（只发给 agent 连接） |
| `KICKED_BY_PLAYER` | 玩家让 agent 下线（只发给 agent 连接） |
| `ACCOUNT_DISABLED` | 账号被管理员禁用（发给这个账号的所有连接，玩家和 agent 都有） |

### 5.11 推送 `notice`：系统消息

```jsonc
{"type": "notice", "data": {"level": "info", "text": "欢迎来到石器时代"}}
```

`level` 取 `info`、`warning` 或 `error`。客户端把它写进消息栏，类型显示为“系统”，颜色按 `docs/ui.md` 的约定。

### 5.12 `status`：查看当前状态（查看类，世界中）

```jsonc
{"id": 9, "type": "status", "data": {}}
{"id": 9, "type": "status", "ok": true, "data": {
  "character": { ... }, "position": { ... }, "status": { ... },
  "control": {"controller": "agent", "you": "observer"},
  "encounter": false}}
```

响应的内容与 `enter_world` 相同，反映服务端此刻的状态；`status` 里的 `ride_level` 是骑乘等级（0 / 40 / 80 / 120 / 200，新角色 0，见 5.49，STAGE-122）；`encounter` 是遇敌开关的当前状态（5.29，战斗中时另有 `battle` 字段，见 10.11）。玩家作为观察方时也可以发起（见第 9 节）。

### 5.13 `turn`：转向（操作类，世界中）

```jsonc
{"id": 10, "type": "turn", "data": {"direction": 2}}
{"id": 10, "type": "turn", "ok": true, "data": {}}
```

- `direction` 取值 0–7（含义同 `enter_world`），缺少或超出范围回复 `BAD_REQUEST`。
- 成功后服务端给世界中的**每一条**连接推送 `position_changed`（见 5.14）。
- 移动、战斗等玩法还没有做；`turn` 是最小的一个“操作”类请求，让第 9 节的控制权规则能被完整验证，以后追加更多操作类请求时照同样的规则处理。观察方发起时回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`。

### 5.14 推送 `position_changed`：角色的位置或朝向变了

```jsonc
{"type": "position_changed", "data": {"floor": 1006, "map_name": "萨姆吉尔的村长家", "x": 15, "y": 22, "direction": 2}}
```

`data` 的结构与 `enter_world` 响应里的 `position` 相同。角色在世界中的所有推送，都同时发给玩家连接和 agent 连接（包括发起这个操作的那一条，推送总是先于响应到达），保证两边看到的状态一致。

### 5.15 `places`：取地点列表（查看类，世界中）

```jsonc
{"id": 11, "type": "places", "data": {}}
{"id": 11, "type": "places", "ok": true, "data": {"places": [
  {"id": 1, "name": "萨姆吉尔村", "floor": 1000, "x": 50, "y": 116},
  {"id": 2, "name": "玛丽娜丝渔村", "floor": 2000, "x": 72, "y": 33},
  {"id": 3, "name": "加加村", "floor": 3000, "x": 74, "y": 54},
  {"id": 4, "name": "卡鲁它那村", "floor": 4000, "x": 100, "y": 96},
  {"id": 5, "name": "萨姆吉尔近郊", "floor": 100, "x": 600, "y": 550},
  {"id": 6, "name": "玛丽娜丝近郊", "floor": 100, "x": 30, "y": 580},
  {"id": 7, "name": "加加村近郊", "floor": 200, "x": 760, "y": 445},
  {"id": 8, "name": "卡鲁它那近郊", "floor": 200, "x": 275, "y": 575}
]}}
```

- 地点表由服务端维护，客户端不带任何地图数据；增删地点只改服务端一处，客户端重新取一次列表即可。
- `id` 从 1 开始，是 `warp` 用的编号；`name` 是显示名；`floor`、`x`、`y` 是传送后角色所在的位置（地图编号和格子坐标，含义同 `position`）。
- 分两组（STAGE-40）：1–4 是四个出生村，站在**村内**（村内地图没有遇敌区域，`encounter on` 会提示没有敌人）；5–8 是各村的初级野外练级点，站在大岛上的遇敌区域里，打开遇敌开关（5.29）就能一场接一场地打。村内坐标是 `mapwarp.txt` 里从大岛走进村后站的位置；客户端按 `docs/map.md` 的入口对照表把村内地点标在村子入口上。
- 玩家作为观察方时也可以发起（查看类）。

### 5.16 `warp`：传送（操作类，世界中）

```jsonc
{"id": 12, "type": "warp", "data": {"place": 3}}
{"id": 12, "type": "warp", "data": {"floor": 1006}}
{"id": 12, "type": "warp", "ok": true, "data": {}}
```

- 目标二选一（STAGE-82）：
  - `place` 是 `places` 里的地点编号（整数），落在地点表给的 `floor`、`x`、`y`；
  - `floor` 是地图编号（整数，即 `position.floor`），落在该地图的默认落点上，落点是服务端数据，客户端不需要知道坐标。地图名由 `floor` 决定，和 `position_changed` 里的 `map_name` 一致。
- 两个分支走**同一道准入**（STAGE-161）：目标 `floor` 必须出现在快捷传送目录（5.42）里，`place` 指向的 `floor` 也要在目录里。目录之外的测试图、占位图、没有可站立地形的图不能经手写 `warp`、按名字补全或跨图找 NPC 绕进来。
- 同时给了 `place` 和 `floor`、两个都没给（包括两个都是 null），或者不是整数（字符串、小数）回复 `BAD_REQUEST`，角色位置不变。
- 地点编号不在列表里、或者地图编号不在快捷传送目录里，都回复 `PLACE_NOT_FOUND`，位置不变。
- 传送**免费**，不扣石币、不消耗道具（STAGE-82）；`floor` 与 `place` 的落点都不做可走格校验。
- 成功后角色立刻出现在该地点的 `floor`、`x`、`y`，没有任何移动过程，朝向不变；服务端给世界中的**每一条**连接推送 `position_changed`（见 5.14，推送先于响应到达），位置随角色一起保存。
- 属于“操作”类：观察方发起时回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同 `turn`。
- 战斗中回复 `IN_BATTLE`（见第 10 章），位置不变。交易中不能传送、传送冷却和频率限制，做这些玩法时再决定，届时追加错误码。

### 5.17 `inventory`：查看背包（查看类，世界中）

```jsonc
{"id": 13, "type": "inventory", "data": {}}
{"id": 13, "type": "inventory", "ok": true, "data": {
  "gold": 1000000,
  "slots": [
    {"slot": 5, "item_id": 0, "name": "小斧头", "effect": "攻 +9 防 -3 敏 -3", "type": 1, "count": 1}
  ]
}}
```

- 槽位（`slot`）是 0–19 的绝对编号：**0–4 是装备位**（0 头、1 身、2 手、3、4 饰品），**5–19 是背包**；页面上的编号就是以后命令的参数（`item equip 5`）。
- `slots` 只列出有物品的格子，空的格子不出现，按 `slot` 从小到大排列。
- `item_id` 是物品模板编号，来自原版服务端 `gmsv/data/itemset.txt`（从 0 开始）；`name` 和 `effect`（效果文字，如“攻 +9 防 -3 敏 -3”）已经由服务端转成 UTF-8，客户端不需要任何物品数据；`type` 是物品表里的类型（穿脱校验用它，见 `docs/ui.md` 背包页 B）；`count` 是数量，不可堆叠的物品为 1。
- `target_type` 是物品在战斗中的目标类型（STAGE-78，取值见 10.5）：能在战斗中用的消耗品是 `self`、`ally` 或 `ally_all`；装备、材料和只能在战斗外用的物品是 `none`（战斗中用回复 `ITEM_UNUSABLE`）。
- `field_target_type`（STAGE-326）：世界中（背包里的「使用」）的目标类型，只在和 `target_type` 不同时才带，缺省同 `target_type`。只能在战斗外用、目标是他人或自己的消耗品（如安抚之素、高级宠物食品）带 `ally`（要选宠物）或 `self`，`target_type` 仍是 `none`；战斗列表只看 `target_type`。
- **装备耐久**（STAGE-209，原版“道具损坏”）：有耐久的物品（最大耐久 > 0）多带三项：`durability`（当前耐久）、`max_durability`（最大耐久）、`crush_level`（受损等级：0 完好、1 受损、2 毁坏、3 不堪使用；为 0 时省略，缺省按 0 处理）。没有耐久的物品不带这三项。耐久降到 0 的装备在战斗中被删掉，背包推送里不再出现。只有人物本人的装备会损坏，宠物和怪物没有。
- **精工**（STAGE-298，5.73）：类别码非空的物品多带 `inlay` 对象：可镶装备为 `{"role": "gear", "free": 0–3, "codes": [码或 null ×3]}`，材料为 `{"role": "material"}`；镶过东西的装备，`effect` 是镶后重写的文字。没有类别码的物品不带 `inlay`，现有数据下没有任何物品带它。
- `gold` 与 `status` 里的 `gold` 相同；物品变化时以 `inventory_changed` 推送里的为准。
- 玩家作为观察方时也可以发起（查看类）。客户端进入世界后请求一次，之后靠推送更新。

### 5.18 推送 `inventory_changed`：背包变了

```jsonc
{"type": "inventory_changed", "data": {"gold": 1000000, "slots": [ ... ]}}
```

- `data` 的结构与 `inventory` 响应相同，反映推送这一刻的完整背包。
- 角色的背包变化后，服务端给世界中的**每一条**连接（玩家和 agent）都推送，规则同 `position_changed`。
- 一期背包变化的唯一来源是管理命令发物品（`stoneage-admin give`）：管理命令直接写数据库并发出 PostgreSQL 通知，服务端收到通知后重读这个角色的物品再推送；角色不在线时只写库，等下次进入世界时可见。

### 5.19 `pets`：查看宠物（查看类，世界中）

```jsonc
{"id": 14, "type": "pets", "data": {}}
{"id": 14, "type": "pets", "ok": true, "data": {
  "max_slots": 20,
  "pets": [
    {"slot": 0, "species_id": 1, "species": "乌力", "name": "乌力", "level": 5, "transmigration": 0,
     "exp": 0, "next_exp": 67, "hp": 35, "max_hp": 35,
     "attack": 5, "defense": 6, "agility": 7, "loyalty": 8,
     "elements": {"earth": 80, "water": 20, "fire": 0, "wind": 0},
     "growth": {"basis": "raised", "from_level": 1,
                "rates": {"hp": 7.25, "attack": 1.1, "defense": 0.85, "agility": 0.92}},
     "quality": [{"stat": "vital", "points": 22, "tier": -1}, {"stat": "strength", "points": 18, "tier": -2},
                 {"stat": "toughness", "points": 20, "tier": 0}, {"stat": "dexterity", "points": 24, "tier": -4}],
     "state": "rest", "skills": [{"id": 1, "name": "攻击", "slot": 1}]}
  ]
}}
```

- `max_slots` 是宠物栏位数（20，编号 0–19，页面上的编号就是命令的参数，如 `pet status 2`；STAGE-346 起从 5 格扩到 20 格）；`pets` 只列有宠物的栏位，按 `slot` 从小到大排列，空栏位由客户端按 `max_slots` 补上。
- `species_id` 是宠物编号（原版服务端 `gmsv/data/enemybase.txt` 的模板编号），`species` 是种族名；`name` 是宠物自己的名字（给宠物时等于种族名，改名见 5.23）。
- `transmigration` 是宠物的转生次数（STAGE-196，原版 `CHAR_TRANSMIGRATION`），新宠物为 0，旧宠物也是 0。骑乘要求不超过 2（见 5.47 第 8 条）。宠物转生窗口已在 STAGE-236 做完（写入转生次数），这个字段也可由管理命令 `stoneage-admin setpettrans <角色名> <栏位> <次数>` 设（测试用，写库并通知，在线的连接立即收到 `pets_changed`，见第 6 节）。
- `exp` 是本级内已攒的经验，`next_exp` 是从当前级升到下一级所需的经验（每级单独计，与角色的经验表相同，出处 `gmsv/char/char_data.c`；已是最高级时为 -1）。经验不跨级累计：每升一级，服务端按原版 `CHAR_HandleExp` 从经验里扣掉这一级所需的量，剩下的留给下一级（STAGE-57）。
- `hp` 与 `max_hp`，`attack`、`defense`、`agility` 都是服务端按旧公式算好的整数（能力以 1/100 点保存，显示时舍去小数）。**模板资质本身仍不发**；只发这只宠物的素质点和它距离满值的档位（`quality`，见下），按 STAGE-212 的要求公开。
- `growth` 是这只宠物的成长率（STAGE-216）。由服务端按宠物的起始快照（`character_pets` 的 `origin_*`，STAGE-215）现算，不存：
  - `basis`：`raised` 是起始等级为 1，“从 1 级起养”；`caught` 是起始等级大于 1，“捕获或获得时已有等级”，成长率只统计到手之后的升级；`unknown` 是历史宠物，没有起始记录，不倒推。
  - `from_level`：起始等级；`unknown` 时为 `null`。
  - `rates`：`hp`、`attack`、`defense`、`agility` 四项，每升一级平均涨多少，JSON 数字，四舍五入到两位小数（例如 `4` 就是 4.00）。公式：成长率 =（现在的值 − 起始值）÷（现在的等级 − 起始等级）。“值”用上面发给客户端的整数：`hp` 用**最大 HP**（不是当前 HP，受伤和治疗不影响）；`attack`、`defense`、`agility` 用同名字段的算法；起始值用起始能力按同一算法算出。例：1 级起养的宠物现在 5 级，最大 HP 从 20 涨到 49，则 `hp` = (49 − 20) ÷ (5 − 1) = 7.25。
  - 算不出时 `rates` 仍是对象，四项都是 `null`（不是 0）：`basis` 为 `unknown`，或现在的等级不高于起始等级（刚获得、还没升过级）。
- `quality` 固定四项，顺序是 `vital`（体力）、`strength`（腕力）、`toughness`（耐久力）、`dexterity`（速度）：
  - `points` 是生成时保存的素质点，即库里的 `base_*`（生成时 ±2 之后、追加 10 点之前的值，原版 `CHAR_ALLOCPOINT`）。不是现在的能力，也不含升级增量。
  - `tier` 是档位 = `points` − (模板对应资质 + 2)。素质点最多比模板值高 2，所以正常范围是 −4 到 0；0 表示满，客户端据此显示。超出范围就照实发，不截断。
- `loyalty` 是忠诚（0–100），由服务端按主人当前的等级、魅力和转生次数，以及宠物的等级现算；战斗里增减的可变部分（`CHAR_VARIABLEAI`，STAGE-127）存在宠物上，算进结果里。
- `elements` 是宠物的地、水、火、风（模板值）。
- `state` 取值：`rest` 休息、`standby` 待命、`battle` 出战、`ride` 骑乘（STAGE-123，见 5.47，同时最多 1 只；骑着的宠物本身不是出战或待命，另一只宠物仍可以出战或待命，STAGE-149）。管理命令给的宠物都是 `rest`，之后玩家用 `pet_state`（5.22）改；“已倒下”不是状态，客户端看到 `hp` 为 0 自行显示。
- `skills` 是技能列表：`slot` 从 1 开始（技能栏位编号，最多 7 个），只列有技能的栏位；`name` 取自原版服务端 `petskill.txt`，已转成 UTF-8；`target_type` 是技能的目标类型（STAGE-78，取值见 10.5），例如攻击是 `enemy`，待机是 `none`，只能在战斗外用的加工、料理也是 `none`。
- 玩家作为观察方时也可以发起（查看类）。客户端进入世界后请求一次，之后靠推送更新。

### 5.20 推送 `pets_changed`：宠物变了

```jsonc
{"type": "pets_changed", "data": {"max_slots": 20, "pets": [ ... ]}}
```

- `data` 的结构与 `pets` 响应相同（每只宠物都带 `growth`、`quality`，STAGE-216），反映推送这一刻的完整宠物列表。
- 角色的宠物变化后，服务端给世界中的**每一条**连接（玩家和 agent）都推送，规则同 `inventory_changed`。
- 宠物变化有两个来源：玩家或 agent 发的 `pet_state`、`pet_rename`、`pet_drop`（5.22–5.24，服务端写库后直接推送）；管理命令 `stoneage-admin givepet`（管理命令直接写数据库并发出 PostgreSQL 通知，服务端收到后重读这个角色的宠物再推送；角色不在线时只写库，等下次进入世界时可见）。

### 5.21 `addpoint`：加点（操作类，世界中）

```jsonc
{"id": 15, "type": "addpoint", "data": {"stat": "vital", "points": 2}}
{"id": 15, "type": "addpoint", "ok": true, "data": {}}
```

- `stat` 是要加点的能力：`vital`（体力）、`strength`（腕力）、`toughness`（耐久力）、`dexterity`（速度），键名与 `status.stats` 相同。缺少，或不在这四个值里，回复 `BAD_REQUEST`。
- `points` 是加的点数，可以省略，省略时为 1；给出时必须是正整数，0、负数、小数、字符串都回复 `BAD_REQUEST`。
- 每点给选中的能力 +1，同时扣掉 1 点未分配点数（`status.skill_points`）。出处 `gmsv/char/char.c` 的 `CHAR_SkillUp`：原版服务端每个请求只加 1 点，没有点数时什么都不做。这里一次可以加多点，结果等同于连续加这么多次。
- **整条命令成败**：未分配点数不够 `points` 时回复 `NOT_ENOUGH_POINTS`（错误说明里写明现有点数），能力和点数都不变，不做部分加点。
- 角色已死亡（`hp` 为 0）回复 `CHARACTER_DEAD`。新服务端还没有战斗，正常游戏中角色不会死亡，这个检查是为战斗做好之后预留的。
- 能力加上以后，服务端按 5.8 的公式重算 `max_hp`、`attack`、`defense`、`agility`；**当前的 `hp` 不变**，不会因为最大血量变大而自动补满。`charm`、`luck` 不受影响。单项能力最高 32767（数据库里是 `smallint` 列），超过回复 `BAD_REQUEST`。
- 加点直接写库，不等离开世界：扣点数和加能力在同一条 `UPDATE` 里完成，成功后才改内存。所以管理命令发的点数和加点共用同一份数据，不会互相覆盖；断线、服务端重启也不丢。
- 战斗升级得到的未分配点数，战斗结算后立即写库（不等离开世界），升级后马上就能加点（STAGE-151）。如果库里的点数比内存少（只会是有人绕过服务端直接改库），加点回复 `NOT_ENOUGH_POINTS` 的同时推送一次 `status_changed`，内容是库里的权威状态。
- 成功后服务端给世界中的**每一条**连接（玩家和 agent）推送 `status_changed`（见 10.10），内容是加点之后完整的 `status`，推送先于响应到达。
- **战斗中也能加点**（STAGE-189）：开着遇敌连场打时，两场之间只有约 2 秒空档，点数又是战斗结算才发的，不能让玩家为了加点先停战。战斗中加点的规则和平时完全一样（写库、扣点、推送 `status_changed`），区别只有两条：
  - 战斗单位用的是开战时的能力快照，所以**加的点从下一场起才生效**，当前这场的攻防、速度不变，不影响战斗平衡；战斗结束只回写血量、MP，不会把加的点覆盖掉；
  - 战斗中推送的 `status.hp`、`status.mp` 取战斗里的实时值（不是开战前的值），`max_hp` 按新的体力重算。
- 属于“操作”类：观察方发起时回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同 `warp`。
- 错误码：`BAD_REQUEST`、`NOT_ENOUGH_POINTS`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。

### 5.22 `pet_state`：设置宠物状态（操作类，世界中）

```jsonc
{"id": 21, "type": "pet_state", "data": {"slot": 2, "state": "battle"}}
{"id": 21, "type": "pet_state", "ok": true, "data": {}}
```

- `slot` 是宠物栏位（0–19 的整数），`state` 取 `battle`（出战）、`standby`（待命）、`rest`（休息）。缺少字段、栏位不对、`state` 不在这三个值里，回复 `BAD_REQUEST`。
- 栏位没有宠物回复 `NO_PET`。
- 骑乘中的宠物（`state=ride`）不能用本命令改状态，回复 `PET_RIDING`，先 `pet_unride`（5.48，STAGE-123）。
- **出战**最多 1 只：原来的出战宠自动变成 `rest`，两只宠物的新状态在同一次推送里一起给出。出处 `gmsv/char/pet.c` 的 `PET_SelectBattleEntryPet`。
- **待命**最多 3 只（原版服务端 `SPET` 待命掩码的上限，`gmsv/callfromcli.c` 的 `lssproto_SPET_recv`）。待命的宠物已经满 3 只时，再设一只待命回复 `STANDBY_FULL`，要设的这只不算在内。
- **休息**不限数量。
- 已倒下（`hp` 为 0）的宠物不能出战，回复 `PET_FAINTED`；倒下的宠物可以设为待命或休息。
- 设成它已经是的状态（例如对已出战的宠物再设出战）：成功，不改任何东西，也不推送。
- **整条命令成败**：校验不过时数据不变，不会出现“旧出战宠收回了、新宠没上场”的情况。
- 状态写库在一个事务里完成，断线、服务端重启都不丢。写库成功后服务端给世界中的**每一条**连接（玩家和 agent）推送 `pets_changed`（5.20），推送先于响应到达。
- 属于“操作”类：观察方发起时回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同 `warp`；战斗中回复 `IN_BATTLE`（战斗还没做，服务端暂时不会返回这个错误）。
- 错误码：`BAD_REQUEST`、`NO_PET`、`PET_FAINTED`、`STANDBY_FULL`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.23 `pet_rename`：给宠物改名（操作类，世界中）

```jsonc
{"id": 22, "type": "pet_rename", "data": {"slot": 2, "name": "小白"}}
{"id": 22, "type": "pet_rename", "ok": true, "data": {}}
```

- `slot` 同 5.22。`name` 可以省略，或者是空字符串，表示恢复种族名（5.19 的 `species`）。
- `name` 先去掉首尾空白，然后：
  - 1–16 个字符（按 UTF-8 字符数计；原版服务端 `gmsv/callfromcli.c` 的 `lssproto_KN_recv` 限 16 字节，即 GBK 的约 8 个汉字，新协议不按字节算）；
  - 不能含空白或控制字符。
  - 违反回复 `INVALID_NAME`，错误说明里写明原因。与角色名的规则相比，不查保留名（“华义”等），但查屏蔽词（词表同 `docs/chat.md` 1.3），命中同样回复 `INVALID_NAME`，`message` 不回显词。
- 名字和种族名相同时，等同于恢复种族名。
- 改名不受出战、待命、倒下影响，战斗中回复 `IN_BATTLE`。
- 守护兽不能改名（原版服务端 `CHAR_inputUserPetName`）。一期的宠物表没有守护兽标志，暂不检查，待核对。
- 写库、推送 `pets_changed`、观察方的限制，都与 5.22 相同。
- 错误码：`BAD_REQUEST`、`INVALID_NAME`、`NO_PET`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.24 `pet_drop`：放生（操作类，世界中）

```jsonc
{"id": 23, "type": "pet_drop", "data": {"slot": 2, "confirm": "yes"}}
{"id": 23, "type": "pet_drop", "ok": true, "data": {}}
```

- `slot` 同 5.22。`confirm` 必须是字符串 `"yes"`；缺少或是别的值，回复 `BAD_REQUEST`，说明里写明“放生要确认”。客户端要求玩家再输入一次 `yes` 才会发这条请求，`confirm` 是服务端这一层的保险。
- 放生后宠物从栏位里删除，**不可恢复**。新服务端没有局部地图，不会像原版服务端那样掉在地上（`PET_dropPetAbsolute`）。
- 栏位没有宠物回复 `NO_PET`。出战、待命、休息、已倒下的宠物都能放生；放生出战宠之后，这个角色暂时没有出战宠。
- 守护兽不能放生（同 5.23，暂不检查）；骑乘中的宠物不能放生，回复 `PET_RIDING`（STAGE-123）；交易中不能放生（交易还没做，暂不检查）。交易以后补上时，会在这里加错误码。
- 写库、推送 `pets_changed`、观察方的限制，都与 5.22 相同；战斗中回复 `IN_BATTLE`。
- 错误码：`BAD_REQUEST`、`NO_PET`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。
### 5.25 `item_equip`：穿上装备（操作类，世界中）

```jsonc
{"id": 25, "type": "item_equip", "data": {"slot": 5}}
{"id": 25, "type": "item_equip", "ok": true, "data": {}}
```

- `slot` 是背包格（5–19）。缺少，或不是 0–19 的整数，回复 `BAD_REQUEST`；格子是装备格（0–4，已经穿着）回复 `BAD_REQUEST`；格子是空的回复 `EMPTY_SLOT`。
- 物品不是装备（如石、料理）回复 `NOT_EQUIPMENT`。
- **部位由物品类型决定**（出处 `gmsv/item/item.c` 的 `ITEM_getEquipPlace`）：头盔到 0 号（头），铠甲到 1 号（身），斧、棍、枪、弓、投掷类到 2 号（手），手镯、乐器、项链、戒指、腰带、耳环、鼻环、护身符到 3 号或 4 号（饰品）。盾牌一期不支持。
- 饰品先放 3 号；3 号有东西放 4 号；两个都有东西时与 4 号互换。
- **等级不够**（`level` 小于物品要求的等级）回复 `REQUIREMENT_NOT_MET`，错误说明写明要求的等级和现在的等级。物品要求的等级来自原版服务端 `itemset.txt` 的 `lv` 列。腕力、速度、转生的要求（`ITEM_NEEDSTR` 等）来自原版服务端的 `itemset4.txt`，仓库里的数据目录没有这个文件，一期不检查，待核对。
- 目标部位已有装备时两件**互换**：原来的装备回到这一背包格（不会因为互换而失败，除非原装备不能穿在这个背包格要去的地方，此时 `NOT_EQUIPMENT`，见 5.27）。
- 数量不是 1 的格子（叠放的物品）不能穿，回复 `BAD_REQUEST`，说明里写明数量。
- **整条命令成败**：任何一项检查不过，格子都不变。
- 成功后服务端给世界中的**每一条**连接（玩家和 agent）推送 `inventory_changed`（5.18）。如果攻击、防御、敏捷的衍生数值变了，再推送 `status_changed`（10.10），内容是穿脱之后完整的 `status`；推送先于响应到达。
- **装备加成**：攻击 = 能力公式的结果（5.8）加上穿着的装备的攻击合计；防御、敏捷同理。装备数值来自 `itemset.txt`：攻击取 `ma` 列，防御取 `mh` 列，敏捷取 `mq` 列（统计上与效果文字“攻 / 防 / 敏”对得上；这些列在数据里各有两列写同一个值，取第一列）。加成后的数值不低于 0（小斧头的防 -3 不会让防御变成负数）。魅力修正取第 49 列（`ITEM_MODIFYCHARM`），运气修正取第 47 列（`ITEM_MODIFYLUCK`），加在 `charm`、`luck` 的基础值上，夹在 5.8 所述的范围内（STAGE-61）；地水火风一期不受装备影响，数据里没有可靠的列。
- 角色已死亡（`hp` 为 0）回复 `CHARACTER_DEAD`。观察方发起时回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同 `warp`；战斗中回复 `IN_BATTLE`（战斗中的限制见第 10 章；战斗还没做，服务端暂时不会返回这个错误）。
- 错误码：`BAD_REQUEST`、`EMPTY_SLOT`、`NOT_EQUIPMENT`、`REQUIREMENT_NOT_MET`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.26 `item_unequip`：卸下装备（操作类，世界中）

```jsonc
{"id": 26, "type": "item_unequip", "data": {"slot": 2}}
{"id": 26, "type": "item_unequip", "ok": true, "data": {}}
```

- `slot` 是装备格（0–4）。缺少，或不是 0–19 的整数，回复 `BAD_REQUEST`；背包格（5–19）回复 `BAD_REQUEST`；格子是空的回复 `EMPTY_SLOT`。
- 装备放进**第一个空的背包格**（5–19）。背包 15 格都有东西时回复 `BAG_FULL`，什么都不改。
- 推送、死亡、观察方、战斗中的规则与 5.25 相同。
- 错误码：`BAD_REQUEST`、`EMPTY_SLOT`、`BAG_FULL`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.27 `item_move`：移动或互换物品（操作类，世界中）

```jsonc
{"id": 27, "type": "item_move", "data": {"from": 5, "to": 6}}
{"id": 27, "type": "item_move", "ok": true, "data": {}}
```

- `from` 和 `to` 都是 0–19 的整数，缺少或类型不对回复 `BAD_REQUEST`；两者相同回复 `BAD_REQUEST`；`from` 是空格回复 `EMPTY_SLOT`。
- **背包格之间**：目标为空就移过去，有东西就互换。
- **背包格放进装备格**：按 5.25 检查（数量、类型、部位、等级）；目标部位已有装备时互换。
- **装备格放到背包格**：按 5.26 卸下；目标背包格有东西时互换，而且背包里的那件要能穿在 `from` 这个装备格上，不能穿时回复 `NOT_EQUIPMENT`。
- **装备格之间不能直接换**（先卸下再穿），回复 `BAD_REQUEST`。
- 整条命令成败，推送与观察方、死亡、战斗中的规则与 5.25 相同。
- 错误码：`BAD_REQUEST`、`EMPTY_SLOT`、`NOT_EQUIPMENT`、`REQUIREMENT_NOT_MET`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.28 `item_drop`：丢弃物品（操作类，世界中）

```jsonc
{"id": 28, "type": "item_drop", "data": {"slot": 6, "count": 1, "confirm": "yes"}}
{"id": 28, "type": "item_drop", "ok": true, "data": {}}
```

- `slot` 是 0–19 的格子。`count` 可以省略，省略时丢弃整格；给出时必须是正整数，且不能超过这格的数量，否则回复 `BAD_REQUEST`。
- `confirm` 必须是字符串 `"yes"`，缺少或是别的值回复 `BAD_REQUEST`，说明里写明“丢弃要确认”。客户端在命令后面带上 `yes` 才会发出这条请求（`item drop 6 yes`）。
- **丢弃就是销毁**：新服务端没有局部地图，物品不会掉在地上，不可恢复。丢弃穿着的装备时，攻击等数值变化，推送规则同 5.25。
- 这格是空的回复 `EMPTY_SLOT`。
- 推送与观察方、死亡、战斗中的规则与 5.25 相同。
- 错误码：`BAD_REQUEST`、`EMPTY_SLOT`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.29 `encounter`：遇敌开关（操作类，世界中，STAGE-40）

```jsonc
{"id": 29, "type": "encounter", "data": {"set": "on"}}
{"id": 29, "type": "encounter", "ok": true, "data": {"encounter": true}}
```

- `set` 取 `on` 或 `off`，可以省略；省略时查询当前状态，只回 `{"encounter": true/false}`。
- **`on`**：角色所在位置（地图 + 坐标）必须落在旧版 `encount.txt` 的一个能抽出敌人的遇敌区域里。不在（比如村子里）回复 `NO_ENEMY_HERE`，说明文字“这里没有敌人，遇敌开关没有打开”，开关保持关闭，什么都不推送。在的话马上开第一场战斗：先推 `battle_start`、`battle_turn`（第 10 章），再推 `encounter_changed`，最后回响应。已经在战斗中时只打开开关（这一场结束后按开关决定要不要开下一场），不立刻开战。
- **一场接一场**：开关打开期间，每场 `battle_end` 之后隔一段时间（默认 2 秒，服务端配置可调，见第 7 节）自动开下一场；敌人的组成按遇敌表抽取（旧版 `gmsv/char/enemy.c` 的 `ENEMY_getEnemy`：按区域权重抽敌群、按敌群权重和等级范围生成敌人，最多人数按区域上限）。
- **`off`**：战斗中也可以关；当前这场照常打完，之后不再开新的一场。
- **自动关闭**（服务端推 `encounter_changed` 带原因，并给一条 `notice` 说明）：打输且**被打飞**（`lose`，人物被送回记录点或出生地，见 10.9）；`warp` 传送走（传送在战斗中被拒，所以发生在两场之间的空档）；**控制权转交**（`control`，第 9.3 节：收回、交还、agent 接管、原操控方离开）——开关跟着控制方走，当前这一场照常打完，之后不再开新的一场。打输但**被打死**（人物原地 1 血复活，位置不变）时开关保持开着，隔一段时间开下一场，和赢了、逃跑一样（STAGE-194）。离开世界或掉线时开关随角色一起关掉（连接已经没了，不推送）。逃跑成功不关，继续打下一场。
- 开关状态可以在 `status` 和 `enter_world` 的响应里看到（`encounter` 字段，boolean），不额外发请求。
- 属于“操作”类：观察方发起回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`（第 9.3 节）。
- 错误码：`BAD_REQUEST`、`NO_ENEMY_HERE`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。

### 5.30 推送 `encounter_changed`：遇敌开关变了（STAGE-40）

```jsonc
{"type": "encounter_changed", "data": {"encounter": false, "reason": "warp"}}
```

- `encounter` 是开关的新状态；手动开关时不带 `reason`，自动关闭时带：`lose` 打输且被打飞传走（只有这种情况；原地复活不关）、`warp` 传送走、`no_enemy` 开下一场时这里已经没有敌人（位置被被打飞的回城改掉之外的兜底）、`control` 控制权转交（见 9.3；开关跟着控制方走，当前这一场照常打完）。
- 自动关闭的同时给一条 `notice`（5.11）说明原因；手动开关只有本推送。
- 与其他推送一样，同时发给该角色的玩家连接和 agent 连接；`encounter on` 触发时排在 `battle_start`、`battle_turn` 之后。

### 5.31 `item_use`：使用消耗品（操作类，世界中，STAGE-30）

```jsonc
{"id": 31, "type": "item_use", "data": {"slot": 6, "target": 1}}
{"id": 31, "type": "item_use", "ok": true, "data": {"target": "player", "name": "勇者小明", "hp": 43, "delta": 34}}
```

- 使用背包里的消耗品。一期支持回复 HP（物品表参数“体N”）、回复 MP（“气N”）、增加宠物忠诚（“忠N”，STAGE-326，见下面的“忠诚”）、
  解除异常状态（“毒”“麻”等，只在战斗中有效，见 10.5）；“全”是 HP 回满加 MP 回满（或解掉全部异常）；
  英雄的祝福（18543，STAGE-334）给自己加一点未分配能力点（见下面的“英雄的祝福”）；
  龙王的守护（18544，STAGE-340）使用后一段时间不遇敌（见下面的“龙王的守护”）。
  复活类道具（复活药 1530–1532、回魂之玉 1839、萨顿家的秘药 2450、真诚的眼泪 1815、铁壁之石 1821）物品表是“只能在战斗中用”（`AbleUse=1`），战斗外使用同样回复 `ITEM_UNUSABLE`，不消耗（STAGE-262，见 10.5）；传送、投掷毒药等其他效果另立需求，回复 `ITEM_UNUSABLE`。
- `slot` 是背包格（5–19），装备格（0–4）回复 `BAD_REQUEST`（穿着的请用 `item_equip`）；
  空格回复 `EMPTY_SLOT`。
- `target` 可以省略，省略时是自己。`0` 是自己，`1–20` 是宠物栏位 + 1（`target` 为 N 就是 5.19 的 `slot` N−1；STAGE-346 起 20 格，原版和一期早先的 `1–5` 是 0–4 号栏位），一期没有队友，不再有 6–10 的队友编号
  （超出 0–20 回复 `INVALID_TARGET`）。物品表的目标字段（`ITEM_TARGET`）为“自己”的物品
  选了别的目标时也回复 `INVALID_TARGET`；“他人”的物品也可以对自己使用。
- **回复量**：`RAND(0.9N, 1.1N)`（N 是物品表的使用参数），HP 再乘恢复率（人物 1+0.0001×体力、
  宠物 1+0.00005×体力，原版 `GetRecoveryRate`），MP 不乘；最后按上限截断（气力上限 100）。
  `delta` 是血量增量，`hp` 是回复后的血量；有气力变化时附带 `mp` 与 `mp_delta`。
  “全”回复全部血量；物品表没有使用函数或效果不支持时回复 `ITEM_UNUSABLE`。
  没有可回复的内容时同样回复 `ITEM_UNUSABLE`，道具不扣、状态不变（STAGE-179）：对自己，血量和气力都满（回血的物品只看血量，回气的只看气力）；对宠物，血量已满。
- **可使用场合**：物品表的 `ITEM_ABLEUSEFIELD` 为“战斗中”的物品在世界中使用回复
  `ITEM_UNUSABLE`，说明里写明“只能在战斗中使用”。目标宠物已经倒下（血量 0）回复
  `INVALID_TARGET`，数量不变。
- **忠诚**（STAGE-326，原版 `item_event.c` 的 `ITEM_useRecovery_Field` 的忠诚分支）：参数“忠N”（安抚之素 1598 为忠30，高级宠物食品 12763 为忠10）只给宠物用，`target` 必须是 1–20（宠物栏位 + 1），选自己（0 或省略）回复 `INVALID_TARGET`。增量是 N 点整（原版取 RAND(0.9N, 1.1N)，这里按需求固定为 N），写进宠物的可变忠诚，封顶 100（宠物页显示的忠诚）。忠诚已经是 100 回复 `ITEM_UNUSABLE`，道具不扣、宠物不变。成功时 `data` 带 `loyalty`（回复后的忠诚）和 `loyalty_delta`（增加了多少），推 `pets_changed` 与 `inventory_changed`，不推 `status_changed`。目标宠物倒下同样回复 `INVALID_TARGET`。“忠N”只在可用场合为“战斗外”的物品上生效，战斗中用回复 `ITEM_UNUSABLE`（原版战斗里不认“忠”）。
- **英雄的祝福**（STAGE-334，物品表使用函数 `ITEM_useSkup`，原版 `item_event.c` 的 `ITEM_useSkup`）：战斗外给自己使用，未分配的能力点 +1，扣 1 个物品；原版没有别的限制，血量和气力满了也能用。`target` 省略或 0 是自己，选 1–20 回复 `INVALID_TARGET`（说明“只能对自己使用”），道具不扣。成功时 `data` 为 `{"target": "player", "name": ..., "skill_points": 回复后的未分配点数, "skill_points_delta": 1}`，推 `status_changed`（未分配点数在 status 里）与 `inventory_changed`，点数立即写库。战斗中回复 `IN_BATTLE`（同上）。
- **龙王的守护**（STAGE-340，物品表使用函数 `ITEM_useNoenemy`，原版 `item_event.c` 的 `ITEM_useNoenemy` 与 `net.c` 的 `setNoenemy`）：战斗外给自己使用，扣 1 个物品，推 `notice`（`info`，“你感受到周边的杀气消失了。”）。之后 5 分钟内不主动遇敌：`encounter` 打开遇敌开关、`encounter_start`（5.50）、1 级宠物目录开战（传送模式不算）都回复 `NO_ENEMY_HERE`（说明“龙王的守护还在生效，周边没有敌人”），开关和位置都不动。遇敌开关已经开着时，守护生效后的下一场不开，开关按没有敌人自动关掉（`encounter_changed` 的 `reason` 为 `no_enemy`）。`target` 省略或 0 是自己，选 1–20 回复 `INVALID_TARGET`；成功时 `data` 为 `{"target": "player", "name": ...}`，推 `inventory_changed`。持续时间取 5 分钟（原版计数是 6 次 60 秒，实际 300–360 秒，取决于使用时间在计数格点上的位置）；重复使用重新计时，不叠加；守护只在内存里，角色离开世界即清零（原版挂在连接上，掉线清零）。到期不推提示。除剂 1844、神签 2615（大吉）的使用函数也是 `ITEM_useNoenemy`，同样生效。
- **未实现的使用函数**（STAGE-340 盘点，只列不做）：物品表 `ITEM_USEFUNC` 有值、`UseEffect` 不认的（括号内为道具数）：`ITEM_useStatusChange`（52，战斗中的乱、催眠等状态）、`ITEM_useWarp`（34）、`ITEM_useCaptureUp`（26）、`ITEM_useOtherEditBase`（20）、`ITEM_useMaxRedSocksNew`（7）、`ITEM_petFollow`（7）、`ITEM_useWarpForNum`（5）、`ITEM_metamo`（4）、`ITEM_useSkillCanned`（4）、`ITEM_useFieldChange`（4）、`ITEM_Constitution`（4）、`ITEM_useMic`（3）、`ITEM_Addexp`（3）、`ITEM_useRenameItem`（3）、`ITEM_ChikulaStone`（2）、`ITEM_useImprecate`（2）、`ITEM_Refresh`（2）、`ITEM_useEncounter`（2，炎之圣石 18558、神签 2620）、`ITEM_useMaxRedSocks`（1）、`ITEM_ThrowItemBox`（1）、`ITEM_LoverSelectUser`（1）、`ITEM_WaterWordStatus`（1）、`ITEM_useDeathcounter`（1）、`ITEM_useEffectTohelos`（1）。已实现的 `ITEM_useRecovery` 里参数只有“魅”“GETEXP”“UPUPLV”的道具也不能用（`UseEffect` 取不到回复量就回复 `ITEM_UNUSABLE`）。炎之圣石是四圣石任务里交给考古队员换物，不靠使用。
- 使用成功后服务端推送：目标自己的 HP/MP 变了，或加了未分配点（英雄的祝福）推 `status_changed`；宠物血量变了推
  `pets_changed`（血量照原版立即写库）；数量减 1 推 `inventory_changed`，用完时该格消失。
- 属于“操作”类：观察方发起回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`（第 9.3 节）；
  死亡时回复 `CHARACTER_DEAD`；战斗中回复 `IN_BATTLE`（战斗中使用走 10.5 的 `item` 指令）。
- 错误码：`BAD_REQUEST`、`EMPTY_SLOT`、`ITEM_UNUSABLE`、`INVALID_TARGET`、`CHARACTER_DEAD`、
  `CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.32 `npcs`：查看本地图 NPC（查看类，世界中，STAGE-68）

```jsonc
{"id": 32, "type": "npcs", "data": {}}
{"id": 32, "type": "npcs", "ok": true, "data": {"npcs": [
  {"id": 225, "name": "村长家的厨师", "type": "对话", "x": 16, "y": 16},
  {"id": 198, "name": "特产品贩卖员", "type": "商店", "x": 64, "y": 112}
]}}
```

- 返回角色**当前所在地图**上的全部 NPC（`server/gamedata/npcs.tsv`，由 `tools/gen-npcs.py`
  从原版 NPC 数据生成），按 `id` 升序。没有 NPC 的地图返回空数组 `"npcs": []`，不报错。
- `id` 是 NPC 的编号（全表唯一，与地图无关），以后的 NPC 交互命令（对话、买卖等）沿用；
  `name` 是名字；`type` 是显示用的类型名（本期有对话、告示牌、商店、护士、存档点、宠物店、宠物技能店、
  传送员、骑乘训练师、猜谜（认定场的猜谜大师，STAGE-332）；任务、事件、赌博等类型以后增量收录）；`x`、`y` 是格子坐标（出生区域的中心，
  本期没有走路，只用来展示和以后做地图用，不要求站在旁边）。
- 玩家看不到的 NPC（隐藏的事件触发点、地图出入口 `npcgen_warp` 等）不在数据表里，不进列表。
- 玩家和 agent 都可以发起（查看类）；agent 操控、玩家观察时观察方也能查看。
- 客户端进入世界时取一次，之后每次换地图（`position_changed` 里 floor 变了）重新取一次；
  同一张地图内位置变化不用重取。
- 例外（STAGE-93）：任务脚本的 `NpcWarp` 会把 NPC 移到别的地图或坐标。移动后服务端给这张地图（原来的和新的）
  上的连接推送 `npcs_changed`（见 5.46），内容就是新的完整列表，客户端用它替换，不用重取。

### 5.33 `talk`：和 NPC 对话（操作类，世界中，STAGE-69）

```jsonc
{"id": 10, "type": "talk", "data": {"npc": 225}}
{"id": 10, "type": "talk", "ok": true, "data": {}}
```

- 发起对话后，服务端推送一个对话窗口 `npc_window`（见 5.34）；玩家和 agent 都可以发起
  （操作类：agent 操控、玩家观察时观察方被拒）。
- `npc` 是 5.32 列表里的 NPC 编号。编号不存在回复 `NPC_NOT_FOUND`；编号存在但不在
  角色**当前所在地图**上回复 `NPC_NOT_HERE`（本期没有走路，编号照着地图页的
  「本地图 NPC」表输入，输错地图是最常见的错）。
- 本期支持对话的类型和窗口正文（文字与原版参数文件一致，多句时随机取一句，原版
  `npc_townpeople.c` 的行为）：
  - **对话（`npcgen_man`，村民）**：原版参数里的多句话随机一句；
  - **告示牌（`npcgen_signboard`）**：固定一段文字，原文里的 `\n`（字面两个字符）转成
    真换行，客户端按多行显示；
  - **报时（`npcgen_timeman`）**：按原版 `npc_timeman.c` 的 `time`/`main_msg`/
    `change_msg` 选择台词；时段内从 `main_msg` 取一句，时段外从 `change_msg` 取一句。
    游戏时间由服务端当前时间按原版服务端 `handletime.c` 的映射换算（5400 秒为一天，分为
    1024 个时刻）；只有一组台词时全天使用这一组，避免缺少 `change_msg` 时无话可说；
  - **摆设（`npcgen_mugon`）**：没有参数，固定回「……」；
  - **任务（`changeevent`，STAGE-90）**：原版任务脚本（`data/npc` 里的事件文件），由 `tools/gen-npcs.py`
    转成 `server/gamedata/npc-event.json`。只有放开的脚本（`server/gamedata/npcevent.go` 的白名单，
    第一批是「成人仪式」）才能对话，其余回复 `NPC_NOT_TALKABLE`；规则和分批见 `docs/quest.md`。
    脚本里还有没完成的暗号块时，闲聊或台词窗口上多一个选项 `1 说暗号`（STAGE-92，见 5.35、5.37）。
  - **战斗（`npcenemy`，STAGE-96）**：原版战斗 NPC（参数见 `server/gamedata/npc-enemy.json`，由 `tools/gen-npcs.py` 生成）。
    只有放开的参数（`server/gamedata/npcenemy.go` 的白名单，第一批只有里昂蛙王）才能打，其余回复 `NPC_NOT_TALKABLE`。
    前置道具（参数的 `items`）不全，或者 NPC 正被别的战斗占用（`one_battle`）时，推一个**没有选项**的窗口，
    正文是原版的拒绝或占用台词；都通过时推确认窗口，正文是原版的确认台词（多行按换行连起来），选项 `1 是`、`2 否`（5.35）。
    选“是”开战，推送见 10.3。道场弟子和分路传送的 NPC 本期标不支持，一律回复 `NPC_NOT_TALKABLE`。
- **猜谜（`npcgen_quiz`，STAGE-332，原版 `npc_quiz.c`）**：本期只有认定场 812 的猜谜大师（`npc` 1814，坐标 28,14）能说话，
    其余猜谜人物没有收录（`npcs` 列表里没有，编号也就不存在）。离它超过一格时服务端先走过去（同 5.33 的宠物技能店，`talkReach`）。
    流程：组队回复台词「团队无法参加喔！」；任务 2「猜谜3兄弟」（事件 44）没完成推一句台词「还没有完成猜谜三兄弟的课题，先去找他们办完吧。」
    （本单补的，原版没有这一句）；都满足时人类连接推开始窗口（选项 `1 是`、`2 否`，见 5.35）。选「是」：没有猜谜专家的会员证 2753 推原版
    `NoEntryMsg` 台词；同一位人物已有 8 人在答时推等待台词；都通过就交出 2753，按题库逐题出题：选择题用 `npc_choose`（选项序号），
    文字题用 `npc_reply`（窗口 `accepts_text`，见 5.34、5.37，判题照原版：文字包含正确答案、区分大小写）。
    每答完一题，窗口先推 `npc_window_closed`（`CHOSEN`）再推下一题；答满 100 题后按全对给谜之箱子 2759（推 `inventory_changed` 和
    「拿到谜之箱子。」），背包放不下时消息栏「道具栏已满」、座位保留（原版同样）；最后推认定台词（消息栏 notice）并释放座位。
    窗口关掉、换图、战斗开始时，这一场的座位释放（`npc_close`、`MOVED`、`BATTLE` 的 `npc_window_closed`）；选项引发的换题不释放。
    - **agent 直接认定（方案 A，`agent` 连接，STAGE-332）**：agent 连接对猜谜大师 `talk` 时不出题、不开窗口：没有入门证 2753 推 `NoEntryMsg`
      窗口（没有选项）；已经持有谜之箱 2759 时只在消息栏说认定台词（不再扣证、不再发，幂等）；否则扣 2753、发 2759，推
      `inventory_changed`，消息栏依次是「拿到谜之箱子。」和认定台词。原版大师本身不查宠物等级（77 级的检查在伊那多那里），这里也不查。
      之后仍由大师的弟子（1227）收箱子、给左迪洛斯、完成任务 3（`docs/quest.md`）。
- 商店 NPC（`npcgen_shop` / `npcgen_limitshop`）走买卖菜单，见 5.38；护士
  （`npcgen_winhealer`）走治疗确认窗口，见 5.39；美容师（`npcgen_charm`）走三步窗口，见 5.64；宠物技能店（`npcgen_petskillshop`）走技能列表，见 5.51；
  传送员（`npcgen_warpman`）走传送窗口：免费或付费都弹窗；免费条件不满足又不能付费、或依赖庄园等本期不支持的条件时回复 `NPC_NOT_TALKABLE`，不弹窗，也不关掉玩家已开着的窗口（STAGE-177）。
  其余还没有对话逻辑的类型同样回复 `NPC_NOT_TALKABLE`，不弹窗；以后的功能需求逐类接入。
- 已经开着窗口时再 `talk` 另一个 NPC：直接替换（关旧开新），不要求先关闭。
- **同图超出对话距离时先走过去**（STAGE-263）：有的 NPC 要求角色离它不超过一定距离（目前只有宠物技能店，2 格，
  切比雪夫距离；登记在 `server/character/talk_approach.go` 的 `talkReach`）。角色和 NPC 在同一张图上、但超出这个距离时，
  服务端先把角色移到 NPC 相邻的空格（离角色最近的一格；不能是 NPC 所在格、普通出口的触发格、地图外，或同图其他角色占着的格），
  推送 `position_changed`（5.14，先于 `npc_window`），再照常弹窗。这一步等价于原版客户端自己走过去。找不到空格时角色不动，
  回复该 NPC 原来的距离错误（宠物技能店为 `ACTION_NOT_ALLOWED`，“离超级饲育员太远了，走近一点再说”）。
  不同图的 NPC 不走，仍照上面回复 `NPC_NOT_HERE`（本期没有走路，要先传送）。`npc_choose` 的距离检查照旧，不移动角色。
  agent 也会收到这条位置推送，对话之后位置可能变了。
- 战斗中回复 `IN_BATTLE`。

### 5.34 推送 `npc_window` / `npc_window_closed`：服务端驱动的对话窗口（STAGE-69）

```jsonc
{"type": "npc_window", "data": {
  "npc": 225, "name": "村长家的厨师", "type": "对话",
  "text": "今天的特餐是乌力烤肉。",
  "options": []
}}
{"type": "npc_window_closed", "data": {"reason": "MOVED"}}
```

- `npc_window` 在 `talk` 成功后推给这个角色的**所有连接**（玩家和 agent 看到同一个窗口）。
  `name`、`type` 是 NPC 的名字和显示类型；`text` 是正文（可含换行）；`options` 是选项
  列表，按 `no`（从 1 开始）排列。普通对话没有选项（空数组）；商店的买卖菜单（5.38）、
  护士的确认（5.39）和任务 NPC 的“是/否”问答（5.35，STAGE-90）带选项。
  `accepts_text` 为 `true` 时窗口接受文字回复（`npc_reply`，5.37），只出现在任务 NPC 的暗号输入窗口和猜谜的文字题上，
  其余窗口没有这个字段。
  决斗强者表的排行窗口（5.75）多一个字段 `ranking`（排行数据），其余窗口没有这个字段。
- 窗口是**服务端的状态**：每个角色同一时间最多开一个窗口，服务端记住它是谁、有哪些
  选项；`npc_choose`、`npc_close` 都对着它操作。
- `npc_window_closed` 通知所有连接窗口关了，`reason` 取值：
  - `CHOSEN`：选项已被选择（见 5.35），或者暗号窗口收到了文字回复（见 5.37）；
  - `CLOSED`：某条连接发 `npc_close` 主动关闭（见 5.36）；
  - `MOVED`：角色被传送（`warp`）或以其他方式换了地图，窗口自动关闭；窗口上的 NPC 被任务脚本移到了别的地图，
    同样关闭（STAGE-93，见 5.46；触发移动的那个连接自己开着的窗口不关）；
  - `BATTLE`：战斗开始，窗口给战场让路。
- 角色下线（主动离开、被踢、断线）不推 `npc_window_closed`：窗口状态随角色一起销毁，
  连接自己已经在处理下线。

### 5.35 `npc_choose`：回应对话窗口（操作类，世界中，STAGE-69）

```jsonc
{"id": 11, "type": "npc_choose", "data": {"no": 1}}
{"id": 11, "type": "npc_choose", "ok": true, "data": {}}
```

- 选择当前窗口里的一个选项。没有打开的窗口回复 `WRONG_STATE`；窗口没有选项，或者
  `no` 不在选项里，回复 `NPC_BAD_CHOICE`。
- 选中有选项窗口的选项后，服务端关掉窗口并推 `npc_window_closed`（`reason=CHOSEN`）；
  选项引发的后续窗口（商店清单、确认页等）由各自的功能再推新的 `npc_window`。
- 战斗 NPC 的确认窗口（STAGE-96，选项同任务 NPC 的是/否）：选 `2` 关窗；选 `1` 先推 `npc_window_closed`（`CHOSEN`），
  再重新检查一次前置道具和占用（窗口开着的时候道具可能丢了，或者别人先开战了）：通过就推 `battle_start`（10.3，开战），
  不通过推对应的台词窗口（没有选项，玩家用 `npc_close` 关掉）。
- 玩家和 agent 都可以发起（操作类：观察方被拒）。

- 任务 NPC 的问答窗口（STAGE-90，原版 `npc_exchangeman.c`）：选项 `1` 是、`2` 否，其余编号回
  `NPC_BAD_CHOICE`（窗口保持不关）。选中后服务端先推 `npc_window_closed`（`CHOSEN`），再按这一步的结果推
  `inventory_changed`、`status_changed`（金币或魅力变了）、`quests_changed`（任务进度变了），最后推下一个
  `npc_window`（没有下一句话就不推）。选前服务端按条件重新选块，块已经不是开窗时的那一块（条件变了）就只关窗。
  领物品、交物品的交易要么全做、要么全不做：背包空格或金币不够时只推对应的台词（`ItemFullMsg` 等），不改数据。
  任务也可能收走或送出宠物（STAGE-197，`docs/quest.md`「宠物」）：交易成功后推 `pets_changed`（5.20），收走、送出的宠物各有一条
  `notice`（“交出XX。”“拿到XX。”）；宠物收不成（数量不够、战斗中）时什么都不推，宠物栏满时只推 `PetFullMsg` 台词，两种都不改数据。
- 任务 NPC 的“说暗号”（STAGE-92，原版 `npc_exchangeman.c` 的 `KeyWord`）：闲聊或台词窗口上只有一个选项 `1 说暗号`
  （其余编号回 `NPC_BAD_CHOICE`）。选中后服务端先推 `npc_window_closed`（`CHOSEN`），再推一个文字输入窗口
  （`accepts_text: true`、`options: []`、正文「你想说什么？」），玩家用 `npc_reply`（5.37）说出话来。
  问答窗口（是/否）上没有“说暗号”。
- 任务 NPC 的分页台词（STAGE-97，`docs/quest.md`「分页台词」）：窗口的正文是当前页，选项随页变化。问话（ACCEPT、REQUEST）
  非最后一页是 `1 下一页`（第 2 页起加 `2 上一页`），最后一页是 `1 是`、`2 否`（第 2 页起加 `3 上一页`）。读完的台词（感谢话、
  MESSAGE 台词）没有“是/否”：非最后一页是 `1 下一页`（第 2 页起加 `2 上一页`），最后一页只有 `1 上一页`（第 2 页起），脚本还有没完成的
  暗号块时最后一页上是 `1 说暗号`（第 2 页起在它后面加 `上一页`）。翻页时服务端先推 `npc_window_closed`（`CHOSEN`），再推新的
  `npc_window`（正文是新的一页）。问话窗口翻页前按条件重新选块，块变了就只关窗；读完的台词翻页不选块、不改数据。

### 5.36 `npc_close`：关闭对话窗口（确认类，世界中，STAGE-69）

```jsonc
{"id": 12, "type": "npc_close", "data": {}}
{"id": 12, "type": "npc_close", "ok": true, "data": {}}
```

- 客户端按 `Esc` 或以其他方式关掉窗口时发它，服务端清掉窗口状态并给所有连接推
  `npc_window_closed`（`reason=CLOSED`）。没有打开的窗口时也回复成功（幂等，不报错）：
  两条连接各自都能关，先到先关。
- 归为**确认类**而不是操作类：关窗口不改变角色数据，观察方也可以关（和 `battle_ack`
  一样），玩家看着 agent 对话时随时可以关掉窗口。

### 5.37 `npc_reply`：窗口的文字回复（操作类，世界中，STAGE-70）

```jsonc
{"id": 13, "type": "npc_reply", "data": {"text": "3"}}
{"id": 13, "type": "npc_reply", "ok": true, "data": {}}
```

- 对当前打开的窗口输入一段文字（商店买东西时报数量等）。没有打开的窗口回复
  `WRONG_STATE`；窗口不接受文字回复（普通对话）回复 `BAD_REQUEST`。空白（只有空格或换行）的文字回复 `BAD_REQUEST`。
- 文字内容按窗口的功能解释（见 5.38 的买流程）；解释不了（不是数字、不是正数）按
  各自的错误返回，窗口不动。
- **任务 NPC 的暗号窗口**（`accepts_text`，STAGE-92）：文字是玩家说的暗号，服务端只去掉首尾的换行，**不去空格、
  区分大小写和全半角**，和脚本里的暗号逐字比对。服务端先推 `npc_window_closed`（`CHOSEN`），再按这句话选块：
  - 命中暗号块：和普通块一样执行（交易、问答），推送与 5.35 的问答相同（`inventory_changed`、`status_changed`、
    `quests_changed`，再推下一个 `npc_window`）；
  - 没有块命中（包括说错了）：推一个闲聊窗口，选项仍是 `1 说暗号`，不改背包、金币和任务进度；
  - 不支持的块命中：和普通对话一样拒绝（只关窗、不改数据）。
- 玩家和 agent 都可以发起（操作类：观察方被拒）。客户端文字命令是 `reply 文本`（文字原样发送，不去掉空格）。

### 5.38 商店：和商店 NPC 买卖（STAGE-70）

和商店 NPC（5.32 的 `type` 是「商店」，原版 `npcgen_shop` / `npcgen_limitshop`）`talk`
不弹普通的对话窗口，而是一个**买卖菜单**（`npc_window`，正文是店里的欢迎语
`main_msg`，选项按这家店的参数生成）：

```jsonc
{"type": "npc_window", "data": {"npc": 198, "name": "特产品贩卖员", "type": "商店",
  "text": "欢迎光临！", "options": [{"no": 1, "label": "买"}, {"no": 2, "label": "卖"}]}}
```

- **买**：选「买」后服务端推商品列表（`buy_main` 的引导语；每个选项是一件商品，
  `label` 是「名字（单价）」，编号从 1 开始，最多 60 件（STAGE-302 由原版 `MAXSHOPITEM=33` 提高，给后续留余量；
  件数超过一屏时列表由客户端滚动，不截断）。
  选一件商品后服务端推数量询问（`what_msg` 加「名字：单价」），玩家用 `npc_reply`
  报数量，成交后窗口随 `npc_window_closed`（`CHOSEN`）关闭。
- **卖**：选「卖」后服务端推回收列表（`sell_main` 的引导语；每个选项是背包 5–19 格里
  回收价大于 0 的一格，`label` 是「名字（回收 金币）」，选项编号就是格子号）。
  选一格当场成交（原版就是选完即卖），窗口关闭。
- **覆盖买价**（STAGE-326）：商店参数里的 `prices`（物品编号字符串 → 买价）覆盖这件商品的单价，不乘 `buy_rate`；没写的商品仍按基价 × `buy_rate`。安抚之素（1598）在 NPC 830（ＳＢＣ的药师）和 884（ＪＢＡ的药剂师）的买价是 10000 石币。卖回价不变：基价 × `sell_rate`，安抚之素基价 500 × 0.2 = 100 石币，与买价覆盖无关。
- **价格**：单价 = 物品基价 × 这家店自己的 `buy_rate`（回收 = 基价 × `sell_rate`，
  高价回收列表里的物品按 `special_rate`），向下取整；倍率逐店不同，不是全服统一的数。
  参数来自 `server/gamedata/npc-shop.json`（`tools/gen-npcs.py` 从原版商店参数文件生成），
  台词（欢迎光临、`what_msg`、`level_msg`、`itemfull_msg`、`stone_msg` 等）也逐字取自它。
- **买入的检查**，按顺序：等级不够（物品的穿戴等级高于角色）回复
  `REQUIREMENT_NOT_MET`，说明是店里的 `level_msg` 原话；金币不够回复 `NOT_ENOUGH_GOLD`；
  背包（5–19 格）一个空位都没有回复 `BAG_FULL`，说明是 `itemfull_msg` 原话，在选数量那一步就回复，不进入确认。
  空位比数量少时按原版截断（买得下几个是几个，STAGE-192）：**报价时**就把 `count` 封顶为空格数，
  `trade` 的 `count`、`total` 是截断后的数，窗口正文前面写明“背包只剩 N 格，按空位买 N 件”；
  确认时背包没变（报价快照），不会再截断。每格放一件（`count=1`）。
- **卖出的检查**：卖出后金币不能超过携带上限 10,000,000（原版 `CHAR_MAXGOLDHAVE`），
  超过回复 `GOLD_FULL`，说明是 `stone_msg` 原话，物品不变。整格卖出（`count` 件一起，
  回收价按件数算）。
- 买卖成交都推 `inventory_changed` 加 `status_changed`（金币和背包立刻刷新），
  然后推 `npc_window_closed`（`CHOSEN`）。任何一步被拒都只回错误，数据不变、窗口不动。
- 战斗中回复 `IN_BATTLE`；观察方被拒（操作类）。
- 窗口开着时 `talk` 别的 NPC 直接换窗口（关旧开新，同 5.33）。
- 限量店（`npcgen_limitshop`）本期与普通商店同规则：按 `sell_rate` 收购背包里基价大于
  0 的物品，不实现原版 `LimitItemType` / `LimitItemNo` 的收购范围限制。

### 5.39 护士：付钱治疗（STAGE-71）

和护士 NPC（5.32 的 `type` 是「护士」，原版 `npcgen_winhealer`）`talk` 弹治疗确认窗口
（`npc_window`，正文按状态生成，文案取自原版 `npc_windowhealer.c`）：

```jsonc
{"type": "npc_window", "data": {"npc": 54, "name": "监狱医生", "type": "护士",
  "text": "＜耐久力气力回复＞\n\n是要回复耐久力气力没错吧！\n\n收您 100 的 STONE。",
  "options": [{"no": 1, "label": "治疗"}]}}
```

- **治疗内容**：人物 HP、MP 回满，并回满背包里全部宠物的 HP（原版
  `NPC_WindowHealerAllHeal`）。异常状态（中毒、麻痹）新服务端还没有实现，做了自然会解除。
- **费用**：只有不满的那项收费——HP 费 = 角色等级 × 这家店的 `hp_rate`、MP 费 =
  等级 × `mp_rate`（向下取整，至少 1）；宠物回复免费（原版「宠物回复(免费)」）。
  人物全满且宠物全满时没有「治疗」选项，正文是「似乎没有必要回复唷！」；只有宠物受伤时
  也不报价（费用 0，选项仍在）。参数来自 `server/gamedata/npc-healer.json`
  （`tools/gen-npcs.py` 从创建块的 `enemy=npcgen_winhealer|level|hp_rate|mp_rate|range` 生成）。
- **免费线**：角色等级低于护士参数的 `level` 时治疗免费，正文是「现在的等级可以免费帮你
  回复唷！」（新手村里的护士大多免费，原版如此）。
- 选「治疗」（`npc_choose` 1）成功后推 `status_changed`、`pets_changed`，窗口随
  `npc_window_closed`（`CHOSEN`）关闭；金币不够回复 `NOT_ENOUGH_GOLD`（原版
  「真可惜似乎所带的金钱不够唷！」），没有可治的回复 `ACTION_NOT_ALLOWED`——两者都不扣钱，
  窗口留着。其余选项编号回复 `NPC_BAD_CHOICE`。
- 战斗中回复 `IN_BATTLE`；观察方被拒（操作类）。

### 5.40 存档点：记录战败回城点（STAGE-72）

和存档点 NPC（5.32 的 `type` 是「存档点」，原版 `npcgen_savepoint`）`talk` 弹记录窗口
（正文是参数文件里的 `Request` 文案，比如「给我一个凯比特的肉我就帮你记录。」）：

```jsonc
{"type": "npc_window", "data": {"npc": 205, "name": "萨姆吉尔的储存点", "type": "存档点",
  "text": "给我一个凯比特的肉\n我就帮你记录。…", "options": [{"no": 1, "label": "记录"}]}}
```

- 选「记录」（`npc_choose` 1）时，服务端按参数里的 `GetItem` 找背包里一组齐全的物品
  （候选之间任一组即可；同一组里 `&` 连着的几件都要有、各占一格、数量够）：找到就把这组
  物品收走、把存档点记在角色上并推 `inventory_changed`，窗口换成成功话（收了物品时
  `Really` 在前、接 `Ok`，无选项，按数字或 `Esc` 关）；没有合格物品回复
  `ACTION_NOT_ALLOWED`（正文是 `Request` 原话），不记录、不消耗。参数里没有 `GetItem`
  的存档点不要东西，选「记录」直接记录（只推 `Ok` 窗口，不推 `inventory_changed`）。
- 收物品时整格删行；一格的数量比要收的多，就只减去要收的件数（`count` 写库）。
- 存档点存在角色的数据库行里（`characters.savepoint`，存档点 NPC 的编号），重新登录
  仍然有效；`status` 等查询不展示它（回城是服务端行为，客户端不需要）。
- **战败送回**：战斗失败且**被打飞**（协议 10.9 的 `lose`，`penalty.in_place` 为假）时送回记录的存档点落点（参数文件里的
  `Born`），没有记录过回出生地（规则同现状）；血量剩 1 等其余规则不变。被打死则原地复活，不送回（STAGE-194）。
- 已记录过同一个存档点再 `talk`，正文用 `Normal` 文案（原版「已经记录了」）；再选「记录」
  也只换成 `Normal` 文案的窗口，不查物品、不收东西、不推 `inventory_changed`（原版
  `npc_savepoint.c` 的已记录分支）。参数来自 `server/gamedata/npc-savepoint.json`。
- 战斗中回复 `IN_BATTLE`；观察方被拒（操作类）。

### 5.41 宠物店：买卖、寄放宠物（STAGE-73）

和宠物店 NPC（5.32 的 `type` 是「宠物店」，原版 `npcgen_petshop`）`talk` 弹菜单
（`npc_window`，正文是店里的欢迎语）：支持寄放的店（参数 `pool_flg=1`）选项是
「寄放宠物 / 领取宠物 / 卖宠物 / 买宠物」，否则只有「卖宠物 / 买宠物」。列表页的
选项编号就是栏位号（随身 0–19，寄放池 0 起，格数见下；20 格的随身列表在窗口里照常逐项列出）。

- **卖**：随身宠物列表 → 确认页（原版格式：`是[名字]没错吧？` + 空行 + `realy_msg` +
  空行 + `买卖价格 N STONE`）→ 确认后宠物从栏位消失、金币增加，推 `pets_changed` +
  `status_changed` 并关窗。估价按原版 `NPC_GetCost`：
  `((等级²×10 − 捕获时等级²×10) + 等级×10) × 稀有系数 × 店的倍率 × 魅力系数`，逐步向下取整。
  稀有系数取模板的 `CHAR_RARE`（旧数据 `E_T_RARE`，不是品质）：0→1、1→5、2→8，其余原值；
  `special_pet` 列表里的模板按 `special_rate`，其余 `nomal_rate`。魅力系数 =
  `max(人物有效魅力 + 宠物忠诚, 20) / 2`（百分比；忠诚同宠物页的 `loyalty`）。捕获时等级
  暂按 1 算（新服务端还没有这一列）。卖出后金币超过携带上限回复 `GOLD_FULL`（`over_msg`
  原话）。
- **寄放**（pool 店）：随身宠物列表 → 确认页（原版格式：`寄放[名字]` + 空行 + `寄放价格
  N stone`；寄放费 = 50 + 等级×4，原版 `NPC_GETPOOLCOST`；参数 `pool_cost` 在原版代码里被
  注释掉没有使用）。寄放池满时回复 `ACTION_NOT_ALLOWED`（`poolfull_msg` 原话；格数随转生：`min(转生×2+5, 15)`，STAGE-309，0 转 5 格、5 转起 15 格）；金币
  不够回复 `NOT_ENOUGH_GOLD`（`cost_msg` 原话）。寄放的宠物 `state=3`、栏位 = 50 + 池位落库，
  随身列表（5.19 的 `pets`）不再显示；它不是随身宠物：不能出战、不占随身栏位、不被护士回血、
  战后状态也不改（服务端只认 0–19 的随身栏位）。
- **领取**（pool 店）：寄放池列表（`pooltanks_msg`）→ 选一只直接取回到第一个空栏位
  （免费，原版如此），等级、经验、资质不变；随身栏满时回复 `ACTION_NOT_ALLOWED`
  （`getfull_msg` 原话）。
- **买**：原版宠物店不卖宠物（只有卖/寄放/领取），这是新功能：商品清单与定价是
  **占位数据**（本期用出生宠物同款三只模板、价格 = 同一估价公式 ×10），等商品表确定后
  只换数据。确认后新宠按出生宠同一套资质规则生成、放进第一个空栏位，推
  `pets_changed` + `status_changed` 并关窗；栏位满回 `ACTION_NOT_ALLOWED`
  （`getfull_msg`）、金币不够回 `NOT_ENOUGH_GOLD`（`cost_msg`）。
- 台词（欢迎语、`realy_msg`、`over_msg`、`thanks_msg`、`cost_msg`、`pooltanks_msg`、
  `poolfull_msg`、`getfull_msg`）逐字取自 `server/gamedata/npc-petshop.json`
  （`tools/gen-npcs.py` 从原版 `ps_*` 参数文件生成）。战斗中 `IN_BATTLE`；观察方被拒。
  到达宠物店的移动（室内地图）本期靠协议用例的 `move_character` 管理命令（见第 6 节）。
- 骑乘中的宠物（`state=ride`，5.47）不能卖、不能寄放，回复 `PET_RIDING`（STAGE-123）。

### 5.48 / 5.49 普通出口（STAGE-148）

`exits` 查看本图出口、`use_exit` 使用具体出口，完整字段、权限、错误码和副作用见 [普通出口协议](protocol/exits.md)。`npcs` 响应新增 `floor`，用于校验读取地图。

### 5.50 / 5.51 遇敌区域与按区域遇敌（STAGE-152）

`encounter_areas` 查看本图所有可选遇敌区域（含敌人预览、代表落点、可用性），`encounter_start` 定位到所选区域并立刻开战，
`mode` 取 `once`（打一场就停）或 `infinite`（连场到手动停止）。
`encounter_areas` 的可选参数（STAGE-258）：`offset`、`limit`，都是非负整数，缺省 `offset` 为 0，缺省 `limit` 返回本图全部区域；显式 `limit` 超过 100 按 100 处理；负数或非整数回复 `BAD_REQUEST`。响应多一个 `total`，是本图区域总数（分页之前）。每项多一个 `level1_pets`（STAGE-286）：本区域能遇到的 1 级宠物名，没有时为 `[]`，见下面的字段表。大岛的区域多、整包约 88 KB，agent 应先 `limit: 20` 翻页。完整字段、可用性规则、推送顺序和状态字段见
[遇敌区域协议](protocol/encounter-areas.md)。5.29 的 `encounter on|off` 与它共用同一套调度状态：`on` 等于“当前位置无限遇敌”，
`off` 停止。


## 6. 协议用例

协议用例是一份份“对话剧本”，写明在某个场景下客户端发什么、服务端必须回什么。它和本文一样是协议约定的一部分，有三个用途：

- 服务端的自动化验收；
- 客户端的模拟服务端；
- 给 agent 当作具体示例。

**文件格式**：

- 用例文件放在 `docs/protocol/cases/*.jsonl`，每个文件是一个场景，每行是一条消息：

  ```jsonc
  {"conn": "player", "dir": "c2s", "msg": {"id": 1, "type": "hello", "data": {"protocol": 1, "client": "case", "role": "player"}}}
  {"conn": "player", "dir": "s2c", "msg": {"id": 1, "type": "hello", "ok": true, "data": {"protocol": 1, "server": "*", "heartbeat_seconds": 20, "idle_timeout_seconds": 60, "register_enabled": true}}}
  ```

- `dir`：`c2s` 表示客户端发出的消息，`s2c` 表示服务端应当回复或推送的消息。
- `conn`：这一行属于哪条连接，用于需要同时开多条连接的场景，例如重复登录、agent 与玩家同时在线。不写时视为 `"player"`。
- **期望值里的 `"*"`**：表示这个字段必须存在，但不比较它的值，用于时间戳、版本号、提示文字这类不固定的内容。
- **前置条件**：场景需要的前置数据，比如账号已经存在，直接写成剧本里前面的几步，例如先注册再登录。前置步骤放在 `conn` 为 `setup`（或 `setup2`、`second` 等非 `player` 名字）的连接上：服务端测试客户端照常执行；**模拟服务端只扮演 `player` 连接，其余连接的行一律跳过**。
- **`stage`**（`meta` 里）：这个用例由哪个需求负责实现（`STAGE-2`、`STAGE-3`……）。服务端的回归测试只跑已经实现的阶段，所以每个用例都必须写。
- **`raw` 和 `raw_len`**（`c2s` 行，代替 `msg`）：`raw` 是原样发送的一行文本，用来测试坏数据；`raw_len` 是发送一行 N 个字节的数据，用来测试超长行。这两种行只用于服务端测试，模拟服务端不支持含有它们的用例。
- **`closed`**（`s2c` 行，代替 `msg`）：`{"conn": "player", "dir": "s2c", "closed": true}` 表示服务端此时应当已经关闭这条连接。
- **`admin`**（管理员操作行）：`{"conn": "setup", "dir": "admin", "do": "disable_account", "account": "alice"}` 表示这一步由测试工具以管理员身份直接操作服务端，不经过协议，用来构造“账号被禁用”这类场景。`do` 的取值：`disable_account`（禁用账号，在线连接会立即被踢，见 5.4）、`enable_account`（启用账号）、`unlock_account`（解除登录锁定）；`account` 是目标账号。另有 `give_item`、`give_pet`（见 5.18、5.19；可带 `loyalty`，宠物忠诚的可变部分，单位是点，`100` 即封顶，STAGE-127——战斗用例里宠物忠诚低于 80 会不听话，要它一直听话就给 `100`）、`clear_pets`（删掉角色 `character` 的全部宠物，用来让不涉及出生送宠物的用例恢复“角色起初没有宠物”的前提，见 5.6）、`give_points`（`points` 是点数，见 5.21）、`faint_pet`（`slot` 是栏位，把那只宠物的血量设为 0，相当于战斗中倒下，战斗还没做时用来造出倒下的宠物，见 5.22）和 `start_battle`（让角色 `character` 立刻进入一场战斗，`scenario` 指定剧本，见第 10 章）、`move_character`（把角色 `character` 挪到 `floor`/`x`/`y`，用在进入世界之前的 setup 里，室内 NPC 的用例靠它到店）、`set_pet_transmigration`（把角色 `character` 栏位 `slot` 上宠物的转生次数设为 `trans`，只写库并通知，STAGE-196，用例 `pet_ride_trans`；命令行版本是 `stoneage-admin setpettrans`，见 `docs/server.md`）、`set_level`（把角色 `character` 的人物等级设为 `level`，只写库，用在进入世界之前，STAGE-123）、`set_ride_level`（把骑乘等级设为 `level`，同管理命令 `ridelevel`，STAGE-122）、`set_setting`（改全服设置，`key` 是设置名、`value` 是值，如 `exp_rate` 2，见 `docs/server.md` 管理命令；改过的用例要在末尾改回默认值，不带到后面的用例）。另有 `chat_mute`（`character` 角色、`seconds` 秒数：从现在起禁言这么多秒，测试用，STAGE-278）和 `chat_unmute`（`character`：立即解除禁言）。服务端测试客户端要能直接操作数据库才能执行（`stoneage-casetest` 需加 `-reset-db`，`go test` 自动提供），否则跳过这个用例；**模拟服务端一律跳过 `admin` 行**，所以 `admin` 行只放在 `setup` 之类的非 `player` 连接上。
- **捕获与引用**：有些值事先不知道，比如 `create_agent_token` 返回的令牌。`s2c` 行里写 `"*>名字"`（或带样例的 `"*>名字=样例"`）表示“不比较具体值，但把实际值记下来，叫这个名字”；之后 `c2s` 行里值恰好是 `"$名字"` 的字符串，会换成记下的值（例如 `{"token": "$tok"}`）；`s2c` 行里写 `"$名字"` 表示这个字段必须等于记下的值。名字在整个用例里共用，不分连接。模拟服务端发送样例，并按同样的规则核对客户端发来的值。
- **战斗编号不写死**（STAGE-95）：`battle_id` 是服务端进程内累加的（`b_1`、`b_2`……，同一进程里第二场战斗就是 `b_41` 之类），用例里不写 `"b_1"`：第一次在 `s2c` 出现时写 `"*>battle_1=b_1"`，之后同一用例里的 `s2c` 和 `c2s` 都写 `"$battle_1"`。一个用例里有几场战斗就用几个名字（如 `battle_1`、`battle_2`）。
- **连接内的 `id`**：每条连接各自从 1 开始递增，和客户端的实际行为一致（`hello` 是 1，下一个请求是 2……）。
- **请求里的通配**：`c2s` 行的值里也可以写 `"*"`，仅用于 `client` 这类自由文本字段（服务端测试客户端会把它当普通字符串发出去）。
- **`"*?=样例"`**：可空通配，字段仍必须存在，值可以为 null；用于随时段切换的 `unavailable_reason` 等可空字段。模拟服务端按 JSON 类型发送样例，原有 `*` / `*=样例` 仍不接受 null。
- **`"*=样例"`**：`s2c` 行里用，表示“比较时不检查具体值”，同时告诉模拟服务端发送“样例”（能按 JSON 解析的按 JSON 发，如 `"*=897"` 发数字 897，否则发字符串）。服务端测试客户端把以 `*` 开头的字符串都当通配。
- **`delay_ms`**（可选）：写在 `s2c` 行上，表示模拟服务端等这么多毫秒再发，让客户端界面有时间停在上一个状态；服务端测试客户端把它加到等待回复的时限上。写在 `c2s` 行上，表示服务端测试客户端等这么多毫秒再发（用来测心跳和空闲超时），模拟服务端忽略它。
- **首行 `meta`**（可选）：`{"meta": {"desc": "场景说明", "server_config": {...}, "manual": true}}`。`server_config` 是跑这个用例时服务端需要的配置（如未开放注册、空闲超时 2 秒）；`manual` 为 `true` 表示服务端的自动化测试跳过它（需要人工触发，如服务器维护），只用于模拟服务端和阅读。

**修改顺序**：协议要改动时，先改本文和用例文件，再改代码。

## 7. 默认值一览（已确认，2026-10-09）

以下数值已经确认。以后要调整时，先改本文，再改代码：

| 项目 | 默认值 |
|---|---|
| 服务端端口 | 7065（可改，见 `docs/server.md`） |
| 心跳间隔 / 空闲超时 | 20 秒 / 60 秒 |
| 账号格式 | 3–15 个字符，小写字母开头，只含小写字母、数字、下划线，不区分大小写 |
| 密码格式 | 6–64 个字符 |
| 注册频率 | 每个 IP 每小时 50 次 |
| 登录锁定 | 同一账号 10 分钟内失败 5 次，锁定 15 分钟；同一 IP 10 分钟内失败 20 次，限制登录 |
| 角色名 | 全服唯一 |
| 每个账号的角色数 | 2 |
| 每个角色的宠物栏位 | 20 个（编号 0–19，STAGE-346） |
| 宠物出战数量 | 最多 1 只（原版服务端 `PET_SelectBattleEntryPet`） |
| 宠物待命数量 | 最多 3 只（原版服务端 `SPET` 掩码） |
| 宠物名字 | 1–16 个字符（UTF-8 字符数，去掉首尾空白后；原版服务端 16 字节 GBK） |
| 管理命令 `givepet` 不写等级时的等级 | 1（原版服务端 `setup.cf` 的 PETLV） |
| 管理命令 `givepet` 的最高等级 | 160（原版服务端 `setup.cf` 的 MAXLEVEL） |
| agent 令牌 | 每个账号最多 5 个有效令牌；有效期默认 30 天，最长 365 天 |
| 在线 agent 的令牌到期检查 | 每 60 秒检查一次；到期的 agent 先保存再收到 `AGENT_TOKEN_REVOKED` 并被断开（所以到期后最多再在线约 1 分钟） |
| 战斗：每回合出招时限 | 30 秒（待 STAGE-41 对照原版确认） |
| 战斗：回放确认等待上限 | 20 秒，超时视为已确认（待 STAGE-41 确认） |
| 遇敌：两场战斗的间隔 | 0.5 秒（`STONEAGE_ENCOUNTER_INTERVAL_MS`，见 `docs/server.md`） |
| 决斗（STAGE-306）：角色决斗点数 `duel_point` 默认 100，范围 0–100000000；社交开关 `duel`（接受决斗）默认关、不存库 | 见 `docs/pk.md` 2.2、3.1 |
| 社交开关（STAGE-276）：隐身、拒绝邀请默认都开（`visible`、`invite` 都是 `true`），存角色上；STAGE-296 起加接收交易邀请（`trade`，默认 `true`） | 角色字段，见 `docs/server.md` |
| 公开聊天（STAGE-278）：本图每人 1 秒 1 条，世界每人 30 秒 1 条，世界最低等级 5，文本 1–70 字符，同一句 10 秒内重复静默丢弃 | 全服设置，见 `docs/server.md` |

## 8. 服务端配置

已移到 `docs/server.md`。本节编号保留，避免其他文档和需求里引用的章节号错位。

### 5.42 `maps`：取快捷传送目录（查看类，世界中，STAGE-161）

可选参数（STAGE-258，不传则与原来一致，全部返回）：
- `parts`：字符串数组，取 `"maps"` / `"nodes"`，只返回选中的部分，没选的键不出现；缺省或空数组 = 全部；其他取值回复 `BAD_REQUEST`。
- `keyword`：字符串，对 `maps` 平面表按名字做子串过滤，不区分大小写，去掉首尾空白后为空则不过滤；不影响 `nodes`；只带 `keyword` 不带 `parts` 时按 `parts=["maps"]` 处理，只返回过滤后的平面表。没有命中时 `maps` 是空数组。
- 节点树（`parts` 含 `nodes`）原文约 81 KB，不声明 gzip 的连接取它会收到 `RESPONSE_TOO_LARGE`，要先在 `hello` 里声明 gzip。

`catalog_version` 始终返回。例：`{"id": 13, "type": "maps", "data": {"parts": ["maps"], "keyword": "村"}}` 只返回名字含“村”的地图。

```jsonc
{"id": 13, "type": "maps", "data": {}}
{"id": 13, "type": "maps", "ok": true, "data": {
  "catalog_version": "2026-10-17.1",
  "maps": [
    {"floor": 100, "name": "萨伊那斯"},
    {"floor": 1000, "name": "萨姆吉尔村"},
    {"floor": 1006, "name": "萨姆吉尔的村长家"}
  ],
  "nodes": [
    {"id": "group:sainasu", "parent_id": null, "name": "萨伊那斯", "floor": 100, "order": 0},
    {"id": "map:1000", "parent_id": "group:sainasu", "name": "萨姆吉尔村", "floor": 1000, "order": 0},
    {"id": "map:1006", "parent_id": "map:1000", "name": "萨姆吉尔的村长家", "floor": 1006, "order": 5},
    {"id": "group:sainasu.dungeons", "parent_id": "group:sainasu", "name": "洞窟与通路", "floor": null, "order": 6},
    {"id": "map:10001", "parent_id": "group:sainasu.dungeons", "name": "阿布的洞窟地下１楼", "floor": 10001, "order": 0}
  ]
}}
```

三样东西来自**同一份**审核过的正式目录（`server/gamedata/travel_catalog.json`，由 `tools/build-travel-catalog.py` 展开成运行表）：

- `catalog_version`：目录版本号，层级或落点配置变了才会变。客户端可以据此判断缓存是否过期。
- `maps`：平面表，每张正式地图一条，按 `floor` 升序。名字和 `position.map_name` 一致；同名的地图（如几张“迷宫”、两张“梦幻的洞窟”）各占一条，名字可能重复，靠 `floor` 区分。用来按名字搜索和 Tab 补全 `warp`；搜索只在客户端做。
- `nodes`：包含树，`parent_id` 为 `null` 的是根。字段：
  - `id`：稳定标识。地图节点固定为 `map:<floor>`，纯分组用 `group:` 前缀，客户端不要靠名字或数组下标认地图。
  - `parent_id`：上级节点 id，根为 `null`。
  - `name`：显示名。同一层级里重名的地图在名字后面补了 ` · floor` 消歧（如 `迷宫 · 117`），纯分组的名字不带后缀。
  - `floor`：地图编号；纯分组为 `null`，不能传送。
  - `order`：同级排序号，从 0 连续递增，客户端按它排序，不要按名字或 floor 重排。
- 一致性：每张正式地图恰好对应一个地图节点；`nodes` 里 `floor` 非 null 的集合与 `maps` 的 `floor` 集合完全相等；树没有环、没有孤儿、没有重复 floor。全部可传送目标都在这份目录里，`warp` 的准入见 5.16。
- 数量级：约 684 张正式地图、748 个节点。进世界和重连后各取一次即可，客户端不要每次进页面都取。
- 玩家和 agent 都可以查看，观察方也能查看；不在世界中回复 `WRONG_STATE`。
- 旧服务端（只返回 `maps` 不返回 `nodes`）时，客户端要把快捷传送页显示成「目录不可用」并提供重试，不能悄悄退回不完整的列表还声称是全部地图。

### 5.43 `npc_find`：按名字找 NPC（查看类，世界中，STAGE-84）

```jsonc
{"id": 14, "type": "npc_find", "data": {"name": "村长家的厨师"}}
{"id": 14, "type": "npc_find", "ok": true, "data": {"npcs": [
  {"id": 225, "name": "村长家的厨师", "type": "对话", "floor": 1006, "map_name": "萨姆吉尔的村长家", "x": 16, "y": 16}
]}}
```

- `name` 是 NPC 名字，去掉首尾空白后不能为空；缺少、不是字符串或为空回复 `BAD_REQUEST`。
- 匹配：先找完全同名的；没有时找名字里包含这个词的。空名字的 NPC 不参与。先按这一步定下的一组匹配，再去掉去不了的地图上的（下一条），所以同名的都去不了时结果为空，不会退到包含匹配。没找到返回空数组 `"npcs": []`，不报错。
- 只列两类 NPC：角色**当前地图**上的，以及能传到（`floor` 在快捷传送目录里，见 5.16、5.42）的地图上的。去不了的地图（如不在目录里的测试图、占位图）上的同名 NPC 不列出。
- 排序：当前地图的在前；当前地图内按离角色的距离（格子数，取较大的一个方向）近的在前；其余按 `floor` 升序；最后按 `id`。最多返回 20 条，同名的很多时只给前 20 条。
- 每项的 `id` 是 NPC 编号，直接给 `talk`（5.33）用；`floor` 是地图编号，给 `warp`（5.16）的 `floor` 用；`map_name` 是地图名，只用来显示。`x`、`y` 同 5.32。
- 玩家和 agent 都可以查看；观察方也能查看（同 5.32）。不在世界中回复 `WRONG_STATE`。
- 本请求只查询，不移动角色、不打开窗口。要去别的地图对话，由客户端先问玩家，确认后再发 `warp` 和 `talk`。

### 5.44 `quests`：查看任务进度（查看类，世界中，STAGE-86）

```jsonc
{"id": 15, "type": "quests", "data": {}}
{"id": 15, "type": "quests", "ok": true, "data": {"quests": [
  {"no": 3, "name": "猜谜3兄弟 拿蓝暴", "state": "doing"},
  {"no": 1, "name": "成人仪式", "state": "done"}
]}}
```

- 只列有进度的任务，即进行中或已完成的；没接过的不列。进行中的排在前面，同一状态内按 `no` 从小到大。没有任何任务时 `"quests": []`，不报错。
- `no` 是任务编号（原版服务端 `gmsv/data/jobdaily.txt` 的任务编号）；`state` 取值 `doing`（进行中）、`done`（已完成）；`name` 是任务名，已转成 UTF-8，只用来显示，客户端不需要任务表。
- 任务表共 110 个：原版第 61 号“动物园任务 拿里拉拉的凯”的完成和进行中事件号都是 0，无法对应到一个进度，暂不收录（`tools/gen-quests.py`）。另有一个自定义任务 111“萨姆吉尔的勾玉”（STAGE-333，完成和进行中都是事件 185，不来自 jobdaily.txt）。
- 进度只存服务端（迁移 `0013_character_quests.sql`，每个任务编号一行）。目前只有管理命令 `stoneage-admin quest` 会改（见 `docs/server.md`）；接任务、交任务的 NPC 对话由后续需求提供。
- 玩家和 agent 都可以查看，观察方也能查看（查看类）。客户端进入世界后请求一次，之后靠推送更新。不在世界中回复 `WRONG_STATE`。

### 5.45 推送 `quests_changed`：任务进度变了（STAGE-86）

```jsonc
{"type": "quests_changed", "data": {
  "quests": [ ... ],
  "changed": [3]
}}
```

- `quests` 与 5.44 的响应相同，反映推送这一刻的完整任务进度（条数很少，直接全量发）。
- `changed` 是这次变化的任务编号列表。从没有进度到进行中、进行中变成已完成，都算变化；被清除（管理命令 `clear`）的任务也在 `changed` 里，但它已经不在 `quests` 里，客户端只更新列表，不提示。
- 角色的进度变化后，服务端给世界中的**每一条**连接（玩家和 agent）都推送，规则同 `inventory_changed`；没有实际变化时不推送。
- 一期变化的唯一来源是管理命令 `stoneage-admin quest`：写库并发出 PostgreSQL 通知，服务端收到后重读这个角色的任务再推送；角色不在线时只写库，等下次进入世界时可见。

任务 NPC 的事件（接任务、交任务、放弃、重置，STAGE-90）也通过这个推送更新任务页，`changed` 同样列出变化的编号。

### 5.46 推送 `npcs_changed`：本地图的 NPC 变了（STAGE-93）

```jsonc
{"type": "npcs_changed", "data": {"floor": 30001, "npcs": [
  {"id": 1415, "name": "多洛加", "type": "任务", "x": 10, "y": 6}
]}}
```

- 任务脚本的 `NpcWarp`（见 `docs/quest.md` 的「NPC 移位」）把 NPC 移到别的地图或坐标时推送。`floor` 是这张地图的编号，
  `npcs` 是推送这一刻这张地图上**全部** NPC 的列表（格式同 5.32，按 `id` 升序；上面的例子只列了一项），客户端整张替换。
- 推给所在地图是**移动前或移动后**那张地图的每一条连接（玩家和 agent，规则同 `inventory_changed`）；其他地图上的连接不收到。
  一条连接同一次移动只收到一次推送。
- 推送在触发移动的请求（`talk` 或 `npc_choose`）的响应之前发出。移动后原地图上别的连接正开着这个 NPC 的窗口时，
  先推 `npc_window_closed`（`reason=MOVED`，见 5.34），再推 `npcs_changed`。
- 移动之后，原地图上对这个 NPC 的 `talk` 回复 `NPC_NOT_HERE`；新地图上的 `talk` 照常（5.33）。
- 移动只在内存里（全服共用一份），服务重启后回到 `npcs.tsv` 的初始位置，轮转进度也归零。
- 协议用例：`docs/protocol/cases/npc_warp.jsonl`。

### 5.47 `npc_list`：NPC 目录（查看类，世界中，STAGE-134）

`npc_find`（5.43）是「认名字找人」：必须有名字、只给匹配到的前 20 条、也拿不到总数。
客户端的 NPC 目录页要的是**能翻的完整目录**，所以另开这个请求：按分类取，可按名字搜索，带分页和总数。

```jsonc
{"id": 16, "type": "npc_list", "data": {"scope": "reachable", "name": "村长", "offset": 0, "limit": 50}}
{"id": 16, "type": "npc_list", "ok": true, "data": {
  "npcs": [
    {"id": 1415, "name": "多洛加", "type": "对话", "floor": 1000, "map_name": "萨姆吉尔村", "x": 61, "y": 111, "here": true},
    {"id": 225, "name": "村长家的厨师", "type": "对话", "floor": 1006, "map_name": "萨姆吉尔的村长家", "x": 16, "y": 16, "here": false}
  ],
  "total": 137, "offset": 0, "limit": 50, "scope": "reachable"
}}
```

- `scope`：`reachable`（默认）是「全部可达」——角色当前地图上的，加上能传到（`floor` 在快捷传送目录里，见 5.16、5.42）的地图上的；
  `here` 只给当前地图的。别的取值回复 `BAD_REQUEST`。
- `name`：可选，按名字做**子串**搜索，去掉首尾空白后为空等于不搜索。
  这与 `npc_find` 的「先精确、没有再按包含」不同——目录页要的是「把所有含这个词的都列出来」。
  空名字的 NPC 不进目录。
- `offset` / `limit`：分页。`offset` 不能是负数；`limit` 至少 1，最多 200（超了压到 200），默认 50。
- `total` 是**分类 + 搜索之后**的全部条数（不是这一页的条数），客户端靠它显示「共几条」并判断有没有下一页；
  `offset` 不小于 `total` 时返回空数组 `"npcs": []`，不报错。
- 每项字段同 5.43，另加 `here`：这个 NPC 是否就在角色当前地图上（客户端列表里标「本图」）。
- 排序与 `npc_find` 一致：当前地图的在前，当前地图内按离角色的距离（格子数，取较大的一个方向）近的在前，
  其余按 `floor` 升序，最后按 `id`。
- 玩家、agent 和观察方都能查看；不在世界中回复 `WRONG_STATE`。只查询，不移动角色、不打开窗口。
- 协议用例：`docs/protocol/cases/npc_list.jsonl`。

**为什么不用 `npc_find` 拼目录**：它不传名字就报错、有条数上限、也不返回总数，拿它一页页拼出来的列表既不全也稳定不了
（名字一改就变），所以目录必须由服务端给。

### 5.52 `pet_ride`：把宠物骑上去（操作类，世界中，STAGE-123）

设计与规则见 `docs/ride.md`。

```jsonc
{"id": 41, "type": "pet_ride", "data": {"slot": 1}}
{"id": 41, "type": "pet_ride", "ok": true, "data": {}}
```

- `slot` 是宠物栏位（0–19 的整数），缺少或不对回复 `BAD_REQUEST`。
- 按下面的顺序检查，第一条不满足就回复对应错误，数据不变（出处 `gmsv/char/family.c` 的 `FAMILY_RidePet`）：
  1. 战斗中 `IN_BATTLE`；
  2. 栏位没有宠物 `NO_PET`；
  3. 已经在骑别的 `ALREADY_RIDING`（要骑已经在骑的这只：成功，不改任何东西，也不推送）；
  4. 这只宠物是出战或待命状态 `PET_NOT_REST`（要骑的宠物必须是休息状态，先 `pet_state` 改成休息）；
  5. 角色 `ride_level` 小于宠物等级 `RIDE_NOT_LEARNED`，说明写“你目前只能骑乘等级小于等于 N 级的宠”；`ride_level` 为 0（一次都没学过）时写“你还没有学过骑乘，先去骑乘训练师那里学习”（STAGE-181）；
  6. 宠物忠诚（5.19 的 `loyalty`）小于 100 `RIDE_LOYALTY_LOW`；
  7. 人物等级加骑乘等级差（服务端配置 `ride_level_gap`，默认 5）小于宠物等级 `RIDE_LEVEL_GAP`；
  8. 宠物转生超过 2 次 `RIDE_TRANS_LIMIT`（转生次数见 5.19 的 `transmigration`，STAGE-196 起检查）；
  9. 人物外形与宠物种族不在骑乘表里 `RIDE_NOT_MATCH`（只认原版原生的 4 种虎 × 颜色、5 种雷龙 × 外形，见 `docs/ride.md` 第 3 节）。
- 倒下（`hp` 为 0）的宠物不检查，照样能骑（原版如此）。
- 骑乘期间其他宠物照常可以用 `pet_state` 设成出战或待命（STAGE-149），骑着的那只不受影响；出战的那只与骑着的同时参战，见 10.4 和 `docs/ride.md` 第 5 节。
- 成功后该宠物 `state` 变为 `ride`，写库后给世界中的**每一条**连接（玩家和 agent）推送 `pets_changed`，推送先于响应到达。
- 属于“操作”类：观察方发起回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同 `warp`。
- 错误码：`BAD_REQUEST`、`IN_BATTLE`、`NO_PET`、`ALREADY_RIDING`、`PET_NOT_REST`、`RIDE_NOT_LEARNED`、`RIDE_LOYALTY_LOW`、`RIDE_LEVEL_GAP`、`RIDE_TRANS_LIMIT`、`RIDE_NOT_MATCH`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。

### 5.53 `pet_unride`：骑下来（操作类，世界中，STAGE-123）

```jsonc
{"id": 42, "type": "pet_unride", "data": {}}
{"id": 42, "type": "pet_unride", "ok": true, "data": {}}
```

- 正在骑的宠物 `state` 回到 `rest`，写库后推送 `pets_changed`（规则同 5.52）。没有在骑：成功，不改任何东西，也不推送。
- 战斗中 `IN_BATTLE`；观察方被拒。错误码：`IN_BATTLE`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。
- 骑乘中的宠物不能放生、寄放、卖掉、用 `pet_state` 改状态，都回复 `PET_RIDING`（说明写“骑乘中的宠物不能X，先下骑”）；交易、宠物邮件、转生等以后做的功能同理。唯一的例外是任务 NPC 收宠物（`DelPet`，STAGE-197）：按原版先自动下骑再收走，不回复 `PET_RIDING`（5.20 `pets_changed`）。

### 5.54 骑乘训练师：学骑乘课程（STAGE-122）

骑乘训练师 NPC（5.32 的 `type` 是「骑乘训练师」，原版 `riderman`，`talk` 的 `npc` 是 1784–1787 四个编号，四大村的家各一个：1784 在 1040（萨姆吉尔）、1785 在 2030（玛丽娜丝）、1786 在 3030（加加）、1787 在 4030（卡鲁它那）。1763–1783 是宠物技能店，不是训练师）。`talk` 弹主菜单（`npc_window`），文字和跳转逐字取自原版 `riderman.conf`，由 `tools/npcriderman.py` 生成 `server/gamedata/npc-riderman.json`：

- 主菜单「关于骑乘宠物 / 查询可骑乘的宠物 / 学习骑乘宠物」。
- **关于骑乘宠物**：子菜单「学习骑乘的技能 / 骑乘用宠物的条件」，各一页说明，选项 `1 上一页` 回子菜单。
- **查询可骑乘的宠物**：一页文字，列出当前外形能骑的虎和雷龙（`你的外形能骑的宠物：\n虎：…\n雷龙：…`），选项 `1 上一页`。
- **学习骑乘宠物**：课程菜单「初级班 / 中级班 / 高级班 / 特级班」。选一级时服务端先检查（不弹确认页）：
  - 已经学过（骑乘等级 ≥ 该级）：弹消息页「你已经学会X级班了呀。\n不需要再学了。」；
  - 没学前一级（骑乘等级 < 前一级）：弹消息页「你必须先学过X级班才能来Y级班唷。」；
  - 都通过：弹确认页「学习骑乘X级班不需要学费。你确定要学习吗？」，选项 `1 是`、`2 否`。
- **确认**：`1 是` 再检查一次（窗口开着时可能已经学过）。通过则写库，`骑乘等级` 变为 40 / 80 / 120 / 200，关窗（`npc_window_closed`，`CHOSEN`），推 `status_changed`（`status.ride_level` 变了）和系统消息 `notice`（「恭喜你！你已经完成骑乘X级班了。可以骑乘 Lv40 以下的宠物，请小心骑乘喔！」，特级是「可以骑乘所有等级的宠物」）。`2 否` 直接关窗，不改数据。
- 消息页没有选项：回 `NPC_BAD_CHOICE`，玩家用 `npc_close` 关掉。
- **规则**：必须按顺序学；学费免费（原版 5000 / 10000 / 15000 / 20000 石币，本项目取消，也不做村庄家族收入）。新角色骑乘等级为 0，`status.ride_level` 显示当前值（5.12）。
- 战斗中 `IN_BATTLE`；观察方被拒。
- 管理命令 `stoneage-admin ridelevel <角色名> <0|40|80|120|200>` 直接设骑乘等级（运行中的服务端立即推送 `status_changed`），方便验收。

### 5.55 `pet_skill`：战斗外使用宠物技能（操作类，世界中，STAGE-144；加工、料理的合成 STAGE-207）

```jsonc
{"id": 50, "type": "pet_skill", "data": {"slot": 0, "skill": 3, "items": [5, 6, 7]}}
{"id": 50, "type": "pet_skill", "ok": true, "data": {"result": "created", "item": {"slot": 5, "item_id": 4200, "name": "合成枪1"}, "consumed": [5, 6, 7]}}
{"id": 50, "type": "pet_skill", "ok": false, "error": {"code": "SKILL_UNUSABLE", "message": "防御 只能在战斗中使用"}}
```

- `slot` 是宠物栏位（0–19，即 5.19 的 `slot`），`skill` 是宠物页的技能栏位（1–7，即 5.19 `skills` 的 `slot`）。`slot` 缺失或越界、`skill` 缺失或越界都回复 `BAD_REQUEST`。
- `items` 是合成用的背包格号（5–19，即 5.17 的 `slot`），**只有加工（200）、料理（201）用**，其他技能忽略它（但类型不对，如不是整数列表，仍回复 `BAD_REQUEST`）。加工、料理时 `items` 必填：缺失、件数不在 2–15 之间、格号不在 5–19、格号重复，都回复 `BAD_REQUEST`。
- 战斗中回复 `IN_BATTLE`：战斗中用技能走 10.5 的 `skill` 指令。
- 校验顺序：控制权（观察方回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，第 9.3 节）→ 战斗中（`IN_BATTLE`）→ 角色倒下（血量为 0，`CHARACTER_DEAD`）→ 这个栏位没有宠物（`NO_PET`）→ 技能栏位是空的（`SKILL_UNUSABLE`）→ 技能的场合是战斗中（如攻击、防御，`petskill.txt` 场合为 1）回复 `SKILL_UNUSABLE`，说明“只能在战斗中使用” → `items` 参数（`BAD_REQUEST`）→ 背包一个空格都没有（`BAG_FULL`，说明“合成时，最少需空一格物品栏位！”）→ 各格是空的（`EMPTY_SLOT`，说明格号）→ 各格不能作材料（`ITEM_UNUSABLE`，说明物品名和格号）→ 冷却 → 合成。前面任何一步失败都不扣任何东西。
- **加工、料理的合成**（原版 `PETSKILL_Merge` → `ITEM_mergeItem_merge`，规则见 `docs/synthesis.md`）：
  - 加工和料理是同一套规则，由第一件材料决定类型（食物或道具）。食物与道具混合：扣光材料，没有产物，`result` 为 `illegal`。
  - 每个材料格扣 1 件（叠放的格子只扣 1 个）；参与的材料不论结果如何都扣掉。
  - **冷却**：距上次合成不满 `5 + (件数 − 2)` 秒（2 件 5 秒，3 件 6 秒），结果 `cooldown`，材料扣光，随机还回一件材料的复制品。冷却中的请求同样扣料，并且**冷却时间戳每次进入都更新**，包括冷却中的请求。冷却只在内存里，重新进入世界清零。
  - **匹配失败**（5 次都没有配方）：结果 `fallback`，材料扣光，随机还回一件材料的复制品。
  - **匹配成功**：结果 `created`，产物放进扣料之后的第一个空格（可能正是某个被扣光的格子）。只有这个结果的产物在库里带合成品标记（STAGE-209 再接效果，协议暂不下发）。
- 成功时 `data` 为：
  - `result`：`created`（合成出了配方产物）、`fallback`（5 次失败，还回一件材料）、`cooldown`（冷却中，还回一件材料）、`illegal`（食物与道具混合，没有产物）；
  - `consumed`：被扣走的背包格号，按请求顺序；
  - `item`：放进背包的那件，`{"slot", "item_id", "name"}`，`slot` 是放进的格号。`illegal` 没有 `item`；
  - `message`：只有 `cooldown` 和 `illegal` 有，分别是“合成料理过於频繁，休息一下比较好哟。”和“非法的合成方法”。
- 推送：四种结果成功后都推送 `inventory_changed`（先于响应到达，同 5.31 的规则；`illegal` 和 `cooldown` 也推，材料已经扣掉）。扣料和放入产物在一个数据库事务里完成，成功后才改内存、推送；写库失败回复 `INTERNAL`，背包不变，不推送。
- 战斗外技能的目标类型都是 `none`，带了 `target` 也忽略（与 5.19 的规则一致，见 10.5 的“目标类型”）。
- 宠物倒下（血量为 0）不影响战斗外技能，原版也不检查。
- 属于“操作”类：观察方发起回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`（第 9.3 节）。
- 错误码：`BAD_REQUEST`、`NO_PET`、`SKILL_UNUSABLE`、`IN_BATTLE`、`CHARACTER_DEAD`、`EMPTY_SLOT`、`ITEM_UNUSABLE`、`BAG_FULL`、`INTERNAL`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。
- 清点结果（各技能的场合、目标和原版数值）见 `docs/pet-skill.md`。

### 5.51 `magic_use`：战斗外使用精灵（操作类，世界中，STAGE-203，STAGE-204 加对象和治疗）

```jsonc
{"id": 70, "type": "magic_use", "data": {"slot": 0, "target": 0}}                 // 变幻的精灵：变成 0 号宠物
{"id": 70, "type": "magic_use", "data": {"slot": 0}}                              // 变幻的精灵：不带 target 变回自己
{"id": 70, "type": "magic_use", "ok": true, "data": {"metamo": {"pet": "乌力", "image": 100250, "until": 1792412345678}}}
{"id": 70, "type": "magic_use", "ok": true, "data": {"metamo": null}}             // 变回自己
{"id": 71, "type": "magic_use", "data": {"slot": 1, "to": "pet", "target": 2}}    // 滋润的精灵：对 2 号宠物
{"id": 71, "type": "magic_use", "ok": true, "data": {"heal": {"amount": 58, "to": "pet", "target": 2}}}
{"id": 72, "type": "magic_use", "data": {"slot": 0}}                              // 治愈的精灵：只回自己
{"id": 72, "type": "magic_use", "ok": true, "data": {"heal": {"amount": 0, "to": "self"}}}  // 满血，回复 0
{"id": 73, "type": "magic_use", "ok": true, "data": {"heal": null}}              // 复活、解状态、防御类：没有效果
```

- `slot` 是精灵栏位（0–4，即 5.8 `status.magic` 的 `slot`，也包括装备自带的精灵）。
- **对象**：`to` 是 `self`（自己）、`pet`（宠物）或 `member`（队友），`target` 是对象的栏位（0–19；`pet` 时即 5.19 的 `slot`）。`to` 缺省时：不带 `target` 是自己，带了 `target` 是宠物（5.51 原来的写法，变幻的精灵只用这一种）。`to` 是 `pet` 或 `member` 时必须带 `target`，缺少回复 `BAD_REQUEST`；`to` 不是这三个值也回复 `BAD_REQUEST`；`target` 不是 0–19 也回复 `BAD_REQUEST`。一期没有队友：分身队的分身只存在于内存里（5.57），不算队友，所以 `member` 的对象一律无效（见下面“先扣 MP 再判断对象”）。
- **能在战斗外用的精灵**（按魔法表的函数，原版 `magic/magic.c`）：
  - **变幻的精灵**（350、351、352、355、410）：对象是自己（不带 `target`）或宠物；`to` 为 `member` 回复 `INVALID_TARGET`。
  - **治愈的精灵**（0–7）和**恩惠的精灵**（20–26）：战斗外只回自己（原版 `MAGIC_Recovery_Field` 不看对象），`to` 和 `target` 都忽略，不会因为对象而失败。
  - **滋润的精灵**（10–15）：对象可以是自己、宠物或队友（一期只有自己和宠物）。
  - **复活、解状态、防御类**（函数为 `revive`、`cure`、`defend`、`revive_defend`，即复活精灵、各种解状态精灵、光、镜子、守护、复活防御）：照原版扣 MP、没有效果、没有台词，`heal` 为 `null`；对象不检查，`to` 和 `target` 都忽略。
  - 攻击、状态、场地、召唤龙阵等其他精灵战斗外不生效：回复 `MAGIC_UNUSABLE`，说明“这个精灵不能在战斗外使用”，不扣 MP。栏位是空的也回复 `MAGIC_UNUSABLE`（“这个栏位没有精灵”）。
- **回复量**（治愈、滋润；原版 `MAGIC_Recovery_Field`、`MAGIC_OtherRecovery_Field`）：`RAND(p×0.9, p×1.1)`，`p` 是魔法表的威力（治愈 Lv1 为 80，滋润 Lv1 为 65），取整数，加到当前血量上，封顶为最大血量（人物的最大血量，或宠物的最大血量）。不乘恢复率（原版 `_MAGIC_REHPAI`）。`amount` 是实际回复量：满血时为 0，照样回复，台词也照样是 0。倒下的宠物（血量 0）是合法对象，照原版直接加血，加血后血量大于 0。
- **台词**（推送 `notice`，`level` 为 `info`，同 5.8 的写法）：回自己是“耐久力回复N”，回宠物是“某某的耐久力回复N”（某某是宠物名，N 是 `amount`）。
- **推送**：治愈、滋润、复活等扣了 MP 的情况都推送 `status_changed`（5.8 的 `status`，血量和 MP 可能变了）；回了宠物的血量，宠物血量写库，并推送 `pets_changed`（5.19）。
- **先扣 MP、再判断对象**（照原版 `MAGIC_Recovery`、`MAGIC_OtherRecovery`）：治愈、滋润先判断 MP，不够回复 `NOT_ENOUGH_MP`，什么都不变；MP 够就扣，扣完再判断对象：对象无效（`pet` 的栏位没有宠物，或 `member`）回复 `INVALID_TARGET`，**这时 MP 已经扣了**，血量不变。这是本协议里唯一一种“出错但 MP 已扣”的情况，其他错误都不改状态。
- **中毒限制**：原版中毒的施放者自己回血无效（MP 已扣，没有台词）。新服务端的中毒只在战斗中，战斗外没有中毒状态，所以这条规则暂不生效。
- **校验顺序**：控制权（`CONTROLLED_BY_*`）→ 战斗中 `IN_BATTLE` → 倒下 `CHARACTER_DEAD` → 参数（`BAD_REQUEST`）→ 栏位和能否战斗外用（`MAGIC_UNUSABLE`）→ 之后按精灵：
  - 变幻的精灵：骑乘中（`REQUIREMENT_NOT_MET`，“无法变身，骑乘中不能变身！”）→ 手上是远距离武器（弓、投掷斧、投掷石、回旋标，`REQUIREMENT_NOT_MET`，“使用远距离武器无法变身！”）→ 目标（`target` 栏位没有宠物回复 `INVALID_TARGET`；目标是加美，图像 100259，`REQUIREMENT_NOT_MET`，“不能变身成加美！”）→ MP 不够 `NOT_ENOUGH_MP`；
  - 治愈、滋润、复活、解状态、防御类：MP 不够 `NOT_ENOUGH_MP` → 扣 MP → 对象（只有滋润和 `pet`、`member` 会判断）。
- **变幻的精灵**（照原版 `MAGIC_Metamo`）：**变身**：扣 MP，`until` = 现在 + 秒数（到期时间）；提示“变身成乌力！”。**变回自己**（不带 `target`）：也扣 MP，变身清掉，提示“变回自己！”。
- 变身的效果只改外观：等级、属性、能力、HP、MP 上限和战斗数值都不变。变身中仍可以正常战斗、使用物品。
- 成功后服务端给世界中的每条连接推送 `status_changed`（10.10，`status.metamo` 变化）和一条消息栏通知 `notice`（5.8 的写法，`level` 为 `info`）。响应里的 `metamo` 与推送的 `status.metamo` 一致，变回自己时为 `null`。
- **到期**：`until` 到了服务端自动变回，推送 `status_changed`（`metamo` 消失）和 `notice`“变身失效了。”。
- **卸下身体装备**（装备格 1，铠甲）或换掉它时变身失效，推送 `status_changed` 和 `notice`“变身失效了！”（`item_unequip`、`item_move` 都算，5.26、5.27）。
- **变身中不能穿远距离武器**：`item_equip`（5.25）或 `item_move` 把远距离武器穿到手上，回复 `REQUIREMENT_NOT_MET`，说明“变身中无法装备远距离武器”（原版在战斗里出手时拦，这里在穿的时候拦）。
- **战斗中的 `magic` 指令**（10.5）遇到变幻的精灵回复 `MAGIC_UNUSABLE`，说明“变身只能在战斗外使用”，不会扣 MP。战斗中的治愈、滋润等见 10.5 和 10.7，不在本节。
- 变身状态只在内存里，不写库。登出、被挤下线、服务端重启、重新进入世界都回到自己的外观。
- 错误码：`BAD_REQUEST`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`、`MAGIC_UNUSABLE`、`INVALID_TARGET`、`REQUIREMENT_NOT_MET`、`NOT_ENOUGH_MP`、`NO_PET`（宠物模板不认识，正常不会发生）。
- 不在本节范围内（一期不做）：战斗中变身、变身后的图像在终端里的显示（只记录 `image`）、NPC、狐狸、猪的变身，以及变身期间的属性变化；精工（镶宝石）不在这里。

### 5.56 宠物技能店：花钱给宠物学技能（STAGE-143）

和宠物技能店 NPC（5.32 的 `type` 是「宠物技能店」，原版 `npcgen_petskillshop`）`talk` 弹技能列表
（`npc_window`，正文是店里的欢迎语，选项是这家店上架的技能，编号从 1 起，`label` 是「名字（价格 金币）：说明」）：

```jsonc
{"type": "npc_window", "data": {"npc": 1763, "name": "超级饲育员", "type": "宠物技能店",
  "text": "这个村里的超级饲育员\n好了！要教它什么好呢？",
  "options": [{"no": 1, "label": "待机（500 金币）：什么也不做"}, {"no": 2, "label": "攻击（1000 金币）：通常攻击"}, "……"]}}
```

- **上架**：店的 `pet_skill` 参数按原版顺序列出技能；行首 T 的非法技能（`petskill.txt` 的 `PETSKILL_ILLEGAL`，
  如 T四段攻击）和编号查不到的不上架。0 号待机是合法技能，照常上架（`pet_skill` 里有它就列出来）。
- **价格**：技能的学习价格（`petskill.txt` 第 11 列 `PETSKILL_COST`）× 店的 `skill_rate`，向下取整。
  学习价格只在这里收费，使用技能本身不花钱。
- **选择顺序**（每一步都是一个 `npc_window`，选项编号从 1 起）：
  1. 技能列表 → 选一项；
  2. 宠物列表（正文「要教哪一只宠物？」，`label` 是「名字 Lv」，编号是 `slot + 1`（1–20）；寄放的宠物不列）→ 选一只；
  3. 技能栏位列表（正文「要换掉哪一个技能栏位？」，按这只宠物的栏位数列出，`label` 是「N 号栏位：现在的技能」，空栏位写「（空）」）→ 选一栏；
  4. 确认页（正文「学习[技能]（说明）…给[宠物]的 N 号技能栏位（现在是…）…学习价格 N 金币」，选项 `1 学习`）。
- **成交**：确认页 `npc_choose` 1 重新检查后，技能写进选中的栏位（覆盖原来的技能，原版 `CHAR_setPetSkill`），
  金币扣掉学习价格，推 `pets_changed` 和 `status_changed`，然后**回到技能列表**（同原版，可以接着学，窗口不关）。
- **失败**：金币、宠物、栏位都不变，窗口留在原页面，回复错误：
  - 金币不够（确认时才检查）：`NOT_ENOUGH_GOLD`，正文说明技能名和价格；
  - 栏位超过这只宠物的栏位数（如凯比 6 栏，选 7 号）：`NPC_BAD_CHOICE`；
  - 这个编号的随身栏位没有宠物：`NO_PET`；
  - 选项编号不在当前列表里：`NPC_BAD_CHOICE`；
  - 离店太远：离 NPC 超过 2 格（切比雪夫距离，同原版 `NPC_PetSkillShopWindowTalked`）：同图时 `talk` 先把角色走到
    NPC 旁边（5.33，STAGE-263），找不到空格或者每次 `npc_choose` 时回复 `ACTION_NOT_ALLOWED`，正文「离超级饲育员太远了，走近一点再说」。角色不在 NPC 所在的地图时
    `talk` 照 5.33 回复 `NPC_NOT_HERE`。
- 战斗中回复 `IN_BATTLE`（`talk` 和 `npc_choose` 都是）。骑乘中的宠物也能学（学技能与骑乘无关；骑乘功能见 5.52，尚未实现）。
- 观察方被拒（操作类，第 9.3 节）。窗口开着时 `talk` 别的 NPC 直接换窗口（同 5.33）。
- 上架的技能和价格来自 `server/gamedata/npc-petskillshop.json`（`tools/gen-npcs.py` 从原版 `genout/psks_*` 生成），
  技能的学习价格、说明、非法标记来自 `server/gamedata/petskill.tsv`（`tools/gen-petbase.py` 生成）。
- 错误码：`NOT_ENOUGH_GOLD`、`NPC_BAD_CHOICE`、`NO_PET`、`ACTION_NOT_ALLOWED`、`NPC_NOT_HERE`、`IN_BATTLE`、`WRONG_STATE`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。
- 用例：`npc_petskillshop`（服务端协议用例，覆盖成交和各类失败）、`ui_petskill_shop`（客户端场景）。
- 技能清点和学习价格的依据见 `docs/pet-skill.md`。

### 5.57 分身队：`party` / `party_clone` / `party_disband` 与推送 `party_changed`（STAGE-162）

分身队是“队伍”的一种特殊用法：玩家当队长，另外 4 个是“当前状态的自己”的分身，一起走路、一起打架。队长解散队伍，分身全部消失。
本期没有真人组队（邀请、名片、同意、踢人都不做）；服务端只有最小的队伍后台，分身只存在于内存里。

```jsonc
{"id": 60, "type": "party", "data": {}}
{"id": 60, "type": "party", "ok": true, "data": {"party": null}}
{"id": 61, "type": "party_clone", "data": {}}
{"id": 61, "type": "party_clone", "ok": true, "data": {"party": { ... }}}
{"id": 62, "type": "party_disband", "data": {}}
{"id": 62, "type": "party_disband", "ok": true, "data": {}}
{"type": "party_changed", "data": {"reason": "created", "party": { ... }}}
```

**队伍对象 `party`**（没有队伍时在所有出现的地方都是 `null`）：

```jsonc
{"kind": "clone", "leader": "勇者小明", "members": [
  {"index": 0, "name": "勇者小明", "leader": true, "clone": false, "level": 1,
   "hp": 44, "max_hp": 44, "mp": 100, "max_mp": 100,
   "pet": {"name": "乌力", "level": 5, "hp": 30, "max_hp": 30, "skills": ["攻击", "防御"]},
   "position": {"floor": 1006, "map_name": "萨姆吉尔的村长家", "x": 15, "y": 22},
   "stats": {"vital": 8, "strength": 4, "toughness": 4, "dexterity": 4},
   "derived": {"attack": 6, "defense": 6, "agility": 4},
   "equipment": [{"slot": 2, "name": "小木棒"}],
   "magic": [{"slot": 0, "name": "岩石撞击"}]},
  {"index": 1, "name": "勇者小明·分身1", "leader": false, "clone": true, "level": 1, "...": "字段同上"}
]}
```

- `kind` 目前只有 `clone`。`members` 队长在第一位，后面是分身，最多 5 项；`index` 是成员的稳定编号（队长 0，分身 1–4），客户端用它保持选中。
- `hp`/`mp` 是此刻的值（队长是角色的实时值，分身是自己的值）；`pet` 是出战宠物，没有出战宠物时为 `null`；`position` 所有成员都和队长一致。
- `stats`、`derived`、`equipment`（装备位 0–4 上的物品，`slot` 是装备位编号）、`magic`（精灵栏位）和 `pet.skills`（技能名，空栏位不列）是详情页用的快照。
- **分身的内容**：创建那一刻队长的等级、四维属性、HP/MP、装备、精灵、出战宠物及宠物状态的快照，之后不再跟随队长变化。**不复制**背包物品、金钱、任务进度。
  名字是 `队长名·分身N`（N 为 1–4）。分身不建账号、不写数据库，不能登录，不能被邀请、交易、私聊。

**`party`（查看类，世界中）**：回复当前队伍，没有队伍时 `party` 为 `null`。

**`party_clone`（操作类，世界中或战斗中）**：组成分身队，成功回复整支队伍，并向角色的**每条连接**推 `party_changed`（`reason` 为 `created`）。
- **战斗中组队（STAGE-189）**：战斗中照常成功，不回复 `IN_BATTLE`。分身按此刻队长的状态做，但队长这一场的经验还没结算，所以这支队伍标记为“战后重建”：战斗结束时服务端按结算后的队长重新做一次分身（等级、属性跟着队长），再恢复满血，并推 `party_changed`（`reason` 为 `battle_end`）。
- 创建时队长的分身队不能再邀请别人，也不能再创建第二支。
- **带自动战斗方案（STAGE-195）**：`data.auto_profile` 可选，是客户端「保存并应用」后正在生效的那份方案（客户端的 `AutoProfile` 序列化，**外加** `"version": 1`）。
  服务端只认下面这些字段，其余字段（出招延迟 `playback`、逃跑 `flee`、停止保护 `stop`、换宠 `swap`、物品与补气的细节等）一律忽略：
  - `version`：必须是 `1`；`target`：`combo`（缺省）或 `random`；
  - `player`：`act`（`attack` / `magic` / `guard`）、`magic_slot`（0–4）、`magic_name`（保存时的名字，对不上就退回普通攻击）；
  - `pet`：`act`（`attack` / `skill` / `guard`）、`skill_slot`（1–7，缺省 1）、`skill_name`；
  - `first`：`enabled`（缺省 false），`player` / `pet` 同上（只在第 1 回合用）；
  - `recover`：`player_heal`（缺省 true）、`player_heal_pct`（缺省 30）、`pet_heal`、`pet_heal_pct`、`player_mp`、`player_mp_pct`（服务端只认作解析，分身不补血宠物、不补气，见下）；
  - `capture`：`enabled`（缺省 false）、`level_min`（缺省 1）、`level_max`、`names`、`keep`（STAGE-264，缺省＝四项不限，格式同 10.5 的 `capture.keep`；`keep` 的类型或取值不对，整个方案按 `ignored` 处理）。`keep` 只是存进方案：分身目前不自己捕捉（见下面“不做”），所以它暂时不影响任何捕捉，要等分身捕捉做了才生效。
  缺的字段取客户端默认值。响应多一个字符串字段 `auto_profile`：
  - `applied`：方案收下了，之后每场战斗里分身按它出招（方案随队伍存在内存里，解散就丢；队长之后再改方案**不会**自动重推，要重新 `party_clone`）；
  - `ignored`：带了但不认（`version` 不是 1、JSON 不合法、字段类型不对、`target` / `act` 取了不认识的值）。**不报错**，请求照常成功，分身走默认的普通攻击。
  - 请求没带 `auto_profile` 时，响应里**不出现** `auto_profile` 字段。
  例：`{"id": 61, "type": "party_clone", "data": {"auto_profile": {"version": 1, "target": "combo", "player": {"act": "magic", "magic_slot": 0, "magic_name": "岩石撞击"}}}}` → `{"party": {...}, "auto_profile": "applied"}`。

- 已有队伍回复 `ALREADY_IN_PARTY`；角色倒下（`hp` 为 0）回复 `CHARACTER_DEAD`；
  观察方（agent 操控期间的玩家连接，或玩家操控时的 agent 连接）回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`。以上失败都不产生分身。
- 校验顺序：控制权 → 倒下 → 已有队伍。

**`party_disband`（操作类，世界中或战斗中）**：解散队伍，回复 `{}` 并推 `party_changed`（`reason` 为 `disbanded`，`party` 为 `null`）。分身立即从队伍里消失。
- **战斗中解散（STAGE-189）**：战斗中也照常成功，不回复 `IN_BATTLE`。正在打的这一场，分身是开战时的拷贝，仍然出场、打完为止（中途不会把它们从战场上拿掉）；战后不再有这些分身。想让下一场带新的分身，就在战斗中（或战后）再 `party_clone`。
- 战斗中解散与重组都不需要先关遇敌（遇敌开着时连场的下一场也一样）：服务端不再因为连场挡住队伍操作。

没有队伍回复 `NOT_IN_PARTY`；观察方被拒同上。

**推送 `party_changed`**：`data` 是 `{"reason": ..., "party": 队伍或 null}`，推给角色在世界中的每条连接，`party` 是权威的最新队伍。`reason`：

| reason | 时机 |
|---|---|
| `created` | `party_clone` 成功 |
| `disbanded` | `party_disband` 成功，`party` 为 `null` |
| `moved` | 队长的位置变了（`warp`、`use_exit`、NPC 传送、战败回城等）：紧跟在 `position_changed` 之后，所有成员同步到队长的位置 |
| `battle_end` | 一场战斗结束：紧跟在 `battle_end` 之后；分身战后自动恢复满血、满 MP，宠物也恢复满血 |

**消失条件**：队长解散、队长的**所有连接**都断开或退出、服务端重启，分身全部消失；队长重新登录后不保留分身。
同账号的 agent 连接还在线时，玩家连接断开不算（角色还在世界里，队伍保留）。`enter_world` 和 `status` 的响应带当前队伍（字段 `party`，没有时为 `null`），断线重连后客户端以它为准。

**位置**：分身跟随队长，没有自己的位置状态；队长移动、切图、传送时，成员的 `position` 就是队长的新位置。

**战斗**：队长遇敌或触发战斗时，分身和它们出战的宠物一起上场，占己方 1–4 号与 6–9 号格子（人 0–4、宠物 5–9，宠物号 = 主人号 + 5），
`battle_start` 的 `units` 里它们带 `"auto": true`（10.2）。分身由服务端自动出招，**不进 `battle_turn.needs`，也不需要玩家下指令**，队长（和宠物）交齐指令就结算。
没有方案时是人物攻击加宠物攻击，目标取合击顺序的第一个（STAGE-312）；组队时带了方案（`auto_profile` 为 `applied`）就按方案出招（见上面的 `auto_profile`，STAGE-195）：
- 目标：`combo`（缺省）取敌方前排从上到下、再后排（`ROW_ORDER` 的列序）第一个活着的，人物、分身和分身宠物打同一个，才打得出合击；`random` 在活着的敌人里随机。没有方案时一律按 `combo`。
- 自动捕捉回避：敌方有符合方案筛选（名字包含、等级在 `level_min`–`level_max` 之内、不超过队长等级 +5）的怪物时，所有分身和分身宠物一律防御，不出手。
- 人物补血：HP ≤ `player_heal_pct`% 时，按栏位从小到大找能用的治愈精灵（自愈 `heal` 或治疗他人 `heal_other`，气力够），找不到就照常出招（不卡回合）；分身没有背包，补血只用精灵，不用道具，也不补气。
- 首回合：`first.enabled` 时第 1 回合用首回合动作，之后回到常规动作。
- 常规动作：人物的精灵（栏位名字对得上、气力够、敌方一个 / 己方一个 / 不要目标的才用，复活和己方全体不用）、宠物的技能（栏位技能名字对得上，战斗技能）；用不了一律退回普通攻击，没有敌人就防御。
- 目标在出手时倒下，攻击类按合击顺序顺延到下一个还能打的敌人（STAGE-312；方案是 `random` 时仍随机改选）；治疗、防御和不要目标的指令不改选。
- 不做：分身逃跑（跟着队长走）、补气、战宠喂药、自动换宠、停止保护、出招延迟、分身自己捕捉。玩家给分身指定了目标（`clone:N`）的，仍按指定的打。
玩家想指挥的话，可以在回合里给每个能行动的分身和分身的宠物各指定一个攻击目标（STAGE-186）：`battle_turn.optional` 列出可以指定的分身，
`battle_command` 用 `actor: "clone:N"` / `"clone_pet:N"` 发出（10.4、10.5）；没指定、或来不及指定（回合超时）的，按合击顺序打第一个（STAGE-312，同没有方案时的默认）。
**分身不领奖励**：不拿经验、不掉落、不涨宠物忠诚、不计击杀数；奖励按队长单人身份结算，`battle_end.rewards` 里只有队长和队长的宠物。
分身倒下就在战场上倒下，战后恢复满血；分身倒下**不影响**胜负判定：队长和队长自己的宠物都倒下才算落败，分身还站着也输。
队长逃跑成功时分身和它们的宠物一起离场（`escape` 事件的 `left` 里有它们）。

- 错误码：`ALREADY_IN_PARTY`（已有队伍）、`NOT_IN_PARTY`（没有队伍可解散）、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。
- agent 的先问玩家分级见 `docs/agent-guide.md` 第 6 节：`party_clone`、`party_disband` 都是第二级（做完汇报）。
- 用例：`party_clone`（成功、各类失败、跟随移动、解散）、`party_battle`（分身同场战斗、奖励只给队长；战斗中解散再重组，战后分身等级跟着队长）、`party_agent`（agent 操控时的拦截与重连）。

### 5.58 NPC 菜单 v2：`npc_open`、`npc_confirm`、`npc_back`（操作类，世界中，STAGE-135）

菜单 v2 通过独立的 `npc_open` 启用，旧 `talk` 的买卖语义保留。三个请求都是操作类（观察方被拒，同 `talk`）：

- `npc_open {npc}`：打开 NPC 的菜单 v2，校验与 `talk` 一致，响应为空；窗口由 `npc_window` 推送，带 `window_id`，之后的动作都要带它。
- `npc_confirm {window_id, confirm}`：只用于商店报价。`confirm` 为 `false` 回到商品列表；为 `true` 核验并原子成交。报价 60 秒有效，单次消费，重复确认不成交。
- `npc_back {window_id}`：返回上一层，数量→商品，商品→买卖菜单，菜单→关闭；报价取消回到商品列表。其他 NPC 返回等同关闭。

`window_id` 过期或已被替换时回复 `WRONG_STATE`。窗口字段、交易字段和确认时的重验规则见 [NPC 菜单协议](npc-menu-v2.md)。agent 的先问玩家分级见 `docs/agent-guide.md` 第 6 节：`npc_open`、`npc_confirm` 归第二级，`npc_back` 归第一级。

### 5.59 `quest_npcs`：任务 NPC 查询（查看类，世界中，STAGE-136）

`quest_npcs {no}` 查看任务 `no` 的候选 NPC（玩家和 agent 都可以查）。响应 `{no, state, npcs: [{id, name, type, floor, map_name, x, y, here, relation}], reason}`。`relation` 为 `current`（当前实际命中的、关联该任务的开放脚本块）或 `reference`（脚本相关，不保证是下一步）。

- 不存在的任务回复 `BAD_REQUEST`；没有可靠关系时返回空数组和原因。
- 不暴露暗号和奖励条件。最终对话仍由 `npc_open` 按即时任务条件执行，客户端收到任务变化后必须让已打开的候选失效。
- 完整契约见 [任务 NPC 查询契约](protocol/cases/quest_npcs.md)。

### 5.60 宠物转生：精灵王（STAGE-236）

精灵王 NPC（`npc_transmigration`，显示类型「宠物转生」，地图 32021）是原版 `npc_transmigration.c` 的 PETTRANS 段：把 79 级玛蕾菲雅（718，或转过一次的 401）转成新的宠物。`talk` 它时按身上的玛蕾分流：

- 身上没有 718/401：回复 `NPC_NOT_TALKABLE`（人物转生本期不做）。
- 玩家不到 80 级：只弹一句文字窗口「由于你的等级小于80,没能力进行转宠！」，没有选项。
- 79 级玛蕾的四项资质有一项不到 15：文字窗口「操，你太不爱惜我了，还要我帮你转宠？没门！」。
- 身上有两只 79 级玛蕾：文字窗口「你怎会有那麽多玛雷菲雅呢？」。
- 恰好一只 79 级玛蕾：开始窗口（`npc_window`，选项 `1 下一步`）。

窗口的选项编号从 1 开始，每一步的选项如下（`npc_choose` 选中后，服务端先推 `npc_window_closed`（`CHOSEN`），再推下一个 `npc_window`）：

| 步骤 | 窗口正文（原版 ten.arg 的 PETTRANS 段） | 选项 | 选中后 |
| --- | --- | --- | --- |
| 开始 | 玛蕾菲雅：谢谢你让我和精灵王重聚，… | `1 下一步` | 资格检查 |
| 资格检查 | 通过：玛蕾菲雅：你的宠物已经准备好要转生了吗？ | `1 是`、`2 否` | 1 选宠；2 取消 |
| 资格检查 | 不通过（玩家不到 80 级，或事件 4、69、70 未完成）：玛蕾菲雅：你的宠物还无法转生喔！ | 没有（`npc_close` 关窗） | — |
| 选宠 | 玛蕾菲雅：请选出一只要接受转生的宠物吧。 | 每只随身宠物一项（`宠物名 Lv等级`，按栏位从小到大），最后一项 `取消` | 选宠物 → 确认；取消 → 取消 |
| 确认 | 是[宠物名]没错吧？ 玛蕾菲雅：你确定好了吗？… | `1 是`、`2 否` | 1 转生；2 取消 |
| 转生 | 玛蕾菲雅：宠物转生成功了！请好好对待它吧。… | 没有 | — |
| 取消 | 玛蕾菲雅：等你考虑清楚了以后再来吧！ | 没有 | — |

- 确认之后的检查（原版 case 4）：骑乘中的宠物「骑乘中的宠物不能转生喔！」；不到 80 级「接受转生祝福的宠物必需要等级80以上喔！」；本身是 718/401「这只宠物不能转生喔！」；转生次数超过 1「这只宠物已经接受过祝福了喔！」。这些都是一句提示，窗口没有选项，`npc_close` 关窗。
- 转生本身：在一个事务里删掉身上的玛蕾（只看随身栏位、79 级、转生次数为 0 的宠物找 718 或 401，转过一次的只找 401），按 STAGE-235 的重算更新被转生的宠物（等级 1、经验 0、资质和能力重算、血量补满、转生次数 +1、个体等级上限 -1）。宠物留在原来的栏位，不需要空位；名字、技能、起始值不变。同一个事务里还删掉背包和装备里编号在 19625–19632、19672–19687 的道具（灵石、灵项链，整格删，不看数量，没有也能转生；原版 `DelItem`），并把事件 71 设为进行中（任务 51 变成 `doing`）。成功后依次推 `pets_changed`、`inventory_changed`（删了道具才推）、`status_changed`（装备加成变了才推）、`quests_changed`（任务 51 进行中），再推「玛蕾菲雅：宠物转生成功了！…」窗口。
- 状态检查失败（找不到玛蕾、或宠物模板不认识）：原版直接返回，不发新窗口，也不改数据；客户端停在确认窗口上，玩家可以再选一次或选否（原版行为，见 `docs/pet-rebirth-quest.md` 第 8 节第 3 条）。
- 灵石不查数量：转生成功删 19625–19632、19672–19687 范围内的道具，身上没有也能转生（STAGE-237，见上面的转生本身）。
- 战斗中不能对话（同 `talk` 的其他 NPC）；窗口开着时进入战斗，窗口按 `BATTLE` 关闭。

### 5.61 `pet_talk`：和宠物对话（操作类，世界中，STAGE-239）

```jsonc
{"id": 42, "type": "pet_talk", "data": {"slot": 0}}
{"id": 42, "type": "pet_talk", "ok": true, "data": {"text": "这就是萨姆吉尔村啊！～～", "limit_changed": true}}
```

- 依据原版 `gmsv/char/pet_event.c` 的 `PET_Talkfunc`。原版是玩家面向宠物说话时触发的；新协议改成玩家主动请求，不靠聊天。选块、台词、`LimitLevel` 与 `AddItem` 的规则见 `docs/quest.md` 的“宠物对话”一节。
- `slot` 是宠物栏位（0–19 的整数），缺少或不对回复 `BAD_REQUEST`。
- 按下面的顺序检查，第一条不满足就回复对应错误，数据不变：
  1. 战斗中 `IN_BATTLE`；
  2. 栏位没有宠物 `NO_PET`；
  3. 宠物是休息（`rest`）或骑乘（`ride`）`WRONG_STATE`：原版只有在地图上的宠物（出战、待命）才会被玩家面向说话；休息和骑乘中的宠物不在地图上。
- 通过后，服务端按原版选出一句台词，返回 `ok: true`：
  - `text` 是宠物说的话。宠物没有对话表（原版没有 `pettalk` 文件）时 `text` 为空字符串，原版什么也不发生。
  - `limit_changed` 为 `true` 表示这次对话改写了宠物的个体等级上限（原版 `LimitLevel` 动作）。上限写库后推送 `pets_changed`，推送先于响应到达。上限不在 5.19 的宠物视图里，`pets_changed` 的内容可能和之前一样。
  - 台词的道具发放（原版 `AddItem`）按整块算：背包的空位给不下这一块要发的道具时，整块失败，`text` 为 `．．．．！`，`limit_changed` 为 `false`，上限不改，但仍回复 `ok: true`（原版 `PET_Talkfunc` 的同一句）。发成功时推送 `inventory_changed`。
- 没有命中任何台词时说 `OTHER` 里的一句（原版的兜底台词）。
- 原版的“陌生人不能互动”（`PET_Talkfunc` 里主人不是说话的人，说 `NoPlayerMsg`）在新协议里不会出现：宠物都在角色自己的栏位里，`slot` 指的一定是自己的宠物。
- 没有检查宠物和玩家的距离与朝向（原版要求面对面或宠物面向玩家）：新服务端不记录宠物的位置，宠物跟着角色走。
- 属于“操作”类：观察方发起回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同 `warp`。
- 错误码：`BAD_REQUEST`、`NO_PET`、`WRONG_STATE`、`IN_BATTLE`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。

### 5.62 `pet_encounter_catalog`：1 级宠物目录（查看类，世界中，STAGE-251）

请求 `{}`，全量一次返回，不分页。`pets` 是 1 级可遇的宠物（一只宠物 = 一个模板，同名模板各占一行），`spots` 是它们的可遇点。

```json
{"catalog_version":"f345a2eaedef",
 "pets":[{"pet_id":92,"name":"扬奇洛斯","aliases":["蓝人龙"],"series":"人龙系","atoms":{"earth":0,"water":30,"fire":70,"wind":0},"size":1,"catch_difficulty":3,"sort_key":"d1efc6e6c2e5cbb9","spot_count":1}],
 "spots":[{"pet_id":92,"area_id":"100-21-582-399-783-560","number":21,"floor":100,"map_name":"萨伊那斯","anchor":{"x":600,"y":550},"rect":{"x1":582,"y1":399,"x2":783,"y2":560},"max_enemies":3,"steps":26,"also_level1":["布鲁顿"]}]}
```

- `pets` 已按 `sort_key`（名字的 GBK 字节序，近似拼音序）排好，客户端不改序；同名的宠物按模板编号区分。
- `aliases`（民间俗称）、`series`（系列标签，只作搜索词）可能为空：空数组、空字符串，客户端按“有就显示”处理。`atoms` 只用于详情展示，不是分类依据。
- `spots` 的 `anchor` 是开战时的落点（区域生效的格，不是出口格）；`rect` 已裁到地图边界；`also_level1` 是同一区域其他 1 级宠物的名字；`steps` 是 CEP 期望步数（约多少格一场，取整）。`area_id` 与 5.50 的区域 `id` 相同。
- `catalog_version` 是数据指纹，随目录内容变化；客户端缓存后用它判断要不要重新取。目录只在世界中取（同 5.50）。

### 5.63 `pet_encounter_start`：按 1 级宠物开战或只传送（操作类，世界中，必须拥有控制权，STAGE-251）

请求 `{"pet_id":92,"area_id":"100-21-582-399-783-560","mode":"once"}`，`mode` 取：

- `once` / `infinite`：跨图定位到该可遇点的代表落点，**立刻**开第一场战斗（语义同 5.50 / 5.51：次数、停止、2 秒间隔、打输清计划，全部沿用）。
- `travel`：只定位到代表落点，不开战。这是本单加的第三种模式，任务书里的“只传送不遇敌”（`warp` 只能到地图默认落点，所以另做一个模式）；原有遇敌开关随之关闭，同 `warp`。

客户端不能给坐标；地图与落点都由服务端从目录和区域数据解析。`pet_id`（整数）必填，缺省回复 `BAD_REQUEST`。

一次角色操作锁内检查，顺序如下（全部在改动位置之前完成，失败没有任何副作用）：控制权（`CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`）→ 非战斗（`IN_BATTLE`）→ 目录里有这只宠物在这个区域的可遇点（`PET_ENCOUNTER_NOT_FOUND`，也包括宠物编号不存在）→ 区域当前可用（`ENCOUNTER_AREA_UNAVAILABLE`，message 说明原因）→ 区域当前的可遇敌人里有这只宠物（道具条件变了则 `ENCOUNTER_AREA_UNAVAILABLE`）→ 目录落点与服务端的代表落点一致（否则 `ENCOUNTER_AREA_UNAVAILABLE`，提示重新取目录）。

成功时的推送顺序固定：

- `once` / `infinite`：`position_changed`（可以换地图）→ `battle_start` → `battle_turn` → `encounter_changed` → 响应。响应里的状态字段与 5.50 的 `encounter_changed` 相同（`encounter`、`encounter_area_id`、`encounter_area_number`、`encounter_mode`、`encounter_continues`）。
- `travel`：`position_changed` →（遇敌开关开着时）`encounter_changed` `reason: "warp"` → 响应。

错误码：`BAD_REQUEST`（参数或 mode 错）、`PET_ENCOUNTER_NOT_FOUND`、`ENCOUNTER_AREA_UNAVAILABLE`、`IN_BATTLE`、`CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`。
协议用例见 `protocol/cases/pet_encounter*.jsonl`。

### 5.64 美容师：花石币买魅力（STAGE-255）

和美容师 NPC（5.32 的 `type` 是「美容师」，原版 `npcgen_charm`，原版 `gmsv/npc/npc_charm.c`）
`talk` 弹三步窗口，全部用 `npc_window` / `npc_choose` / `npc_window_closed`，没有新的消息：

1. **第一步**：正文是「美容师 / 欢迎光临，今天要做什麽呢？ / 《　给我魅力　》 / 《 什麽也不做 》」，
   选项 `1 给我魅力`、`2 什么也不做`。选 2 关窗（`CHOSEN`），什么都不变。
2. **报价**：选 1 先推 `npc_window_closed`（`CHOSEN`），再推报价窗口，正文含「要将你的魅力上升五点的话」
   和「需要 N 的 stone唷！」，选项 `1 是`、`2 否`。选 2 关窗，什么都不变。
3. **结果**：选 1 时扣 N 石币，人物魅力（基础值，`charm`）+5、封顶 100，然后推 `status_changed`
   （`derived.charm` 和 `gold` 变了）、`pets_changed`（随身宠物的 `loyalty` 按新魅力现算），
   再推结果窗口（没有选项，正文按人物形象：第一行「嗯,这样你的魅力就上升罗！」，第二行按
   `appearance.body`：0、6 儿童「真的变得好可爱唷！」，1–3 男孩「是不是比刚才要帅多了呢？」，
   4–5 男人「变得更酷了呢！」，7–9 女孩「愈来愈可爱了呢！」，10–11 女人「可真是变得愈来愈美了呢！」）。
   结果窗口任意选择（`npc_choose`）都只关窗（`CHOSEN`）；关窗后再选回 `WRONG_STATE`。

- **费用**（原版 `NPC_CharmCost`，整数运算）：`N = 等级 × 10 × ⌊魅力 / 3⌋ × (转生次数 + 1)`。
  魅力按**基础值**算（不含装备），魅力 0 和 1 按 3 算（原版只改 0 和 1，所以魅力 2 的价钱是 0，照搬）。
  例：20 级、魅力 60、未转生 = 4000；转生 1 次翻倍。
- **魅力已满**（基础值 ≥ 100）：第一步选 1 直接给结果窗口，正文「你的魅力真是完美 / 请永远保持你的魅力唷。」，
  不报价、不扣钱。
- **石币不够**：选「是」回复 `NOT_ENOUGH_GOLD`（正文「哎呀!你的钱不够唷！先去存好钱下次再来吧！」），
  不扣钱、不加魅力，窗口留着（和护士一致，原版是 OK 窗口，这里改成错误回复）。
- **差异**：原版要求面对面（`isFaceToFace(2)`）才弹窗，新服务端的 NPC 交互不做距离和朝向（同护士、商店）。
  原版把报价里的 `%6d` 右对齐补空格，这里直接写数字。
- 寄放的宠物也跟着现算忠诚（宠物视图对所有宠物现算，见 5.19）；原版只重算随身的 5 只，这一点没有照搬。
- 战斗中回复 `IN_BATTLE`；观察方被拒（操作类）。魅力的增减途径和 0–100 钳制见 `docs/quest.md`「魅力」。

### 5.65 人物转生：主城精灵王（STAGE-266）

四个主城（萨姆吉尔 1000、玛丽娜丝 2000、加加 3000、卡鲁它那 4000）存档点旁各有一个精灵王（5.32 的 `type` 是「转生」，
名字「精灵王」，原版 `gmsv/npc/npc_transmigration.c` 的 TENSEI 段、`data/npc/777/ten.arg`）。它和漆黑洞窟 20 楼的宠物转生精灵王（5.60）
是两个不同的 NPC 类型，互不影响。`talk` 弹窗口，全部用 `npc_window` / `npc_choose` / `npc_window_closed`，没有新的消息。

**资格**（`talk` 时按顺序判断，只弹一句文字窗口、没有选项、数据不变）：

1. 转生次数 ≥ 5：「你好像已经把最强的力量 / 给激发出来了 / 若再进行转生的话会有危险啊。」
2. 等级 < 80：「转生所需要的条件 / 你好像还没有达成哦。 / 等你达成之后，再来我这里。」
3. 通过：进入窗口流。**不检查**原版的事件 39/40/42/46 和守护兽 693–696。

**窗口流**：

1. 开始：「你已经具备接受转生仪式的资格了。 / 那我就如你所愿，来进行转生的仪式吧。」选项 `1 下一步`。
2. 确认：正文是原版 `mainmsg`（「转生是把你最强的力量给激发出来……你下定决心要进行转生了吗？」），选项 `1 确定`、`2 取消`。
   选 2 回「这样子吗，如果你想转生的话，欢迎随时来我这里。」（选项 `1 确定`，关窗），什么都不变。
3. 执行：选 1 时完成转生（见下），推送顺序 `status_changed` → `pets_changed` → `quests_changed`（任务进度有变化时）→
   `notice`（「拿到X。」）→ 结果窗口 `yesmsg1`（选项 `1 下一步`）；再选 1 推 `yesmsg2`（选项 `1 确定`）；再选 1 关窗。
   每一步窗口切换前都先推 `npc_window_closed`（`CHOSEN`）；关窗后再选回 `WRONG_STATE`。台词按转生次数 +1 取 TENSEI 第 n 段。

**转生执行**（单个事务，全部成功或全部回滚）：

- 转生次数 +1；等级 1、经验 0；HP 按新的最大 HP 补满（MP 不动）；骑着的宠物下骑。
- 四项能力重算：`ans = 截断(总点数/12 + 累计任务数/4 + (累计等级 − 转生次数×85)/4)`，总点数是转生前四项之和（不含未分配点），
  转生次数已 +1，累计值先累加再代入；四项按各自占比分配 `ans`，四舍五入到整点、负数归 0（原版是 ×100 定点，每项最多差 0.5 点）。
  累计任务数每次固定 +20（**不读任务状态**，有意与原版不同），累计等级每次 +min(当前等级, 130)；两个累计值存在角色上，不下发。
- `skill_points` 覆盖成转生次数 × 10（转生前没分配的点数会丢，照原版）。
- 事件清空：清空全部任务与事件进度，只恢复原本已完成的 69/70/71/72/109/118/122/127/131/158/159 和原本进行中的 69/70/71/72，
  再按新的转生次数把 `48`–`52` 的前 n 个设为已完成。
- 赠一只 1 级宠物，放进第一个空栏位，按所在主城和新的转生次数：

  | 主城 | 1 转 | 2 转 | 3 转 | 4 转 | 5 转 |
  | --- | --- | --- | --- | --- | --- |
  | 萨姆吉尔 | 乌力斯坦 | 洛卡伦恩 | 多鲁宝 | 奇娜 | 帖拉所伊朵 |
  | 玛丽娜丝 | 洛卡伦恩 | 多鲁宝 | 奇娜 | 乌力斯坦 | 帖拉所伊朵 |
  | 加加 | 多鲁宝 | 奇娜 | 乌力斯坦 | 洛卡伦恩 | 帖拉所伊朵 |
  | 卡鲁它那 | 奇娜 | 乌力斯坦 | 洛卡伦恩 | 多鲁宝 | 帖拉所伊朵 |

- **宠物栏 20 格全满**：整次转生拒绝，弹文字窗口「你的宠物栏满了，先腾出一个位置再来。」，不推 `status_changed` / `pets_changed`，什么数据都不改
  （原版会出现数据已改但送宠失败的半成品，不照搬）。
- **差异**：不删守护兽、不传送回村长家、不复位外观；`startmsg` 去掉了「打败帖拉所依朵」那句。用例见
  `world_tensei_{lowlevel,maxed,cancel,done,petsfull}`；战斗中 `talk` / `npc_choose` 的限制同其他 NPC，转生窗口开着时进入战斗按 `BATTLE` 关窗。

### 5.66 `players`：附近玩家（查看类，世界中，STAGE-273）与推送 `players_changed`

“附近”指同一张地图（同 `floor`），不给距离和坐标。玩家和 agent 都能发，观察方也能查看。详细规则见 `docs/social-players.md` 第一节（设计依据）。

```jsonc
{"id": 80, "type": "players", "data": {}}
{"id": 80, "type": "players", "ok": true, "data": {"floor": 1000, "truncated": false, "players": [
  {"name": "阿牛", "level": 30, "transmigration": 1, "title": "", "in_battle": true, "duel": true, "party": null},
  {"name": "小红", "level": 12, "transmigration": 0, "title": "", "in_battle": false, "duel": false, "party": null}
]}}
```

- 列表只含同图、已进入世界的**其他**角色（不含自己；分身不是角色，不出现）。agent 操控中的角色照常出现。
- 每项只有：`name`（角色名，全服唯一）、`level`、`transmigration`、`title`（本期固定 `""`）、`in_battle`（是否战斗中）、`duel`（是否接受决斗，即 5.69 的 `duel` 开关，STAGE-306；隐身者不出现）、`party`（没有真人队时为 `null`，有时为 `{"leader": 队长名, "size": 人数, "max": 5}`，见 5.67；分身队不对外显示；隐身的队员不计入 `size`，隐身的队长写 `leader: null`，见 5.69）。不含账号名、HP/MP、石币、装备、宠物、背包、任务。
- 排序：有队伍的在前（队长排最前），其次等级从高到低，再按名字的 UTF-8 字节序。同队的人排在一起。
- 最多返回 50 人；超出时 `truncated: true`。没有其他玩家时返回空数组 `"players": []`，不报错。
- 只读取数据，不改变角色；不属于操作类，所以观察方也能发，不返回 `CONTROLLED_BY_*`。

推送 `players_changed`：推给**这张地图上所有在世界中的连接**（玩家和 agent，规则同 `npcs_changed`），增量而非整表：

```jsonc
{"type": "players_changed", "data": {"floor": 1000, "changes": [
  {"op": "enter",  "player": {"name": "小明", "level": 5, "transmigration": 0, "title": "", "in_battle": false, "party": null}},
  {"op": "update", "player": {"name": "阿牛", "level": 31, "transmigration": 1, "title": "", "in_battle": false, "party": null}},
  {"op": "leave",  "name": "小红"}
]}}
```

- `enter` 与 `update` 的语义相同：`player` 是这名玩家的整条数据，客户端按 `name` 替换；列表里没有这个名字时就新增。`leave` 按 `name` 删除，列表里没有也忽略。客户端遇到不认识的 `op` 忽略该项。
- 触发：进入世界（`enter`）；离开世界、断线（`leave`，原地图）；换图（旧图 `leave`，新图 `enter`，与 `position_changed` 同时出现）；升级、转生、战斗开始或结束（`update`，`in_battle` 或等级变了）；组队、离队、队长变化、被请出（`update`，`party` 变了，见 5.67）。
- **合并发送**：同一张地图的变化攒 1 秒再发一次（一条推送带多项 `changes`）；同一玩家在窗口内多次变化只留最后状态。窗口内先进入又离开的玩家按 `leave` 发送。
- 推送不发给玩家自己；只有同图别人有变化时才推。
- 推送可能丢失或晚到：客户端在进入世界、换图、重连后以一次 `players` 的整表为准。
- 称号本期固定为 `""`，以后有变化时再开推送。`party` 的变化见上面的触发条件。

### 5.67 真人组队：`party_invite` / `party_join` / `party_reply` / `party_cancel` / `party_leave` / `party_kick` 与推送 `party_request`、`party_request_result`（STAGE-274）

一队最多 5 个真人，与分身队互斥。规则依据 `docs/social-players.md` 第二节（2.6 战斗不在本期，见 STAGE-275）。

**配置**：服务端配置 `party.players_enabled`，默认 `false`（配置位置见 `docs/server.md`）。关闭时，本节的六个请求和 `party_leader`（STAGE-277）一律回复 `WRONG_STATE`（`message` 写“真人组队尚未开放”），不推送；`party_disband` 与分身队不受影响。

**请求（操作类，世界中）**：玩家连接和 agent 都能发；agent 操控或玩家观察时按 `CONTROLLED_*` 拦截（同分身队）。

| 请求 | data | 谁能发 | 成功回复 |
|---|---|---|---|
| `party_invite` | `{"name": 对方角色名}` | 没有队伍者，或真人队的队长 | `{"request": 请求对象}` |
| `party_join` | `{"name": 对方角色名}` | 没有队伍者（name 必须是对方队长） | `{"request": 请求对象}` |
| `party_reply` | `{"id": 请求 id, "accept": true/false}` | 请求的对象 | 接受：`{"party": 队伍}`；拒绝：`{}` |
| `party_cancel` | `{"id": 请求 id}` | 发起人 | `{}` |
| `party_leave` | `{}` | 任何队员（含队长） | `{}` |
| `party_kick` | `{"name": 队员名}` | 队长 | `{}` |
| `party_leader` | `{"name": 队员名}` | 队长 | `{}` |

```jsonc
{"id": 20, "type": "party_invite", "data": {"name": "路人乙"}}
{"id": 20, "type": "party_invite", "ok": true, "data": {"request": {"id": "r-17", "kind": "invite", "from": "勇者小明", "to": "路人乙", "expires_in": 30}}}
{"id": 21, "type": "party_reply", "data": {"id": "r-17", "accept": true}}
{"id": 21, "type": "party_reply", "ok": true, "data": {"party": { ... }}}
```

**请求对象**（`request`）：`id`（字符串，服务端生成）、`kind`（`invite` 邀请 / `apply` 申请）、`from`（发起人）、`to`（对象）、`expires_in`（固定 `30`，秒）。推送给对象时另带 `party`：请求相关队伍的摘要 `{"leader": 队长名, "size": 人数, "max": 5}`，没有队伍时为 `null`（邀请时是邀请者的队伍，申请时是对方的队伍）。发起人的请求只由响应带出，不在 `status` 里返回。

**校验顺序**（第一个不满足的回复对应错误码，请求不建立）：

- `party_invite`：控制权 → 自己倒下（`CHARACTER_DEAD`）→ 自己战斗中（`IN_BATTLE`）→ 自己队伍战斗中（`PARTY_IN_BATTLE`）→ 对方存在、在线且可见（`PLAYER_NOT_FOUND`，隐身者视同不存在，见 5.69）→ 同图（`PLAYER_NOT_NEARBY`）→ 不是自己（`INVALID_TARGET`）→ 对方不接收组队邀请（`TARGET_REFUSES`，见 5.69）→ 对方战斗中（`TARGET_IN_BATTLE`）→ 对方已有队伍（`TARGET_IN_PARTY`）→ 自己是队员（`NOT_PARTY_LEADER`）→ 自己队伍已满（`PARTY_FULL`）→ 自己有分身队（`ALREADY_IN_PARTY`）→ 同一对已有待处理请求（`REQUEST_PENDING`）→ 对方待处理请求已满 5 条（`TARGET_BUSY`）→ 同一发起人对同一对象 10 秒内再发（`RATE_LIMITED`，带 `retry_after_seconds`）。`TARGET_REFUSES`（对方关闭了“接收组队邀请”，只对邀请有效，见 5.69）本期返回。
- `party_join`：同上，但不做 `TARGET_IN_PARTY` 与 `NOT_PARTY_LEADER` 的检查；对方不是队长或没有队伍，都回复 `TARGET_NOT_LEADER`；对方队伍已满回复 `PARTY_FULL`；自己有任何队伍回复 `ALREADY_IN_PARTY`。
- `party_reply`：请求不存在、已结束，或答复者不是请求的对象，回复 `REQUEST_NOT_FOUND`。**拒绝**到此为止。
- `party_reply` 接受时**重新校验**：发起人还在线（否则 `PLAYER_NOT_FOUND`）；邀请时发起人仍是队长或无队伍（否则 `NOT_PARTY_LEADER` / `ALREADY_IN_PARTY`）；申请时答复者仍是队长（`NOT_PARTY_LEADER`）；队伍没满（`PARTY_FULL`）；答复者仍无队伍（`ALREADY_IN_PARTY`）；双方和队伍都不在战斗中（`IN_BATTLE` / `TARGET_IN_BATTLE` / `PARTY_IN_BATTLE`）。校验失败时请求作废，发起人收到 `party_request_result`（`cancelled`，并带 `code` 写明原因）。
- `party_cancel`：请求不存在或已结束回复 `REQUEST_NOT_FOUND`。
- `party_leave`：没有队伍回复 `NOT_IN_PARTY`（分身队用 `party_disband` 解散）。
- `party_kick`：没有队伍回复 `NOT_IN_PARTY`；不是队长回复 `NOT_PARTY_LEADER`；目标不是队员回复 `NOT_PARTY_MEMBER`；踢自己回复 `INVALID_TARGET`（离队用 `party_leave`）。
- `party_leader`（转让队长，操作类）：没有队伍回复 `NOT_IN_PARTY`；不是队长回复 `NOT_PARTY_LEADER`；队伍战斗中回复 `PARTY_IN_BATTLE`；对象是自己回复 `INVALID_TARGET`（队长不能转给自己）；对象不是队员回复 `NOT_PARTY_MEMBER`。全部通过后：新队长排到 `index` 0，原队长排到第 1 位，其余队员相对顺序不变（不是重新按入队先后排）；发起人先收到 `party_changed`（`leader_changed`）再收到响应 `{}`；全体队员收到 `party_changed`（`leader_changed`，`who` 为新队长）；同图玩家收到 `players_changed`（party 摘要里的队长变了）。转让不改位置，不影响未结束的请求。
- `party_disband` 对真人队：只有队长能解散，队员回复 `NOT_PARTY_LEADER`；解散后全体收到 `party_changed`（`disbanded`）。

**推送 `party_request`**：只推给对象在世界中的每条连接，`data` 为 `{"request": 请求对象}`。

```jsonc
{"type": "party_request", "data": {"request": {"id": "r-17", "kind": "invite", "from": "勇者小明", "to": "路人乙", "party": null, "expires_in": 30}}}
```

**推送 `party_request_result`**：`data` 为 `{"id", "kind", "status", "from", "to"}`，`status` 取值：

| `status` | 含义 | 推给 |
|---|---|---|
| `accepted` | 对方接受 | 发起人 |
| `declined` | 对方拒绝（不说明原因） | 发起人 |
| `expired` | 30 秒到期 | 双方（在线的） |
| `cancelled` | 发起人撤回、接受校验失败、任一方离开世界或断线 | 双方（在线的）；校验失败时多一个 `code` 字段 |

- 接受时的顺序：发起人先收到 `party_request_result`（`accepted`），再收到 `party_changed`；答复人先收到 `party_changed`，再收到响应。
- 到期由服务端定时器扫描，不为每条请求建单独的定时器。

**接受时的位置**：答复者与队长不在同一张地图时，服务端把答复者传送到队长的位置（与跟随相同，`position_changed` 带 `reason: "follow"`）；答复者正开着 NPC 窗口则先推 `npc_window_closed`。

**真人队的 `party` 对象**（`enter_world`、`status`、`party`、`party_changed` 通用）：沿用 5.57 的外层结构，`kind` 为 `players`，成员条目少给：

```jsonc
{"kind": "players", "leader": "勇者小明", "members": [
  {"index": 0, "name": "勇者小明", "leader": true, "clone": false, "online": true,
   "level": 1, "hp": 44, "max_hp": 44, "mp": 100, "max_mp": 100,
   "pet": {"name": "乌力", "level": 5, "hp": 36, "max_hp": 36}, "position": {"floor": 1006, "map_name": "萨姆吉尔的村长家"}},
  {"index": 1, "name": "路人乙", "leader": false, "clone": false, "online": true,
   "level": 1, "hp": 44, "max_hp": 44, "mp": 100, "max_mp": 100, "pet": null, "position": {"floor": 1006, "map_name": "萨姆吉尔的村长家"}}
]}
```

- 不给队员的属性、装备、精灵技能（`stats`、`derived`、`equipment`、`magic`、`pet.skills`），只有分身队才有。没有出战宠物时 `pet` 为 `null`。`position` 不给坐标，全员与队长一致。
- `index`：队长永远是 `0`；队员离队或被请出后，剩余成员按入队先后重新编号；转让队长时只换新队长和原队长的位置（新队长 0、原队长 1）。客户端用 `name` 保持选中。
- `online` 本期总是 `true`（离线即离队，见下）。

**队伍变化 `party_changed`**（真人队的变化带 `who`，分身队的沿用 5.57 不带）：

| `reason` | 发给 | `party` | `who` |
|---|---|---|---|
| `created` | 建队的两人的所有连接 | 新队伍 | 答复者 |
| `member_joined` | 队内所有连接，包括新人 | 新队伍 | 新加入的人 |
| `member_left` | 剩下的队员 | 新队伍 | 离队者 |
| `leader_changed` | 队长离队时剩下的队员；转让时全体队员（含原队长） | 新队伍 | 新队长 |
| `member_kicked` | 剩下的队员 | 新队伍 | 被请出者 |
| `kicked` | 被请出者 | `null` | 被请出者 |
| `left` | 离队者 | `null` | 离队者 |
| `disbanded` | 解散时的全体成员 | `null` | 解散的人（队长解散时是队长；只剩 1 人自动解散时是离队 / 被请出的人） |
| `moved` | 全体队员（队长位置变化，见下） | 新队伍 | 队长 |

- 离队或被请出后只剩 1 人，队伍自动解散：剩下的人只收到 `disbanded`，不再收到 `member_left` / `member_kicked`。
- 队长离队且剩 2 人以上：剩下的人先收到 `member_left`，再收到 `leader_changed`；新队长是入队最早的剩余成员。
- 断线：成员的所有连接都断开（同账号 agent 连接还在线不算）即离队，按 `party_leave` 处理；其待处理请求按 `cancelled` 结束。

**跟随（2.5）**：队长的位置变化（`warp`、`use_exit`、NPC 传送、`encounter_start`、`pet_encounter_start`、战败回城等）后，每个队员：先推 `npc_window_closed`（若开着 NPC 窗口），再推 `position_changed`（带 `"reason": "follow"`；其他原因的 `position_changed` 不带该字段），紧跟 `party_changed`（`moved`）。队伍战斗中位置不变（见 2.6，本期不会发生）。

**队员的换图和遇敌请求**回复 `PARTY_FOLLOWER`（`message` 写“跟随队长，离队后才能自己行动”）：`warp`、`use_exit`、`encounter_start`、`pet_encounter_start`（含 `mode: travel`）、`encounter`（遇敌开关）、传送员 NPC 的传送选项。队员仍可以对话、买卖、用道具、整理背包、查任务等原地操作。

**错误码**（真人组队新增 14 个，下表；其余沿用）：

| 错误码 | 含义 |
|---|---|
| `PLAYER_NOT_FOUND` | 没有这个在线角色；隐身的角色也回这个（5.69） |
| `PLAYER_NOT_NEARBY` | 不在同一张地图 |
| `TARGET_IN_BATTLE` | 对方在战斗中 |
| `TARGET_IN_PARTY` | 对方已有队伍（只用于邀请） |
| `TARGET_NOT_LEADER` | 申请的对象不是队长，或没有队伍 |
| `TARGET_BUSY` | 对方待处理的请求已满 5 条 |
| `TARGET_REFUSES` | 对方关闭了“接收组队邀请”（5.69） |
| `PARTY_FULL` | 队伍已满 5 人 |
| `NOT_PARTY_LEADER` | 需要队长身份 |
| `NOT_PARTY_MEMBER` | 目标不是队员 |
| `PARTY_IN_BATTLE` | 队伍战斗中不能变更 |
| `PARTY_FOLLOWER` | 队员不能自己换图、遇敌 |
| `REQUEST_NOT_FOUND` | 请求不存在、已结束，或不是你的 |
| `REQUEST_PENDING` | 已有一条待处理的同类请求 |

沿用：`ALREADY_IN_PARTY`、`NOT_IN_PARTY`、`CHARACTER_DEAD`、`IN_BATTLE`、`INVALID_TARGET`、`RATE_LIMITED`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`WRONG_STATE`（功能关闭时）。

**存储**：队伍和请求都只在内存里，服务重启全部消失（与分身队一致）。

**用例**：`party_players_invite`（两个账号：申请被拒、建队、重复邀请、拒绝、撤回、频率、离队自动解散、解散后组分身队与真人邀请互斥）、`party_players_errors`（单账号：各类错误码）。转让队长的用例是 `party_leader_transfer`（转让成功）和 `party_leader_errors`（各错误码），STAGE-277 服务端实现前不在 `enabledStages` 里；跟随、踢人、队长接任（离队顺位）、过期、功能关闭的用例尚未写，见 `docs/social-players.md` 第二节。

### 5.68 `chat`：公开聊天与队伍聊天（聊天类，世界中，STAGE-278、STAGE-277）与推送 `chat_message`

公开聊天有两个频道：`map`（本图）和 `world`（世界），所有人都能看到。`party`（队伍，STAGE-277）只发给本队的真人成员，复用本节的请求与推送，规则见下文「队伍频道」。设计依据与界面见 `docs/chat.md`。

```jsonc
{"id": 90, "type": "chat", "data": {"channel": "map", "text": "有人去加加村吗"}}
// 推送先于响应到达（同 control_changed）；接收者见下文
{"type": "chat_message", "data": {"channel": "map", "from": "阿牛", "level": 30, "text": "有人去加加村吗", "time_ms": 1760000000000}}
{"id": 90, "type": "chat", "ok": true, "data": {}}
```

- **谁能发**：玩家连接和 agent 连接都能发，前提是账号的角色在世界中（同 `players`，战斗中也能发）。不受控制权限制，观察方也能发，不返回 `CONTROLLED_BY_*`。agent 在 `map`、`world` 的公开发言属于第三级，要先问玩家；`party` 队伍频道属于第二级，做完汇报（`agent-guide.md` 第 6 节）。
- **请求字段**：
  - `channel`：`map`、`world` 或 `party`，省略时为 `map`。其他取值回复 `BAD_REQUEST`。`party` 要求发送者在队伍里（见下文）。
  - `text`：去掉首尾空白后 1–70 个 UTF-8 字符（中英文都算 1 个），不能含控制字符（换行、制表等）。
- **检查顺序**（与第 4 节一致）：先 `WRONG_STATE`（不在世界中），再 `BAD_REQUEST`（频道、文本），再 `CHAT_MUTED`；`party` 查 `NOT_IN_PARTY`，`world` 查 `CHAT_LEVEL_LOW`，最后 `RATE_LIMITED`。被前面拒绝的请求不影响后面的计时。
- **响应**：`ok: true`，`data` 为空对象，只表示服务端收下了。响应里不回显文字；发送者要看自己说的话，以推送为准。客户端不要自己回显。
- **推送 `chat_message`**：`from` 是角色名，`level` 是发出时的等级，`time_ms` 是服务端发出的时间。不带账号，也不带实际发出者（`by`），实际发出者只写在审计里（9.6）。
- **接收者**（推送即时发出，不合并，不补发历史；重连、换图、进入世界都不补发）：
  - `map`：同一张地图（同 `floor`）上、在世界中的所有角色的**玩家连接**；
  - `world`：全服在世界中的所有角色的**玩家连接**；
  - `party`：发送者所在队伍里**真人成员**的玩家连接（含发送者自己），与队伍在哪张图无关；分身队的成员没有连接，只推给队长本人的连接。
  - 发送者是玩家连接时也收到推送；发送者是 agent 时，agent 自己不收推送。
  - **agent 连接收不到任何公开聊天和队伍聊天**（有意）：公开文字谁都能写，进了 agent 的上下文就是提示词注入的入口。agent 要知道玩家在说什么，仍用 `agent_say` 私聊（9.5）。

**限流、重复与屏蔽**（计时按角色算，不按连接算）：

- `map` 每个角色每 1 秒 1 条（`chat.map_interval_ms`）；`world` 每个角色每 30 秒 1 条（`chat.world_cooldown_seconds`）；`party` 与 `map` 相同，每个角色每 1 秒 1 条，沿用 `chat.map_interval_ms`，不另开配置。三个频道各自计时。超出时回复 `RATE_LIMITED`，`error.data.retry_after_seconds` 是剩余秒数（向上取整）。计时只从成功推送的那条开始算，被拒绝或被静默丢弃的都不算。
- **重复静默丢弃**：同一角色、同一频道，10 秒内发出与上一条成功推送相同的文字（替换屏蔽词、去掉首尾空白之后比较）：响应照常 `ok: true`，不推送，不报错，所以刷屏的人看不出被拦。
- **屏蔽词**：词表由 `.env` 的 `STONEAGE_CHAT_FILTER_FILE` 指定（没配则用 `server/gamedata/chat_filter.txt`），规则见 `docs/chat.md` 1.3：匹配前归一化（全角转半角、大小写、丢掉空白标点符号零宽字符），命中的原文整段换成等长的 `*`，消息照常发出，响应不提示。

**禁言**：管理员用 `stoneage-admin mute` 禁言后，禁言期内公开频道（`map`、`world`、`party`）回复 `CHAT_MUTED`，`error.data.until_ms` 是解除时间（毫秒时间戳）。禁言不影响 `agent_say` 私聊。

**队伍频道**（`party`，STAGE-277）：只在队伍里能用，和公开频道的区别如下：

- 接收者只有本队真人成员的玩家连接，同图但不在队里的玩家收不到；发送者自己也收到推送。
- 不检查等级（没有 `CHAT_LEVEL_LOW`）；文本、重复、屏蔽词、禁言与 `map` 相同；战斗中照常能发。
- 没有历史：入队前的队聊不补发；离队或被请出的人从离开那一刻起收不到；队伍解散后全部停止。
- 分身队不能发（分身没有连接，只发给自己没有意义）：只有在真人队里才能发，分身队和没有队伍一样回复 `NOT_IN_PARTY`。规则见 `docs/chat.md` 1.5。

**错误码**：

| 错误码 | 含义 |
|---|---|
| `BAD_REQUEST` | 文本为空（或只有空白）、超过 70 个字符、含控制字符；`channel` 不认识 |
| `NOT_IN_PARTY` | `channel` 为 `party` 但没有队伍（同 5.67） |
| `WRONG_STATE` | 账号的角色不在世界中 |
| `CHAT_MUTED` | 被管理员禁言；`error.data.until_ms` 为解除时间 |
| `CHAT_LEVEL_LOW` | `world` 频道的等级不够；`error.data.min_level` 为要求的等级（默认 5） |
| `RATE_LIMITED` | 频率超限；`error.data.retry_after_seconds` 为多少秒后可以再发 |

配置是全服设置，见 `docs/server.md`（`chat.world_min_level`、`chat.world_cooldown_seconds`、`chat.map_interval_ms`）。用例见 `docs/protocol/cases/chat_*.jsonl`；队伍频道的用例见 `docs/protocol/cases/party_chat.jsonl`（STAGE-277）。

### 5.69 `social_settings`：社交开关（操作类，世界中，STAGE-276）与推送 `social_changed`

三个开关都是角色自己的设置，立即生效：**隐身**（`visible`，别人看不见自己）、**拒绝邀请**（`invite`，不接收别人的组队邀请）和**接收交易邀请**（`trade`，STAGE-296，`false` 时别人邀请不了自己交易，规则见 `docs/trade.md` 3.4、5.3）。前两个的规则依据 `docs/social-players.md` 第三节第 4 项。

```jsonc
{"id": 81, "type": "social_settings", "data": {"visible": false}}
// 推送先于响应到达（同 encounter_changed）
{"type": "social_changed", "data": {"social": {"visible": false, "invite": true}}}
{"id": 81, "type": "social_settings", "ok": true, "data": {"social": {"visible": false, "invite": true}}}
```

- **请求字段**：`visible`、`invite`、`trade`、`duel`（STAGE-306），都是布尔值，都可以省略（省略的不改）；四个都省略时只回当前值。其他字段，或值不是布尔，回复 `BAD_REQUEST`（拼错字段名时不静默失效）。
- **响应**：`data.social` 是改完之后的完整值，`{"visible": bool, "invite": bool, "trade": bool, "duel": bool}`。
- **推送 `social_changed`**：只在值真的变了时推，发给该角色的玩家连接和 agent 连接（同 5.30），`data.social` 同上。
- **默认值**：`visible`、`invite`、`trade` 都是 `true`（都开）。存在角色上，重新登录、断线重连后保留，删除角色时一并删除。`duel`（STAGE-306）默认 `false`，不存库：进入世界（登录、断线重连）时重置为 `false`，每名人物打完一场决斗（`kind` 为 `duel`）离开战斗时自动改回 `false`，见 5.74 与 `docs/pk.md` 3.1。
- **生效**：立即生效，战斗中也能改。`party.players_enabled` 关闭时（5.67）两个开关照常可以设置，只影响它们的效果范围；`trade` 不受它影响。
- **谁能发**：玩家连接和 agent 连接，属于操作类；观察方和 agent 操控时回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`（第 9.3 节）。
- **读取**：`enter_world` 与 `status` 的响应带 `social` 字段（同上结构，见 5.8、5.12），客户端社交页以它为准，不另发请求。
- **错误码**：`BAD_REQUEST`、`WRONG_STATE`（不在世界中）、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。

**决斗（`duel: true`，STAGE-306）**

- 打开后同图的玩家可以用 `duel_challenge`（5.74）直接向他发起决斗，不再弹确认；关闭时对方收到 `TARGET_REFUSES`。
- 开关改动推 `social_changed`，同上。战斗中也能改，不影响正在打的那场。
- 发起决斗的一方，自己的 `duel` 开关开不开都可以。

**隐身（`visible: false`）**

- 同一张地图上其他在世界中的角色（玩家和 agent 连接都收到）：开关关闭时推 `players_changed` 的 `leave`（按名字）；恢复可见时推 `enter`（整条数据）。隐身期间，他自己的数据变化（升级、战斗、队伍等）不推给别人，即不推 `update`。
- 进入世界时已隐身：不推 `enter`；换图、离开世界或断线时已隐身：不推 `leave`（别人的列表里本来就没有他）。
- 别人的 `players` 和 `players_changed` 里都不出现隐身者。队伍摘要（5.66 的 `party`）对别人只计可见队员：`size` 为可见队员数，队长隐身时 `leader` 为 `null`。
- 1 秒合并窗口（5.66）内先隐身又恢复（或反过来），只看窗口结束时的状态：与窗口开始时相同则不推。
- 隐身者自己：`players` 照常列出别人（可见的那些），仍能主动邀请（`party_invite`）、申请（`party_join`），队员表和队内的一切照常。
- **隐身者被别人按名字指向**（`party_invite`、`party_join`）：回复 `PLAYER_NOT_FOUND`，不回 `PLAYER_NOT_NEARBY`，也不回 `TARGET_REFUSES`。理由：不泄露他的存在，也不让人通过“不在同图”判断他在线。隐身队长的队伍不能按名字申请加入。
- 开关改动不撤销已有请求：隐身前发出的请求、别人发给他的待处理请求，照常可以答复或撤回。
- 隐身者已在队伍里：队伍不受影响，队员之间的推送（5.67）照常。
- 隐身者主动申请或邀请别人时，对方收到的 `party_request` 里 `from` 仍是他的名字（他主动找上门，对方已知他存在）。

**拒绝邀请（`invite: false`）**

- 别人对他发 `party_invite`：回复 `TARGET_REFUSES`，不建立请求，不推 `party_request`；这次失败不计入 `RATE_LIMITED` 和 `TARGET_BUSY`。
- 检查位置：在 `INVALID_TARGET` 之后、`TARGET_IN_BATTLE` 之前（5.67 的校验顺序），拒绝者的战斗和队伍状态不会因此暴露给邀请者。
- 不影响：他主动邀请别人、他申请加入别人、别人申请加入他的队伍（他是队长时），以及他对已有请求的答复。
- 开关关闭前已经收到、还在待处理的邀请不撤销。

**接收交易邀请（`trade: false`，STAGE-296）**

- 别人对他发 `trade_invite`：回复 `TARGET_REFUSES`（`message` 为“对方不接收交易邀请”），不建立交易请求，不推 `trade_request`；这次失败不计入 `RATE_LIMITED`。
- 检查位置：和 `trade_invite` 的其他校验一起，在 `INVALID_TARGET` 之后、`TARGET_IN_BATTLE` 之前（`docs/trade.md` 2.2），拒绝者的战斗和交易状态不会因此暴露。隐身者仍先按隐身规则回 `PLAYER_NOT_FOUND`。
- 不影响：他主动邀请别人交易、他对已有交易请求的答复、正在进行的交易，以及组队的一切。开关关闭前已经收到、还在待处理的交易请求不撤销。

**三个开关的组合**：互相独立。隐身且拒绝邀请时，别人既看不到他，也不能组队邀请他，但他仍可以主动申请和邀请别人；拒绝交易邀请只影响交易邀请，不影响组队。

**存储**：迁移 `0029_character_social.sql`（编号以合并时为准）给 `characters` 表加 `social_visible boolean not null default true` 与 `social_invite boolean not null default true`。`social_trade boolean not null default true` 由迁移 `0030_trade.sql` 加（STAGE-294 已合入，STAGE-296 起读写）。

**用例**：`social_hidden`（隐身、恢复、隐身时被按名字邀请得 `PLAYER_NOT_FOUND`）、`social_refuse_invite`（拒绝邀请、他的申请与主动邀请不受影响）、`social_refuse_trade`（拒绝交易邀请、他主动交易不受影响、开关在重新登录后保留）、`social_persist`（设置在重新登录后保留）。

### 5.70 `leveling_catalog`：练级传送目录（查看类，世界中，STAGE-279）

按 10 级分段，列出全地图野外遇敌点（口径见 `docs/map.md`「练级传送目录」与 `docs/encounter.md`「练级传送口径」）。目录是静态数据，与角色无关。响应超过 64 KiB 的分段不一次给全，所以分两步取：

- 请求 `{}`：只返回分段摘要，13 段全部返回（含空段）。
- 请求 `{"segment": 11}`：在摘要之外再返回这一段的全部条目。`segment` 是段的下限，只能取 `1, 11, 21, …, 121`，其他值（包括负数、非整数）回复 `BAD_REQUEST`。

```json
{"catalog_version":"3f9ac0d2b7e1",
 "segments":[{"min":1,"max":10,"entry_count":47,"map_count":18},{"min":11,"max":20,"entry_count":63,"map_count":39}, "…共 13 项"],
 "segment":{"min":11,"max":20,"entry_count":63,"map_count":39},
 "entries":[{"monsters":[{"name":"乌卡鲁","level_min":11,"level_max":12},{"name":"乌宝宝","level_min":11,"level_max":12},{"name":"威威","level_min":11,"level_max":12}],
             "spots":[{"area_id":"200-189-521-435-527-454","number":189,"floor":200,"map_name":"加鲁卡","anchor":{"x":521,"y":435},"rect":{"x1":521,"y1":435,"x2":527,"y2":454},"max_enemies":3,"steps":26}]}]}
```

- `catalog_version` 是数据指纹，随目录内容变化；客户端缓存后用它判断要不要重新取（和 5.62 一样）。
- `segments` 每段一项：`min` / `max` 是段的上下限（固定 1–10、11–20……121–130），`entry_count` 是条目数，`map_count` 是这一段条目里涉及的不同地图数。空段 `entry_count` 为 0，`entries` 为 `[]`，客户端写“本段没有野外遇敌点”。
- 带 `segment` 请求时多两个字段：`segment`（这一段的摘要，同 `segments` 里的一项）和 `entries`。
- `entries` 是这一段的练级点，按段内最低等级升序（同等级按最高等级、再按怪物名的 GBK 字节序）。每个条目：
  - `monsters`：该段实际能遇到的怪物，每只一项，`name` 是模板名，`level_min` / `level_max` 是**段内裁切后**的等级区间（不超出 `min`–`max`，不出现跨段标签）。同一条目里的怪物按等级、再按名字排。只剩 Lv1 的敌人不列（Lv1 刷点看 5.62 的 1 级宠物目录）。
  - `spots`：这一组怪物能遇到的全部区域（同段内“怪物名 + 段内等级”完全相同的区域合成一条），按地图编号、再按区域 ID 排。每个落点：`area_id`（与 5.50 的区域 `id` 相同）、`number`、`floor`、`map_name`、`anchor`（开战时的代表落点，区域生效的格，不是出口格）、`rect`（已裁到地图边界）、`max_enemies`（同场上限）、`steps`（CEP 期望步数，约多少格一场，取整）。
- 口径要点：只计算区域的常态敌群（道具条件 `GROUP_APPEARBYITEMID` 的敌群不进目录，它们只在持有道具时出现；`NOTAPPEARBYITEMID` 的敌群不挡，持有道具时才不出现，见 `docs/encounter.md`）；CEP 上限为 0 的安全区、不在快捷传送目录的地图、裁完为空或找不到代表落点的区域都不进目录。
- 数量级：全目录 323 条、808 个落点（同一区域出现在几段里就各算一次）；单段最多约 140 个落点（11–20 段），整段约 30 KB。
- 不在世界中回复 `WRONG_STATE`；玩家和 agent 都可以查看，观察方也能查看。

### 5.71 `leveling_start`：按练级点开战或只传送（操作类，世界中，必须拥有控制权，STAGE-279）

请求 `{"area_id":"200-189-521-435-527-454","mode":"once"}`，`mode` 取：

- `once` / `infinite`：跨图定位到该区域的代表落点，**立刻**开第一场战斗（语义同 5.50 / 5.51 与 5.63：次数、停止、2 秒间隔、打输清计划，全部沿用）。
- `travel`：只定位到代表落点，不开战；遇敌开关随之关闭，同 `warp`。

`area_id` 必填且是字符串，缺省或为空回复 `BAD_REQUEST`；`mode` 不认识也回复 `BAD_REQUEST`。客户端不能给坐标；地图与落点都由服务端从练级目录和区域数据解析。一个区域可能出现在好几段的条目里，但落点（代表格）相同，所以请求只带区域 ID，不带段和怪物。

一次角色操作锁内检查，顺序如下（全部在改动位置之前完成，失败没有任何副作用）：控制权（`CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`）→ 非战斗（`IN_BATTLE`）→ 区域在练级目录里（`LEVELING_SPOT_NOT_FOUND`，也包括区域 ID 不存在或只有 Lv1 敌人的区域）→ 区域当前可用（`ENCOUNTER_AREA_UNAVAILABLE`，message 说明原因）→ 目录落点与服务端的代表落点一致（否则 `ENCOUNTER_AREA_UNAVAILABLE`，提示重新取目录）。

成功时的推送顺序与响应同 5.63：`once` / `infinite` 为 `position_changed`（可以换地图）→ `battle_start` → `battle_turn` → `encounter_changed` → 响应；`travel` 为 `position_changed` →（遇敌开关开着时）`encounter_changed` `reason: "warp"` → 响应。

错误码：`BAD_REQUEST`（参数或 mode 错）、`LEVELING_SPOT_NOT_FOUND`、`ENCOUNTER_AREA_UNAVAILABLE`、`IN_BATTLE`、`CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`。
协议用例见 `protocol/cases/leveling*.jsonl`。
### 5.73 `item_inlay`：精工，把材料镶到武器或防具上（操作类，世界中，STAGE-298）

```jsonc
{"id": 60, "type": "item_inlay", "data": {"target": 5, "materials": [6, 7]}}
{"id": 60, "type": "item_inlay", "ok": true, "data": {
  "applied": 2, "consumed": [6, 7],
  "item": {"slot": 5, "item_id": 20, "name": "轻的斧头", "effect": "攻+18 防-1 敏-13 HP+0 MP+0 伤+0 吸+0", "type": 1, "count": 1,
           "inlay": {"role": "gear", "free": 1, "codes": ["RUBY", "RUBY", null]}}}}
{"id": 61, "type": "item_inlay", "ok": false, "error": {"code": "INLAY_FULL", "message": "武器或防具已经镶满了。", "data": {"applied": 1, "consumed": [6]}}}
```

- 请求字段：`target` 是目标格（0–19 的整数），目标是武器或防具（类别码含 `INSLAY`），可以在背包，也可以穿在身上（0–4）；`materials` 是材料格，1 到 3 个整数，都在背包格（5–19），不能重复，不能和 `target` 相同。格号和 `inventory` 一致。多出其他字段、字段类型不对或缺少字段，回复 `BAD_REQUEST`。
- **一件装备最多镶 3 格，镶了不能卸下或洗掉**。目标加材料最多 4 件（即最多 3 件材料）。
- **校验顺序**（第一个不满足的回复对应错误码，什么都不改）：

| # | 检查 | 错误码 | 台词（原版原文） |
|---|---|---|---|
| 1 | 控制权（同其他操作类） | `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER` | |
| 2 | 战斗中 | `IN_BATTLE` | 战斗中不能这样做（原版静默返回） |
| 3 | 请求形状（见上） | `BAD_REQUEST` | |
| 4 | 材料多于 3 件 | `INLAY_TOO_MANY` | 精工每次只能选择四个物品。 |
| 5 | 目标或任一材料的格子是空的 | `EMPTY_SLOT` | |
| 6 | 目标或材料的类别码为空或 `NULL` | `INLAY_NOT_SUITABLE` | 这样物品不适合精工。 |
| 7 | 材料里有类别码含 `INSLAY` 的（第二个武器或防具） | `INLAY_MULTI_TARGET` | 精工每次只能选择一个武器或防具。 |
| 8 | 目标的类别码不含 `INSLAY` | `INLAY_NO_TARGET` | 精工必须选择一个武器或防具。 |
| 9 | 目标不是单件（`count` 不为 1） | `BAD_REQUEST` | |
| 10 | 依次镶，遇到目标已经镶满 | `INLAY_FULL`（带 `applied`、`consumed`） | 武器或防具已经镶满了。 |

  第 6、7 项按目标在前、材料依次在后逐件检查，第一个撞到的先报；第 8 项要全部读完才查。
- **逐个镶**：按 `materials` 的顺序，每个材料占一个空的镶嵌格，并对目标做下面的改动。**镶满了不回滚**，前面已经镶上、已经扣掉的保留（原版行为）。`applied` 是实际镶上的件数，`consumed` 是被扣掉的材料格（按请求顺序，每格 1 个）。
- **扣料**：材料叠放（`count` 大于 1）只扣 1 个，到 0 才删掉这一格（同加工、料理，不同于原版的整件删除）。
- **每个材料的改动**（存的是镶嵌累加量，不存总值；最终值 = 模板 + 累加量）：
  - 攻击、防御、敏捷、运气、HP、MP、伤、吸 各加材料模板的同名值，没有上限；
  - 材料的魔法编号大于 0 时，覆盖目标的魔法编号和魔法 MP（不带魔法的材料不覆盖）；
  - 覆盖使用函数与参数（材料没有的也会清空，同原版）；
  - 效果文字整个重写为 `攻±a 防±b 敏±c HP±d MP±e 伤±f 吸±g`，数字是累加后的总值（含装备本来的），非负带 `+`，负数自带 `-`；镶过带魔法的材料时，后面接魔法名。运气不显示。
- **一个数据库事务**：扣料和写目标的镶嵌状态一起提交。提交后才更新内存，推送 `inventory_changed`；目标穿在身上且攻击、防御、敏捷或魔法变了，再推 `status_changed`（10.10）。推送先于响应。事务失败则回滚，回复 `INTERNAL`，内存不变。
- 成功时没有台词，客户端根据响应显示。`applied` 等于材料件数时 `ok: true`；少于件数（中途镶满）回复 `ok: false`，错误码 `INLAY_FULL`，`error.data` 带 `applied` 和 `consumed`。物品已经改了，以之后的 `inventory_changed` 为准。
- **响应**：`applied`、`consumed` 同上；`item` 是镶完之后的目标（格子不变），字段同 5.17。
- **数据**：可镶的物品由 `itemset.txt` 第 4 列（类别码）和覆盖表（`server/gamedata/inlay_overlay.tsv`，默认只有表头）决定；没有覆盖表时所有物品都是 `INLAY_NOT_SUITABLE`。
- **agent 分级**（`agent-guide.md` 第 6 节）：第三级，必须先问。材料被消耗，镶嵌永久，会改变装备的数值和精灵，无法撤销。
- 错误码：`BAD_REQUEST`、`EMPTY_SLOT`、`INLAY_TOO_MANY`、`INLAY_NOT_SUITABLE`、`INLAY_MULTI_TARGET`、`INLAY_NO_TARGET`、`INLAY_FULL`、`IN_BATTLE`、`INTERNAL`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。
- 暂不做：装备的 HP、MP、伤、吸在战斗里还没有效果（另立项，docs/inlay.md 2.6）；交易中不能精工（交易 STAGE-294 合进来后再加 `IN_TRADE`）。
- 协议用例：`item_inlay_basic`、`item_inlay_equipped`、`item_inlay_full`、`item_inlay_errors`（`protocol/cases/item_inlay*.jsonl`），用测试覆盖表 `server/casetest/testdata/inlay_overlay.tsv`。设计见 `docs/inlay.md`。

### 5.72 交易（STAGE-294）：`trade_invite` / `trade_reply` / `trade_cancel` / `trade_offer` / `trade_lock` / `trade_unlock` / `trade_confirm` 与推送 `trade_request`、`trade_request_result`、`trade_updated`、`trade_ended`（操作类，世界中）

两个玩家在服务端完成一笔交易：先邀请、对方同意，各自放上最多 2 件物品、一些石币、1 只宠物；双方看清后锁定、再确定，东西才一起换过去。设计依据 `docs/trade.md` 第二、三、四节。

**总开关**：服务端配置 `trade.enabled`，默认 `false`（做法同 `party.players_enabled`）。关闭时下面所有请求回复 `WRONG_STATE`（`message` 为“交易尚未开放”），不推送。

**请求**

| 请求 | data | 成功回复 |
|---|---|---|
| `trade_invite` | `{"name": 对方角色名}` | `{"request": 请求对象}` |
| `trade_reply` | `{"id": "t-17", "accept": true/false}` | 接受：`{"trade": 交易对象}`；拒绝：`{}` |
| `trade_cancel` | `{"id": "t-17"}` | `{}` |
| `trade_offer` | `{"id": "t-17", "items": [5, 8], "gold": 5000, "pet": 2}` | `{"trade": 交易对象}` |
| `trade_lock` | `{"id": "t-17", "rev": 4}` | `{"trade": 交易对象}` |
| `trade_unlock` | `{"id": "t-17"}` | `{"trade": 交易对象}` |
| `trade_confirm` | `{"id": "t-17", "rev": 4}` | 第一个确定：`{"trade": 交易对象}`；第二个确定（交换成功）：`{"done": true}` |

- `trade_cancel`：发起人（请求阶段）或参与者（交易阶段）都可以发。
- `trade_offer`：`items`（背包格号数组，最多 2 个，每个是 5–19 的整数且不重复）、`gold`（非负整数，0 为不出）、`pet`（宠物栏位 0–19，`null` 为不出）。三个字段都可以省略，省略的不改；其他字段回复 `BAD_REQUEST`。报价是整体替换，任何一项不过则整条失败、报价不变。
- `trade_lock`、`trade_confirm` 必须带客户端当前看到的 `rev`，不一致回复 `TRADE_STALE`。
- `trade_confirm` 只在双方都已锁定后可用，否则 `WRONG_STATE`。
- `trade_lock` 前服务端做预演（同 2.4、2.6 的校验），不通过则拒绝锁定，错误码和 `error.data.who` 写明是谁的问题（`"me"` 或 `"peer"`）。对方的满格只说“对方物品栏已满”，不泄露格子数；对方的石币超上限说“对方石币已达上限”。

**对象**

```jsonc
// 请求对象（trade_invite 的响应，trade_request 的推送多两个字段）
{"id": "t-17", "from": "勇者小明", "to": "路人乙", "expires_in": 30}
// 交易对象
{"id": "t-17", "rev": 2, "idle_expires_in": 300,
 "me":   {"name": "勇者小明", "items": [{"slot": 5, "item_id": 20, "name": "小斧头", "effect": "攻 +9 防 -3 敏 -3", "count": 1, "durability": 80, "max_durability": 100, "crush_level": 0}],
          "gold": 5000, "pet": null, "locked": false, "confirmed": false},
 "peer": {"name": "路人乙", "items": [], "gold": 0, "pet": null, "locked": false, "confirmed": false},
 "last_error": null}
```

- `me.items` 的条目与 `inventory` 的物品条目相同；`peer.items` 没有 `slot`（对方的背包格号与你无关），其余字段相同。
- `me.pet` 是 `pets` 里的宠物条目（含 `slot`、`state`）；`peer.pet` 是对方宠物条目去掉 `slot` 和 `state`，素质档位、成长率、技能都给（见 `docs/trade.md` 待拍板第 4 条）。
- `last_error`：交换失败回到编辑阶段时非空，`{"code": 错误码, "who": "me" | "peer"}`；其他时候为 `null`。
- `idle_expires_in`：距离无操作自动取消还剩的秒数（超时 5 分钟，见下）。

**响应示例**

```jsonc
{"id": 90, "type": "trade_invite", "data": {"name": "路人乙"}}
{"id": 90, "type": "trade_invite", "ok": true, "data": {"request": {"id": "t-17", "from": "勇者小明", "to": "路人乙", "expires_in": 30}}}
{"id": 91, "type": "trade_offer", "data": {"id": "t-17", "items": [5], "gold": 5000, "pet": null}}
{"id": 91, "type": "trade_offer", "ok": true, "data": {"trade": {"id": "t-17", "rev": 2, "idle_expires_in": 300, "me": {"...": "..."}, "peer": {"...": "..."}, "last_error": null}}}
```

**推送**

- `trade_request`：推给对象在世界中的每条连接，`data` 为 `{"request": 请求对象}`，请求对象多 `from_level`、`from_transmigration`（让对方判断是不是新手号）。
- `trade_request_result`：`data` 为 `{"id", "status", "from", "to"}`，`status` 为 `accepted`（发起人收到，紧接着收到 `trade_updated`）、`declined`、`expired`、`cancelled`（带 `code` 时说明原因）。
- `trade_updated`：交易对象有变化（建立、对方改报价、对方锁定或取消锁定、对方确定、交换失败回到编辑）时推给**对方**的所有连接，`data` 为 `{"trade": 交易对象}`（`me` 和 `peer` 已按接收者视角换好）。自己的操作靠响应拿到，不推给自己。
- `trade_ended`：推给双方（包括发起结束的人），`data` 为 `{"id", "status": "completed" | "cancelled", "by": 角色名 | null, "code": 取消原因 | null, "summary": 摘要}`。`code` 取值：`CANCELLED`（对方取消）、`PEER_LEFT`（离开世界或断线）、`TIMEOUT`、`CONTROL_CHANGED`、`INTERNAL_ERROR`。`summary` 只在 `completed` 时有，`{"gave": {...}, "got": {...}}`，结构同报价（`items`、`gold`、`pet`）。两方各收到自己视角的摘要（`gave` 是自己给出的，`got` 是自己拿到的）。
- 交换成功的推送顺序：先 `inventory_changed`、`pets_changed`（双方都推，石币在 `inventory_changed.gold`），再 `trade_ended`（`completed`）。
- 推送可能丢失或晚到：客户端重连后丢弃交易状态；收到 `trade_updated` 的 `rev` 比本地小的忽略。

**规则要点**（完整见 `docs/trade.md` 第二节）

- 交易的范围：双方在同一张地图、都在世界中、都没倒下、都不在战斗中、都不在真人队里（分身队不限制）。一个角色同一时间只能有一笔进行中的交易。
- 报价内容：物品最多 2 格（整格，含全部数量），石币（独立于物品位），宠物最多 1 只。不能放进报价的：穿着的装备（装备格 0–4）、`VANISHATDROP` 物品、骑乘中的宠物、家族守护兽（家族未做，暂不检查）。
- 任何修改（报价变化、一方取消锁定）都清零双方的 `locked` 和 `confirmed`，`rev` +1。
- 邀请有效期 30 秒；交易窗口 5 分钟内双方都没有任何交易操作则 `TIMEOUT` 取消。
- 交易中，参与者除 `chat` 和查看类请求外，所有操作类请求回复 `IN_TRADE`（`message` 为“交易中，先完成或取消交易”）。组队请求同样回复 `IN_TRADE`；别人邀请交易中的人组队回复 `TARGET_IN_TRADE`。
- 断线、离开世界、被抢走控制权（见第 9.3 节）都会取消交易，不恢复；服务重启后交易全部消失（只存内存）。
- 交换前（预演、锁定、确定、实际交换）统一走同一套校验。石币持有上限统一为 1000 万（`battle.MaxHaveGold`，不随转生变化；`docs/trade.md` 第 6 条已拍板）。

**交换结果**：

- 第二个确定的人触发交换，成功：`{"done": true}`，随后推送见上。
- 预演后状态又变（例如管理命令刚发走了东西）：交易不关闭，回到编辑阶段，`trade_updated` 带 `last_error`，失效的报价项被自动移除。第一个确定的人的 `trade_confirm` 响应仍是 `{"trade": ...}`，`last_error` 非空。
- 数据库错误：回滚，交易取消，双方收到 `trade_ended`（`cancelled`，`code: INTERNAL_ERROR`），内存不变。

**`status` 与 `enter_world`**：响应带 `trade: null`（进入世界时总是 `null`，因为断线即取消）。`social` 对象多一个 `trade` 字段（`{"visible", "invite", "trade"}`）；`social_settings` 请求新增可选布尔字段 `trade`，其余规则同 5.69。

**谁能发**：玩家连接和 agent 连接都能发；观察方或 agent 操控时按 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER` 拦截（第 9.3 节）。分身不能被邀请，也不能邀请。

**错误码**（新增 8 个，总表见第 1 节错误码表中的交易一行）

| 错误码 | 含义 |
|---|---|
| `IN_TRADE` | 自己交易中，不能做这件事；或发起交易时已经在交易中 |
| `TARGET_IN_TRADE` | 对方已在交易中 |
| `TRADE_NOT_FOUND` | 交易不存在、已结束，或不是你的（请求阶段沿用 `REQUEST_NOT_FOUND`） |
| `TRADE_STALE` | `rev` 与当前不一致（对方刚改了报价） |
| `ITEM_UNTRADABLE` | 物品不能交易（`VANISHATDROP`） |
| `PET_UNTRADABLE` | 宠物不能交易（守护兽，家族做时启用） |
| `PET_FULL` | 宠物栏已满（`error.data.who` 说明是谁的） |
| `PET_TOO_STRONG` | 宠物比接收方高太多级，收不了（说明写明是谁的宠物、谁的等级） |

沿用：`WRONG_STATE`、`BAD_REQUEST`、`PLAYER_NOT_FOUND`、`PLAYER_NOT_NEARBY`、`INVALID_TARGET`、`TARGET_REFUSES`、`TARGET_IN_BATTLE`、`TARGET_IN_PARTY`、`TARGET_BUSY`、`REQUEST_NOT_FOUND`、`REQUEST_PENDING`、`RATE_LIMITED`、`CHARACTER_DEAD`、`IN_BATTLE`、`ALREADY_IN_PARTY`、`EMPTY_SLOT`、`NOT_ENOUGH_GOLD`、`GOLD_FULL`、`BAG_FULL`、`PET_RIDING`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`。

**存储**：迁移 `0030_trade.sql`（编号以合并时为准）：`characters` 加 `social_trade boolean not null default true`；新表 `trade_logs`（一笔成功交易一行，列见 `docs/trade.md` 2.7）。交易本身和请求只放内存。

**用例**：`trade_basic`、`trade_gift`、`trade_decline_cancel`、`trade_stale_rev`、`trade_errors`、`trade_bag_pet_full`、`trade_blocked_ops`、`trade_disconnect`。


### 5.74 `duel_challenge`：发起决斗（操作类，世界中，必须拥有控制权，STAGE-306）

设计依据 `docs/pk.md`：校验顺序 3.3，规则 2.1，决斗点数 2.2，开关 3.1。开关就是同意：对方打开了 `duel`（5.69）就直接开战，没有确认和等待答复的步骤。

```jsonc
{"id": 90, "type": "duel_challenge", "data": {"name": "阿牛"}}
// 成功：先推送双方所有参战者的 battle_start（kind 为 duel），再回响应
{"id": 90, "type": "duel_challenge", "ok": true, "data": {"battle_id": "b_31"}}
```

- **请求字段**：`name`（对方角色名，字符串，必填）。
- **校验顺序**（第一个不满足的回复对应错误码）：
  1. 功能开关 `duel.enabled` 关闭 → `WRONG_STATE`（`message`：“决斗暂未开放”）
  2. 控制权 → `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`
  3. 自己倒下 → `CHARACTER_DEAD`；自己战斗中 → `IN_BATTLE`；自己交易中 → `IN_TRADE`
  4. 自己是队员 → `NOT_PARTY_LEADER`；自己有分身队 → `ALREADY_IN_PARTY`
  5. 对方存在、在线、可见 → `PLAYER_NOT_FOUND`；同图 → `PLAYER_NOT_NEARBY`
  6. 不是自己、不是自己队友 → `INVALID_TARGET`
  7. 对方（是队员时看他的队长）没开 `duel` → `TARGET_REFUSES`（“对方没有接受决斗”），不计入 `RATE_LIMITED`
  8. 对方队伍正在战斗 → `TARGET_IN_BATTLE`；对方交易中 → `TARGET_IN_TRADE`；对方有分身队 → `TARGET_IN_PARTY`；对方（队长）倒下 → `TARGET_DEAD`
  9. 频率：同一挑战者对同一目标 10 秒内最多发起一次 → `RATE_LIMITED`（带 `retry_after_seconds`）
- **成功**：发起方的真人（单人或整队）与被挑战方的真人（单人或整队）同场开战。`battle_start.kind` 为 `duel`，`allow` 为 `{"escape": true, "capture": false}`；单位号发起方 0–4、被挑战方 10–14，宠物 +5；对方人物只给血量百分比（同怪物）。`duel` 对象和 `battle_start` 的其余字段见 10.3。每名参战者离开战斗时（打完、逃跑、被踢出、掉线）自己的 `duel` 开关自动关闭，值变了就推 `social_changed`（5.69）。
- **被挑战方是队员**：看对方队长的 `duel`，整队应战。开战前在锁下再核对双方每名队员：倒下、战斗中、交易中、有分身队，发起方的队员回 `CHARACTER_DEAD` / `IN_BATTLE` / `IN_TRADE` / `ALREADY_IN_PARTY`，被挑战方的队员回对应的 `TARGET_` 开头的码；队伍在核对前变了回 `WRONG_STATE`。
- **战斗规则**：逃跑必成功；没有经验、石币、掉落、魅力与忠诚惩罚；倒下的人物 1 血留在原地；偷窃对人物无效；复活精灵对人物无效（道具照样扣）。结算见 `docs/pk.md` 2.1、2.3，决斗点数见 2.2，断线与离场见 2.4。
- **错误码**：`TARGET_DEAD`（新增）和上面校验里的错误码，加 `BAD_REQUEST`。
- **分级**：第三级，agent 必须先问玩家（`agent-guide.md` 第 6 节）。
- **用例**：`duel_refused`（校验顺序）、`duel_switch_reset`（重新登录开关为关）、`duel_rate`（限频）、`duel_basic`（一击倒下的点数转移）、`duel_escape`（免罚与扣两成）、`duel_party`（队对队，单位号 10–14）、`duel_offline`（掉线扣点数）。

### 5.75 `duel_ranking`：决斗强者表（查看类，世界中，STAGE-308）与强者表 NPC

设计依据 `docs/pk.md` 3.5、5.3（原版 `npc/npc_duelranking.c`）。

```jsonc
{"id": 91, "type": "duel_ranking", "data": {"page": 1}}
{"id": 91, "type": "duel_ranking", "ok": true, "data": {
  "top": [{"rank": 1, "name": "勇者小明", "level": 80, "duel_point": 2310, "self": false}],
  "page": 1, "pages": 7,
  "me": {"rank": 152, "duel_point": 100}
}}
```

- **请求字段**：`page` 可省略（默认 1）；有的话必须是 1 以上的整数，否则回 `BAD_REQUEST`。
- **排序**：全服所有角色（含离线）按决斗点数从高到低；同分先达到的排前（角色的 `duel_point_at`，即决斗点数最后一次变化的时间），再同则按名字。每页 10 人，`rank` 是全服名次，从 1 开始。
- **响应**：`top` 是本页条目，每条带 `self`（是请求者自己为 `true`）；`page` 超过最后一页时 `top` 为空数组。`pages` 是总页数，至少为 1。`me` 是请求者自己的名次和决斗点数，按同样的规则算。
- 查看类，不改变任何状态，玩家和 agent 都能用。分级：第一级（`agent-guide.md` 第 6 节）。
- **强者表 NPC**（类型「强者表」，竞技场地图 130 和各村共 8 个，`talk` 同 5.33）：弹窗正文「要看哪一个？」，选项 `1 强者们`、`2 自己`（5.35 的选择规则）。
  - 选 `1`：先推 `npc_window_closed`（`CHOSEN`），再推第 1 页的排行窗口。
  - 选 `2`：先推 `npc_window_closed`（`CHOSEN`），再推请求者自己所在那一页的排行窗口。
  - 排行窗口的 `ranking` 字段就是上面 `data` 的同样内容（`page`、`pages`、`top`、`me`），`text` 为「世界的强者们」，选项按页给出：有下一页时 `1 下一页`；不在第 1 页时 `上一页`，编号是有下一页时的 `2`，否则为 `1`。没有选项时窗口只能用 `npc_close` 或 `Esc` 关掉。
  - 在排行窗口上选翻页：先推 `npc_window_closed`（`CHOSEN`），再按当前数据推新一页。编号不在选项里回 `NPC_BAD_CHOICE`，窗口保持不关。
  - 排行窗口和 `duel_ranking` 请求读的是同一份数据，翻页时重新读取，不缓存。
- **客户端显示**：用带黄色表头的 `Table`，列为「名次 / 决斗点数 / 等级 / 名字」，`self` 为 `true` 的那行名字前加星标。
- **用例**：`duel_ranking`（排序、同分、翻页、自己的名次、强者表 NPC 的菜单与翻页、错误编号）。

### 5.76 `gold_drop`：丢弃石币（操作类，世界中，STAGE-328）

```jsonc
{"id": 76, "type": "gold_drop", "data": {"amount": 1000, "confirm": "yes"}}
{"id": 76, "type": "gold_drop", "ok": true, "data": {"gold": 9998950}}
```

- `amount` 必须是 JSON 整数，且不小于 1，并且不能超过当前持有的石币。缺少、`0`、负数、小数、科学计数法、字符串，或超过持有，回复 `BAD_REQUEST`，石币不变。
- `confirm` 必须是字符串 `"yes"`，缺少或是别的值（包括 `"YES"`）回复 `BAD_REQUEST`，说明里写明“丢弃石币要确认”。客户端在命令后面带上 `yes` 才会发出这条请求（`gold drop 1000 yes`）。
- **成功**：石币减去 `amount`，写库后推送 `status_changed`（`status.gold` 是新值），响应 `data.gold` 是丢弃后的持有量。写审计与实际活动，和 5.28 相同（agent 发出时）。
- **丢弃就是销毁**：石币不会掉在地上，不可恢复。丢完之后持有量低于上限，卖东西（5.37 的卖出）不再因为 `GOLD_FULL` 失效。
- 检查顺序：控制权（守卫，同 5.28）→ 战斗中 → 死亡 → 参数 → 数量超过持有。
- 错误码：`BAD_REQUEST`、`CHARACTER_DEAD`、`CONTROLLED_BY_AGENT`、`CONTROLLED_BY_PLAYER`、`IN_BATTLE`。

### 5.60 道具仓库：寄存、领取道具（STAGE-309）

和便利店的道具仓库管理员（5.32 的 `type` 是「道具仓库」，原版 `npcgen_poolitemshop`，
四张便利店地图 1009/2009/3009/4009 各一位；原版的斗技场寄放店不收）`talk` 弹菜单
（`npc_window`，正文「欢迎光临」），选项是「寄存」「领取」（原版叫「寄放道具」「取回道具」，
沿用需求里定的名字）。选项编号：寄存列表是背包格号（5–19），领取列表是仓库格号（0 起）。

- **寄存**：背包列表（只列背包格 5–19，装备格 0–4 的道具不列；原版寄存列表也从背包格开始）→
  确认页（`寄存[名字]` + 空行 + `寄存价格 N stone` + `真的要那个吗`，N = 50，原版 `pis_*` 的
  `cost:50`，按件收费）→ 确认后整件道具离开背包进仓库（堆叠数量、耐久、合成、精工原样，
  堆叠物品整堆寄存，一件一格、收一次 50），扣 50 石币，推 `inventory_changed` + `status_changed`
  并关窗。仓库满回复 `ACTION_NOT_ALLOWED`（`poolfull_msg`「超过的无法处理喔」）；金币不够回复
  `NOT_ENOUGH_GOLD`（`stone_msg`「钱不够喔!」）。两种失败都不扣钱、不动物品。
- **领取**：仓库列表（选项编号是仓库格号）→ 选一件直接取回到第一个空背包格（免费，原版如此），
  推 `inventory_changed` 并关窗；背包满回复 `ACTION_NOT_ALLOWED`（`itemfull_msg`「道具不是已经满了吗」）。
- **格数固定 50 格**（STAGE-347）：所有角色（含已转生的老角色）都是 50 格，0–49；不再随转生变化
  （原版 `CHAR_getCharPoolItemIndexElement` 的 `min(转生×4+10, 30)` 只作历史对照）。第 50 件寄存时
  回复 `ACTION_NOT_ALLOWED`（`poolfull_msg`），已有道具不受影响。宠物寄放池的格数仍随转生，见 5.41。
- 仓库存在 `character_pool_items`（迁移 0032），和背包的 `character_items` 分开：仓库里的道具不出现在
  背包快照（5.17）里，寄存与领取不影响装备、交易和合成。
- 战斗中、观察方被拒，同 `talk` / `npc_choose` 的通用规则。
- 原版的「使用仓库」（大仓库，`_NPC_DEPOTITEM`）本期不做，窗口里没有这一项。

## 9. agent 协同

玩家可以让自己的 agent（AI 助手程序）登录游戏、代为游玩，自己同时在线观看和聊天。设计说明见 `docs/agent.md`，本节只定义消息。

### 9.1 令牌管理（玩家连接，“账号”类）

agent 不使用账号密码，而是使用玩家生成的**专用令牌**。令牌只能以 agent 身份登录，不能用来注册、改密码、建角或删角。

```jsonc
// 生成令牌：令牌原文只在这一次响应里出现，服务端只保存哈希值
{"id": 20, "type": "create_agent_token", "data": {"label": "我的 Claude", "expires_in_days": 30}}
{"id": 20, "type": "create_agent_token", "ok": true, "data": {
  "token_id": "t_7Hq2", "token": "sat_…（随机 32 字节）", "label": "我的 Claude",
  "expires_at": "2026-11-08T12:00:00Z"}}

// 列出令牌（不含令牌原文）
{"id": 21, "type": "agent_tokens", "data": {}}
{"id": 21, "type": "agent_tokens", "ok": true, "data": {"tokens": [
  {"token_id": "t_7Hq2", "label": "我的 Claude", "created_at": "…", "expires_at": "…", "last_used_at": "…"}]}}

// 吊销令牌：正在使用这个令牌的 agent 连接会收到 disconnect（AGENT_TOKEN_REVOKED）后被断开
{"id": 22, "type": "revoke_agent_token", "data": {"token_id": "t_7Hq2"}}
{"id": 22, "type": "revoke_agent_token", "ok": true, "data": {}}
```

- `expires_in_days` 取 1–365，不填时为 30；`label` 可以不写，最长 32 个字符，不能含控制字符，首尾空白会被去掉。超出范围回复 `BAD_REQUEST`。
- 令牌是 `sat_` 加 43 个字符的随机串（256 位随机数）；服务端只保存 SHA-256 哈希；协议日志里令牌原文一律显示为 `***`。
- `agent_tokens` 只列出有效的令牌（没吊销、没过期），按生成顺序排列；`last_used_at` 从未用过时为 `null`。
- 过期和已吊销的令牌不占 5 个名额。
- 只能吊销自己账号的令牌；吊销已经吊销过的、不存在的、别人的，都回复 `TOKEN_NOT_FOUND`。
- 错误码：
  - `TOKEN_LIMIT`：有效令牌已达上限；
  - `TOKEN_NOT_FOUND`：要吊销的令牌不存在。

### 9.2 `agent_login`：agent 登录（agent 连接）

```jsonc
{"id": 2, "type": "agent_login", "data": {"token": "sat_…"}}
{"id": 2, "type": "agent_login", "ok": true, "data": {"account": "alice", "token_id": "t_7Hq2"}}
```

- 错误码：
  - `INVALID_TOKEN`：令牌不存在、已过期或已被吊销；
  - `ACCOUNT_DISABLED`：账号已被禁用；
  - `RATE_LIMITED`：同一 IP 失败次数过多，限制规则同 `login`。
- 登录成功后，agent 连接可以发 `characters`、`enter_world` 和世界中的各种请求，也可以发 `agent_say`；不能建角或删角，不能管理令牌，不能发第 9.4 节的玩家专用请求。
- 玩家连接发 `agent_login`、agent 连接发 `login` / `register` 都回复 `WRONG_ROLE`。
- 失败（`INVALID_TOKEN`）按 IP 计数，和 `login` 的密码错误**合计**：同一 IP 10 分钟内失败 20 次就限制，之后连正确的令牌也回复 `RATE_LIMITED`。`ACCOUNT_DISABLED` 只在令牌有效时才回复。
- 同一账号的第二条 agent 连接登录成功后，旧的 agent 连接收到 `KICKED_DUPLICATE_LOGIN` 被断开（agent 挤 agent，不影响玩家连接）。
- 每次 `agent_login`（成功或失败）都写入 `auth_audit`；令牌的生成和吊销也写。

### 9.3 控制权规则

同一账号同一时间只有一个角色在世界中，这个角色的**操控方**只有一个：

| 情况 | 操控方 |
|---|---|
| 只有玩家在世界中 | 玩家 |
| 只有 agent 在世界中 | agent |
| 两者都在世界中 | 默认是 agent；玩家执行 `take_control` 后改为玩家，执行 `release_control` 后交还 agent |

- **进入世界**：
  - agent 进入世界时，如果玩家已经在世界中操控**同一个**角色，控制权自动转给 agent，玩家变为观察方；
  - 玩家进入世界时，如果 agent 正在操控，玩家直接以观察方进入；
  - 两种情况都要求进入的是同一个角色，否则回复 `CHARACTER_IN_USE`。
- **离开世界或断开连接**：操控方离开世界或断开连接后，如果另一方还在世界中，控制权自动转给另一方。
- **观察方的限制**：观察方发起“操作”类请求时，服务端拒绝：玩家被拒时回复 `CONTROLLED_BY_AGENT`，agent 被拒时回复 `CONTROLLED_BY_PLAYER`。观察方仍然可以发起“查看”和“聊天”类请求，也可以 `leave_world`。
- **同步**：服务端把这个角色在世界中的所有推送，同时发给玩家连接和 agent 连接，保证两边看到的状态一致。

控制权变化时，服务端给世界中的**每一条**连接都推送 `control_changed`（包括引起这次变化的那一条，推送先于响应到达；操控方离开时只剩下另一条连接，只发给它）：

```jsonc
{"type": "control_changed", "data": {"controller": "player", "you": "controller", "reason": "TAKEN_BY_PLAYER"}}
```

`reason` 取值如下：

| 取值 | 含义 |
|---|---|
| `AGENT_ENTERED` | agent 进入世界，接管控制 |
| `TAKEN_BY_PLAYER` | 玩家收回控制权 |
| `RELEASED_BY_PLAYER` | 玩家把控制权交还 agent |
| `PEER_LEFT` | 原操控方离开世界、断线、被踢下线或令牌被吊销 |

- `enter_world` 的响应里也有 `control` 字段，告诉进入者当前是谁在操控、自己是操控方还是观察方。
- 玩家以观察方进入，或观察方离开，操控方不变，不推送。
- **控制权转交会关掉遇敌开关**（STAGE-311）：控制权变化时如果遇敌开关开着，服务端在 `control_changed` 之后接着推 `encounter_changed`（`encounter` 为 `false`，`reason` 为 `control`）和一条 `notice`。开关跟着控制方走：当前这一场照常打完，之后不再开新的一场；新操控方要连场遇敌就自己重新发 `encounter`（5.29）或 `encounter_start`（5.51）。用例 `control_encounter_off`。

### 9.4 玩家专用请求（玩家连接，世界中）

```jsonc
// 收回控制权：agent 变为观察方，仍保持连接
{"id": 30, "type": "take_control", "data": {}}
// 把控制权交还 agent
{"id": 31, "type": "release_control", "data": {}}
// 让 agent 下线：agent 连接收到 disconnect（KICKED_BY_PLAYER）后被断开，令牌仍然有效
{"id": 32, "type": "kick_agent", "data": {}}
```

- 错误码：
  - `AGENT_OFFLINE`：agent 不在线，或者不在世界中；
  - `WRONG_ROLE`：agent 连接不能发这三种请求。
- 这三种请求属于“账号”类，玩家作为观察方时也可以发起。
- `AGENT_OFFLINE` 的情形：agent 没有连接、或者连着但不在世界中。`take_control` / `release_control` 在已经是目标状态时什么都不改、不推送，直接回复成功。
- `kick_agent` 先保存数据、把控制权交回玩家（agent 原本是操控方的话推送 `PEER_LEFT`），再给 agent 推送 `disconnect`（`KICKED_BY_PLAYER`）并断开；令牌仍然有效。
- `revoke_agent_token` 吊销令牌时同理：正在使用它的 agent 收到 `AGENT_TOKEN_REVOKED` 后被断开。令牌在 agent 在线期间到期也一样：服务端每 60 秒检查一次，到期的 agent 走同一条路径（先保存、控制权按 `PEER_LEFT` 转回玩家、推送 `AGENT_TOKEN_REVOKED`）。
- 禁用账号、吊销令牌与登录同时发生时不会漏：连接登记到账号名下之后，服务端会再确认一次账号没被禁用、令牌没被吊销（否则登录失败，回复 `ACCOUNT_DISABLED` / `INVALID_TOKEN`）。

### 9.5 `agent_say` 与推送 `agent_message`：玩家与 agent 私聊（聊天类）

```jsonc
{"id": 40, "type": "agent_say", "data": {"text": "先去加加村，别打架"}}
{"id": 40, "type": "agent_say", "ok": true, "data": {}}
// 对方收到的推送
{"type": "agent_message", "data": {"from": "player", "text": "先去加加村，别打架", "time_ms": 1760000000000}}
```

- **可见范围**：只在同一账号的玩家连接和 agent 连接之间传递，其他玩家看不到。双方只要登录了就能私聊，不要求在世界中。
- **对方不在线**：回复 `PEER_OFFLINE`。
- **长度**：`text` 去掉首尾空白后为 1–200 个字符，不能含控制字符，否则回复 `BAD_REQUEST`。私聊内容不写入数据库，协议日志里会有（和其他消息一样）。
- **公开发言**：不在本节。玩家和 agent 以角色身份的公开发言（`map` 本图、`world` 世界）是 `chat` 请求，见 5.68，属于聊天类，不受控制权限制；服务端在审计记录里标明实际由谁发出（9.6）。agent 连接收不到任何公开聊天。

### 9.6 审计

agent 连接发起的每个“操作”类请求（包括被拒绝的，比如 `CONTROLLED_BY_PLAYER`），以及控制权的每次变化（包括 `kick_agent`），都写入审计表 `agent_audit`：账号、角色、连接身份、令牌编号、请求类型（控制权变化时是原因）、结果、时间。“查看”和“聊天”类请求不写，唯一例外是 agent 连接发出的公开聊天 `chat`（5.68），成功发出的每条都写一条，注明由 agent 发出。战斗中的 `battle_command` 同样是“操作”类，同样写审计。

### 9.7 计划、状态与活动（STAGE-119）

玩家在「Agent」页看到 agent 的计划、在线状态和实际做过的事。计划是 agent 自己报的，可能不准；活动是服务端按 agent 实际发出的请求记录的，是事实。

**`agent_plan`：报告计划（agent 连接，聊天类，登录后即可发，不要求在世界中）**

```jsonc
{"id": 5, "type": "agent_plan", "data": {"goal": "带主角去加加村，买三瓶回复药", "steps": [
  {"text": "走到加加村的村长家", "state": "done"},
  {"text": "和道具商买回复药", "state": "doing"},
  {"text": "回村长家交任务", "state": "todo"}]}}
{"id": 5, "type": "agent_plan", "ok": true, "data": {}}
// 同一账号的玩家连接收到
{"type": "agent_plan", "data": {"goal": "带主角去加加村，买三瓶回复药", "steps": [
  {"text": "走到加加村的村长家", "state": "done"}, {"text": "和道具商买回复药", "state": "doing"}, {"text": "回村长家交任务", "state": "todo"}],
  "updated_at_ms": 1760000000000}}
```

- `goal`：去掉首尾空白后 1–100 个字符；`steps`：可以省略或为空数组，最多 20 项，每项 `text` 为 1–60 个字符、`state` 只能是 `todo`（待做）、`doing`（进行中）、`done`（已完成）。文本不能含控制字符。不合法回复 `BAD_REQUEST`。
- 每次都是**整份替换**，不是增量：agent 每换一步都发完整的计划。
- 频率：没有单独的频率限制，和 `agent_say` 一样。
- 玩家连接发 `agent_plan` 回复 `WRONG_ROLE`。计划只存在服务端内存里，不入库。

**`agent_status`：agent 上线和下线（推送，玩家连接）**

```jsonc
{"type": "agent_status", "data": {"online": true, "token_id": "t_7Hq2"}}
{"type": "agent_status", "data": {"online": false, "token_id": null}}
```

- agent 登录成功时推送 `online: true`，断开时（无论什么原因）推送 `online: false`。`token_id` 是这条 agent 连接所用令牌的编号，玩家用它去 `agent_tokens` 的列表里查备注和到期时间。
- 同一账号的第二条 agent 连接把旧连接挤下线时，只有当前有效的那条连接断开才推送 `online: false`，被挤下线的旧连接不推送。

**`agent_activity`：实际活动（推送，玩家连接）**

```jsonc
{"type": "agent_activity", "data": {"full": false, "items": [
  {"time_ms": 1760000000000, "request": "turn", "summary": "转向 direction=3", "ok": true, "code": null},
  {"time_ms": 1760000001000, "request": "warp", "summary": "传送 place=12", "ok": false, "code": "CONTROLLED_BY_PLAYER"}]}}
```

- `full`：`true` 表示用 `items` 替换整个活动列表（玩家进入世界时的补发，以及 agent 登录时的清空）；`false` 表示把 `items` 追加到列表末尾（实时推送，每次一条）。
- 每条的字段：`request` 是请求类型名；`summary` 是服务端生成的中文摘要（类别名加参数，参数只取请求里的标量值，写成 `名=值`，超长截断）；`ok` 为是否成功；`code` 失败时是错误码，成功时为 `null`。
- **记录什么**：agent 连接发出的每个“操作”类请求（`protocol.Requests` 里 `CategoryOperation` 的请求，包括 `enter_world` 和 `leave_world`），不论成功还是被拒绝，都记一条。查看类、聊天类、连接类请求不记；控制权变化（`take_control`、`release_control`、`kick_agent`）也不记，它们已经由 9.3 的 `control_changed` 体现。
- 每个账号在内存里最多保存 200 条，超出时丢弃最旧的。服务端重启后清空，不入库。
- 摘要的中文类别名由服务端决定，客户端只显示，不解析。

**补发和清空的时机**（只在状态变化时推送；客户端的默认状态是离线、计划和活动为空）：

| 时机 | 推送给玩家连接的内容 |
|---|---|
| 玩家 `enter_world` 成功 | 响应里多一个 `agent` 字段（`online`、`token_id`，和 `agent_status` 的 `data` 一样），告诉玩家进入时 agent 是否在线；有计划时推送 `agent_plan`，有活动时推送 `agent_activity`（`full: true`，内容为当前全部记录）。推送先于响应到达，和 `control_changed` 一样。没有计划、没有活动时什么都不推 |
| agent 登录成功 | `agent_status`（`online: true`）总是推送；上一次有计划的话再推一条空计划；上一次有活动的话再推 `agent_activity`（`full: true`，`items` 为空）。以上是清空上一次的内容 |
| agent 断开 | 只推送 `agent_status`（`online: false`）；计划和活动保留，玩家仍能看到上一次的内容，直到下一次 agent 登录 |
| agent 报计划或做了操作 | 实时推送 `agent_plan` 或 `agent_activity`（`full: false`，一条） |

推送只发给同一账号、登录状态（已登录或世界中）的玩家连接。

## 10. 战斗

客户端的界面见 `docs/battle-ui.md`。**规则（伤害公式、命中、暴击、逃跑和捕获成功率、经验掉落）不在协议里写**，照搬原版 `original-data/gmsv/battle/`，由服务端战斗引擎实现；协议只规定双方说什么话。触发方式是**遇敌开关**（STAGE-40，5.29）：玩家在野外打开 `encounter` 后一场接一场地自动开战，管理口的 `start_battle` 只给协议用例用；无论怎样触发，服务端都以 `battle_start` 推送开始一场战斗。

### 10.1 概念

- **单位号**：一场战斗最多 20 个单位，号码 0–19。**每方 10 格**：人 0–4、宠物 5–9，宠物的号码是主人的号码加 5；对方整体加 10（原版 `SIDE_OFFSET`）。`side = no / 10`；`no % 10 < 5` 是后排，否则是前排。怪物可以占任意格。协议里的号码和界面上的“我 1–10 / 敌 1–10”不是一回事，客户端按 `docs/battle-ui.md` 的站位表换算。
- **我是哪一方**：`battle_start` 的 `you` 是自己（人物）的单位号，`you / 10` 就是己方。己方一般是 0–9，但客户端不要写死。
- **战斗中**是“世界中”的一个子状态：从 `battle_start` 推送起，到 `battle_end` 推送止。战斗中：
  - `turn`、`warp`、`leave_world` 回复 `IN_BATTLE`，位置和状态不变；`addpoint` 例外，战斗中照常可以加点（STAGE-189，见 5.21；原版服务端在战斗中直接忽略加点，`lssproto_SKUP_recv`，这里有意不同）；`encounter` 的 `off` 例外，可以在战斗中关掉遇敌开关（5.29：当前这场照常打完）；`enter_world` 回复 `WRONG_STATE`（已经在世界中）；
  - `status`、`places`、`inventory`、`pets`、`agent_say`、控制权相关请求（`take_control` 等）照常可用；
  - 最后一条连接断开（断线、被踢、关服）时，按原版 `CHAR_logout` → `BATTLE_Exit` 立即退出战斗：人物和宠物离场，战斗作废——不给经验金币、不判负、不回出生点；当前血量、魔法和宠物血量照常保存（人物倒下则血量回 1），出招与确认的计时器随战斗销毁。重连进入世界后角色不在战斗中。真人队同场的战斗（`kind` 为 `party`）不适用“战斗作废”：断线的队员按 10.14 立即退场并离队，其余人继续打。
- **一回合的时序**（人物指令 → 宠物指令 → 结算 → 播放 → 确认）：

  ```text
  battle_turn ─▶ battle_command(人物) ─▶ battle_command(宠物) ─▶ battle_progress ─▶ battle_round ─▶ battle_ack ─▶ battle_turn …
  服务端推送      客户端请求（回复 next）    客户端请求             服务端推送（所有玩家都交了）  服务端推送   客户端请求      或 battle_end
  ```

- **推送的范围**：与其他推送一样，战斗的所有推送同时发给该角色的玩家连接和 agent 连接，两边看到的完全一致。
- **请求的类别**：`battle_command` 是**操作类**（观察方发起回复 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`，规则同第 9.3 节）；`battle` 是**查看类**；`battle_ack` 是确认类，观察方也可以发。

### 10.2 单位对象 `unit`

```jsonc
// 己方单位（含真人队友）：给出准确血量；其中人物单位（自己和真人队友）还带 mp、max_mp
{"no": 0, "kind": "player", "name": "勇者小明", "level": 1, "hp": 44, "max_hp": 44, "hp_pct": 100,
 "mp": 100, "max_mp": 100, "status": [], "ride": false, "elements": {"earth": 60, "water": 40, "fire": 0, "wind": 0}}
// 对方单位：只给血量百分比（原版里敌人的精确血量是隐藏信息）
{"no": 15, "kind": "monster", "name": "野猪", "level": 3, "hp_pct": 100, "status": [], "ride": false,
 "elements": {"earth": 100, "water": 0, "fire": 0, "wind": 0}}
```

- `kind`：`player`（人物）、`pet`（宠物）、`monster`（怪物）。
- `hp_pct`：0–100 的整数，活着的单位至少是 1；0 表示已倒下。己方单位另有 `hp`、`max_hp`，对方没有这两个字段。
- `status` 是状态标记的列表，取值：`poison` 毒、`paralysis` 麻、`sleep` 睡、`stone` 石、`drunk` 醉、`confusion` 乱、`weaken` 虚（虚弱，STAGE-201）、`barrier` 障（魔障，STAGE-201）、`nocast` 默（沉默，STAGE-201）、`magic_resist` 魔抗、`fire_resist` 火抗、`thunder_resist` 电抗、`ice_resist` 冰抗（魔抗系，STAGE-228，含义见 10.7，火、电、冰抗只占状态槽、不改数值）、`guard` 防御、`buff` 增益、`debuff` 减益、`reversed` 属性反转（STAGE-200：属性反转精灵翻过开关、四属性已互换，客户端画一个“反”字；开关关回去时去掉），以及 STAGE-199 的防御次数标记 `absorb` 吸收、`reflect` 反射、`vanish` 无效（有剩余次数时挂着，次数用完去掉；三者互相独立，可以同时挂着，见 10.7 的 `result`）。骑乘用 `ride` 表示，不放在 `status` 里。骑乘时人物的 `ride` 为 `true`，骑着的宠物不作为单位出现（不在 `battle_start` 的单位列表里，不出手、不被选为目标）；出战的另一只宠物照常作为单位出现，有自己的宠物指令（STAGE-149）；它的血量并进人物的挨打，战报里只有人物的扣血；它的血量不作为单位下发，而是人物单位上的 `mount` 对象（STAGE-250，见上），每回合随 `state` 刷新。宠物血量归零后人物仍是 `ride: true`，伤害全由人承担（`docs/ride.md` §5，STAGE-124）。
- `mount`（STAGE-250）：骑乘的人物单位才带（即 `ride` 为 `true`），没骑乘时整个字段省略，老客户端忽略即可。它是人物骑着的宠物的战斗状态：`name` 是宠物名字，`hp_pct` 是 0–100 的整数（0 表示已倒下），己方（和观看者同一方）另有 `hp`、`max_hp`，对方的人物没有这两个字段。骑宠不作为单位出现，它的血量只在这里给出：`battle_start` 和 `joined` 里是当时的值，之后每回合随 `battle_round` 的 `state` 刷新（见 10.7）。例：`{"no": 0, "kind": "player", "name": "老迪", "hp": 45, "max_hp": 60, "hp_pct": 75, "ride": true, "mount": {"name": "贝鲁卡", "hp": 30, "max_hp": 40, "hp_pct": 75}, ...}`
- `auto`（STAGE-162）：分身队的单位（分身人物和它出战的宠物，见 5.57）为 `true`，由服务端自动出招，不要给它们下指令；
  其他单位（包括队长自己和队长的宠物、对方的怪物）**不带这个字段**，缺失按 `false` 处理。分身是己方单位，带准确的 `hp`/`max_hp`，不带 `mp`。
- `elements`：地水火风，选目标时显示在目标信息栏。取值 0–100，与宠物页（5.19）的 `elements` 同一个刻度；人物是创建角色时的点数（0–10）乘以 10。
- 以后增加的字段，客户端忽略即可（见第 1 节）。

### 10.3 推送 `battle_start`：战斗开始

```jsonc
{"type": "battle_start", "data": {
  "battle_id": "b_1", "kind": "wild", "scene": "野外", "you": 0,
  "allow": {"escape": true, "capture": true},
  "units": [ {"no": 0, ...}, {"no": 5, ...}, {"no": 15, ...}, {"no": 16, ...} ]}}
```

- `kind`：`wild`（玩家对野外怪物）或 `npc`（战斗 NPC 发起的战斗，STAGE-96）；真人队同场（STAGE-275）时为 `party`，野外遇敌和战斗 NPC 都是，NPC 的 `npc` 对象照带。决斗（STAGE-306，`docs/pk.md`）时为 `duel`，`allow` 为 `{"escape": true, "capture": false}`，单位号与 `duel` 对象见 5.74。竞技场以后追加新的取值。
- `scene`：给玩家看的地点说明，显示在战场标题上（野外是“野外”，NPC 战斗是地图名）。
- `allow`：本场战斗是否允许逃跑、捕捉。不允许的指令提交后回复 `ACTION_NOT_ALLOWED`，界面上置灰。
- `npc`（只有 `kind` 为 `npc` 时）：发起战斗的 NPC，`{"id": 1735, "name": "里昂蛙王", "say": "看我怎么修理你们这些人类！"}`。
  `say` 是开战时 NPC 说的话（原版 `startmsg`），客户端写进战斗记录。
- `units`：开局时场上所有的单位。`kind` 为 `npc` 时敌方的单位按 NPC 参数里的敌人编号顺序入场（最多 10 个，
  前排 15–19 先排，满了接着用后排 10–14）。
- `party`（只有 `kind` 为 `party` 时，STAGE-275，见 10.14）：同场的真人队员，`{"members": [{"no": 0, "name": "勇者小明", "leader": true}, {"no": 1, "name": "路人乙", "leader": false}]}`。每个队员各收一条 `battle_start`，只有 `you` 不同，其余字段相同。
- `field`（STAGE-200，只在场地有属性时带）：当前场地，内容同 10.4。
- 紧接着服务端推送第 1 回合的 `battle_turn`。

### 10.4 推送 `battle_turn`：回合开始

```jsonc
{"type": "battle_turn", "data": {
  "battle_id": "b_1", "turn": 1, "deadline_ms": 1760000030000, "server_time_ms": 1760000000000,
  "needs": ["player", "pet"], "progress": {"done": 0, "total": 1}}}
```

- `turn` 从 1 开始。`deadline_ms` 是这一回合出招的截止时间（毫秒时间戳），`server_time_ms` 是发出时服务端的时间，客户端用两者之差做倒计时，不依赖本机时钟。超过截止时间仍没有提交的指令，由服务端按原版的默认动作处理，结算里以 `skip`（原因 `timeout`）出现（单人战斗；真人队同场时 `deadline_ms` 为 `null`，计时和超时规则见 10.14，超时是踢出而不是默认动作）。
- `needs`：**这个角色**本回合要下的指令，按顺序：`player`（人物指令）、`pet`（宠物指令）。睡着、石化等不能行动时没有 `player`；没有出战宠物或出战宠物已倒下时没有 `pet`。骑着的宠物不算出战宠物，骑乘本身不影响 `pet`（STAGE-149）。`needs` 为空时客户端直接显示“等待”。
- `optional`（可选，STAGE-186）：分身队战斗里，这个回合玩家**可以**指定攻击目标的分身，值是 `actor` 字符串：`clone:N`（分身 N，单位号 N）、`clone_pet:N`（分身 N 的宠物，单位号 5+N），N 为 1–4，按单位号从小到大排。倒下、睡着、石化、麻痹这类这回合不能行动的分身不在其中；没有分身时整个字段省略。**它们不在 `needs` 里**：交不交都不影响“队长交齐就结算”，所以分身的目标要在队长最后一条指令之前发出。
- `progress`：本回合已经交完指令的真人数（`done`）和本回合需要交指令的真人数（`total`，即 `needs` 非空的真人，含自己）。真人队同场时界面显示“等待其他玩家 done/total”（`docs/battle-ui.md`）；单人战斗里 `total` 为 1。
- `field`（STAGE-200，只在场地有属性时带，无属性时省略）：当前的场地，`{"attr": "fire", "pow": 60, "left": 3}`。`attr` 是 `earth`/`water`/`fire`/`wind`，`pow` 是威力（0–100，场地对属性的加成按它算），`left` 是剩余回合（回合末递减，减到 0 恢复成无属性）。双方共用一个场地。

### 10.5 `battle_command`：出招（操作类，战斗中）

```jsonc
{"id": 20, "type": "battle_command", "data": {"turn": 1, "actor": "player", "action": "attack", "target": 15}}
{"id": 20, "type": "battle_command", "ok": true, "data": {"next": "pet"}}
```

`actor` 取 `player` 或 `pet`；分身队里还可以取 `clone:N` / `clone_pet:N`（见下面“分身的指令”）；不属于该 `actor` 的 `action` 回复 `BAD_REQUEST`。

| actor | action | 参数 | 说明 |
|---|---|---|---|
| player | `attack` | `target` | 攻击；回旋标、连击这类变体由装备决定，不用单独的指令 |
| player | `magic` | `magic`（0–4）、`target` | 精灵（魔法）；`target` 是否要带、能选哪一方由这个精灵的目标类型决定（见下面的“目标类型”）。状态精灵（中毒、麻痹、睡眠、石化、醉酒、混乱）按原版成功率挂上，没中时效果的 `result` 为 `miss`、不挂状态，已经带着任何异常状态的目标也不中；魔障（`barrier`，敌方全体）、虚弱（`weaken`，单人或敌方全体）的回合数是精灵的 turn + 1，沉默（`nocast`，敌方全体）是 turn 且不挂宠物（STAGE-201）；带着魔障的单位不能行动，带着沉默的人物不能交 `magic` 指令（回复 `MAGIC_UNUSABLE`），虚弱的说明文字里的防御、敏捷降 20% 原版源码没有实现，它只有“回合末冻结”这一条效果（见 10.7）；解状态每次只解一种（原版规则：按 毒、麻、眠、石、醉、乱、虚、障、默 的顺序取有回合数的最后一种，“全”只在前六种里找，所以“全”也只解这一种，有沉默或魔障、虚弱时“全”可能没有效果），没有对应的状态时没有效果；净化魔障、沉默、虚弱（404–409、411）按同一条规则，各解自己的那种；复活回复 RAND(威力×0.9, 威力×1.1) 的血量（至少 1，不超过最大血量），`result` 为 `heal`，`down` 为 false，下一回合起能行动；防御精灵（光、镜子、守护）对己方一个挂上防御次数，**覆盖**原有次数（不叠加），挂上时 `status_add` 带 `absorb`、`reflect` 或 `vanish`，`result` 为 `none`；复活防御精灵先复活（同上）再挂上防御次数。防御次数只在被物理攻击时减少，不随回合减少，见 10.7 的 `absorb`、`reflect`、`vanish`；召唤龙阵（STAGE-202，`func` 为 `dragon`，原版 `MAGIC_ToCallDragon`）对敌方每个活着的单位各打一下：先过**魔法闪避**，人物的闪避率是运气 × 3（百分点，运气 1–5），宠物和怪物是等级 × 0.2（百分点，最多 30）；掷 1–100，点数大于闪避率才没闪过，闪过的 `result` 为 `dodge`、伤害为 0。没闪过的伤害按物理伤害公式算（攻击力是精灵的威力，攻击方无属性、没有武器、没有暴击），用 `hit` 结算，骑乘和倒下照常。它不反击，不附带异常状态，不受光、镜子、守护（STAGE-199）影响。奔龙阵只打选定的一个（`target_type` 为 `enemy`，必须带 `target`）；强恩阵（`enemy_all`，原版 `BATTLE_MultiToCallDragonMagic` 的 toNo 23–26 循环）**实际等同敌方全体**：它的循环限定在敌方的两排，选哪一排都会打到这一方的两排，照原版实现，不另做成选排；奔雷阵、GM 烟火是 `enemy_all`。召唤龙阵的 MP 照扣。攻击精灵（`attack`）也对每个被打的目标先判魔法闪避，规则同上，闪过的目标效果为 `dodge`、伤害为 0。攻击精灵的命中范围由原版 `magic.txt` 的目标码和 `attmagic.bin` 的 3×5 掩码共同决定（STAGE-218，见 `docs/magic-range.md`）：`target_type` 只表示要不要选目标（`enemy` 选一个、`enemy_all` 不用选），一次 `magic` 事件可能对多个目标给出 `effects`；目标码 9 以选中的人为中心按掩码取格（巨岩撞击是十字、大冰山是单体），目标码 10 以选中敌人所在的一排取格（土石流打 5 个），目标码 11 遍历敌方两排（熔岩爆发、暴风冰刃的掩码只有一排，只打 5 个）；精灵版宠物技能（STAGE-202，`func` 为 `attskill`，参数是 `petskill.txt` 的技能编号）用这个宠物技能的效果，施放者是人物，MP 照扣，目标类型取技能自己的（见 5.8），事件只有一个 `skill`（见 10.7），不另给 `magic` 事件 |
| player | `item` | `slot`（5–19）、`target` | 道具，`slot` 是背包槽位。效果与战斗外的 `item_use`（5.31）一致：回复 HP（“体N”）、回复 MP（“气N”）、解除异常状态、“全”。`target` 是否要带由这个道具的目标类型决定，见下面的“目标类型”；给敌方目标或给已倒下的目标回复 `INVALID_TARGET`（复活类道具相反，见下）。复活类道具（`ITEM_useRessurect`、`ITEM_ResAndDef`，STAGE-262，原版 `battle_item.c` 的 `ITEM_useRessurect` / `ITEM_ResAndDef`）的 `target` 必须带，选己方一个**倒下**的单位（人或宠物，怪物不行，目标类型 `ally_down`）；缺省回复 `INVALID_TARGET`，选了活着的单位回复 `INVALID_TARGET`（说明“是活着的，不需要复活”），选了敌方回复 `INVALID_TARGET`，道具都不扣。复活量是参数里的数字 N，实际回复 RAND(N×0.9, N×1.1)，参数带 `%` 时 N 是最大血量的百分比，N 为 0 回满，至少 1，封顶最大血量；`result` 为 `heal`，`down` 为 false，同复活精灵，不乘恢复率。`ITEM_ResAndDef`（真诚的眼泪：吸收 1 回合；铁壁之石：无效 2 回合）复活之后再对同一个单位挂上防御次数，`status_add` 带 `absorb` 或 `vanish`（同防御精灵，10.7）。道具扣 1 |
| player | `guard` | 无 | 防御 |
| player | `capture` | `target`、`keep`（可选） | 捕捉（真人队同场时宠物归出手的队员，10.14）；`keep` 是资质筛选，见下；随身宠位已满时判定失败，怪物留在场上（STAGE-267，见下） |
| player | `escape` | 无 | 逃跑。真人队同场时只让自己和自己的宠物退场，成功后离队；队长成功则解散全队，战斗继续（10.14） |
| player | `swap_pet` | `pet_slot`（0–19，-1 表示收回） | 换宠：让指定栏位的宠物出战，或把出战宠物收回 |
| pet | `attack` | `target` | 宠物攻击 |
| pet | `skill` | `skill`（1–7）、`target` | 宠物技能，`skill` 是宠物页的技能栏位；`target` 是否要带由这个技能的目标类型决定，见下面的“目标类型”；只能在战斗外用的技能回复 `SKILL_UNUSABLE` |
| pet | `guard` | 无 | 宠物防御 |
| clone:N / clone_pet:N | `attack` | `target` | 分身（或分身的宠物）普通攻击指定的敌方目标（STAGE-186）；只开放 `attack`，其他动作回复 `BAD_REQUEST` |

- `target` 是单位号（0–19）。表里写明必带的参数（如 `attack` 的 `target`）缺少、类型不对、数值超出上面写的范围，回复 `BAD_REQUEST`；范围内但不合法（那个格子没有单位、已倒下，或选的不是这个动作能选的一方）回复 `INVALID_TARGET`。
- **目标类型**（`target_type`）：精灵（5.8 的 `magic`）、道具（5.17 的 `slots`）和宠物技能（5.19 的 `skills`）各自带这个字段，服务端的校验和客户端的选择都按它决定要不要选目标、能选哪一方：
  - `none`：不需要目标，不带 `target`。装备、材料和只能在战斗外用的物品也是这个取值（战斗中用回复 `ITEM_UNUSABLE`）；
  - `self`：只能对自己用，不带 `target`；
  - `ally`：己方一个活着的单位（自己或宠物，一期没有队友），客户端要让玩家选。`target` 缺省时落在自己身上；
  - `ally_all`：己方全体，不带 `target`；
  - `enemy`：敌方一个活着的单位，客户端要让玩家选，**必须带 `target`**（缺少回复 `BAD_REQUEST`，选了己方或倒下的回复 `INVALID_TARGET`）。攻击精灵选的是“中心目标”，实际命中范围可能不止一个（目标码 1/9 按掩码在目标周围取格、目标码 10 打目标所在的一排，见 `docs/magic-range.md`）；
  - `enemy_all`：不用选目标，不带 `target`；攻击精灵的实际命中个数由掩码决定（目标码 11，满员时通常是 10 个，熔岩爆发、暴风冰刃只有 5 个）；
  - `ally_down`：己方一个倒下的单位（人或宠物，怪物不能复活），客户端要让玩家选，**必须带 `target`**（选了活着的或怪物回复 `INVALID_TARGET`）。复活精灵、复活防御精灵，以及复活类道具（复活药、回魂之玉、萨顿家的秘药、真诚的眼泪、铁壁之石）是这个取值。
  - `any`（STAGE-200）：己方或敌方任意一个活着的单位，客户端要让玩家选，**必须带 `target`**（选了倒下的或空格子回复 `INVALID_TARGET`）。只有属性反转精灵（彩虹、极光）是这个取值；极光的效果落在所选单位所在的一方全体（见下面的 `magic` 说明）。
  - 精灵和宠物技能带了 `none`、`self`、`ally_all`、`enemy_all` 的 `target` 时服务端忽略它，结果不变；道具则按物品的目标校验（`self` 的道具带了别人回复 `INVALID_TARGET`，`ally_all` 的道具带了 `target` 时只对那一个单位生效）。
  - 普通攻击（`attack`）、捕捉（`capture`）和宠物攻击都是 `enemy`；防御、逃跑、换宠没有目标。
- **捕捉的资质筛选（STAGE-264）**：`capture` 可以带 `keep` 对象，键只认 `vital`（体力）、`strength`（腕力）、`toughness`（耐久力）、`dexterity`（速度），值是 −4 到 0 的整数，表示这一项的**资质档位**（口径同 5.19 的 `quality[].tier`，0 为满）最差不低于多少。四项都不带（`keep` 缺省、为 `null` 或 `{}`）就是不限，行为与没有这个功能时逐字一致。设了几项就是 AND：全部达标才留下，只设一项就只看它。例：`{"actor": "player", "action": "capture", "target": 16, "keep": {"vital": -2, "strength": -1}}`。
  - 校验：`keep` 不是对象、带了别的键、某项不是整数或不在 −4…0 之间，回复 `BAD_REQUEST`，战斗状态不变。`keep` 在别的 `action` 上也照样校验，但只对 `capture` 生效。
  - 判定在**抓成功之后**做，成败概率不变（`captureCheck` 与原版一致）。资质不合格的宠物**不入库、不占宠位**，但战斗里仍算抓成功：怪物离场（`left` 里有它），`capture` 事件 `ok` 为真、带 `discarded`、没有 `pet_slot`（10.7）；`rewards.pets` 不含它，它也不会出现在 `pets` 里。
  - **与宠位满的关系**：先看宠位，后看资质。随身宠位已满（20 只，寄放的不算）时这次捕捉**判定失败**（STAGE-267，与原版 `BATTLE_Capture` 的 `PET_createPetFromCharaIndex` 返回 −1 时 `flg = 0` 一致）：即使 `captureCheck` 本来判定成功，`capture` 事件也是 `ok` 为 `false`，怪物留在场上（`left` 里没有它），没有 `pet_slot`、没有 `discarded`，`rewards.pets` 也不含它。宠位满时不看资质，所以不会变成“抓成功被丢弃”。
  - 没被筛掉的宠物入栏位的规则不变：`pet_slot` 是放进的栏位号（0 号栏位也出现，STAGE-268），`rewards.pets` 带它。
- **场地属性与属性反转**（STAGE-200，原版 `BATTLE_FieldAttChange`、`BATTLE_MultiAttReverse`）：
  - 场地精灵（大地、水的精灵、火炎、疾风的 Lv1–5 和调和的精灵）的目标是 `none`，不用选，对整场生效，MP 照常扣。每次施放整体覆盖当前场地（属性、威力、回合都换成新的）；调和的精灵把场地设成无属性。施放后紧跟一个 `field` 事件（第 10.7 节），`magic` 事件的 `func` 是 `field`。
  - 场地有属性时，每个单位的伤害在属性相克之后乘 `At_FieldPow / Df_FieldPow`（攻守双方各算一个，场地无属性时两边都是 1，伤害不变）：单位的加成是 `0.5 + 它在场地属性上的值 × 威力 × 0.0001 × 0.5`（值是它的地/水/火/风数值，0–100）。精灵伤害、普攻和物理技能都带这个加成。
  - 回合结算的最后（状态递减之后）场地的剩余回合减一，减到 0 恢复成无属性，发一个 `field` 事件（`attr` 为 `none`）。
  - 属性反转精灵：彩虹的精灵（240）是单人，极光的精灵（241）是所选单位所在的一方全体，目标都是 `any`（要选一个活着的单位，任一方）。对每个目标：反转标志翻转一次；标志开时四属性互换（地↔火、水↔风），标志关时属性不动。所以第一次用是互换，第二次只关标志、属性仍是换过的，第三次再互换换回来（原版行为）。效果的 `result` 为 `none`，标志用 `status_add` / `status_remove` 带上 `reversed`。战斗结束不恢复。
- **分身的指令**（STAGE-186）：`actor` 为 `clone:N`（N 为 1–4，分身人物）或 `clone_pet:N`（分身的宠物），`action` 只能是 `attack`，`target` 必须是敌方一个活着的单位（选了己方、倒下的、空格子回复 `INVALID_TARGET`，缺少或超出 0–19 回复 `BAD_REQUEST`，`actor` 的 N 不在 1–4 回复 `BAD_REQUEST`）。这个 `actor` 不在本回合的 `optional` 里（没有这个分身、倒下或不能行动）回复 `COMMAND_NOT_NEEDED`。它是**可选**指令：不进 `needs`，没有先后顺序（可以在人物指令之前、之后、宠物指令之前发），成功响应的 `next` 是队长接下来还要交的第一个 `actor`（`player` / `pet`），都交齐了是 `wait`；**结算前可以对同一个 `actor` 重新指定，以最后一次为准**，不回复 `COMMAND_SUBMITTED`；不改变 `battle_progress`。结算时指定过的分身打指定的目标，没指定的（含回合超时没来得及发的）按合击顺序打第一个（STAGE-312）；指定的目标在轮到它行动时已经倒下，按合击顺序顺延到下一个还能打的敌人（STAGE-312；方案是 `random` 时仍按原版 `BATTLE_TargetAdjust` 随机改选），不报错。回合已经结算（进入 `replay`）后再发回复 `WRONG_TURN`。观察方（agent 操控时的玩家连接）发这类指令回复 `CONTROLLED_BY_AGENT`，同其他 `battle_command`。
- **先人物后宠物**：同一回合先交 `player` 指令，再交 `pet` 指令。成功响应的 `next` 告诉客户端下一步：`pet` 表示还要交宠物指令，`wait` 表示自己交完了，等其他玩家或结算。人物这回合用了 `swap_pet` 时，新上场的宠物这回合不能行动，`next` 是 `wait`。
- **人物指令在宠物指令交上之前可以重交**（STAGE-105 定，STAGE-109 实现）：上一条人物指令回复了 `next: pet`、宠物指令还没交时，再交一条人物指令，校验通过就**覆盖**先交的那条，回复照常（还是 `next: pet`；改成 `swap_pet` 时是 `wait`）。重交照样校验，不通过时回复错误，先交的那条仍然有效。这是给客户端“宠物指令阶段按 Esc 回到人物指令”用的（`docs/battle-ui.md`「一回合的流程」）：人物指令一直留在服务端，玩家改到一半赶上超时，按先交的那条执行，不会变成默认动作。
- 除此之外，**一个指令只能提交一次**：自己这回合的指令交齐以后（交了宠物指令；或者回复是 `wait`——不需要宠物指令、用了 `swap_pet`），人物、宠物指令都不能再改，再交回复 `COMMAND_SUBMITTED`。校验不通过时回复错误，指令没有被消耗，可以继续提交。
- 重交不改变 `battle_progress`：它只在一名玩家交齐全部指令时推送，重交发生在交齐之前，其他玩家看不到。
- 服务端先判断请求类型、身份、状态，再按下表顺序判断参数。校验错误码：

| 错误码 | 含义 |
|---|---|
| `NOT_IN_BATTLE` | 当前不在战斗中 |
| `WRONG_TURN` | `turn` 不是当前回合，或这一回合已经进入结算；`error.data.turn` 给出当前回合 |
| `COMMAND_NOT_NEEDED` | 这个 `actor` 本回合不需要指令（不在 `needs` 里；分身指令则是不在 `optional` 里） |
| `COMMAND_ORDER` | 还没交人物指令就交宠物指令 |
| `COMMAND_SUBMITTED` | 这个 `actor` 本回合已经交过指令，而且不能再改（宠物指令交过了，或者自己的指令已经交齐；见上面的“可以重交”） |
| `INVALID_TARGET` | 目标不存在（那个格子没有单位）、已倒下，或选了这个动作不能选的一方（攻击、敌方类的精灵和技能选了己方，治疗选了敌方，`self` 的道具选了别人） |
| `ACTION_NOT_ALLOWED` | 本场战斗不允许（`allow` 里关闭了逃跑或捕捉），或当前状态不能这样做 |
| `NOT_ENOUGH_MP` | 精灵所需的 MP 不够 |
| `MAGIC_UNUSABLE` | 精灵栏位为空，或这个精灵不能在战斗中使用；人物带着沉默（`nocast`）时交 `magic` 也回复这个码，message 为“沉默中，不能使用精灵”（STAGE-201） |
| `ITEM_UNUSABLE` | 槽位里没有物品、没有使用函数或效果还不支持、场合不对（物品表标记为只能在战斗外使用）、不能给宠物用气力药 |
| `SKILL_UNUSABLE` | 宠物技能栏位为空、这个技能现在不能用，或战斗技能在战斗外用（见 5.55；加工、料理在战斗外用的是合成，不回复这个码） |
| `PET_UNAVAILABLE` | `swap_pet` 的栏位没有宠物、宠物已倒下、已经出战，或等级太高不听指挥 |

### 10.6 推送 `battle_progress`：有玩家交完指令

```jsonc
{"type": "battle_progress", "data": {"battle_id": "b_1", "turn": 1, "done": 1, "total": 2}}
{"type": "battle_progress", "data": {"battle_id": "b_1", "turn": 1, "done": 0, "total": 3, "deadline_ms": 1760000120000, "server_time_ms": 1760000000000}}  // 真人队同场：第一条指令被接受
```

每有一名玩家交完他需要交的全部指令（人物和宠物）就推送一次，**包括自己**。所有玩家都交完后，服务端马上结算并推送 `battle_round`。

真人队同场（`kind` 为 `party`，10.14）时还有两点：①本回合第一条被接受的 `battle_command`（任何队员的）也推送一次，`done` 可能为 0，并带 `deadline_ms`、`server_time_ms`，120 秒的交指令截止时间从这一刻算起；②之后每有一名队员交齐就推送一次（同上）。超时踢人、掉线之类不在回合结算里的离场另有 `battle_leave`（10.14），之后 `total` 以它为准。

### 10.7 推送 `battle_round`：一回合的结算

```jsonc
{"type": "battle_round", "data": {"battle_id": "b_1", "turn": 1,
  "events": [
    {"kind": "attack", "actor": 0, "weapon": "axe", "effects": [
      {"target": 15, "result": "hit", "delta": -9, "hp_pct": 70, "down": false}]},
    {"kind": "attack", "actor": 16, "effects": [
      {"target": 5, "result": "dodge", "delta": 0, "hp": 35, "hp_pct": 100, "down": false}]}
  ],
  "state": [ {"no": 0, "hp": 44, "hp_pct": 100, "mp": 100, "status": [], "down": false}, ... ]}}
```

服务端把一回合的结果算好，按发生顺序给出**事件序列**；客户端按序逐条播放（节奏见 `docs/battle-ui.md` “结算回放”），播完后用 `state` 校正最终状态。

**事件**的通用字段：`kind`（种类）、`actor`（行动者的单位号，系统产生的事件没有）、`effects`（对各目标产生的结果，可以没有）、`left`（本事件后离开战场的单位号）、`joined`（本事件后加入战场的单位，结构同 `unit`）。一个事件里 `effects` 有多项，表示群体攻击或连击，播放时合并成一个节拍。合击（`combo`）事件另有 `actors`（见下）。

**效果 `effect`**：

| 字段 | 含义 |
|---|---|
| `target` | 目标单位号 |
| `result` | `hit` 命中、`crit` 暴击、`miss` 未命中、`dodge` 闪避、`block` 全防（被完全挡住）、`heal` 回复（骑乘的人物被回复时，总量按缺口水位拆给人和骑宠，效果只写人物那一份，见 `docs/ride.md` §5，STAGE-257）、`none` 只改变状态、`absorb` 吸收、`reflect` 反射、`vanish` 无效（STAGE-199，见下） |
| `delta` | 血量变化：伤害为负，回复为正，没有变化为 0；对双方都给出，界面上飘字用 |
| `mp_delta` | 气力回复的增量（STAGE-30 的道具回复 MP 用，可省略）；血量变化仍在 `delta` |
| `hp`、`hp_pct` | 这一刻目标的血量：`hp_pct` 总有，`hp` 只在目标是己方单位时有（同 10.2 的隐藏规则） |
| `down` | 目标这一刻倒下了 |
| `status_add`、`status_remove` | 目标这一刻增加、去掉的状态标记（取值同 10.2），没有变化时可以省略 |
| `intent` | **想打的那个目标**（STAGE-167）。忠犬代受、守护之类让实际承伤者变了时，`target` 是真正挨打的单位、`intent` 还是原目标；两者相同时省略。客户端据此把出手方向和动作画到原目标身上，伤害落在实际承伤者身上 |
| `flown` | 这一次被打飞（STAGE-167，A05）。打飞和普通倒下不同：客户端要把单位沿攻击方向移出、保留离场标记，不能只画成「血空变灰」。省略表示没有打飞 |

**防御结果**（STAGE-199，原版 `BATTLE_GetDamageReact` / `BATTLE_DamageSub`）：目标身上有防御次数（10.2 的 `absorb`、`reflect`、`vanish`）时，只有**物理类直接攻击**（普通攻击、合击、连击、回旋镖、异常攻击、反击、宠物的物理技能）会触发，精灵伤害和中毒扣血不触发；带着任何防御次数的目标**不会闪避**。伤害大于 0 的一下按 无效 > 吸收 > 反射 的优先级取第一个次数大于 0 的，消耗一次：
- `vanish`（无效）：这一下伤害为 0，`effect` 的 `target` 是被打的单位，`delta` 为 0；
- `absorb`（吸收）：这一下伤害不扣血，改成回复被打的单位，`delta` 为正（回复量是这一下的伤害值，不超过最大血量）；
- `reflect`（反射）：这一下伤害由**攻击者**承受，`effect` 的 `target` 是攻击者、`delta` 为负（攻击者扣血，可能倒下），`intent` 指回反射的那一方（被打的单位）。攻击者装备投掷武器（回旋标、投掷斧、投掷石）时不反射，按普通伤害结算，计数器不减。
次数用完时，`status_remove` 带上这个标记：吸收、无效记在被打的单位那条 `effect` 上；反射则在被打的单位身上另起一条 `result` 为 `none` 的 `effect`。合击（`combo`）的 `effects` 与 `actors` 一一对应，合击里不另起条目，反射次数用完的标记只在回合结束的 `state` 里体现。

**表现字段**（STAGE-167，`docs/battle-ui.md`「谁在打谁」）——只用于动作与特效，不参与任何规则判定：

| 字段 | 出现的事件 | 含义 |
|---|---|---|
| `weapon` | `attack`、`combo` | 出手的武器身份：`fist`（空手/拳套）、`axe`、`club`、`spear`、`claw`、`bow`、`throw_axe`、`throw_stone`、`boomerang`。客户端按它挑动作和轨迹，**不按名字或伤害猜**。认不出来的值按中性处理，仍保留真实来源与结果 |
| `magic` | `magic` | 精灵的表现信息：`{id, func, elem, all, status}`。`func` 是 `attack`/`heal`/`heal_other`/`revive`/`cure`/`status`/`field`/`reverse`/`defend`/`revive_defend`（`field`、`reverse` 是 STAGE-200 的场地属性与属性反转，它们的效果分别见 `field` 事件和 `effect` 的 `reversed`；`dragon` 是 STAGE-202 的召唤龙阵，每个目标的 `effect` 是 `hit` 或 `dodge`，按普通伤害显示），`elem` 是攻击系的 `earth`/`water`/`fire`/`wind`，`all` 表示不用选目标（攻击精灵的目标码 11，实际命中格仍由掩码决定），`status` 是该精灵挂上或解掉的状态。`name` 只用来显示 |
| `row` | 回旋镖的 `attack` | 这一趟扫过的横排（0–3）：每方 10 格，0–4 是人物排、5–9 是宠物排，整场 20 格共 4 排。客户端按它把武器画到那一排，不自己按 `effects` 的编号推 |

回旋镖的 `effects` 按扫过的顺序排列，**最多 5 项**（一排 5 格），每个目标独立判定命中/闪避/暴击/代受，伤害各自乘 0.3（原版 `gBattleDamageModyfy`）。空位、已离场、不可选者不出现在 `effects` 里，客户端不要补受击。

**事件种类 `kind`**：

| kind | 含义 | 附加字段 |
|---|---|---|
| `attack` | 攻击 | |
| `counter` | 反击（STAGE-292）：普通攻击和攻击类宠物技能结算后，被打的一方与攻击方轮流反击，同一次攻击后可以连着出现多个 `counter` 事件（最多 5 次，`actor` 是这一下反击的一方）。反击只在被打一方（或攻击方）此刻的指令是攻击或不防守战法时才有：没交攻击的、防御、精灵、道具的单位不反击；突击或地球一周释放出手的单位不能被反过来反击，投掷武器（弓、投斧、回旋镖）两边都不反击。会心、被防御挡下、打空或被打倒时连环结束，`counter` 事件的 `effects` 伤害为普攻的 75%，不触发忠犬替挡 | 无 |
| `combo` | 合击（STAGE-126）：同一方挨着出手的几个单位合成一次行动，打同一个目标 | `actors` 全部出手者，按出手顺序，`actor` 是第一个；`effects` 与 `actors` 一一对应，每人一条 |
| `guard` | 防御（`effects` 里给自己加 `guard`） | |
| `magic` | 精灵（魔法） | `name` 精灵名 |
| `item` | 使用道具 | `name` 物品名 |
| `skill` | 宠物技能 | `name` 技能名。精灵版宠物技能（STAGE-202，`magic` 的 `func` 为 `attskill`）不另给 `magic` 事件，直接是这个 `skill` 事件（`actor` 是人物），客户端的技能字幕照用 |
| `capture` | 捕捉 | `target`、`ok`；成功时 `pet_slot` 是放进的宠物栏位（0 号栏位也出现，STAGE-268），目标在 `left` 里。资质被 10.5 的 `keep` 筛掉时（STAGE-264）`ok` 也为真、目标同样在 `left` 里，但没有 `pet_slot`，改带 `discarded`：`{name, level, quality: [{stat, points, tier}×4], reason}`，`quality` 顺序固定为 vital、strength、toughness、dexterity，`reason` 写第一个不达标的项，如 `"体力 −3 < −2"`。随身宠位已满时（STAGE-267）`ok` 为 `false`、怪物留在场上：没有 `left`、`pet_slot`、`discarded`，也不进 `rewards.pets` |
| `escape` | 逃跑 | `ok`；成功时 `actor` 和他的宠物在 `left` 里 |
| `swap_pet` | 换宠 | `pet_slot` 是换到的栏位（0 号栏位也出现，收回是 `-1`，STAGE-268）；`actor` 是**主人**（旧宠物收回他、新宠物从他处放出，STAGE-167），旧宠物在 `left` 里、新宠物在 `joined` 里，顺序固定为先收后放；只收回或只放出时只出现其中一项 |
| `disobey` | 宠物这回合不听话（STAGE-127）：紧接在它本回合的行动之前，之后的事件就是它实际做的 | `actor` 是宠物，`reason`：`retarget` 换了目标、`random` 乱来随机出招、`owner` 打主人、`enemy` 打敌人、`escape` 跑掉、`no_skill` 没有技能不出手 |
| `pet_escape` | 宠物不听话跑掉（STAGE-127）：人物不跑，战斗继续 | `actor` 是宠物，宠物在 `left` 里 |
| `skip` | 本回合没有行动 | `reason`：`timeout` 超时、`sleep`、`stone`、`paralysis`、`confusion`、`barrier`（魔障，STAGE-201）、`disobey`（不听话，不出手）等 |
| `status_tick` | 回合末的状态结算（如中毒扣血） | 没有 `actor` |
| `field` | 场地属性变化或恢复（STAGE-200）：施放场地精灵后紧跟在它的 `magic` 事件之后；回合末场地的剩余回合减到 0 时在 `status_tick` 之后 | `field`：`{attr, pow, left}`，`attr` 是 `earth`/`water`/`fire`/`wind`，无属性是 `none`（这时 `pow`、`left` 为 0）；没有 `actor` |

- **资质筛选丢弃（STAGE-264）**：抓成功但被 10.5 的 `keep` 筛掉，例（用例 `battle_capture_keep_discard`）：`{"kind": "capture", "actor": 0, "ok": true, "left": [15], "target": 15, "discarded": {"name": "乌力", "level": 1, "quality": [{"stat": "vital", "points": 21, "tier": -1}, {"stat": "strength", "points": 10, "tier": -4}, {"stat": "toughness", "points": 15, "tier": -2}, {"stat": "dexterity", "points": 27, "tier": 0}], "reason": "体力 −1 < 0"}}`。这只宠物没有入库，`pet_slot` 不出现，`rewards.pets` 不含它，`pets` 列表不变。
- **宠位已满（STAGE-267）**：随身宠位已有 20 只时抓取失败，例（用例 `battle_capture_full`）：`{"kind": "capture", "actor": 0, "ok": false, "target": 15}`。怪物留在场上（没有 `left`），没有 `pet_slot`、`discarded`，`rewards.pets` 不含它，`pets` 列表不变；下一回合腾出栏位后可以重新抓。
- **宠物不听话**（STAGE-127，原版 `BATTLE_PetLoyalCheck`）：宠物的忠诚（`loyalty`）低于 80 时，每回合结算时摇一次 d100 决定它听不听话（忠诚 ≥ 80 一定听话）。忠诚 40–79 有机会换目标（忠诚越低机会越大），20–39 七成乱来（随机出一招），10–19 八成打主人、其余打敌人，10 以下六成打主人、四成跑掉（跑掉的宠物离场，`pet_escape`）；没有任何技能的宠物不论抽中哪一种都不出手。不听话的宠物这回合不参与合击；抽中乱来（`random`）的宠物在它出手时撤销本回合登记的忠犬（原版这时清掉 `CHAR_BATTLEFLG_GUARDIAN`），之后不再替主人挡刀，它出手之前主人挨打仍照常替挡。`disobey` 事件先于它实际的行动发出，玩家的指令不受影响。原版的判定是 `Rand < N`，所以各档的概率比整数百分比少 1%（如 70–79 档是 9%）。
- **魔抗系**（STAGE-228，原版 `MAGIC_MagicStatusChange`、`BATTLE_MultiAttMagic`）：挂载不掷骰、不看异常状态，只与其他魔抗系互斥（带着任何一种就 `miss`、状态不变）；回合数是精灵的 turn（不 +1），回合末和其他状态一起减，减到 0 发 `status_remove`。魔抗的加成（守方抗性 × 数值%，抗性大于 0 才加）只在攻击精灵的伤害里生效；火、电、冰抗只占状态槽，不改数值。
- **魔障与虚弱的回合末冻结**（STAGE-201，原版 `BATTLE_StatusSeq`）：单位带着虚弱或魔障时，回合末它的全部状态（包括虚弱、魔障自己）照常减一，减完又加回 1，回合数卡在原值，不会自己消失；计数减到 0 时照样发 `status_remove`（原版给客户端的“解除”），但内部值仍是原值。只有净化精灵（404–409）、道具或战斗结束能解掉。沉默不在这条规则里，正常每回合减一。中毒的扣血照常。
- **合击**（STAGE-126，原版的 `BATTLE_COM_COMBO`）：服务端在每回合结算前自动判定，玩家不用下指令。本回合都选了普通攻击、能行动、没装投掷武器（回旋标等）、打同一个目标的同一方单位，如果在出手顺序里挨着，起头的那一个按概率发动（人物方 50%，怪物方 20%），后面挨着且条件相同的并入，合成一个 `combo` 事件；中间隔着对方的单位、目标不同或不能行动都会断开这一组。合击不判闪避（`effects` 里不会出现 `dodge`、`miss`），每人至少打 1 点，不会触发被打一方的反击；目标被前面的人打倒时，后面的人不再出手（`actors` 和 `effects` 都只到打倒目标的那一下）。这与原版 `BATTLE_Combo` 一致：起头的那一下已经发出，所以 `actors` 可以只剩起头者，此时仍发 `combo`、界面仍按合击显示，不降级成普通攻击（原版只在轮到起头者时发现组里已没有能动的人，才降级，见 `ComboCheck2`）。播放时合成一个节拍，每个出手者的伤害各占一条 `effects`，界面把几下的伤害合成一个飘字。
- 客户端遇到**不认识的 `kind`** 时，不播放这个事件的动画，但仍然应用它的 `effects`、`left`、`joined`，不报错，这样以后加新的事件种类不会让老客户端卡住。
- `state`：这一回合结束后场上**所有**单位的最终状态（`no`、`hp_pct`、己方单位的 `hp`、自己人物的 `mp`、`status`、`down`）。骑乘的人物项还带 `mount`（STAGE-250，结构同 10.2），骑宠的血量在这里校正；没骑乘的人物项不带这个字段。玩家按 `Space` 跳过回放时直接用它。
- 事件序列里出现过的单位号，都在 `battle_start` 的 `units` 里，或者在之前的 `joined` 里。
- 人物挨打或出手打中时，装备耐久可能变化（STAGE-209）：事件（`attack` 等）带 `crush` 数组，每项 `{"text": "铁甲是受损的。", "red": true}`，`text` 是给玩家看的系统消息，`red` 为真时用红色，否则黄色（“X因过度损坏而消失。”为黄色）。没有变化时不带 `crush`。战斗结束后耐久的最终值随 `inventory_changed` 推送（见 5.18）。
- 道具因出招被消耗时，服务端在 `battle_round` **之前**推送 `inventory_changed`（见 5.18）。人物和宠物的血量、MP、经验、宠物列表的变化，等战斗结束后一次补发（见 10.9）。

### 10.8 `battle_ack`：播完确认

```jsonc
{"id": 22, "type": "battle_ack", "data": {"turn": 1}}
{"id": 22, "type": "battle_ack", "ok": true, "data": {}}
```

- 客户端播完（或跳过）一回合的结算后发出。服务端等这个角色**所有在线连接**（玩家和 agent）都确认，组队时再等全部玩家，或等到超时（默认 20 秒，见第 7 节），然后推送下一回合的 `battle_turn`，或者 `battle_end`。
- 同一回合重复确认直接回复成功。`turn` 不是当前回合回复 `WRONG_TURN`，不在战斗中回复 `NOT_IN_BATTLE`。
- **战斗刚结束后迟到的确认按成功回复**（STAGE-282）：战斗结束（`battle_end` 已推送）之后，如果收到的 `turn` 正好是这场战斗的最后一个回合（等于 `battle_end` 的 `turn`），回复 `{}` 成功，不当错误——客户端或 agent 回放时发出的最后一回合确认，常常比 `battle_end` 晚到。这条只对「刚结束的那一场」生效；从没进过战斗，或 `turn` 对不上最后一回合，仍回复 `NOT_IN_BATTLE`；新战斗一开始，上一场的记录就作废。
- **最后一回合不用确认**：这一回合让战斗结束时，服务端在 `battle_round` 后面**紧接着**推送 `battle_end`，客户端等回放播完再弹结果窗口。

### 10.9 推送 `battle_end`：战斗结束

```jsonc
{"type": "battle_end", "data": {"battle_id": "b_1", "result": "win", "turn": 2,
  "rewards": {
    "exp": [{"who": "player", "name": "勇者小明", "gain": 6, "level_from": 1, "level_to": 2, "skill_points": 3},
            {"who": "pet", "slot": 0, "name": "乌力", "gain": 6, "level_from": 5, "level_to": 5}],
    "gold": 30, "items": [{"item_id": 0, "name": "小斧头", "count": 1}], "pets": []}}}
```

| `result` | 含义 | 随附内容 |
|---|---|---|
| `win` | 胜利 | `rewards`：`exp`（人物和宠物各一项，宠物那一项带 `slot`（宠物栏位 0–19，0 号栏位也出现，STAGE-268），人物那一项没有 `slot`；`gain` 是获得的经验，已乘上全服经验倍数 `exp_rate`——数据库 `game_settings` 表的全服设置，默认 1，管理命令改，金币等其他奖励不受它影响，STAGE-59；升级时 `level_to` 大于 `level_from`，人物升级会带 `skill_points`，是获得的能力点，每级 3 点；宠物升级后的能力和忠诚看随后的 `pets_changed`。升级按原版扣除本级所需经验，随后推送的 `status_changed` / `pets_changed` 里 `exp` 是扣剩下的本级进度）、`gold`、`items`、`pets`（捕获成功得到的宠物，`slot`、`name`、`level`）；NPC 战斗且 NPC 有结束台词（参数的 `endmsg`）时多一个 `npc`：`{"id", "name", "say"}`，`say` 是结束台词（STAGE-96） |
| `lose` | 失败 | `penalty`：`respawn`（失败后人物所在的位置，结构同 `position`；被打飞时是送回的地点，被打死时是原地不动的当前位置）、`in_place`（只有被打死原地复活时为 `true`，被打飞时省略）以及按原版规则的损失 `gold_lost`、`exp_lost`；被打飞时之后服务端推送 `position_changed`，原地复活不推 |
| `escaped` | 自己逃离了战斗（真人队同场时同时离队，10.14） | 没有附加内容 |
| `draw` | 没分胜负（敌人全部逃走、超过战斗时限等）。只要有敌人被打死或捉走，其余逃走的不算存活的敌人，结果是 `win`（STAGE-193） | 没有附加内容 |
| `forced_out` | 真人队同场时超时被踢出战斗（10.14）。没有经验、战利品、金钱，也没有惩罚（不判负、不扣魅力），宠物随之退场，血量和魔法照存。之后收到 `party_changed`（`kicked`，`cause` 为 `battle_timeout`） | 没有附加内容 |

- 己方落败只看人物（STAGE-169）：人物倒下战斗立刻判 `lose`，同一回合里排在后面的单位（含还有血的宠物）不再出手；分身队的单位同样不数。
- 失败后人物的去向（STAGE-194）：
  - **被打死**（人物倒下但没有被打飞）：原地以 1 点血复活，**不传送**，位置、朝向都不变，`penalty.respawn` 是当前位置，`penalty.in_place` 为 `true`。原版出处：`gmsv/battle/battle.c` 的 `_BATTLE_Exit`，人物 `ISDIE` 清掉、`HP` 设为 1，没有传送。
  - **被打飞**（`PlayerFlown()` 为真，见魅力一条）：送回记录的存档点落点，没有记录过回出生地，`penalty.respawn` 是落点，省略 `in_place`，血量剩 1，之后推送 `position_changed`。**“被打飞回记录点”是项目自己的规则，原版源码里没有对应代码**，原版被打倒只在原地复活。
  - 两种情况都有魅力扣减（`charmOnDeath`）、金币和经验的损失，宠物状态等其他结算照旧。

- 真人队同场（`kind` 为 `party`）时每人各收一条 `battle_end`，`rewards` 只含本人的份额，`result` 按本人的结局取值（`escaped`、`forced_out`，或同场的 `win` / `lose` / `draw`）；`lose` 的 `penalty` 按本人的状态（被打死原地复活、被打飞送回）。规则见 10.14。
- 决斗（`kind` 为 `duel`，STAGE-306）：每人一条，没有 `rewards` 和 `penalty`，多一项 `duel_point: {"change": -10, "total": 90}`（`change` 是写回后的实际变化，`total` 是写回后的值，下限 0；逃跑、超时离场的人当场收到自己的这一条；规则见 `docs/pk.md` 2.2）。倒下的人物 1 血留在原地，宠物血量照存。
- 没有的内容可以省略；捕获到宠物但整场战斗输了，宠物也保留，`rewards` 在 `lose` 时不出现，宠物只通过 `pets_changed` 体现。
- 忠诚（STAGE-127，`include/battle.h` 的 `AI_FIX_*`）：可变部分以 1/100 点计，上限 ±100 点。胜利时在场的宠物每打倒一只敌人加一次，敌人比宠物高级 +20（+0.2 点），否则 +1（+0.01 点）；人物被打飞而输，出战的宠物 -1000（-10 点，人物 10 级以下减半）；主人攻击自己的宠物每次 -200（-2 点）；宠物被道具或精灵回复，每场战斗一次 +10（+0.1 点）；被复活精灵救起 +300（+3 点）。PvE 战斗（野外战斗和 NPC 战斗）里单位新倒下时扣（STAGE-146，原版 `BATTLE_NormalDeadExtra`，条件是 `P_vs_E && norisk == 0`；无风险战斗如道场、人物对人物战斗不扣）：人物倒下，出战的宠物 -100（-1 点）；宠物倒下，它自己 -500（-5 点）；主人 10 级及以下都减半。被打飞的人物不在倒下时扣（走上面的打飞结算），战斗开始就是倒下的宠物不扣，复活后再倒下再扣一次。宠物换下去再换回来，血量和可变部分沿用战斗里的值。这些变化随 `pets_changed` 推送，宠物的 `loyalty` 随之现算。
- 魅力（STAGE-61）：胜利且升了级，魅力 +2（升几级只加一次）；失败时被打倒，魅力 -2，等级 10 以下减半为 -1；失败时人物是被打飞的（STAGE-63：单次伤害 ≥ 最大血量×1.2+20，或打倒时溢出的血量跨战斗累计达到同一阈值后打飞，累计清零），魅力 -4，同样 10 级以下减半为 -2，替代普通被打倒的 -2。只有人物会被打飞，中毒扣血不会打飞。结果经 `status_changed` 里的 `derived.charm` 推送，宠物的忠诚随之现算（`pets_changed`）。逃跑、捕捉不改魅力。
- `battle_end` 之后，服务端依次推送数据变化：`position_changed`（只有失败被打飞送回时，或者 NPC 战斗赢了被传送到参数里的落点时，STAGE-96；原地复活不推）、`status_changed`、`inventory_changed`（有变化时，包括 NPC 战斗赢了收走道具）、`pets_changed`（有变化时）。收到 `battle_end` 的那一刻起，连接回到普通的“世界中”。
- NPC 战斗（STAGE-96）：输、逃、平都和野外战斗一样，NPC 不变，可以再打；只有赢了才可能传送（参数 `dieact=1`）。无论哪种结果，这个 NPC 的占用（`one_battle`）都在战斗结束时释放。

### 10.10 推送 `status_changed`：角色数值变了

```jsonc
{"type": "status_changed", "data": {"status": {"level": 2, "hp": 51, "max_hp": 51, "mp": 100, ... }}}
```

`data.status` 的结构与 5.8 里 `enter_world` 响应的 `status` 对象完全相同，是推送这一刻的完整数值。角色的血量、MP、经验、等级、能力点等变化后，服务端给世界中的每一条连接推送，规则同 `inventory_changed`。目前的来源有四个：战斗结束；加点（5.21）；管理命令 `stoneage-admin givepoint`（直接写库并发出 PostgreSQL 通知，服务端收到后重读这个角色的能力点再推送；角色不在线时只写库，等下次进入世界时可见）；管理命令 `stoneage-admin givemagic`（同样直接写库并通知，重读这个角色的精灵栏位再推送）。以后升级等需求沿用同一条推送。

### 10.11 `battle`：查看当前战斗（查看类，世界中）

```jsonc
{"id": 25, "type": "battle", "data": {}}
{"id": 25, "type": "battle", "ok": true, "data": {"in_battle": false}}
{"id": 25, "type": "battle", "ok": true, "data": {
  "in_battle": true, "battle_id": "b_1", "kind": "wild", "scene": "野外", "you": 0, "allow": { ... }, "units": [ ... ],
  "turn": 2, "phase": "command", "deadline_ms": 1760000030000, "server_time_ms": 1760000010000,
  "needs": ["player", "pet"], "submitted": {"player": true, "pet": false}, "progress": {"done": 0, "total": 1}}}
```

- 用于中途加入的连接（agent 在战斗中进入世界，或玩家重新连接）补上战斗的现状。`units` 反映此刻场上的单位。
  场地有属性时还带 `field`（STAGE-200，同 10.4）。
  NPC 战斗（STAGE-96）还带 `npc`：`{"id", "name"}`（不带台词）。
- `optional`、`submitted` 里的分身项（STAGE-186）：分身队战斗里 `phase` 为 `command` 时多一个 `optional`（同 10.4），`submitted` 里为其中每个 `actor` 多一项布尔值（如 `"clone:1": true`），表示这回合玩家有没有给它指定过目标；具体指定了谁不在查询里返回（重连后的客户端只知道“指定过”）。
- `phase`：`command` 表示在等出招，`replay` 表示这一回合已结算、在等播放确认（此时不带 `needs` 和 `submitted`）。真人队同场（10.14）时 `deadline_ms` 在第一条指令被接受之前为 `null`，之后是 120 秒截止时间；`you` 是查询者自己的单位号。
- `enter_world` 的响应里，**只有在战斗中时**多一个 `battle` 字段，内容同上（`in_battle` 恒为 `true`）；不在战斗中时没有这个字段。
- 提交过的指令按**角色**保存，不属于某条连接：控制权在回合中间转手（`take_control` 等），新的操控方接着当前阶段继续，不会重复提交。

### 10.12 观察模式（agent 协同）

- agent 是操控方时，玩家连接发 `battle_command` 回复 `CONTROLLED_BY_AGENT`，玩家用 `take_control` 夺回后从当前阶段继续；反过来 agent 被拒回复 `CONTROLLED_BY_PLAYER`。
- 两条连接收到完全相同的战斗推送。`battle_ack` 两边都可以发，服务端要等两边都确认（或超时），所以观察方可以通过不确认来让战斗“等一等”，但最多等 20 秒。
- 战斗中 agent 的每条 `battle_command` 都写审计（见 9.6）。

### 10.13 用例

`docs/protocol/cases/battle_*.jsonl` 是战斗的协议用例。每条用例的 `meta.stage` 标在各自的文件里（STAGE-43 起）：属于哪个需求，就在那个需求实现后由 `server/casetest` 的回归测试真正重放（此前只有格式检查）。用例用管理员行 `{"conn": "setup", "dir": "admin", "do": "start_battle", "character": "勇者小明", "scenario": "win"}` 让服务端立刻开一场战斗，`scenario` 是预设的剧本名（`win`、`lose`、`escape`、`capture`、`no_escape`、`showcase`、`beaten` 等，由服务端的测试引擎用固定的随机数种子重放，剧本定义在 `server/battle/scenario.go`）。`npc_win`（STAGE-96）是里昂蛙王的一场：NPC、台词取自参数文件，怪物换成与 `win` 相同的两只 1 级怪，保证重放一定胜利（真正的阵容 90 级，1 级人物打不赢）。用例里**依赖随机数的数值**（伤害、血量百分比、经验、掉落）一律写成 `"*=样例"`，固定的是消息的结构、顺序和单位号。模拟服务端只回放 `player` 连接的行，管理员行被跳过，战斗推送直接按顺序发出；`deadline_ms` / `server_time_ms` 的通配由模拟服务端换成发送时的真实时间。校准用例数值的方法见 `docs/server.md` 的“战斗用例”。

- `battle_clone_target.jsonl`（`meta.stage` 是 STAGE-186，已实现）：分身队开战（剧本 `lose`，怪物血厚，第一回合打不完），`battle_turn` 带 `optional`（`clone:1`–`4`、`clone_pet:1`–`4`）；给 `clone:1` 和 `clone_pet:2` 指定打 15 号（`next` 是 `player`），默认按合击顺序打 16 号，指定覆盖默认；分身指令的校验——`guard` 回复 `BAD_REQUEST`、选己方回复 `INVALID_TARGET`、`clone:5` 回复 `BAD_REQUEST`；`battle` 查询带 `optional` 和 `submitted` 里的分身项；队长交齐后 `battle_round` 里 1 号和 7 号打的就是指定的 15 号，没指定的 2、3、4 号等打默认的 16 号（STAGE-312）。`ui_party_battle_clone.jsonl` 是同一流程给客户端场景用的版本（剧本 `win`）。
- `battle_magic_field.jsonl`（`meta.stage` 是 STAGE-200，已实现）：1 级人物、五个精灵（火炎 Lv1、彩虹、极光、大地 Lv1、调和），剧本 `win`，只有人物出手。第 1 回合火炎的精灵（不用选目标）施放，`magic` 后紧跟 `field`（火 60，3 回合），回合末递减为 2（`battle_turn` 带 `field`）；第 2 回合彩虹选空格子回复 `INVALID_TARGET`，再选自己翻转（`reversed`）；第 3 回合极光选 15 号，敌方全体挂上 `reversed`，场地减到 0 时结算末尾推 `field`（`attr` 为 `none`），第 4 回合的 `battle_turn` 不带 `field`；第 4 回合大地的精灵 Lv1（地 60，3 回合）、第 5 回合调和的精灵把场地设成无属性；第 6 回合 `battle` 查询没有 `field`。
- `battle_magic_resist.jsonl`（`meta.stage` 是 STAGE-228，已实现）：1 级人物、精灵魔法防御（392）和火之精灵（419），宠物以倒下进场，剧本 `win`，只有人物出手。第 1 回合魔法防御挂上魔抗（3 回合，不掷骰）；第 2 回合火之精灵对自己 `miss`（已带魔抗，互斥）；第 3 回合防御，回合末魔抗减到 0，推 `status_remove`；第 4 回合火之精灵挂上火抗（只占槽）；第 5 回合魔法防御对自己 `miss`（已带火抗）；第 6 回合 `battle` 查看，人物的 `status` 带着 `fire_resist`。魔抗的伤害加成只在攻击精灵的伤害里对守方生效，本用例不覆盖，由服务端单元测试覆盖。
- `battle_loyalty.jsonl`（`meta.stage` 是 STAGE-127，已实现）：1 级人物带 5 级的乌力，忠诚只有 8；第一回合宠物的听话判定没过（抽中“打主人”），先推 `disobey`（`reason` 为 `owner`），再是它打人物的 `attack`。用例只放第一回合。
- `battle_loyalty_lose11.jsonl`（`meta.stage` 是 STAGE-146，已实现）：11 级人物（不减半）带 1 级乌力，剧本 `lose`。第 1 回合宠物倒下（宠物自己 -5），第 6 回合人物倒下（出战宠物 -1），`pets_changed` 的忠诚 100 → 94。
- `battle_loyalty_fly11.jsonl`（`meta.stage` 是 STAGE-146，已实现）：11 级人物被剧本 `fly` 一下打飞，第 1 回合结束（宠物同回合倒下 -5）。打飞只扣一次（-10），人物倒下那一下不再扣 -1，忠诚 100 → 85。
- `battle_lose_inplace.jsonl`（`meta.stage` 是 STAGE-194，已实现）：人物先 `warp` 到萨伊那斯（place 5，野外），剧本 `lose`（人物被打倒、没被打飞）。战斗中打开遇敌开关（只打开开关，不开新战）。`battle_end` 的 `penalty.respawn` 是原地坐标，带 `in_place: true`，不推 `position_changed`，遇敌开关不关（没有 `encounter_changed` 关闭推送）。`encounter_interval_ms` 设成 60 秒，用例内不会开下一场。
- `battle_fly_encounter.jsonl`（`meta.stage` 是 STAGE-194，已实现）：同样在萨伊那斯开战，剧本 `fly` 一下打飞人物。`penalty` 不带 `in_place`，人物送回出生村（推送 `position_changed`），遇敌开关按 `lose` 关掉（`encounter_changed` 带 `reason: lose`，随后一条 `notice`）。`battle_round` 只有人物那一下（同回合后排的宠物不再出手，STAGE-169），宠物血量保持满的 15。
- `battle_combo.jsonl`（`meta.stage` 是 STAGE-126，已实现）：剧本 `combo`（打开合击，种子让第一回合宠物和人物的起头判定通过）。人物和宠物都攻击 15 号怪，回合结算的第一个事件是 `combo`：`actors` 为 `[5, 0]`（宠物敏捷高先出手），`effects` 两条都打在 15 号上。用例只放第一回合。其余剧本默认关闭合击，免得改变已校准用例里的随机数；真实遇敌和 NPC 战斗打开合击，所以 `encounter_*.jsonl` 的回合结算随它重新校准过。
- `battle_resubmit.jsonl`（`meta.stage` 是 STAGE-109，已实现）：交人物攻击 → 重交人物防御（成功，`next` 仍是 `pet`）→ 重交时目标不合法（`INVALID_TARGET`）。用例停在交宠物指令之前，不进入结算。
- STAGE-109 实现时，`battle_command_errors.jsonl` 里“交了人物攻击再交人物防御 → `COMMAND_SUBMITTED`”那一对已经删掉：按新规则它会成功（见上面“可以重交”）。单人战斗里宠物指令一交就结算，结算后处于回放阶段，再交的指令回复 `WRONG_TURN`，所以 `COMMAND_SUBMITTED` 在单人战斗的协议用例里看不到，只在组队（另一名玩家还没交齐）的场景里出现，现在没有这样的用例。单元测试 `server/battle/battle_test.go` 的 `TestBattleResubmitLocked` 覆盖“宠物指令交上后”和“换宠后”两种不能改的情况。
- 真人队同场战斗（`kind` 为 `party`，STAGE-275）：设计已定（10.14），**暂未写协议用例**。它要两个账号同时在线、两条连接各自收到不同 `you` 的 `battle_start`，还要固定多人的战斗种子（现有剧本只按单人设计），写出来的数值会随随机数变化，等剧本支持后再补（见 `docs/social-players.md` 2.6 的开工核对）。

### 10.14 真人队同场战斗（STAGE-275）

设计依据 `docs/social-players.md` 2.6，规则照原版 2.5（`docs/battle-party-research.md`）。只在 `battle_start.kind` 为 `party` 时适用；单人战斗、分身队战斗不变。

- **每人一条 `battle_start`**：队员各收一条，`you` 是自己的单位号（队长 0，队员 1–4；宠物号 = 主人号 + 5）。`units`、`party` 相同。`battle_turn`、`battle_round` 也按人分别推送，`needs` 按人不同，其余相同；两边的推送内容对同一个人始终一致（同 10.12）。
- **队友**：是己方的 `kind` 为 `player` 的单位，带 `hp`、`max_hp`、`mp`、`max_mp`（10.2），界面用它显示血和蓝。队友的宠物是 `kind` 为 `pet` 的己方单位，号是主人号 + 5。
- **计时**：`battle_turn.deadline_ms` 为 `null`。本回合第一条被接受的 `battle_command`（任何队员的，人物或宠物）起 120 秒是交指令的截止时间，由推送 `battle_progress`（10.6，带 `deadline_ms`）告知。**没有默认指令**。
- **超时踢出**：截止时间到，仍没交齐的真人被踢出：
  - 被踢的人收到 `battle_end`（`result` 为 `forced_out`，10.9），没有奖励、没有惩罚；宠物随之退场；然后收到 `party_changed`（`reason` 为 `kicked`，`party` 为 `null`，`cause` 为 `battle_timeout`）。
  - 其余真人收到 `battle_leave`（见下）和 `party_changed`（`member_kicked`，`who` 为被踢者，`cause` 为 `battle_timeout`）。
  - 被踢的是队长：其余队员收到 `party_changed`（`disbanded`，`who` 为队长，`cause` 为 `battle_timeout`），队伍解散，战斗继续，其余队员留在战斗中。
- **`battle_leave`**（新推送，只推给仍在战斗中的真人）：`data` 为 `{"battle_id", "no": 离场人的单位号, "name": 名字, "cause": "timeout" | "offline", "left": [离场的单位号，含宠物和分身], "progress": {"done", "total"}}`。`progress` 是离场之后的新数字，之后的 `battle_progress` 以它为准。逃跑不推 `battle_leave`，以 `escape` 事件的 `left` 为准。
- **逃跑**：个人行为（2.6）。成功的 `escape` 事件的 `left` 含逃跑者自己和自己的宠物（队长逃跑时还有分身和它们的宠物）。逃跑者收到 `battle_end`（`escaped`），并收到 `party_changed`（`reason` 为 `left`，`party` 为 `null`，`cause` 为 `battle_escape`）；其余队员收到 `party_changed`（`member_left`，`who` 为逃跑者，`cause` 为 `battle_escape`）。队长逃跑成功则全体收到 `party_changed`（`disbanded`，`who` 为队长，`cause` 为 `battle_escape`），队伍解散，其余队员留在战斗中继续交指令。
- **掉线**：立即退场（连接已断，不推送），不结算经验、战利品、金钱；宠物随之退场；血量照存（人物倒下则回 1）。其余真人收到 `battle_leave`（`cause` 为 `offline`），之后收到 `party_changed`：队员离队为 `member_left`（`who` 为掉线者，`cause` 为 `battle_offline`）；队长掉线为 `disbanded`（`who` 为队长，`cause` 为 `battle_offline`）。**不顺位接任**，队伍直接解散。离场的人重连后不在战斗中（10.1）。
- **`party_changed.cause`**（新增的可选字段）：`battle_escape`、`battle_timeout`、`battle_offline`，只出现在战斗中的离队推送里；战斗外的离队、踢人、解散不带它。`reason` 沿用 5.67（STAGE-274）的取值。**5.67 目前在 STAGE-274 分支，未合入 develop**，本节依赖它。
- **胜负**：己方败 = 己方没有存活的非宠物玩家（分身不算）。此时仍在战斗中的真人都收到 `lose`。敌方全灭、被捕获或逃走则仍在战斗中的真人收到 `win`（同 10.9）。
- **结算**：`battle_end` 每人一条，只含本人份额：经验只给出手者（2.6），掉落按 2.6 随机分给出手列表中的一人（死亡者拿不到），捕捉的宠物入出手者的宠位。金钱、宠物忠诚、装备耐久的细节以 2.6 的开工核对为准。
- **整场 3600 秒**：仍在战斗中的真人都以 `draw` 结束，队伍解散（`disbanded`，不带 `cause`）。
- **观察方**：agent 操控某个队员时，`battle_command` 按 10.12 拦截，其余队员不受影响。
