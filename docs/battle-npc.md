# 战斗 NPC（STAGE-91 设计）

原版的战斗型 NPC（`functionset=NPCEnemy`，源码 `gmsv/npc/npc_npcenemy.c`）在新服务端上的设计：玩家对着指定 NPC 开战，
赢了传送或让它消失一阵子，输了按普通战斗失败处理。本文只是设计，**还没有代码**；协议改动（第 6 节）在第一个开发需求里先写进
`docs/protocol.md` 和协议用例，再动代码。

对照源码：`npc/npc_npcenemy.c`（触发、胜负）、`battle/battle.c` 的 `BATTLE_CreateVsEnemy` / `BATTLE_Finish`、`npc/npcutil.c` 的
`NPC_Util_getEnemy`（阵容）、`npc/npc_eventaction.c` 的 `NPC_ActionPassCheck`（NEWEVENT 的条件）。数据：`data/npc/` 下的 `*.template`
（`functionset=NPCEnemy`）、`*.arg`、`*.create`。

## 1. 结论

1. 战斗型 NPC 有 **278 个实例**：200 个是道场弟子（`gym`，`doujyou/`），78 个是普通战斗 NPC（参数文件 60 个，其中
   `100/sb_kinzo.arg` 没有任何 `create` 引用、键名也写成非标准的 `askbattlemsg:`，跑不到，实际生效的是 59 个；
   守护兽、Boss、小偷、宝物袋等）。第一期只做普通战斗 NPC，道场另立需求（第 7 节）。
2. 入口复用现有的 `talk`：战斗 NPC 在 `npcs` 列表里显示为类型「战斗」，对它 `talk` 弹出“是/否”窗口，选是开战。不做走路碰撞触发。
3. 战斗引擎不变：阵容按 `enemyno` 顺序摆，战斗推送沿用第 10 章，只多一个 `kind: "npc"` 和几个可选字段。
4. 赢了：`dieact=1` 的把玩家传到 `warpfl/warpx/warpy`（NPC 留在原地，随时可再打）；`dieact=0` 的 NPC 对全服消失 `time` 秒（默认 120）。
   输了、逃了、断线：和野外战斗完全一样，NPC 不受影响。
5. 战斗**不写任务进度**。任务只通过“能不能打”（持有道具、任务事件位）读进度，通过奖励和传送间接推进——这和原版一致。
6. **第一场：里昂蛙王**（精灵王的传说·前传，地图 30605）。它的开战条件和胜负处理最少（持有金珠、打赢传到王座厅）。
   但 前传 链上的 3 个任务 NPC 台词是分页的（`AcceptMsg1..n`），现在仍是“还不能对话”，所以**战斗做完后前传仍走不通**，
   要和“分页台词”一起排期（第 8 节）。

## 2. 对需求描述的几处纠正

需求里写的和原版代码对不上的地方，设计以代码为准：

| 需求里的说法 | 原版实际 |
|---|---|
| `onebattle` = “是否只能打一次” | 不是每个玩家只能打一次。`onebattle:1` 表示**这个 NPC 同时只能有一场战斗**：已经有人在打它时，别人来了说 `alreadymsg`；打完（赢输逃都算）立刻又能打。（`NPC_NPCEnemy_Encount` 里遍历 `BattleArray` 找 `createindex == meindex`） |
| `item` = “掉什么” | 不是掉落。`item:A,B` 是**开战的前置条件**：背包里要同时有这些道具（编号逐个找，缺一个就拒绝，说 `deniedmsg`）。只有配了 `steal` 才会收走：`steal:0` 开战时收，`steal:1` 赢了才收（数据里只有 `sb_bac` 一个，`steal:1`）。真正的掉落、经验、金币是敌人表（`enemy.txt`）里每个敌人自带的，和野外怪一样 |
| “赢了去哪” | `dieact:1` 才传送（47 个参数文件）；`dieact:0` 的 12 个文件赢了不传送，NPC 自己消失 |
| `entype` 区分触发方式 | 0 只能走路碰、1 只能对话、2 都行。**数据里全是 2**，在没有走路的新服务端上不用区分 |

