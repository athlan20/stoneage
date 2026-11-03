# 冒烟测试（前后端联调）

STAGE-140：断线只读、真实重新登录与完整刷新规则见 [reconnect.md](reconnect.md)。

STAGE-137 的真实联调经过“选角 Enter → 角色动作 Enter → 进入世界”，删除经过完整名字与默认取消按钮。旧测试中直接在场景敲英文命令的步骤已改为显式 `/` 命令栏；不把这些旧命令测试当成系统菜单离开的验收。系统菜单离开与退出确认由 STAGE-139/140 的完整冒烟继续覆盖。

客户端（`docs/client.md`）和服务端（`docs/server.md`）分别用模拟服务端、协议测试客户端验收过；联调是把**真实的客户端**和**真实的服务端、PostgreSQL**连起来，从注册一直走到进入游戏，抓只有连在一起才会出现的问题（时序、超时、编码、界面显示）。需求见STAGE-5。**每次发版前跑一遍。**

## 一键运行

```sh
scripts/smoke.sh          # 约 20 秒：真实客户端二进制走完整流程 + 检查数据库和日志里没有明文密码
scripts/smoke.sh --full   # 再加上空闲 75 秒（验证心跳）和客户端的联调测试，约 2 分钟
```

需要 Go、Rust、python3；数据库连接串取自 `.env`（或环境变量 `STONEAGE_DATABASE_URL`）。装了 `psql` 才会检查数据库里有没有明文密码，没装就跳过这一项。

脚本做的事：

1. 编译服务端、客户端和管理命令；
2. 用 `stoneage-server -temp-schema` 起一个真实的服务端：在数据库里新建临时 schema、TCP 和 WebSocket 各一个随机端口、真实的 Argon2 参数，退出时删掉 schema，**不碰开发库里的数据**，注册和登录的限流计数也从零开始（临时 schema 的名字会打进日志，管理命令用它拼出指向同一 schema 的连接串）；
3. `tools/smoke-pty.py` 在伪终端里运行真实的客户端二进制，用一个很小的终端模拟器把屏幕还原成文字再检查，所以检查的是玩家真正看到的内容：注册 → 登录 → 创建角色 → 进入游戏（看地图名和坐标）→ 角色页（按 `2`，看基本信息、能力、衍生数值和地水火风，STAGE-23）→ 加点（`stoneage-admin givepoint Hero1 3` 发 3 点后角色页**不用刷新就显示未分配 3**，`addpoint 体力 2` 后消息栏写明“已加点：体力 +2，未分配点数还剩 1”，`addpoint 体力 9` 报点数不够，STAGE-24）→ 背包页（按 `4`，看空装备格、空物品格和石币；`stoneage-admin give Hero1 0` 发一把小斧头后**页面不用刷新就出现**，效果文字是“攻 +9 防 -3 敏 -3”，STAGE-28）→ 穿脱整理（`item equip 5` 穿上小斧头，角色页攻击从基础值变成 +9；`item unequip 2` 卸下后恢复，STAGE-29）→ 宠物页（按 `3`，新建角色出生就带 3 只宠物：0 号出生地宠物乌力乌力、1 号配套虎、2 号配套雷龙，STAGE-62；`stoneage-admin givepet Hero1 1 5` 给一只乌力后**页面不用刷新就出现**，`pet status 3` 切换详情，STAGE-31；`pet battle 3` 设出战、`pet rename 3 小白` 改名、`pet rename 3` 恢复种族名，STAGE-32）→ 任务页（按 `5`，新角色是灰色的空提示；`stoneage-admin quest Hero1 1 doing` 后页面不用刷新就出现标题计数和消息栏提示，再设 `done` 计数变化，STAGE-86）→ agent 协同（「Agent」页按 `0` 生成令牌，一条原始 TCP 连接扮演 agent 用令牌登录并接管，菜单栏显示“agent 操控中”，`turn` 被拦下，`take` 收回后服务端拒绝 agent 的操作，私聊，`revoke` 吊销后 agent 被断开）→ `leave` → 再进入（位置不变、未分配点数、物品和宠物还在）→ 战斗 NPC（STAGE-96：`leave` 后 `stoneage-admin move Hero1 30605 7 9` 搬到蛙洞，再进入后地图页的本地图 NPC 表里有「里昂蛙王」；没有金珠时 `talk 1735` 弹拒绝台词，数字键关窗；`stoneage-admin give Hero1 19622` 后再 `talk`，弹两行确认，按 `2` 只关窗口；开战和胜负由协议用例 `npc_enemy_*` 与客户端场景测试覆盖）→ 地图页右侧只有「当前位置 + 本地图 NPC」两块，地点编号标在世界图上（`X3`），没有「地点」表和「朝向」（STAGE-81）→ `warp 加加村` 传送（标题里的坐标更新、`warp 99` 报错且位置不变、离开再进入位置已保存）→ `quit`（退出码 0）；
   走完 TCP 那一遍后，同一个服务端再经 `ws://…/ws` 走一遍完整流程（注册 → 登录 → 建角 → 进入游戏 → 离开 → 再进入 → 退出；换账号和角色名 `Hero2`，不跑管理命令和 agent 段）。
