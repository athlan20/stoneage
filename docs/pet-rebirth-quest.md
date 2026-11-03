# 精灵少女转生（宠物转生）：研究与拆分

> STAGE-198。只做研究和拆分，不改代码、协议和用例。所有取舍按原版走，以原版 C 源码为准。
> 原版源码里没有“少女”这个词（`data/npc`、`char` 下 GBK 搜索无命中），本文把需求里的“精灵少女转生”理解为原版的**宠物转生**（PETTRANS，玛蕾菲雅 NPC）。
> 源码路径以 `original-data/gmsv/` 为根，下文简写为 `gmsv/`；行号按 GBK 解码后的文件计算。

## 一、结论（先看这里）

1. **原版宠物转生**是玛蕾菲雅 NPC 的一段窗口流程（`PETTRANS` 段）：玩家 80 级以上、事件 4/69/70 已完成、身上恰好 1 只 79 级玛蕾（PETID 718，第二次转生用 401），选一只 80 级以上、非骑乘、转生次数不超过 1 的宠物，确认后：
   - 玛蕾被收走；宠物**等级回到 1**，四项能力按“玛蕾 + 宠物”的公式重算，转生次数 **+1**，等级上限解除（`LIMITLEVEL = -1`）；
   - 删掉身上所有 `19625–19632`、`19672–19687` 范围内的物品（灵石、项链等，**不检查数量**，有就删）；
   - 设置事件 71（进行中），任务 51 的入口靠它。
2. **新服务端现状**（STAGE-236 更新）：STAGE-196 的转生次数字段；STAGE-236 做了转生窗口的分流、资格与选宠检查和转生写库（`server/character/` 的转生代码）。玛蕾等级上限对话站、事件 71、物品删除、宠物重算以代码为准，这里没有核对到的，按未做处理。
3. **最大的前置缺口是“玛蕾等级上限对话站”**：原版宠物要靠对话把个体上限从 15 一路抬到 79，才能养到 79 级当玛蕾。新服务端的宠物上限只看模板（`server/gamedata/pets.go:60`），没有按宠物个体动态上限的机制，也没有宠物对话（`pettalk`）支持。
4. **“少女跟随”、灵石的“跟随”在原版找不到对应机制**：原版的跟随是宠物跟随（`CHAR_WORKPETFOLLOW`，宠物项圈），新服务端也没有跟随代码。需求方要确认这一项指的是什么，拆分里先不列入。
5. 建议拆成 **5 个需求**（第七节），依赖顺序：上限对话站 → 转生核心数值 → 转生窗口 → 物品删除与事件 71 → 任务 51 收尾。

## 二、完整链路与原版出处

| 步骤 | 原版位置 | 新服务端现状 |
| --- | --- | --- |
| 前置：成人仪式（事件 4） | `server/gamedata/quests.tsv` 任务 1 用事件 4；原版 `NPC_PetTransManCheck` 检查事件 4（`npc_transmigration.c:1382`） | 任务 1 已在白名单（`docs/quest.md` 第一批） |
| 领养玛蕾菲雅（事件 70） | `king/event69_7`（愿藏的祖母，2000 渔村）给宠物 1479 | king 目录已放开（`server/gamedata/npcevent.go:194–206`）。1479 是敌群表行（`enemies.tsv:1104`，TEMPNO 718）；只说明脚本里的编号，**PETID 以 718 为准**（`npc_transmigration.c:71`） |
| 10~75 级对话站，玛蕾等级上限 15→79 | `data/npc/pettalk/ptalk01.arg`（PETTEMPNO:718）、`ptalk06.arg`（PETTEMPNO:401），见第三节 | **无**：`server` 里没有 pettalk / PetTalk 相关代码 |
| 79 级玛蕾 + 80 级宠物 → 转生 | `npc_transmigration.c` 的 `NPC_TransmigrationTalked`（49–128）与 `NPC_PetTransMan_selectWindow`（1219–1375） | **无** |
| 事件 71 进行中 | `NPC_PetTransManStatus` 末尾 `NPC_NowEventSetFlg(toindex, 71)`（1596） | **无**；`king` 脚本不能设 71（`docs/quest.md` 任务 51 说明） |
| 任务 51 “打封印精灵王”、52–54 | `server/gamedata/quests.tsv`：49–54 对应事件 69–72 | 52/53 依赖 51，51 入口缺（见上一行） |
| 漆黑洞窟线 | `giiru/event/sikkoku/sikkoku.create`（“漆黑的洞窟 事件设定”，`floorid=32002`）；`giiru/event/sikkoku/ten.arg` | `giiru/event/sikkoku01–05`（`server/gamedata/npc-event.json:300–304`）**未放开**（`npcevent.go` 无 sikkoku）。“漆黑二十层精灵王”的具体 NPC 未核对（**待确认**）；地图名见 `mapnames_gen.go:700–702`（32019–32021 为地下 19–20 楼） |