## 3. 原版参数表

下表统计 59 个生效的普通战斗参数文件（“普通”列；`data/npc` 下带 `enemyno` 的参数文件共 60 个，
`100/sb_kinzo.arg` 没有被任何 `create` 引用，不计）和 200 个道场参数文件（“道场”列）里每个键用了多少个文件。

| 键 | 含义 | 普通 | 道场 | 新服务端 |
|---|---|---|---|---|
| `enemyno` | 敌人编号列表（`enemy.txt`），按顺序入场；最多 10 个 | 59 | 200 | 支持。道场里是“候选”，见第 7 节 |
| `entype` | 触发方式 | 59（全 2） | 200（全 2） | 忽略 |
| `dieact` | 赢了之后：1 传送，0 消失后复活 | 47 / 12 | 200（全 1） | 支持 |
| `warpfl/warpx/warpy` | `dieact=1` 的传送落点 | 47 | 200 | 支持。坐标不查地图边界（同 `NpcWarp`） |
| `askbattlemsg1..6` | 开战前的确认台词，按行拼起来，是/否 | 59 | 0 | 窗口正文 |
| `deniedmsg` | 条件不满足的台词 | 59 | 0 | 窗口正文 |
| `startmsg` | 开战时 NPC 说的话 | 35 | 200 | `battle_start.npc.say` |
| `endmsg` | 赢了 NPC 说的话（`NEWNPCENEMY` 的不用这个键，见下） | 7 | 0 | `battle_end.npc.say` |
| `alreadymsg` | 被占用时的台词 | 10 | 0 | 窗口正文 |
| `onebattle` | 同时只能一场（见第 2 节） | 21（10 个 =1，11 个 =0） | 0 | 支持 |
| `time` | `dieact=0` 的复活秒数；没写取 120 | 12 | 0 | 支持 |
| `item` | 前置道具（全部持有） | 7 | 0 | 支持（`2431` 盗贼们的足迹 ×3、`2535` 参加许可证、`2697` 切绳石 ×12 个宝物袋、`19622` 黑蛙王的金珠、`1292` 真．灵力铠） |
| `steal` | 前置道具什么时候收 | 1 | 0 | 支持（`sb_bac`） |
| `gym` | 道场弟子：NPC 的等级，开道场模式（`mode 2`） | 0 | 200 | 不在本期 |
| `enemypetno` | 道场里第二个敌人（宠物）的候选 | 0 | 200 | 不在本期 |
| `NEWNPCENEMY` + `NEWEVENT1..3` | 赢了之后按条件分路传送（只有黑暗精灵王） | 1 | 0 | 第二批，见第 5.5 节 |
| `noitem`、`B_evend`、`B_evnow`、`NEWTIME`、`sktype`、`herobattlefield`、`REPLACEMENT` | 原版代码支持，**数据里没人用** | 0 | 0 | 不实现；转换时遇到就标不支持 |

另外 8 个 NPC 的名字在原文件里是 Big5（繁体）编码，按 GBK 解会乱码（`sb_nusu`、`sb_hag`、`sb_gd`、`sb_sinsa` 的 `.create`）。
转换脚本这几行要退到 Big5 解码，其余都是 GBK。

## 4. 原版的触发与胜负流程

`NPC_NPCEnemy_Encount(meindex, 玩家, mode)`，`mode` 0 是走路碰到，1 是对话：

```text
1. entype 不允许这种触发方式              → deniedmsg
2. item 里的道具没有全部持有              → deniedmsg
3. onebattle=1 且已有战斗是这个 NPC 开的   → alreadymsg
4. 队伍里不是队长（队员触发）              → 什么都不做
5. 有 askbattlemsg1 → 弹是/否窗口，等玩家选；没有 → 直接开战
   选“是”→ NPC_NPCEnemy_BattleIn（注意：这里不再检查 1–3，原版漏洞）
6. BattleIn：BATTLE_CreateVsEnemy(玩家, 1 或 2, NPC)；成功后说 startmsg；steal=0 时收 item
```

战斗结束时 `BATTLE_Finish` 只在**玩家赢**（`winside == -1`）时调用 `NPC_NPCEnemy_Dying`：