4. 用 TCP 那一遍生成的第二个令牌（第一个在 agent 段结尾已吊销），`docs/agent-connect.py` 经 `ws://…/ws` 登录（`agent_login`），`enter_world` 进入世界，`status` 查状态，`leave_world` 离开；输出里不能出现令牌原文（STAGE-118，手册第 7 节）。
5. 在服务端日志、协议日志、数据库的 `accounts`、`auth_audit`、`login_locks`、`characters`、`agent_tokens`、`agent_audit` 里搜密码和令牌原文，找到就失败；最后把 `agent_audit` 里的审计记录打印出来。

## 客户端联调测试

`client/tests/e2e.rs` 用按键驱动真实的客户端逻辑（`App`），连一个真实的服务端进程（同样是临时 schema），每个测试一个独立的服务端：

```sh
cd client
STONEAGE_TEST_DATABASE_URL="$(grep ^STONEAGE_DATABASE_URL= ../.env | cut -d= -f2-)" cargo test --test e2e
```

没设置 `STONEAGE_TEST_DATABASE_URL` 或没有 Go 时，这些测试直接跳过。

## 验收标准与对应的检查

| 验收标准（STAGE-5） | 检查 |
|---|---|
| 注册新账号、登录、创建角色、进入游戏；界面显示的地图名和坐标与所选出生地一致 | `smoke.sh`；`e2e` 的 `register_login_create_enter_leave_reenter`（出生点 1006 (15,22)，界面上有“萨姆吉尔的村长家”和“15,22”，地图页注明“入口位置”） |
| 地图页 `warp` 传送：位置、地图名、@ 同步更新，编号不存在时提示且位置不变，agent 也收到位置更新（STAGE-22） | `smoke.sh` 的“传送”一节；`e2e` 的 `agent_takes_over_player_takes_back`（玩家 `warp 加加村`，agent 收到推送，观察方 `warp` 被拒） |
| 角色页显示基本信息、能力、衍生数值和地水火风（STAGE-23） | `smoke.sh` 的“角色页”一节（屏幕上有“基本信息”“攻击”“未分配”“地水火风”）；`client` 的 `tests/render_character.rs`（数值与公式、80×24、符号降级）；`tests/scenarios.rs` 的 `character_page_shows_status_from_server`（用例 `world_character_panel`，`status` 返回的数值一致）；服务端 `casetest` 的 `world_character_panel` |
| 宠物页显示 20 个栏位和详情；管理命令给宠物后页面不用刷新就更新；`pet status` 切换详情；重新登录宠物仍在（STAGE-31） | `smoke.sh` 的“宠物”一节（新建角色进游戏后宠物页就有出生送的三只，STAGE-62；`givepet Hero1 1 5` 后屏幕上出现乌力，`pet status 3` 切换，重进后仍在）；`client` 的 `tests/render_pets.rs`（空栏位、详情字段、出战优先、切换、已倒下、80×24–200×60）；`tests/scenarios.rs` 的 `pet_page_follows_pets_changes`（用例 `world_pets_give`）；服务端 `casetest` 的 `world_pets`、`world_pets_give`（含 NOTIFY → `pets_changed` 推送与持久化） |
| 宠物管理：`pet battle` 设出战（原出战宠变休息）、`pet standby`/`pet rest`、`pet rename` 改名（不带名字恢复种族名）、`pet drop` 先提示、输入 `yes` 才放生；错误码与权限；改名和出战重新登录仍然生效（STAGE-32） | `smoke.sh` 的“宠物”一节（`pet battle 0`、`pet rename 0 小白`、`pet rename 0` 恢复，重进后标题仍是“乌力 · 0 号 · 出战”）；`client` 的 `tests/scenarios_pet_manage.rs`（用例 `ui_pet_manage`：请求与推送对得上、放生要确认、栏位超出范围本地报错）和 `src/app/pets_manage.rs` 的解析测试；服务端 `server/character/pets_rules_test.go`（出战、待命上限、倒下、名字规则）；服务端 `casetest` 的 `world_pet_manage`、`world_pet_manage_errors`（PET_FAINTED、STANDBY_FULL、NO_PET、INVALID_NAME、放生确认）、`control_pet_manage`（agent 操控时拒绝 `CONTROLLED_BY_AGENT`/`CONTROLLED_BY_PLAYER`） |
| 新建角色出生就带宠物：0 号是出生地宠物（萨姆吉尔乌力乌力、玛丽娜丝凯比、加加村克克尔、卡鲁它那威伯），1、2 号是配套虎和雷龙（按身体图像查表）；都是 1 级、HP 满、资质随机；已有角色进游戏不补发（STAGE-62） | `smoke.sh` 的“宠物”一节（进游戏后 0–2 号栏是乌力乌力、格鲁西斯、布林帖斯，重进后仍在）；服务端 `server/character/startpets_test.go`（四个出生地、资质随机、进游戏不补发）、`server/gamedata/newplayerpets_test.go`（查表、模板存在且 PETFLG > 0、越界）；服务端 `casetest` 的 `character_start_pets`（两个账号四个出生地，宠物页 0–2 号栏） |
| 任务页显示进行中和已完成，标题写两种数量；管理命令改进度后页面不用刷新就更新，消息栏提示进行中或已完成；`quest doing/done/all` 切换范围；重新登录进度仍在（STAGE-86） | `smoke.sh` 的“任务”一节（新角色是空提示；`quest Hero1 1 doing`、`done` 后标题计数变化，重进后“已完成 1”仍在）；`client` 的 `tests/render_quests.rs`（空提示、表格和计数、范围的标题与空提示、翻页）；`tests/scenarios_quests.rs`（用例 `ui_quests`：推送、进行中在前、清除不提示、`quest` 过滤、用法错误、重进）；服务端 `server/character/quests_test.go`（排序、空列表为 []、推送与 changed、同状态不推送、离线可见）、`server/gamedata/quests_test.go`（任务表）；服务端 `casetest` 的 `world_quests`、`world_quests_not_in_world` |
| 背包页显示装备 5 格、物品 15 格和石币；管理命令发物品后页面不用刷新就更新；重新登录物品仍在（STAGE-28） | `smoke.sh` 的“背包”一节（`give Hero1 0` 后屏幕上出现小斧头和效果文字，重进后仍在）；`client` 的 `tests/render_inventory.rs`（空格子、石币、滚动、80×24–200×60）；`tests/scenarios.rs` 的 `bag_page_follows_inventory_changes`（用例 `world_inventory_give`）；服务端 `casetest` 的 `world_inventory`、`world_inventory_give`（含 NOTIFY → 推送与持久化） |
| 穿脱与整理：穿上装备攻击、防御、敏捷立刻变化（status_changed），卸下恢复；不能穿、等级不够、丢弃没有 yes 都给明确提示且数据不变；饰品和手位置正确；重新进入后装备和加成还在（STAGE-29） | `smoke.sh` 的“穿脱整理”一节（`item equip 5` 后角色页攻击变成基础值 +9，`item unequip 2` 恢复）；`client` 的 `tests/scenarios_items.rs`（用例 `ui_item_manage`）和 `src/app/items.rs` 的解析测试；服务端 `server/character/equip_rules_test.go`（加成、互换、部位、等级、数量、饰品顺序、卸下满包、丢弃数量）和 `server/gamedata/items_test.go`（物品表、部位映射）；服务端 `casetest` 的 `world_item_equip`、`world_item_errors`（NOT_EQUIPMENT、REQUIREMENT_NOT_MET、EMPTY_SLOT、BAD_REQUEST、丢弃确认）、`control_item`（agent 操控时拒绝，两端都收到推送）、`ui_item_manage`（模拟服务端回放） |
| 离开世界后再进入、退出客户端后重新登录再进入，位置不变 | 同上（`leave` 再进入；不 `leave` 直接退出，用开发快捷参数重新登录进入） |
| 两个终端同一个账号登录，先登录的被挤下线并显示原因 | `e2e` 的 `second_login_kicks_first_and_shows_reason` |
| 关掉服务端，客户端提示连接断开、回到登录页，不崩溃 | `e2e` 的 `server_shutdown_returns_to_login_page` |
| 连续输错密码后被锁定，客户端显示剩余时间 | `e2e` 的 `wrong_passwords_lock_account_and_show_remaining_time` |
| 全部日志和数据库里没有明文密码 | `smoke.sh` 的第 4 步；`e2e` 里也检查服务端日志和协议日志 |
| 删除角色 | `e2e` 的 `delete_character_flow` |
| agent 用令牌登录、接管、玩家收回、私聊、吊销（STAGE-6） | `smoke.sh` 的 agent 一段；`e2e` 的 `agent::agent_takes_over_player_takes_back`（真实服务端 + 真实客户端 + 原始 TCP 的 agent） |
| 令牌原文不落日志、不落库，只存哈希 | `smoke.sh` 第 4 步 |
| agent 的操作和控制权变化有审计 | `smoke.sh` 最后打印的 `agent_audit` |
| 服务端拒绝名字（含 GM、重名）时，客户端回到出错的一步 | `e2e` 的 `server_rejected_names_return_to_name_step` |
| 中文、emoji 名字原样显示 | `e2e` 的 `unicode_names_round_trip` |
| 角色加点：`addpoint` 扣点数、能力与最大血量变化，消息栏写明结果；点数不够报错且数值不变；管理命令发点数页面不用刷新就更新；重新进入点数还在（STAGE-24） | `smoke.sh` 的“加点”一节（`givepoint Hero1 3` 后角色页显示未分配 3，`addpoint 体力 2` 后消息栏写明剩余点数，`addpoint 体力 9` 报错，重新进入后仍是 1 点）；`client` 的 `tests/scenarios_points.rs`（用例 `ui_addpoint`）；服务端 `casetest` 的 `world_addpoint`、`control_addpoint`、`ui_addpoint`，以及 `server/character/points_test.go`（扣点、上限、死亡、离线发点、持久化） |
| 地图页显示当前地图的 NPC 列表（编号、名字、类型），换地图列表跟着换，没有 NPC 的地图显示“这里没有 NPC”，玩家和 agent 都能查看（STAGE-68） | `smoke.sh`（村长家 2 个 NPC，`warp 加加村` 后列表变成 26 个，名字不乱码）；`client` 的 `tests/scenarios_npc.rs`（用例 `ui_npc_follow`：进世界取一次、warp 后重取）和 `tests/render.rs`（表头、总数、空列表提示、短窗口也能显示，STAGE-81）；服务端 `server/gamedata/npcs_test.go`（四个村的数量与名字、编号连续）和 `server/character/npcs_test.go`（空地图返回 `[]`、观察方也能查）；服务端 `casetest` 的 `world_npcs`（含离开世界后 WRONG_STATE） |
| 地图页右侧只剩「当前位置 + 本地图 NPC」两块：没有「地点」表和「朝向」，面板只写所在/石币/遇敌，地点编号直接标在世界图上（STAGE-81） | `smoke.sh` 的“传送”一节（等到 `X3`，断言「地点 · warp」和「朝向」都不在屏幕上，`warp 99` 的提示说明编号看地图上的 X）；`client` 的 `tests/render.rs`（面板内容、`X编号`、没有地点表、100×24 和 120×40 都能看到 NPC 表） |
| 和 NPC 对话：`talk 编号` 弹出对话窗口，正文与原版参数一致；编号不存在、别处的 NPC、商店类有明确错误不弹窗；Esc 和数字键关窗并同步服务端状态；agent 操控时玩家 talk 被拒、agent talk 两边看到同一窗口；传送和战斗开始自动关窗（STAGE-69） | `smoke.sh`（村长家 `talk 225` 弹窗、Esc 和数字键关窗）；`client` 的 `tests/scenarios_npc_talk.rs`（用例 `ui_npc_talk`：弹窗、choose 报错、两种关窗）和 `tests/render.rs`（窗口正文、选项编号、提示）；服务端 `server/character/dialog_test.go`（校验、战斗与观察拦截、窗口随传送和战斗关闭、观察方关窗）和 `server/gamedata/npcstalk_test.go`（分句、告示牌换行、报时参数与时段切换、摆设默认话）和 `server/character/dialog_timeman_test.go`（报时台词随服务端时钟切换）；服务端 `casetest` 的 `npc_talk`、`npc_talk_control`、`npc_talk_close_on_warp` |
| 商店买卖：`talk 商店` 弹买卖菜单，买（商品列表 → `reply 数量` → 金币减背包增）、卖（回收列表选格 → 金币增背包减），金币不够、等级不够、数量非法有提示且数据不变，战斗中和观察方被拒（STAGE-70） | `smoke.sh`（村长家 warp 后 `talk 198` 菜单）；`client` 的 `tests/scenarios_shop.rs`（用例 `ui_shop`：菜单、商品列表、数量询问、成交关窗）；服务端 `server/character/shop_test.go`（买卖、等级/金币/数量/上限拒绝、战斗与观察拦截）；`server/gamedata/npcshop_test.go`（倍率计价）；服务端 `casetest` 的 `npc_shop` |
| 护士治疗：`talk 护士` 弹确认窗口（受伤报价 = 等级×倍率、免费线以下免费、全满无选项），选治疗回满人物 HP/MP 和宠物 HP 并按公式扣费；钱不够 NOT_ENOUGH_GOLD 不扣钱；战斗中和观察方被拒（STAGE-71） | `client` 的 `tests/scenarios_healer.rs`（用例 `ui_healer`：窗口、治疗、关窗）；服务端 `server/character/healer_test.go`（报价、扣费、宠物回满、免费线、满血拒绝、钱不够、战斗中）；`server/gamedata/npchealer_test.go`（倍率计价）；服务端 `casetest` 的 `npc_healer` |
| 存档点：`talk 储存点` 弹记录窗口，交一件候选物品记录成功（物品被收走、存档点写库、重登仍在），没有合格物品 ACTION_NOT_ALLOWED 不消耗；被打飞送回存档点落点而不是出生地（STAGE-72；被打死则原地复活，STAGE-194） | `client` 的 `tests/scenarios_savepoint.rs`（用例 `ui_savepoint`：窗口、无物品拒绝、记录成功）；服务端 `server/character/savepoint_test.go`（记录收物品、重登仍在、无物品拒绝、战败回存档点/出生地）；服务端 `casetest` 的 `npc_savepoint` |
| 宠物店：`talk 宠物店` 弹菜单（寄放/领取/卖/买），卖按等级×稀有×倍率估价、寄放收 50+等级×4 且宠物从随身列表消失、领取免费取回属性不变、买占位商品入第一个空栏位；栏位满、寄放池满、金币不足有提示且数据不变（STAGE-73） | `server/character/petshop_test.go`（卖/寄放/领取/买、拒绝路径）；`server/gamedata/npcpetshop_test.go`（估价公式）；服务端 `casetest` 的 `npc_petshop`（move_character 到店的完整流程） |
| 宠物技能店：`talk 1763` 弹技能列表（名字、价格、说明，编号 1 起），选技能→宠物→技能栏位→学习：金币扣价格（技能价 × 店倍率）、栏位被覆盖、宠物页立即刷新；金币不够、栏位超出宠物的栏位数、宠物不在、离店太远、战斗中都有提示，金币和技能不变（STAGE-143） | `client` 的 `tests/scenarios_petskill_shop.rs`（用例 `ui_petskill_shop`：列表、选宠物、栏位、学习、回到列表）；`server/character/petskillshop_test.go`（成交、各类拒绝、离店距离）；`server/gamedata/npcpetskillshop_test.go`（上架规则、学习价格取整）；协议用例 `npc_petskillshop`（含 NO_PET、NPC_BAD_CHOICE） |
| 传送员：`talk 传送员` 按通行条件（LV/ITEM/ALLFREE）弹免费或付费确认窗，传送后位置同步、遇敌关闭、门票（DelItem）收走；条件不满足且不能付费不弹窗；钱不够 NOT_ENOUGH_GOLD；依赖庄园条件的传送员跳过 NPC_NOT_TALKABLE（STAGE-74；任务条件 ENDEV/NOWEV 自 STAGE-90 起支持） | `server/character/warpman_test.go`（持票传送与门票收走、无票不弹窗、unsupported 跳过、等级门槛、战斗中）；`server/gamedata/warpcond_test.go`（条件表达式解析、费用公式）；服务端 `casetest` 的 `npc_warpman` |
| 任务 NPC：村长家的「萨姆吉尔的村长」在本地图 NPC 列表里是「任务」，`talk` 回“还不能对话”（脚本没放开）；成人仪式的接任务、领仪玉、交仪玉、完成（STAGE-90）由服务端测试和协议用例覆盖，冒烟没有走路（10204 自 STAGE-161 起已在快捷传送目录里、有落点，可以直接 `warp`） | `smoke.sh` 的“任务 NPC”一节；`server/character/npcevent_test.go`；`docs/protocol/cases/npc_quest_ceremony.jsonl` |
| 战斗中挂机启停：动作层是两行网格（人物 4 列 2 行、宠物 2 列 2 行），↑↓ 按画面跨行、←→ 行内换列、到边界停住；第 8 项（宠物第 4 项）是「开始／停止自动挂机」，不发 `battle_command`、不消耗回合；任何玩家都能按 Tab 打开战斗控制面板从中停止；**主动停止挂机同时关闭后续遇敌**，发出 `encounter off` 后先写“正在关闭”，等服务端确认才写“自动遇敌已关闭”，这一场照常打完，之后多个遇敌间隔都不再开新的一场；失败或超时先查一次状态、给出重试入口，断线和无权限时如实说明（STAGE-153） | `client` 的 `tests/keys_battle.rs`（网格跨行、管理项开关、Tab 不再切目标方）、`tests/battle_menu.rs`（两行渲染、管理项文案、Tab 面板与关闭后重新安排）、`tests/scenarios_auto.rs`（自动出招、手动接管、被动关闭）；`app/battle/grid.rs` 与 `app/battle/halt.rs` 的单元测试（列边界、缺列取最近、停止状态机、重复按下不重发、失败与超时）；手工：`encounter on` + `auto on` 打几场后按 Tab 停止，观察消息栏两句提示与停后无新战斗 |
| 分身队：社交页没有队伍时 `Enter` 直接组成分身队（队长 + 4 个分身，表里带 `[队长]`/`[分身]`，顶部状态行“队伍 5/5”）；动作菜单里解散先确认（默认取消不解散），确认后分身消失；`/party clone`、`/party disband yes` 同样可用；战斗里分身同场自动出手、不领奖励（STAGE-162） | `smoke.sh` 的“分身队”一节（真实客户端 + 真实服务端，`tools/smoke_party.py`，也可以单独跑 `tools/smoke-pty.py --party-only`）；协议用例 `party_clone`、`party_battle`、`party_agent`（服务端回归 `go test ./server/casetest`）；`server/character/party_test.go`、`server/battle/auto_test.go`；`client` 的 `tests/render_social.rs`、`tests/scenarios_party.rs`（用例 `ui_party`、`ui_party_battle`、`ui_party_agent`） |
| 空闲超过 60 秒连接不被断开（心跳） | `smoke.sh --full` |