## 三、玛蕾等级上限对话站（10~75 级，原版 pettalk）

- 文件：`gmsv/data/npc/pettalk/ptalk01.arg`（玛蕾，PETTEMPNO:718）与 `ptalk06.arg`（PETTEMPNO:401，二转玛蕾）。`pettalk.menu` 把编号映射到文件（`pettalk.menu` 第 1 行起）。
- 规则：宠物**等级恰好为 x** 时对话，触发 `LimitLevel` 动作，把个体上限设为新值。ptalk01 的对应行：
  - `PET:LV=10`（:272）→ `LimitLevel:15`（:274）；`LV=15`→20（:259–261）；……；`LV=70`→75（:115–117）；
  - `PET:LV=75`（:102）→ `LimitLevel:79`（:104）；`PET:LV=79`（:95）→ 79（:97）。
  - 另有只说话、不改上限的块（如 :10 的 `LV>74&LV<79` 等）。ptalk06 的结构相同，只是 `LimitLevel:30` 在 :241 多出一次（重复，无影响，**待确认是否笔误**）。
- 写入位置：`gmsv/char/pet_event.c:265`，`PetTalk_RunEvent` 读取 `LimitLevel` 参数后 `CHAR_setInt(meindex, CHAR_LIMITLEVEL, ...)`。
- 升级的上限判断：`gmsv/char/char_data.c:1046–1058`，个体上限 `CHAR_LIMITLEVEL > 0` 且等级达到上限时清经验、停升；`char_data.c:1010–1030` 按 `PETTRANS` 配置分支（见第八节）。
- 新服务端：`server/character/pet_growth.go:27` 与 `server/character/pets.go:156` 只用模板的 `LimitLevel`（`gamedata/pets.go:60`），**不支持个体上限被对话改写**。`server/gamedata/petstats.go:63` 已注明“PETTRANS=-1 时看 CHAR_LIMITLEVEL”，即缺的正是这一条。

## 四、宠物转生窗口全流程（PETTRANS）

### 4.1 入口分流：`NPC_TransmigrationTalked`（npc_transmigration.c:49–128）

按玩家身上 79 级玛蕾的数量分流（718；2 转功能 `_PET_2TRANS` 已开启，401 也计入，`version.h:308`）：

| 条件 | 原版处理（台词为 GBK 解码后原文） |
| --- | --- |
| 玩家 < 80 级，且身上有 718/401 | “由于你的等级小于80,没能力进行转宠！”，结束（:71–78） |
| 玛蕾等级 > 79 | 收走该宠，提示“清除你身上非法玛雷菲雅”（:80 起） |
| 玛蕾等级 = 79，主人 CDKEY/名字不符 | “你身上的玛雷菲雅，并不是你的啊！” |
| 玛蕾等级 = 79，四项能力任一 < 15 | “操，你太不爱惜我了，还要我帮你转宠？没门！”（说话的是玛蕾） |
| 79 级玛蕾 > 1 只（`petNum > 1`） | “你怎会有那麽多玛雷菲雅呢？”（:110–111） |
| 79 级玛蕾 = 1 只 | `NPC_PetTransMan_selectWindow(..., 0, -1)`，进入宠物转生（:113–115） |
| 没有 79 级玛蕾 | 进入人物转生 `NPC_Transmigration_selectWindow`（TENSEI 线，本文不展开，见第八节） |