```text
a. 说 endmsg（给 Side[0] 里的每个玩家）；steal=1 时收 item
b. dieact=0：NPC 的造型改成 0（所有人都看不见，也碰不到），记下死亡时间，之后每 5 秒检查，超过 time 秒恢复
   dieact=1：把 Side[0] 里的每个玩家解散队伍后传到 warpfl/warpx/warpy
```

输了、逃了：没有 NPC 专属的处理，玩家按普通失败处理（被打飞回存档点，被打死原地复活）。战斗里的 `norisk`（不扣魅力）只有道场用。

`BATTLE_CreateVsEnemy` 里和 NPC 有关的：战斗类型给客户端发 `BATTLE_TYPE_BOSS_BATTLE`（6，只影响客户端表现），逃跑、捕捉的规则没有为 NPC 改过。

## 5. 新服务端的设计

### 5.1 入口与列表

- `tools/gen-npcs.py` 把 `functionset=NPCEnemy` 的模板名认出来（现在只认 `npcgen_*` / `changeevent` 等几类）：这些 NPC 进 `server/gamedata/npcs.tsv`，
  原版类型统一写 `npcenemy`，显示类型「战斗」（协议 5.32 的类型表追加一项）。名字、位置照常。位置取 `borncorner` 中心，同其他 NPC。
- 战斗参数转成 `server/gamedata/npc-enemy.json`，每个 NPC 一行（编号与 `npcs.tsv` 一致；下面例子里的编号是示意），结构：

  ```jsonc
  {"id": 1501, "file": "king/event69_5",
   "ask": ["该死的人类，偷了我的东西还敢回来？", "今天给我逮到了，一定要好好教训你！"],
   "denied": "这里不是你们人类该来的地方，快滚！", "already": "等我解决这群人类后马上就轮到你！",
   "start": "看我怎么修理你们这些人类！", "end": "我不甘心啊！…",
   "enemies": [1685, 1681, 1681, 1681, 1681, 1681, 1681, 1681, 1681, 1681],
   "items": [19622], "steal": -1, "one_battle": true,
   "die_act": 1, "warp": {"floor": 30607, "x": 7, "y": 11}, "revive_secs": 0,
   "unsupported": ""}
  ```

  `unsupported` 非空的（道场、`NEWNPCENEMY`、数据缺失）运行时不放行，`talk` 回 `NPC_NOT_TALKABLE`，和任务 NPC 一致。
- 放开用白名单（按参数文件路径，如 `king/event69_5`），和任务 NPC 的 `npcEventEnabled` 是同一种做法；测试里可以用 `Options` 临时放开别的文件。

### 5.2 对话与确认

`talk` 到战斗 NPC（已放开的）：

1. 战斗中、不在同一地图：和现有 `talk` 一样（`IN_BATTLE`、`NPC_NOT_HERE`）。
2. NPC 正处于“消失”状态（`dieact=0` 打过，复活时间没到）：回 `NPC_NOT_HERE`（它不在 `npcs` 列表里，列表本来就会变，见 5.6）。
3. 依次检查：`items` 是否全部持有（背包槽位，不含穿戴；同任务 NPC 的 `ITEM=`）→ `one_battle` 是否被占用。不通过：推一个**没有选项的窗口**，
   正文是 `denied` / `already`（原版是黄字聊天，这里放进窗口，agent 和玩家看到的一样）。`denied` 为空时用 `{名字}没有理你。`（数据里 59 个文件都有，兜底而已）。
4. 通过：推窗口，正文是 `ask` 各行用换行连起来，选项 `1 是` `2 否`（同任务 NPC 的问答窗口，协议 5.35）。没有 `ask` 的直接开战（数据里没有）。
5. `npc_choose 1`：**重新做第 3 步的检查**（原版不重查，窗口开着的时候东西可能没了、或者别人先打了），不通过就换成对应的拒绝窗口；
   通过则关窗（`CHOSEN`）并开战。`npc_choose 2` 关窗。

### 5.3 阵容