## STAGE-161 快捷传送验收

| 验收标准 | 检查 |
|---|---|
| 游戏菜单显示「快捷传送」，菜单入口和场景入口打开同一个目录 | `client` 的 `tests/render_travel.rs`（菜单项文案、两个入口落到同一页）；`app/world.rs` 的 `GAME_MENU` |
| 岛屿/大区逐级选到村庄、建筑、洞窟楼层；上级地图本身可传、纯分组不可传；Esc 逐级返回并恢复光标 | `client/src/app/travel.rs` 的单元测试（有下级的地图首项是「传送到本图」、纯分组不能传送、退回上一层停在原来那一项、根层再退才离开）|
| 全量盘点有正式/排除分类、待审核 0、每张正式地图唯一收录且有落点 | `python tools/build-travel-catalog.py`（任何一项不过就以非 0 退出，报告写 `build/travel-catalog-report.md`）|
| 777 测试图不出现；501/522/纯数字候选有审核结论；817 缺图不可传 | 同上（报告“排除清单”）；`client` 的 `travel` 单元测试；服务端 `server/gamedata/travel_test.go` 的 `TestTravelCatalogRejectsExcluded` |
| 排除地图即使人为构造了合法落点，仍不出现在目录，并被 `warp floor` / `warp place` / 跨图可达拒绝 | `server/gamedata/travel_test.go` 的 `TestTravelCatalogRejectsExcluded`（临时往落点表塞一条）；`docs/protocol/cases/world_warp.jsonl` 的 `warp floor 777`、`warp floor 501` |
| 节点集合与 `maps` 平面集合一致；旧版只读 `maps` 的客户端仍能解析 | `server/gamedata/travel_test.go` 的 `TestTravelCatalogMatchesWarpable`；`docs/protocol/cases/world_warp.jsonl`（`maps` 响应含 `catalog_version` / `maps` / `nodes`）|
| 传送成功回场景、位置双端同步；失败留在原目录；在途不重复发送 | `client` 的 `tests/scenarios_travel.rs`；`app/travel_keys.rs`（在途时不发第二条）|
| 80×24 / 100×30 / 120×40 与 Unicode 关闭时可完整操作；空数据、查询失败、长名单、长路径有清楚提示 | `client` 的 `tests/render_travel.rs`（四个尺寸、Unicode 关掉、目录不可用时的提示）|
| 原 1–8 地点传送、按名/编号传送、Tab 补全、普通出口、NPC 目录、跨图对话、任务 NPC 和回城回归 | `docs/protocol/cases/world_warp.jsonl`（place 1–4 仍可传）；`cargo test`；`go test ./server/...` |