### 4.2 窗口步骤：`NPC_PetTransMan_selectWindow`（1219–1375）与窗口回调（npc_transmigration.c:341–443 的分支 405–430）

| 步骤（case） | 发出的窗口与台词（`ten.arg` PETTRANS 段，第 7–15 行） | 玩家操作与分支 |
| --- | --- | --- |
| 0 开始 | `mainmsg`：玛蕾菲雅：谢谢你让我和精灵王重聚… ；NEXT（:1259–1267） | NEXT → case 1（:405） |
| 1 资格检查 | `NPC_PetTransManCheck`（1377–1397）：玩家 < 80 或事件 4/69/70 未完成 → `failmsg`“玛蕾菲雅：你的宠物还无法转生喔！”，OK 结束（:1268–1292）；通过 → `selectmsg`“你的宠物已经准备好要转生了吗？”，YES/NO | YES → case 2（:411）；NO → case 5（:413） |
| 2 选宠 | `setpetmsg`“请选出一只要接受转生的宠物吧。”，宠物选择窗（:1294–1303） | 取消 → case 5（:419）；选中 → 记 `Pet_Select = datanum`，case 3（:422） |
| 3 确认 | `transmsg`，前面拼上“是[宠物名]没错吧？”，YES/NO（:1304–1318） | YES → case 4（:428）；NO → case 5（:430） |
| 4 执行 | 检查通过后调用 `NPC_PetTransManStatus`（1399–1599）；成功发 `main1msg`，OK 结束（:1319–1360） | 检查不过：只发提示（见 4.3），OK 结束 |
| 5 取消 | `cancelmsg`“玛蕾菲雅：等你考虑清楚了以后再来吧！”，OK 结束，清工作变量（:1361–1368） | — |

结束语：`endmsg`“没事了就赶快离开这里吧，这里对人类来说太危险了…”（ten.arg 第 14 行）。`transmsg` 全文：“你确定好了吗？对宠物来说这可是件不得了的事情，身为它的饲主要好好考虑清楚喔！”。`main1msg` 全文：“宠物转生成功了！请好好对待它吧。那么我们就在这儿分手了，说不定以后还有机会可以再见面喔，请多保重。”。

### 4.3 宠物检查（case 4，:1319–1349）

| 情况 | 原版台词 | 处理 |
| --- | --- | --- |
| 骑乘中（`CHAR_RIDEPET == 选中槽位`） | “骑乘中的宠物不能转生喔！”（:1326–1327） | 只改提示，不清工作变量 |
| 宠物 < 80 级 | “接受转生祝福的宠物必需要等级80以上喔！”（:1330–1332） | 清工作变量 |
| 宠物是 718 或 401 | “这只宠物不能转生喔！”（:1333–1340） | 清工作变量 |
| 转生次数 > 1 | “这只宠物已经接受过祝福了喔！”（:1341–1342，`_PET_2TRANS` 分支） | 清工作变量 |
| 全部通过 | 调用 `NPC_PetTransManStatus`，失败则 `return`（:1350–1351） | **失败时不发任何窗口**，客户端停在确认窗（原版行为，见第八节） |

### 4.4 宠物栏、取消与骑乘

- **宠物栏满**：原版不需要空位。转生复用所选宠物的栏位（`GetNewPet` 只改该宠对象，`enemy.c:1675–1780`），玛蕾的栏位由 `NPC_TransmigrationDelPetDel`（1089–1129）释放。
- **战斗中**：`NPC_TransmigrationDelPetDel` 检查 `BATTLE_CHARMODE_NONE`，战斗中直接返回 FALSE（:1089–1129），`NPC_PetTransManStatus` 随之失败。
- **取消**：三处取消（NO、选宠取消、确认 NO）都走 `cancelmsg`（case 5）。
- **骑乘**：转生前检查骑乘（:1326）；转生后 `RIDEPET` 不处理（见第八节）。转生次数超过 2 的宠物不能骑：`gmsv/char/family.c:2324–2325`（`_PET_2TRANS`）；新服务端已有 `server/character/pets_ride.go:26,74`（`MaxRideTransmigration = 2`）。