- 敌人按 `enemies` 顺序一个个创建：`gamedata.EnemyByID` → `PetByID(模板)` → `battle.NewMonster(模板, 等级, …)`，与野外遇敌共用。
  等级是敌人表里的定值（`LVMin == LVMax`，如里昂蛙王 90、黑蛙 50）；`exp`、`drop` 列照常（黑暗精灵王的 `exp=0`，所以不给经验）。
  **不再走野外的“按权重抽、同场上限”那一套**——Boss 阵容是写死的。
- 大体型（模板 `Size == 1`）的规则照 `NPC_Util_getEnemy`：全场最多 5 个，第 6 个起要和前 5 个里的普通体型换位，找不到换的就不出。
  野外遇敌的 `wildEnemySlots` 已经实现了同样的逻辑，开发时抽成共用函数。
- 敌人超过 10 个、编号不在表里：转换时标不支持（数据里没有）。
- 战斗场景 `scene` 仍是地图名。`b.AllowEscape`、`b.AllowCapture` 保持 `true`，规则不变（原版没有为 NPC 改过逃跑）；
  是否能抓由敌人模板自己的标记决定，开发时对照 `battle_event.c` 的捕捉判定确认 Boss 模板（`PetFlag=0`）被挡住。

### 5.4 单人与组队

新服务端**还没有真人组队**。STAGE-162 起有一种特殊的队伍——分身队（`docs/protocol.md` 5.57：队长加最多 4 个服务端自动出招的分身，只存在内存里）：分身随队长进 NPC 战斗，同样不领奖励，胜利后传送、收道具只按队长算；队伍数据在 `character.Presence.party`（`server/character/party.go`），以后真人组队可以在它上面扩展。除此以外本期一律单人：

- 开战的就是一个人，胜利后传送的也只有他。
- 以后做组队时补两条：只有队长能触发（原版 `CHAR_PARTY_CLIENT` 不能触发）；胜利后 `Side[0]` 里所有人解散队伍并一起传送；
  `NEWNPCENEMY` 的 `CHECKPARTY:FALSE`（整队一起传，原版 `NPC_WarpAllMsg`）。这些在“组队”需求里做，本设计只留位置。

### 5.5 胜负之后

**赢**（`battle_end.result == "win"`）。`finishBattle` 在写回奖励、推 `status_changed` 之后：

1. 收 `steal=1` 的道具（先于 `inventory_changed`）。
2. `endmsg` 放进 `battle_end.npc.say`。
3. `die_act == 1`：改角色位置到 `warp`，推 `position_changed`（和失败回城同一条推送）；同时关掉遇敌开关、关闭对话窗口（`MOVED`）——
   传送走了就按传送处理，复用 `warp` 的收尾。落点不是真实存在的地图时转换阶段就标不支持。
4. `die_act == 0`：NPC 在 `npc` 注册表里标成“消失到 now+revive_secs”（5.6）。玩家原地不动。
5. 遇敌开关还开着的话，沿用现有逻辑（赢了隔 2 秒开下一场；传送走了就关掉）。

**输 / 逃 / 平 / 断线**：和野外一样，NPC 不受影响，`one_battle` 占用释放。输了按普通失败规则处理（被打飞回存档点，被打死原地复活，见协议 10.9），魅力规则不变
（`norisk` 只有道场才有）。

**`NEWNPCENEMY`（黑暗精灵王，第二批）**：赢了之后不是直接传送，而是按顺序试 `NEWEVENT1..3` 的 `FREE` 条件，第一个满足的块说它自己的 `endmsg`
并传送到它的 `WARP`（`;` 分隔多个落点时随机取一个）；都不满足不传送。条件语法就是传送员的 `FREE`（`gamedata.warpcond.go`），
已支持 `ENDEV`、`NOWEV`、`ITEM`、`LV`；还要补 `EQUIT=编号`（该道具**穿在身上**）、`EQUIT!=编号`。

### 5.6 状态、并发与锁

新增运行时状态（只在内存里，服务重启全部清零，和 `NPCBoard` 一样）：