## STAGE-167 战斗动作与特效验收（第二阶段：精灵、换宠、反击、降级）

| 验收标准 | 检查 |
|---|---|
| A04 地水火风不是只换颜色 | `client/src/app/battle/spell.rs` 的 `four_elements_have_four_shapes`（Unicode 与 ASCII 各四种不同字符）；`client/tests/render_battle_motion.rs` 的 `element_spells_land_on_the_real_target_with_their_own_shape` |
| A04 自疗、单疗、群疗、复活、净化起点与范围不同 | 同文件 `self_heal_starts_and_ends_on_the_caster`、`revive_and_cure_look_different_from_heal`；渲染测试 `heal_rises_and_is_not_drawn_as_a_damage_element`、`revive_and_cure_have_their_own_marks` |
| A04 六异常各有形态、持续标记保留 | `spell.rs` 的 `six_statuses_have_six_shapes`；渲染测试 `status_spells_show_the_shape_of_that_status`；卡片上的持续单字见 `ui/battle/card.rs` 的 `marks` |
| A04 范围只落在事件真实作用的目标 | `spell.rs` 的 `all_targets_are_the_events_own_effects`；渲染测试 `all_targets_spell_marks_every_effected_unit_only`（三个目标三个标记，己方不亮） |
| A04 认不出的精灵中性回退 | `spell.rs` 的 `old_servers_without_magic_data_still_play_neutrally` |
| C01/C03 换宠先收回、后放出，只收/只放各按实际阶段 | `client/src/app/battle/swap.rs` 的 `full_swap_has_two_stages_in_order`、`reclaim_only_and_release_only`；渲染测试 `old_pet_stays_until_it_reaches_the_owner_then_the_new_one_appears` |
| C02/C05 宠物回自己的主人，不是本地玩家；主人不在场不移动 | `swap.rs` 的 `owner_comes_from_the_event_not_from_the_local_player`；渲染测试 `a_pet_returns_to_its_own_owner_not_to_the_local_player`；`ui/battle/effect.rs` 的 `swap_rect` 取不到主人就退回原位 |
| C04 同格复用不串身份 | 渲染测试 `old_pet_stays...`（收起后旧宠不再出现、新宠接替）；`playback` 在命中拍才应用 `left`/`joined` |
| C06 跳过、倍速、重连不残留分身 | `playback::clear_anim` 清 `motions`/`spells`/`swaps`；`cargo test --test scenarios_battle`（跳过路径） |
| A03 反击在接触现场完成攻防交换 | `client/tests/render_battle_motion.rs` 的 `counter_keeps_the_attacker_at_the_contact_point`（`hold` 时接触后不回位）；`playback` 只在下一拍是反击时延长 |
| B04 回旋镖沿横排连续扫过、回程不重复结算 | `server/battle/boomerang_test.go`（`19→17→15→16→18`、`8→6→5→7→9`）；`ui/battle/effect.rs` 的 `flight_at` 按 `sweep` 顺序走折线 |
| D08 80×24、Unicode 关闭、减少动态效果 | `render_battle_motion.rs` 的 `sixty_by_twenty_four...`、`unicode_off_falls_back_to_ascii_marks`、`reduce_motion_drops_the_projectile_but_keeps_the_contact_mark` |
| 未实现的机制不画假特效 | `docs/battle-ui.md`「未实现机制的依赖」（守护/镜/光/属性精灵/队友换宠/骑乘身份） |