## 五、转生结果：数值如何重算

### 5.1 原版公式（`NPC_PetTransManStatus` 1399–1599 与 `GetNewPet` enemy.c:1675–1780）

1. **玛蕾**：`total1` = 玛蕾四项（ALLOCPOINT 的四个字节）之和，上限 150（:1420 起附近）。**宠物**：`total2` = 所选宠四项之和，`petLV` = 宠物等级，`rank` = 宠物 PETRANK。
2. **转生值** `NPC_PetTransManGetAns`（`gmsv/char/char_data.c:1344–1378`，`_PET_2TRANS` 版带 `tran` 参数）：
   - `LV` 上限 130；`x = (total1/100)^5 × 1.3`，`x < 1` 时为 0；`Fx = (int)((5 - rank) × 1.2) + 5`；
   - `ans = (int)x + total2 + (LV - 100) / Fx`（C 整数除法，负数向零取整）；
   - 上限：`tran == 0` 时 150，`tran == 1` 时 200。
3. **四项分配**：`work[i] = ans × (玛蕾第 i 项 + 宠物第 i 项 × 4) / (total1 + total2 × 4)`（:1399–1599 内）。
4. **GetNewPet**（enemy.c:1675–1780）：
   - 每项 `+= RAND(0,4) − 2`（即 −2…+2），打包进 ALLOCPOINT（每项 8 位）；
   - `PETRANK`：四项和 ≥130→0，≥100→1，≥95→2，≥85→3，≥80→4，其余→5（`ranktbl`，:1684–1691 附近）；
   - 再随机 10 次，每次给一项 +1；
   - 四项 = 物种基础值（`PARAM_CAL`，level 固定为 1，即 `INITNUM`）× work；
   - **`LV = 1`**，经验清零，`SLOT = 7`，`CHAR_complianceParameter`，HP 满，写主人，`WHICHTYPE = PET`；
   - **转生次数 +1**（`_PET_2TRANS`，enemy.c:1770–1772；非 2 转时固定为 1）。
5. 之后在 `NPC_PetTransManStatus` 里：`LIMITLEVEL = -1`（:1540）；按范围删物品（见 5.2）；`NPC_NowEventSetFlg(71)`（:1596）；发“%s 接受了精灵王的祝福！”并刷新 K 状态。

### 5.2 物品删除（原版不检查、只删除）

`NPC_PetTransManStatus` 的 `DelItem` 表（1399–1599 内）：

| 范围 | 物品（`server/gamedata/itemset.tsv`） |
| --- | --- |
| 19625–19628 | 地灵石、水灵石、火灵石、风灵石（itemset.tsv:12432–12435） |
| 19629–19632 | 19629 为地灵项链（itemset.tsv:12436），其余三件分组待核 |
| 19672–19687 | 16 件，从地灵项链（19672）到风灵项链（19687），具体分组待核 |

- 循环遍历身上全部道具，凡 ID 落在范围内就删（`CHAR_DelItem`），并写 `LogItem`（"WarpManDelItem"）。
- **原版没有数量检查**：`NPC_PetTransManCheck` 只查等级和事件，`errchar` 里的 `erroritem` 没有用到。所以身上没有灵石也能转生，有几个删几个。
- 另一条线 `NPC_TransmigrationDelPetDel` 只删宠物，不删物品。

### 5.3 转生次数与等级上限的影响

- **转生次数**：宠物 `CHAR_TRANSMIGRATION` 从 0 → 1（第一次，玛蕾 718）或 1 → 2（第二次，玛蕾 401）。第二次之后不能再转（`TRANS > 1` 拒绝）。新服务端字段已在 `server/character/pets.go:60`（STAGE-196）。
- **等级上限**：转生后 `LIMITLEVEL = -1`，宠物从 1 级重新练，上限不再由模板或对话决定（`char_data.c:1054` 的 `> 0` 判断对 −1 不生效，但 `char_data.c:1010–1030` 的 PETTRANS 分支对 −1 的行为要单独核对，见第八节）。