| 状态 | 位置 | 说明 |
|---|---|---|
| 战斗占用 | `gamedata.NPCBoard` 新增 `busy[npc] = battleID` | `one_battle=true` 的 NPC 开战时登记，战斗结束（赢输逃平）、玩家掉线时释放。**放在创建战斗那一步原子地“检查并登记”**，不能先查后登记（两个玩家同时点“是”） |
| 消失到几点 | `NPCBoard` 新增 `hiddenUntil[npc]` | `OnFloor` / `ByID` 读取时过滤掉消失中的；读到过期的就当没有，不需要后台循环 |

`NPCBoard` 是叶子锁，登记和释放可以在角色锁里做。**推送给别的玩家要拿别人的角色锁**，和 `NpcWarp` 一样不能在角色锁里做：
消失、复活的 `npcs_changed`（协议 5.46，原来就有）在 `finishBattle` 放锁之后另起 goroutine 推；复活用 `time.AfterFunc(revive_secs)`
到点后推一次，当前在这张地图上的连接收到新列表。

战斗的所有收尾路径都要释放占用：`finishBattle`、`exitBattleOffline`、`StartBattle` 失败回滚；`battleState` 记着开战的 NPC 编号，
这些路径统一调用一个 `releaseNPCBattle(bs)`。

### 5.7 与任务进度的关系

- 战斗 NPC **只读**任务进度，不写。数据里 `FREE` / `B_evnow` 一类条件只出现在黑暗精灵王，其余 58 个文件的条件只有道具。
- 通过战斗推进任务的方式：赢了被传送到下一个房间，那里的任务 NPC（`changeevent`）看你持有什么道具、事件位是什么再说话。
  里昂蛙王就是这样（5.8）。需求要求的“与任务进度的关系”到这里为止，不引入“战斗胜利事件”。
- 前置道具的条件复用任务 NPC 已有的“背包里有没有”逻辑（`npcevent_cond.go`），不另写一份。

### 5.8 客户端

- 地图页的 NPC 表多一种类型「战斗」（`type` 是字符串，客户端不用改代码，只改渲染测试里的样例）。
- 对话窗口复用 `npc_window`（是/否）。
- 战斗画面：`battle_start` 多了 `npc` 对象时，战斗记录第一行写 `npc.say`（战斗类型标签，黄色，格式“里昂蛙王：看我怎么修理你们这些人类！”）；
  `battle_end` 多了 `npc.say`（赢了才有）同样写进战斗记录。`kind: "npc"` 不改界面布局。

## 6. 协议改动（先写进 `docs/protocol.md`，由第一个开发需求落地）

```jsonc
// 5.32 npcs 的 type 追加「战斗」；5.33 talk 对已放开的战斗 NPC 弹是/否窗口（5.34/5.35 的现有格式）

// 10.3 battle_start：kind 追加 "npc"，多一个可选的 npc 对象
{"type": "battle_start", "data": {
  "battle_id": "b_3", "kind": "npc", "scene": "蛙洞", "you": 0,
  "allow": {"escape": true, "capture": true},
  "npc": {"id": 1501, "name": "里昂蛙王", "say": "看我怎么修理你们这些人类！"},
  "units": [ ... ]}}

// 10.9 battle_end：赢了且 NPC 有 endmsg 时多一个 npc.say；赢了被传送时紧接着推 position_changed
{"type": "battle_end", "data": {"battle_id": "b_3", "result": "win", "turn": 5,
  "npc": {"id": 1501, "name": "里昂蛙王", "say": "我不甘心啊！…"},
  "rewards": { ... }}}
```

- `battle`（10.11）的响应带同样的 `npc`（不含 `say`），中途加入的连接能知道对手是谁。
- 新增字段对旧客户端无害（忽略）；不加新的错误码：被拒绝都走窗口，`NPC_NOT_TALKABLE` / `NPC_NOT_HERE` 沿用。
- 协议用例：`npc_enemy_frog.jsonl`（确认窗口 → 开战 → 赢 → 传送）、`npc_enemy_denied.jsonl`（没有金珠）、`npc_enemy_busy.jsonl`（占用中）。
  `start_battle` 的剧本表里加一个 `npc_win` 剧本，让协议用例和模拟服务端不依赖随机数（同第 10.13 节）。