## 手工检查（想亲眼看一遍时）

```sh
scripts/server-restart.sh                              # 终端 1：停掉旧的、重新编译并启动开发服务端（用 .env 里的库，默认开协议日志）
scripts/client.sh                                      # 终端 2、3：两个客户端
```

1. 终端 2：注册 → 登录 → 创建角色 → 进入游戏，看地图名和坐标；输入 `leave` 回到角色选择，再进入；
2. 终端 3：用同一个账号登录，终端 2 应回到登录页，消息栏写着“账号在别处登录”；
3. `scripts/server-stop.sh`：终端 3 应提示“服务器维护中”并回到登录页；
4. 登录页连续输错 5 次密码，再输一次正确的，应提示账号被锁定和剩余时间；`go run ./cmd/stoneage-admin unlock <账号>` 解除；
5. `psql "$STONEAGE_DATABASE_URL" -c 'select name, left(password_hash, 30) from accounts'`，密码列应以 `$argon2id$` 开头；`grep -c <你的密码> runtime/logs/*.log` 应为 0。
6. 在野外 `encounter on` 再 `auto-battle on`，打一两场后按 `Tab` 打开战斗控制、在「自动挂机：开」上按 Enter：消息栏依次出现「自动挂机已停止；正在关闭自动遇敌」和「自动遇敌已关闭；这一场照常打完」，这一场手动打完后再等 5 秒以上不应再出现 `battle_start`（STAGE-153）。