## 六、灵石、跟随、逐站流程：新服务端现状

| 环节 | 原版 | 新服务端 | 结论 |
| --- | --- | --- | --- |
| 灵石消耗（19625–19628） | 转生时按范围删，不查数量（5.2） | 物品表有（`itemset.tsv`），转生删除没有 | 缺口：转生核心 |
| 玛蕾上限站（ptalk） | 第三节，个体上限被对话改写 | 没有 pettalk；上限只看模板 | 缺口：前置 |
| 宠物重算 | 5.1 | 没有 `GetNewPet` 等价物；`pet_transmig.go` 只有测试命令 | 缺口：核心 |
| 事件 71 | `NPC_NowEventSetFlg(71)`（1596） | 没有；king 脚本不能设 | 缺口：任务 51 入口 |
| 转生窗口协议 | PETTRANS 窗口（4.2） | `docs/protocol.md` 里没有宠物选择窗口的消息（grep 无命中） | 缺口：要先改 `docs/protocol.md` 和用例 |
| “少女跟随” | 原版找不到“少女”“跟随”字样；宠物跟随是 `CHAR_WORKPETFOLLOW`（`char.c:2985–2992`，`pet.c:402`）和宠物项圈（`ITEM_petFollow`，`itemset.tsv:2361` 等，ID 2849 起） | 服务端无跟随代码（grep `跟随|follow` 只命中 itemset.tsv 的物品标签） | **待确认**，不纳入拆分 |
| 漆黑线（sikkoku01–05） | 白名单未放开 | 未放开（`npcevent.go` 无 sikkoku） | 白名单要用户定，不在本链路必需 |
| 任务 52–54 | king 脚本，事件 72 等 | 白名单已放开（STAGE-220） | 可用，但要 51 之后才能走 |

- 白名单位置：`server/gamedata/npcevent.go:128` 起的 `npcEventEnabled`（king 在 194–206）。测试开关 `npcevent_open.go`（STAGE-214，仅测试）。
- 转换器：`tools/npcevent.py`、`tools/npcevent_pet.py`、`tools/npcevent_now.py`（“NOW”事件）、`tools/gen-npcs.py`（统计）。本文没有核对每个脚本的覆盖情况。

## 七、拆分建议（按依赖顺序）

| 序 | 小需求 | 一句话内容 | 验收口径 | 依赖 |
| --- | --- | --- | --- | --- |
| 1 | 宠物个体等级上限（pettalk 站） | 支持脚本里的 `PET:LV=x` 和 `LimitLevel` 动作，个体上限可被对话改写；按 `ptalk01` 的 10→79 站验证 | 用例：宠物在 10、15…75 各对话一次，上限依次为 15…79；不对话则不升到 80 | 无 |
| 2 | 宠物转生数值（纯函数） | 实现 `NPC_PetTransManGetAns` 和 `GetNewPet` 的重算：LV=1、SLOT=7、TRANS+1、LIMITLEVEL=-1 | 给定玛蕾/宠物四项和 rank、LV、TRANS，ans 与四项结果可复现（注入随机数） | STAGE-196（已在 develop） |
| 3 | 宠物转生窗口 | 实现 PETTRANS 的分流与 case 0–5、各台词、骑乘/等级/转生次数检查；先补协议的宠物选择窗口 | 协议用例覆盖：开始→选宠→确认→成功；失败台词；取消；战斗中 | 需求 1 可以并行；需求 2 |
| 4 | 转生的物品删除与事件 71 | 转生成功后删 `19625–19632`、`19672–19687`，设置事件 71（进行中） | 持有灵石/项链时转生，范围内物品全删，事件 71 为进行中；无灵石也能转生 | 需求 3 |
| 5 | 任务 51 收尾 | 验证 51（事件 71）→ 52–54 的链路，必要时放开白名单（sikkoku 线另议） | 按 `docs/quest.md` 的 king 表从 49 走到 54；黑盒用 `stoneage-admin quest` 辅助 | 需求 1–4 |