## 7. 道场（另立需求）

200 个道场弟子是同一个机制的另一个模式（`BATTLE_CreateVsEnemy` 的 `mode 2`），不能直接并进来：

- `gym:N` 是**弟子的等级**；每次从 `enemyno` 里随机挑一个、`enemypetno` 里随机挑一个，两个敌人一起上，等级用 `baselevel = N` 覆盖敌人表里的等级。
- 敌人的外观和名字换成弟子的（`CHAR_BASEIMAGENUMBER`、`CHAR_NAME`），设 `norisk = 1`（输了不扣魅力，宠物忠诚不降）。
- 赢了一律传送到下一个弟子；这是 100 级梯度的闯关设计，需要单独看平衡、奖励和进度记录（数据里没有存进度，靠位置）。
- 需要“按等级覆盖”的 `NewMonster` 调用和 `norisk` 标记，都不在本期的范围。

## 8. 第一场战斗：里昂蛙王

选 `king/event69_5`（精灵王的传说·前传，地图 30605，坐标 7,9），理由：

- 开战条件最简单（只要持有 `19622` 黑蛙王的金珠），没有任务事件位条件、没有分路传送。
- 阵容固定（`1685` 黑蛙王 90 级 ×1 + `1681` 黑蛙 50 级 ×9），掉落只有黑蛙的 `13079`（概率 2）。
- 赢了传到 30607（7,11）——王座厅，旁边就是里昂蛙王（`changeevent`，`event69_6`），正好验证“战斗 → 传送 → 任务 NPC”。
- 它是链上最前面的一场，对玩家的等级要求最低（后面的守护者是 100–120 级）。

前传链上的 NPC 和现在的放开状态（`python3 tools/gen-npcs.py` 的统计）：

| 步骤 | NPC | 现状 |
|---|---|---|
| 愿藏的祖父（2000） | `event69_3` | 全部支持，白名单里没放 |
| 库伊老爷爷（30600）给护身符 | `event69_1` | **4 块是分页台词**，不支持 |
| 卡卡金宝（30606）凭护身符放行 | 传送员 915，条件 `ITEM=19621`、`NOWEV=69` | 支持 |
| 新藏（30602）给金珠 | `event69_4` | **3 块是分页台词**，不支持 |
| 里昂蛙王战斗（30605） | 本需求 | 本需求 |
| 里昂蛙王（30607）收金珠，给黑玉 | `event69_6` | **1 块分页台词**，不支持 |

结论：只做战斗，里昂蛙王这一场能单独验收（用管理命令给金珠、`warp` 到 30605 即可），但**前传整条链要等分页台词**。分页台词影响面不止这里：
共 37 个任务 NPC 有分页块，其中 27 个只差它，所以应该独立成一个需求，和战斗并行做。

### 验收口径（第一场）

1. 30605 的 NPC 列表里有「里昂蛙王」，类型「战斗」。没放开（不在白名单）的战斗 NPC 回 `NPC_NOT_TALKABLE`。
2. 没有金珠时 `talk`：窗口说 `这里不是你们人类该来的地方，快滚！`，没有选项，背包不变。
3. 有金珠时 `talk`：窗口正文是两行 askbattlemsg，选项 1 是 / 2 否；选 2 关窗，什么都不发生。
4. 选 1：先关窗，再推 `battle_start`（`kind: "npc"`，`npc.say` 是 startmsg，敌方 10 个单位：1 个黑蛙王 + 9 个黑蛙）。
5. 一个人在打它时，第二个人 `talk`：窗口说 `等我解决这群人类后马上就轮到你！`。第一个人打完或掉线后第二个人能打。
6. 赢：`battle_end` 的 `npc.say` 是 endmsg，随后 `position_changed` 到 30607（7,11）；金珠还在背包里（`steal` 没配）。再回 30605 还能再打。
7. 输：按普通失败处理（被打飞回存档点，被打死原地复活），NPC 不变；逃跑同理。
8. 窗口开着、金珠被丢掉（或被别人占用）后选“是”：重新检查，不开战。

## 9. 与原版的差异