7. 社交页（Esc 打开游戏菜单 → 社交）：没有队伍时 `Enter` 组成分身队，顶部出现“队伍 5/5”；传送到别的地图后表里成员位置一致；在野外 `encounter on`，战场上己方是 5 个人加 5 只宠物，分身卡片带“自动”，只交队长和宠物的指令就结算，胜利窗口写“分身不领奖励”；解散（确认默认取消）后分身消失（STAGE-162）。

> 脚本已按 STAGE-131～140 的界面更新（STAGE-165）：进入世界落在场景页，地图页、角色页等功能页要先 `Esc`、`Enter` 打开功能页再按数字（数字键只在功能页里切页）；命令一律走 `/` 命令栏，命令被拒时命令栏会留着，`command()` 辅助函数据此决定要不要再按 `/`；Agent 令牌和控制权在「系统」页（游戏菜单最后一项，`End` 选中）；`talk` 弹出的是 NPC 菜单 v2 窗口，按正文判断弹出和关闭，商店里 `Esc` 逐层返回。

## 联调发现的问题

联调中发现的问题提成缺陷、归到对应需求下。已发现并修复：

- STAGE-11：主界面底部的消息栏只留了 8 行，扣掉边框、分隔线和输入行后只能显示 4 条消息（设计是 5 条）。已改成 9 行并加了渲染回归测试。
- STAGE-10：角色选择页的名字列只有 20 格，16 个汉字的名字被截成 10 个字（所属 STAGE-1，`client/src/ui/characters.rs`），已加宽并加了渲染回归测试。