另列：**人物转生（TENSEI）线**（`ten.arg` 的 TENSEI:1–5、`NPC_TransmigrationCheck` 528–652）与“漆黑二十层”线，不在本链路；需要时另开需求。

## 八、与原版不一致、拿不准、待确认

1. **玛蕾编号**：`npc_transmigration.c:71` 注释写“玛雷菲雅 1479 ID 为718”。代码判断用的是 PETID 718；脚本里的 1479 是敌群表行（`enemies.tsv:1104`，TEMPNO 718）。以 718 为准。
2. **死代码**：`char_data.c:1380` 的 `PETTRANS_PetTransManStatus` 没有任何调用，只在 `char_data.h:90` 声明。以 `npc_transmigration.c:1399` 的版本为准。
3. **失败不回包**：case 4 中 `NPC_PetTransManStatus` 返回 FALSE 时直接 `return`（:1350–1351），不发窗口，客户端停在确认窗。新服务端要不要照搬，需要决定（“按原版走”的话照搬）。
4. **先删后算**：`NPC_PetTransManStatus` 先删玛蕾（:1515–1528），之后才 `GetNewPet`（:1532）。`GetNewPet` 在 `ENEMY` 索引异常时返回 −1，玛蕾已经没了。另外删玛蕾的循环里，任一只删除失败就直接返回 FALSE，前面已删的不会回滚（**原版风险**）。
5. **骑乘与删除**：`NPC_TransmigrationDelPetDel`（1089–1129）不清 `CHAR_RIDEPET`；人物转生路线在 `npc_transmigration.c:736` 会清。若玩家骑着玛蕾，删除后 `RIDEPET` 是否悬空，**待确认**。
6. **灵石不查数量**：有就删，没有也能转生（5.2）。与玩家预期可能不同，按原版走，需求方确认。
7. **Pet_Select 全局变量**：窗口回调的 `Pet_Select = datanum`（:422）看起来是全局变量，多个玩家同时转生时可能串号。**待确认**声明位置。
8. **LIMITLEVEL = -1 的影响**：`char_data.c:1010–1030` 在等级高于 `getYBLevel()` 时对宠物的判断，对 −1 的结果要对照 `setup.cf` 与 `configfile.c:2172` 的 `yblevel` 默认值（setup.cf 里没有 YB 项，默认值**待确认**）。
9. **配置开关**：`_PET_TRANS`（`version.h:70`）与 `_PET_2TRANS`（`version.h:308`）都开着；`PETTRANS=-1`（`setup.cf:165`，`configfile.c:2164–2170`）让宠物上限走 `LIMITLEVEL`。
10. **ten.arg 前四行**（samugiru/marinasu/jaja/karutarna）：`Item/Pet/Present/Warp` 参数由 `NPC_TransmigratiomWarp`（:447–500）解析 Warp；`Item/Pet/Present` 的用途和调用点**待确认**。
11. **“漆黑二十层精灵王”**：`sikkoku.create` 的 floorid 是 32002（地下 2 楼），与“二十层”的对应未核对，**待确认**。
12. **ptalk06 重复的 `LimitLevel:30`**（:241）：看起来是笔误，无影响，**待确认**。
13. 原版 C 里还有若干 `return 0`（void 函数）和未用的 `errormy/erroritem/errorpet`，不影响行为，不列入拆分。

## 裁定（用户：需要拍板的都尽量按原版走）

- 不检查灵石数量，按范围有几个删几个；case 4 状态检查失败照原版直接返回；删除中途失败不回滚（照原版，实现时尽量在同一事务内完成，不改变可见行为）。
- “少女跟随”原版没有，不做；服务端也不新增 NPC 跟随机制。
- 其余待确认项（骑乘中删玛蕾、`Pet_Select` 全局变量、LIMITLEVEL=-1、精灵王 NPC）是源码事实核对，由对应需求动手时对照原版确认。