1. 入口是 `talk` + 确认窗口，不是走路碰撞；确认后重新检查条件（原版不重查）。
2. 拒绝、占用、确认的台词放进窗口，不是黄色聊天；`startmsg` / `endmsg` 放进战斗记录。
3. 不检查距离、不要求面朝 NPC（原版对话触发要 `NPC_Util_charIsInFrontOfChar`）；本服务端整体没有走路。
4. 队员触发、整队传送：等组队。
5. `B_evend`、`B_evnow`、`NEWTIME`、`sktype`、`noitem`、`herobattlefield`、`REPLACEMENT`：原版代码有，数据没人用，不实现。
6. 消失与占用状态不落库，重启后归零（同原版：都是内存状态）。
7. 前置道具只看背包，不看身上装备：原版的 `item` 检查遍历整个道具数组（含 5 个装备槽，`CHAR_EQUIPPLACENUM`），
   把装甲穿在身上也算持有；本服务端复用任务 NPC 的 `ITEM=`，只查背包。数据里 `1292`（真．灵力铠，`king/event69_13`，
   属第二批）是装备，放开它之前要决定是否补成原版那样。

## 10. 拆分的开发需求

全部是产品类需求，标题以【战斗】或【NPC】开头。顺序按依赖排：

| # | 标题 | 范围 | 依赖 |
|---|---|---|---|
| A（STAGE-96） | 【战斗】战斗 NPC 引擎：先打通里昂蛙王 | 第 5、6 节：转换脚本与 `npc-enemy.json`、列表类型、确认窗口、`kind: "npc"` 战斗、占用登记、`dieact=1` 传送、`steal`、协议文档与用例、模拟服务端、客户端战斗记录；白名单只放 `king/event69_5` | 无 |
| B（STAGE-97） | 【NPC】任务 NPC 支持分页台词 | `AcceptMsg1..7`、`ThanksMsg1..4`、`RequestMsg1..3` 等的翻页（协议里窗口的“下一页”选项）；解开 37 个 NPC 的约 66 块，放开精灵王前传的 3 个 NPC | 可与 A 并行 |
| C | 【NPC】放开精灵王的传说·前传 | 白名单加 `king/event69_1/3/4/6`；走通愿藏的祖父 → 库伊 → 卡卡金宝 → 新藏 → 里昂蛙王战 → 里昂蛙王；冒烟用例 | A、B |
| D | 【战斗】战斗 NPC：dieact=0（消失与复活）和守护兽 | 5.6 的消失状态与 `npcs_changed`；放开不带分路的守护兽、宝物袋、小偷等；每批按目录放开 | A |
| E | 【战斗】战斗 NPC：条件分路传送（黑暗精灵王） | `NEWNPCENEMY`/`NEWEVENT`、`EQUIT` 条件；放开 `king/event69_13`、`15/17/19/21/23`（精灵王的传说·打封印精灵王） | A，C 之后做才好验收 |
| F | 【战斗】道场 | 第 7 节：`mode 2`、`norisk`、200 个弟子 | A |
| G | 【战斗】组队 | 不属于本设计，**战斗 NPC 的队长触发/整队传送**等组队做出来后补进 A 的规则 | 组队 |

A 完成后第一场可以验收；B 与 A 完成后 C 才让玩家真正“走通一个任务”。建议先建 A、B 两个需求。

## 11. 待确认

1. **逃跑**：Boss 战是否仍允许逃跑？本文按原版（允许）。如果希望“打 Boss 不能逃”，是协议里 `allow.escape=false` 一个字段的事，但原版不是这样。
2. **拒绝台词的载体**：本文放进无选项窗口。如果更想放进消息栏（原版黄字），只要把第 5.2 第 3 步改成 `notice`，其余不变。
3. **道场是否做**：200 个 NPC 里 100 级的梯度设计比较特殊，F 的优先级由你定。
4. **穿在身上的装备算不算“持有”**：见第 9 节第 7 条。原版算，本服务端只查背包。里昂蛙王（金珠，不穿戴）不受影响；
   放开 `king/event69_13`（`1292` 真．灵力铠）之前要定，否则玩家穿上就算不持有、打不了封印精灵王。