## Windows 汇总验收（STAGE-140）

Unix 的 tools/smoke-pty.py 依赖 pty/fcntl，Windows 不直接运行。跨平台真实服务端临时 schema 冒烟使用 cargo test --test e2e（设置 STONEAGE_TEST_DATABASE_URL，Go 在 PATH）：原生菜单生成令牌/收回控制权、注册建角、删除、停服、TCP与WebSocket实际断流、密码错误和完整刷新。另在真实 Windows 伪终端运行正式二进制，核对菜单、表单、ASCII、确认与退出恢复；此项不能用 HTML 或只修改 connected 标志替代。

服务端子进程用 -shutdown-stdin 正常关闭，确保临时 schema 清理和维护推送。全量 Rust 回归覆盖四种尺寸与对象/窗口/回合变化；Go 契约回归覆盖 NPC v2、买卖及任务NPC映射。race 检查需要可用的 C 编译器；本机未安装时明确记录未执行，不把普通 go test 当成 race 通过。


## STAGE-148 场景双列出口验收

- 服务端：`go test ./server/gamedata ./server/character ./server/casetest`；设置 `STONEAGE_TEST_DATABASE_URL` 执行真实数据库隔离测试。契约 `world_exits` / `ui_exits` 与 Rust mock 共用。
- 客户端：`cargo test --test scene_exits --test scenarios_exits --test exits_contract` 验证 80×24/120×40、Unicode/ASCII、独立滚动、固定入口、详情、重复 Enter、网络契约。`scene_net_tests` 与重连测试验证世代隔离、失败与刷新屏障。
- 真实联调：设置测试库，`cargo test --test e2e village_doors_npc_named_warp_catalog_and_home`：渔村两处村口分别到 100 (79,614)/(114,660)，返回后右列选 NPC 对话，按名传送、查人、回城。原 e2e 的 agent 控制权与断线重新登录测试一并执行。
- 人工复核：进入渔村左列有 13 个门，2011/2014 的门标早晨/白天；关闭仍可查看原因。吉鲁岛长列表 End 到回城、↑ 到查人/传送，右列滚动后切回仍保留原行。Tab 能读完整名字、全部触发坐标；弹窗左右键不能穿透到场景。
