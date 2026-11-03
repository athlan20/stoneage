# agent 变更记录

给 agent 看的：服务端每次升级，如果协议（`/protocol.md`）或 agent 手册（`/agent.md`）有改动，这里都会登记一条。
`hello` 响应的 `docs_rev` 在这里一定有一条登记；agent 发现 `docs_rev` 变了，就先读这里新增的条目，再读条目提到的章节。

新条目写在最上面，标题格式是 `## 日期 · docs_rev 修订号`，修订号是 `protocol.md` 与 `agent-guide.md` 内容哈希的前 12 位十六进制。
同一份文档在不同分支合入的组合会有不同的修订号，各有一条。`go test ./server/webpages` 会检查当前修订号有没有登记，没有就失败。

## 2026-11-02 · docs_rev 874077146f12

- 文档措辞整理（开源准备）：`protocol.md` 里对原版服务端的出处引用改成通用写法（不再写仓库内路径），“旧服务端”统一为“原版服务端”。没有新增、删除或修改任何消息、字段、错误码和默认值，agent 不需要因此改动。

## 2026-10-30 · docs_rev c555b8b91a1c

- 合并记录：main 合入 develop（STAGE-346 宠物栏 20 格、STAGE-347 道具仓库 50 格、STAGE-334/340）后的修订号，无新增内容，具体变更见下面 bec8d6baeec2、08411530639a、820a2441a922 与 d2157f744f4e 几条。

## 2026-10-30 · docs_rev d2157f744f4e

- 合并记录：STAGE-334（英雄的祝福、丢弃物品的确认）与 STAGE-340（龙王的守护可使用）合并后的修订号，无新增内容，具体变更见下面 cb6fc6ab2792（龙王的守护）、7b3663ca377e、13e40203ffb3、826450f86f3a 几条。

## 2026-10-29 · docs_rev 7b3663ca377e

- 合并记录：STAGE-334 的两处文档改动合并后的修订号，无新增内容，具体变更见下面 13e40203ffb3（agent 手册：丢弃物品的确认）、826450f86f3a（英雄的祝福可使用）两条。

## 2026-10-29 · docs_rev 13e40203ffb3

- `agent-guide.md` 第 6 节增加“丢弃物品的确认”（STAGE-334）：`item_drop` 第一次丢某种东西先问玩家（成批列清单，别一件一问），玩家的答复（同意、拒绝或给出的范围）要记住，之后同一种物品或范围内的东西直接丢、做完汇报，不再重复确认；`gold_drop`、`pet_drop`、镶嵌过的装备和玩家没提过的稀有道具仍然每次都问。纯 agent 行为约定，服务端协议和消息不变。

## 2026-10-30 · docs_rev bec8d6baeec2

- 合并记录：STAGE-347 与 develop（已含 STAGE-346 随身宠物栏 20 格）合并后的修订号，无新增内容；两条变更见下面 08411530639a（道具仓库固定 50 格）与 820a2441a922（宠物栏 5→20 格）。

## 2026-10-30 · docs_rev 08411530639a

- 便利店道具仓库固定 50 格（STAGE-347，`protocol.md` 5.60）：不再随转生（原 0 转 10 格、每转 +4、最多 30）；所有角色（含已转生）都是 0–49 号格，已存的道具不丢，第 50 件寄存回复 `ACTION_NOT_ALLOWED`（`poolfull_msg`）。协议消息和字段不变，寄存每件 50 石币不变；客户端无需改代码，仓库列表 50 行靠 `npc_window` 的列表滚动。

## 2026-10-30 · docs_rev 820a2441a922

- 随身宠物栏从 5 格扩到 20 格（STAGE-346，`protocol.md` 5.19）：`pets` / `pets_changed` 的 `max_slots` 为 20，宠物 `slot` 取值 0–19；宠物店寄放仍从 `slot` 50 起。`pet_state`、`pet_rename`、`pet_drop`、`pet_ride`、`pet_talk`、`pet_skill`、`magic_use` 的 `slot`/`target` 都接受 0–19，`battle_command` 的 `swap_pet` 的 `pet_slot` 接受 0–19。
- `item_use` 的 `target` 从“1–5 是宠物”扩成“1–20 是宠物栏位 + 1”（`target` 为 N 即 5.19 的 `slot` N−1，STAGE-346）；一期没有队友，不再保留 6–10 的队友编号。宠物忠诚道具、英雄的祝福、龙王的守护的 `target` 范围随之变成 1–20。
- 协议消息和字段没有增减，客户端无需改代码就能收 20 格；旧客户端把栏位写死在 0–4 的页面（宠物页、战斗换宠、`pet` 命令）需要跟着升级，见 `docs/ui.md` 宠物页与 `docs/battle-ui.md`「编号规则」。

## 2026-10-29 · docs_rev cb6fc6ab2792

- 龙王的守护（STAGE-340，物品 18544，使用函数 `ITEM_useNoenemy`）：战斗外给自己使用，扣 1 个，推 `notice`（“你感受到周边的杀气消失了。”）；5 分钟内 `encounter` 开关、`encounter_start` 与 1 级宠物目录开战回复 `NO_ENEMY_HERE`（`protocol.md` 5.31 的「龙王的守护」）。协议消息和字段不变（只用了已有的 `notice` 推送和 `NO_ENEMY_HERE` 错误码），客户端无需改动；`agent-guide.md` 不变（`item_use` 已是第二级）。同时在 5.31 盘点了未实现的使用函数（只列不做）。

## 2026-10-29 · docs_rev 826450f86f3a

- 英雄的祝福（STAGE-334，物品 18543，使用函数 `ITEM_useSkup`）：战斗外使用，给自己加 1 点未分配能力点，扣 1 个，推 `status_changed` 与 `inventory_changed`；`item_use` 的 `data` 带 `skill_points` 与 `skill_points_delta`（`protocol.md` 5.31 的「英雄的祝福」）。协议消息和字段不变，客户端无需改动；`agent-guide.md` 不变（`item_use` 已是第二级）。

## 2026-10-28 · docs_rev 0c12c997bd0c

- 任务表增加自定义任务 111「萨姆吉尔的勾玉」（STAGE-333，完成和进行中都是事件 185，不来自原版 jobdaily.txt），任务表共 110 个（`protocol.md` 5.44 的说明已改）。协议字段和消息不变，客户端无需改动；任务页多一行。

## 2026-10-28 · docs_rev 8a3391717f3c

- 合并记录：STAGE-332 与 develop（已含 STAGE-328 丢弃石币）合并后的修订号，无新增内容，具体变更见下面 c76cdf45bb4e、3f8c71423116 两条。

## 2026-10-28 · docs_rev 09ff078fb65a

- 合并记录：STAGE-332 与 develop（STAGE-326 宠物忠诚物品）合并后的修订号，无新增内容，具体变更见下面 3f8c71423116、418a6594480c、72fd64e70cf7 三条。

## 2026-10-28 · docs_rev 3f8c71423116

- STAGE-332 猜谜大师（认定场 812，`npcs` 编号 1814，显示类型「猜谜」）：`talk` 后人类连接按原版答 100 题（选择题用 `npc_choose`，文字题用 `npc_reply`），答满全对给谜之箱子 2759。前置是任务 2 已完成、身上有入门证 2753（扣掉）。**agent 连接直接认定**（不出题，扣 2753、发 2759，属第二级，做完汇报），见 `docs/protocol.md` 5.33 的「猜谜」一条和 `docs/agent-guide.md` 第 6 节。没有入门证时说原版 NoEntryMsg 台词，不改数据。

## 2026-10-28 · docs_rev c76cdf45bb4e

- 丢弃石币（STAGE-328）：新请求 `gold_drop`（协议 5.76），参数 `amount`（正整数，不超过持有）与 `confirm`（字符串 `"yes"`），把自己的石币丢掉一部分（销毁，不可恢复），用来把持有量压到上限以下，让卖东西重新可用。`agent-guide.md` 第 6 节把它列为第三级（必须先问玩家），与 `item_drop` 同级。战斗中回复 `IN_BATTLE`。

## 2026-10-28 · docs_rev 418a6594480c

- 宠物忠诚物品（STAGE-326）补充：`agent-guide.md` 第 6 节分级表里 `item_use` 的措辞改为“回复 HP、MP、增加宠物忠诚的消耗品”，忠诚道具（安抚之素、高级宠物食品）同样是第一级，自己做、不用汇报。协议行为见上一条和 `protocol.md` 5.31“忠诚”，本条无其他新增内容。

## 2026-10-28 · docs_rev 72fd64e70cf7

- 宠物忠诚物品（STAGE-326）：安抚之素（1598，忠30）、高级宠物食品（12763，忠10）在战斗外给宠物用（`item_use` 的 `target` 选 1–5），忠诚封顶 100，已满回复 `ITEM_UNUSABLE` 不扣道具；成功时 `data` 带 `loyalty` 与 `loyalty_delta`，推 `pets_changed`。见 `protocol.md` 5.31“忠诚”。
- 背包物品新增可选字段 `field_target_type`（只在和 `target_type` 不同时才带），世界中的「使用」按它决定要不要选宠物。见 `protocol.md` 5.17 背包（inventory）的物品字段说明。
- 药师店 NPC 830、884 的商品加入安抚之素，买价 10000 石币（店参数 `prices` 覆盖价），卖回价不变（基价 × `sell_rate`，即 100）。见 `protocol.md` 5.37 的“覆盖买价”。

## 2026-10-27 · docs_rev b7dc4c3b260b

- 合并记录（STAGE-310 批量发放石币合入 develop 后的修订号）：无新增内容。石币持有上限统一为 1000 万的口径见 `7d497a2bf7f0`；控制权/遇敌、决斗、强者表、道具仓库、分身目标见 `8f72241d58b2` 及其下已登记的条目。

## 2026-10-27 · docs_rev 8f72241d58b2

- 合并记录（STAGE-311 控制权转交关遇敌同步 develop 后的修订号）：无新增内容。控制权/遇敌规则见 `ffc3849e9666`；决斗、强者表、道具仓库、分身目标见 `c6e30109a5fc` / `c33b81c266ce` / `aa66b7e15c5e` / `0f2a7416a8bb` 等已登记条目。

## 2026-10-27 · docs_rev ffc3849e9666

- STAGE-311（缺陷：收回 agent 控制权后停不下原地遇敌）：控制权转交时遇敌开关跟着控制方走——开关开着就在 `control_changed` 之后自动关掉，推 `encounter_changed`（`reason` 新增取值 `control`）和一条 `notice`；当前这一场照常打完，之后不再开新场。收回（`TAKEN_BY_PLAYER`）、交还（`RELEASED_BY_PLAYER`）、agent 接管（`AGENT_ENTERED`）、原操控方离开（`PEER_LEFT`）四种情况都适用。请求与错误码无增减；新操控方要连场遇敌就自己重新发 `encounter` / `encounter_start`。用例 `control_encounter_off`。
## 2026-10-27 · docs_rev 388c8f231346

- 合并记录（STAGE-312 分身目标同步 develop 后的修订号）：无新增内容。分身默认目标见 `0f2a7416a8bb`；决斗、强者表、道具仓库见 `c6e30109a5fc` / `c33b81c266ce` / `aa66b7e15c5e` 等已登记条目。

## 2026-10-27 · docs_rev c6e30109a5fc

- 合并记录：STAGE-306（决斗服务端）同步 develop（含 STAGE-309 道具仓库）后的修订号。本条无新增协议内容：决斗见下面 `4622a823e2f8` 与 `cca0c6a91299` 条目，道具仓库与宠物寄放池格数见 `aa66b7e15c5e` 条目。
## 2026-10-26 · docs_rev 084627ab61f3

- 合并记录（STAGE-308 合入 STAGE-309 的 NPC 与文档）：无新增内容，见下面已登记的 c33b81c266ce（强者表）与 aa66b7e15c5e（道具仓库）。强者表 NPC 编号因此顺延为 1806–1813。

## 2026-10-26 · docs_rev c33b81c266ce

- 决斗强者表（STAGE-308，设计见 `docs/pk.md` 3.5、5.3）：新请求 `duel_ranking`（5.75，查看类，分级第一级），全服按决斗点数排名，每页 10 人，同分先达到的排前、再按名字；返回 `top`（每条带 `self`）、`page`、`pages` 和自己的 `me`。`npc_window` 新增可选字段 `ranking`（5.34），只在强者表的排行窗口上出现。强者表 NPC（类型「强者表」，竞技场和各村共 8 个，编号 1801–1808）：`talk` 弹「强者们 / 自己」，选项翻页见 5.75。新增用例 `duel_ranking`；`world_npcs`、`ui_npc_follow` 的村里 NPC 列表多了强者表（41 → 43）。

## 2026-10-26 · docs_rev 4622a823e2f8

- 决斗（STAGE-306）服务端开战接入，协议字段与 `cca0c6a91299` 条目一致，补充语义：`duel_challenge`（5.74）通过校验后真正开战，先推双方所有参战者的 `battle_start`（`kind` 为 `duel`，带 `duel.sides` 名单，对方人物只给 `hp_pct`），再回 `{"battle_id"}`；开战前再核对每名队员的状态（倒下、战斗中、交易中、分身队）。每名参战者离开战斗时自己的 `duel` 开关自动关并推 `social_changed`；`battle_end` 没有 `rewards` / `penalty`，`duel_point.change` 是写回后的实际变化。新增用例 `duel_basic`、`duel_escape`、`duel_party`、`duel_offline`、`duel_rate`。

## 2026-10-26 · docs_rev cca0c6a91299

- 决斗（STAGE-306，设计见 `docs/pk.md`，协议先行）：`social_settings` 新增 `duel`（接受决斗，默认关、不存库，进入世界和每场决斗结束后自动关，见 5.69）；`social` 对象与 `social_changed` 带 `duel`；`players` / `players_changed` 的条目加 `duel` 布尔字段（5.66）；新请求 `duel_challenge`（5.74，第三级，先问玩家）；`battle_start.kind` 新取值 `duel`；`battle_end` 新增 `duel_point`；角色新增决斗点数 `duel_point`（默认 100，见第 7 节）；新错误码 `TARGET_DEAD`。服务端的 `duel_challenge` 目前只做发起前的校验（5.74 的 1–8 步）；全部通过时暂回 `WRONG_STATE`（“决斗暂未开放”），双真人战斗还没接入。`duel` 开关和 `duel_point` 字段已可用。

## 2026-10-26 · docs_rev aa66b7e15c5e

- 新增 5.60「道具仓库：寄存、领取道具」（STAGE-309）：便利店 1009/2009/3009/4009 的道具仓库管理员，菜单「寄存」「领取」，寄存每件 50 石币。格数随转生：道具 min(转生×4+10, 30)。没有新增请求，`talk` 和 `npc_choose` 照常用。
- 5.41 宠物寄放池的格数改为随转生：min(转生×2+5, 15)，0 转 5 格（原来固定 15 格）。已经超过新上限的寄放宠物不迁移，只是新上限满之前不能再寄放，领取不受影响。
## 2026-10-27 · docs_rev 0f2a7416a8bb

- STAGE-312（分身默认目标按合击顺序）：没有带 `auto_profile`、或方案的 `target` 是 `combo` 时，分身和分身宠物一律打敌方合击顺序的第一个（前排从上到下、再后排，同客户端的 `enemy_order`），不再各自随机；玩家没给分身指定目标（含回合超时）时也按这个规则，指定过的目标在出手前倒下则按同一顺序顺延到下一个能打的敌人。方案显式选了 `random` 的仍随机改选。请求、推送与错误码无增减。客户端分身面板的默认目标显示跟着改：没指定的行写灰色的 `默认→ 敌N`。

## 2026-10-26 · docs_rev 7d497a2bf7f0

- 石币持有上限统一为 1000 万（`battle.MaxHaveGold`，不随转生变化），商店、交易、战斗拾取和偷取、管理命令发石币共用。本条只改文档口径（`protocol.md` 交易相关的上限描述），请求、推送与错误码无增减；`docs/trade.md` 第 6 条已拍板。

## 2026-10-26 · docs_rev 335079dda27d

- 合并记录：develop（交易、精工、战斗规则对齐、MCP 接入等）合入 main 后的修订号。本条无新增协议内容；遇敌间隔默认 0.5 秒见下面 `446649c6e3a9` 条目，交易、精工、社交开关 `trade` 等见下面各自条目。

## 2026-10-26 · docs_rev 446649c6e3a9

- 遇敌开关两场战斗之间的默认间隔从 2 秒改为 0.5 秒（`STONEAGE_ENCOUNTER_INTERVAL_MS` 默认 500）。只是默认值变化，请求、推送与错误码无增减；`protocol.md` 默认值表里的数字同步更新。

## 2026-10-26 · docs_rev 9c040c3bcc6e

- 合并记录：STAGE-291（宠物技能对齐原版）同步 develop（含 STAGE-292 战斗反击）后的修订号。本条无新增协议内容，请求、推送与错误码无增减；291 的说明见下面 `3dd9dff870e4` 条目，连环反击见 `99fc1f939ce4` 条目。

## 2026-10-26 · docs_rev 3dd9dff870e4

- STAGE-291（宠物技能对齐原版）：`protocol.md` 只改了说明（宠物转生状态、`transmigration` 的说明），请求、推送与错误码无增减。`battle_end` 在失败、逃跑、平手时，若本场捐献偷到了石币，会带 `rewards`（只含 `gold`，其余字段不变）；没偷到时与原来一样不带。客户端不需要改协议解析。

## 2026-10-26 · docs_rev 595f5a98e96c

- 合并记录：STAGE-292（战斗反击）同步 develop 后的修订号。本条无新增协议内容，请求、推送与错误码无增减；连环反击的说明见下面 `99fc1f939ce4` 条目，商品上限 33→60 见 `f5e87a363459` 条目。

## 2026-10-26 · docs_rev 99fc1f939ce4

- STAGE-292（战斗反击）：`counter` 事件的说明补充了连环反击（10.7 事件种类表）。请求、推送与错误码无增减，客户端不需要改协议解析；同一次攻击后可能连着出现多个 `counter`，按顺序播放即可。

## 2026-10-26 · docs_rev ceace0637c6c

- 合并记录：STAGE-302（便利店素材集中化）同步 develop 后的修订号。本条无新增协议内容，请求、推送与错误码无增减；5.38 商品上限 33→60 的变更见下面 `f5e87a363459` 条目。

## 2026-10-26 · docs_rev f5e87a363459

- STAGE-302 便利店素材集中化：商店（5.38）的商品上限由 33 件改为 60 件，萨姆吉尔与卡鲁它那两家便利店的素材店各 6 家、每家 40 件（1–10 级齐全）。没有新增或改动请求、推送与错误码；商品超过一屏时客户端列表滚动，不截断。

## 2026-10-26 · docs_rev 1404c175a696

- 合并记录：STAGE-298（精工 5.73）合入 develop（含 STAGE-294/295/296 交易与开关修复）后的修订号。本条无新增协议内容，请求、推送与错误码无增减；精工见 `fa7e06bad504`，交易见 `df82dcdd51ad` / `4bed29c0f0e4`。

## 2026-10-26 · docs_rev aa277266324e

- 合并记录：STAGE-294 与 STAGE-295 合入 develop（含 STAGE-296 的 `trade` 字段说明）后的修订号。本条无新增协议内容，请求、推送与错误码无增减；变更见下面已登记的 STAGE-294/295/296 条目。

## 2026-10-26 · docs_rev 4f5d3f1e22ae

- 合并记录：STAGE-294 同步 develop（STAGE-300 MCP 接入说明）后的修订号。本条无新增协议内容，请求、推送与错误码无增减，变更见下面已登记的条目。

## 2026-10-26 · docs_rev df82dcdd51ad

- STAGE-294（交易：协议与服务端）缺陷修正，协议行为有两处说明更新（`protocol.md` 5.72）：对方的石币超上限改说“对方石币已达上限”（`GOLD_FULL`，`error.data.who` 为 `"peer"`），不再说“对方物品栏已满”；`trade_ended` 的 `summary` 按收到的那一方的视角给出（`gave` 是自己给出的，`got` 是自己拿到的）。请求、推送的名字和结构都没有变，客户端无需改动。

## 2026-10-26 · docs_rev b09801be1758

- 合并记录：STAGE-295 同步 develop（STAGE-300 MCP 接入说明）后的修订号。本条无新增协议内容，请求、推送与错误码无增减，变更见下面已登记的条目。

## 2026-10-25 · docs_rev 01639ed7ba7b

- 合并记录：STAGE-298（精工）合入 STAGE-294/296（交易）后的组合修订号，无新增内容，具体变更见下面已登记的 `fa7e06bad504`（精工 5.73）、`4bed29c0f0e4`（交易 5.72）、`a10b73025f44`（交易开关 5.69）。

## 2026-10-25 · docs_rev fa7e06bad504

- STAGE-298（精工：协议与服务端）：新增请求 `item_inlay`（5.73，目标格 + 1 到 3 个材料格，操作类，世界中）。`inventory` 与 `inventory_changed` 的格子对象在类别码非空的物品上多带 `inlay`（可镶装备为 `role: gear` 加 `free`、`codes`，材料为 `role: material`）。新增 5 个错误码 `INLAY_TOO_MANY`、`INLAY_NOT_SUITABLE`、`INLAY_MULTI_TARGET`、`INLAY_NO_TARGET`、`INLAY_FULL`。`agent-guide.md` 第 6 节：`item_inlay` 是第三级，必须先问。现有数据下没有物品带 `inlay`，旧 agent 忽略即可。

## 2026-10-25 · docs_rev a10b73025f44

- STAGE-296（社交：接收交易邀请）：`protocol.md` 5.69 的 `social_settings` 新增可选布尔字段 `trade`（接收交易邀请，默认 `true`），`social` 对象（`enter_world`、`status`、`social_settings` 响应、`social_changed` 推送）多一个 `trade` 字段；对方关闭时 `trade_invite` 回复 `TARGET_REFUSES`，自己主动邀请不受影响。存储沿用迁移 `0030_trade.sql` 的 `characters.social_trade`，没有新迁移。新用例 `social_refuse_trade`。
- agent 的分级表（`agent-guide.md` 第 6 节）不变：`social_settings` 原来就在第二级，新字段不改变分级。

## 2026-10-25 · docs_rev 4bed29c0f0e4

- STAGE-294（交易：协议与服务端）：`protocol.md` 新增 5.72 交易（`trade_invite`、`trade_reply`、`trade_cancel`、`trade_offer`、`trade_lock`、`trade_unlock`、`trade_confirm` 七个操作类请求，推送 `trade_request`、`trade_request_result`、`trade_updated`、`trade_ended`），新增 8 个错误码；`agent-guide.md` 第 6 节新增交易分级：`trade_lock`、`trade_confirm` 为第三级，必须先问玩家。对方改了报价（`rev` 变了）必须重新看，不能沿用旧的许可。
- 服务端配置 `trade.enabled` 默认关，关闭时上述请求回复 `WRONG_STATE`；客户端界面不在本单，两边一起进测试后再打开。

## 2026-10-26 · docs_rev dd723d1b2ade

- 新增 MCP 接入说明 `/mcp.md`（STAGE-300）：本地 stdio 的 `stoneage-mcp`，让 Claude Code、Claude Desktop 等直接调用工具玩游戏。`agent-guide.md` 第 7 节加了一段指向它。协议（`protocol.md`）没有改动，请求、推送与错误码无增减，第 3、6 节的规则和分级表无变化。

## 2026-10-23 · docs_rev 2b6ee605d7a8

- 合并记录：STAGE-289（漆黑洞窟 20 层新增 125-130 级蓝雷遇敌区）合入 develop 时的修订号。本单只改游戏数据，没有协议变更，请求、推送与错误码无增减。
- 这个修订号是两批文档合并后的组合：`encounter_areas`（5.50）的 `level1_pets`（STAGE-286，见 docs_rev 3693b793b519）+ 聊天屏蔽词说明（STAGE-287，见 docs_rev 65d08a0064bc）+ agent 手册里战宠补血那句（见 docs_rev d5806e71d9e4）。

## 2026-10-23 · docs_rev 7b9c08de7dd6

- 合并记录：战宠补血（本单之前那次客户端修复）改的是 `agent-guide.md` 第 5 节的托管策略说明——宠物低血由人物出手，喂药或对宠物放精灵（单体精灵把目标指到宠物，全体精灵不带目标）。没有协议变更，请求与推送无增减，第 6 节分级表无变化。

## 2026-10-23 · docs_rev d5806e71d9e4

- 合并记录：本地 develop 合入远端 develop 的「战宠补血」提交（4646edf0，agent 手册 6 节自动战斗补血一句改为「宠物低血由人物出手补（喂药，或对宠物放精灵）」），与 STAGE-287 的屏蔽词说明合在一起后的修订号；无新增请求、推送或错误码，STAGE-287 的内容见下面 docs_rev 65d08a0064bc 一条。

## 2026-10-23 · docs_rev 65d08a0064bc

- STAGE-287：公开聊天与名字的屏蔽词规则说明更新，无新增请求/推送/错误码。

## 2026-10-23 · docs_rev 3693b793b519

- 新增（只读）：`encounter_areas`（5.50）每个区域多一个字段 `level1_pets`，是本区域能遇到的 1 级宠物名（来自 1 级宠物目录，同区域其他宠物也算，去重，没有时为 `[]`）。不影响 `available`、`encounter_start` 与人数；字段说明见 [遇敌区域协议](protocol/encounter-areas.md)。旧客户端忽略该字段即可，无需先问玩家（STAGE-286）。

## 2026-10-23 · docs_rev 845845b9fd3d

- 合并记录：STAGE-282（战斗结束后迟到确认）同步 develop（含 STAGE-274 至 STAGE-281）后的修订号，无新增内容，见下面各条。

## 2026-10-22 · docs_rev 6bc262b755c7

- 调整 `/protocol.md` §10.8（STAGE-282）：战斗刚结束（`battle_end` 已推送）后，`turn` 等于这场战斗最后一个回合的 `battle_ack` 按过期确认回复成功 `{}`，不再回 `NOT_IN_BATTLE`；从没进过战斗或回合对不上仍回 `NOT_IN_BATTLE`，新战斗开始后上一场的记录作废。没有新增请求或推送，第 6 节分级表无变化。
- 对 agent 的影响：回放完最后一回合再发 `battle_ack` 不会再收到错误；仍建议收到 `battle_end` 后不要再发战斗指令。

## 2026-10-23 · docs_rev 94e6b379c183
94e6b379c183

- 合并记录：STAGE-275（真人同场战斗）同步 develop（含 STAGE-277 转让队长与队伍频道）后的修订号，无新增内容，见下面各条。

## 2026-10-23 · docs_rev c7be5d6fcf80
c7be5d6fcf80

- 合并记录：STAGE-275（真人同场战斗 §10.14）同步 develop（含 §5.67–§5.71）后的修订号，无新增内容，见下面各条。

## 2026-10-22 · docs_rev f0d9af4cde7c

- 合并记录：STAGE-275（真人同场战斗，§10.14）同步 develop（含 STAGE-274 真人组队 §5.67、STAGE-278 公开聊天 §5.68）后的修订号，无新增内容，见下面各条。

## 2026-10-22 · docs_rev 35e1e223d18f

- 新增 `/protocol.md` §10.14「真人队同场战斗」（STAGE-275，设计已定、尚未实现；依赖 STAGE-274 的 §5.67，未合入 develop 之前这一条只是设计）：真人队的队长遇敌时队员同场，`battle_start.kind` 新增取值 `party`，每人各收一条 `battle_start`（`you` 不同）；`battle_turn.deadline_ms` 为 `null`，第一条指令之后 120 秒没交齐的真人被踢出战斗并离队，**没有默认动作**；新推送 `battle_leave`（超时、掉线时其余人收到，带 `progress`）；`battle_end.result` 新增 `forced_out`（超时被踢出，无奖励无惩罚）；`party_changed` 新增可选字段 `cause`（`battle_escape` / `battle_timeout` / `battle_offline`，只在战斗中的离队推送里）；真人单位（自己和队友）的 `mp`、`max_mp` 都给出。没有新的操作类请求，`battle_command` 的用法不变，§6 分级表不用改。对 agent 的影响：战斗中逃跑、被踢、掉线都会离队，队伍变化以 `party_changed` 为准；`deadline_ms` 为 `null` 时别按它倒计时；超时没有默认动作，别等它。实现前 agent 不会遇到这些推送，单人战斗的行为没有变化。

## 2026-10-23 · docs_rev d441f5cda24f

- 合并记录：STAGE-277（转让队长、队伍频道）同步 develop（含 STAGE-276、278、279）后的修订号，无新增内容，见下面各条。

## 2026-10-23 · docs_rev 79c33a962c73
79c33a962c73

- 澄清 `/protocol.md` §5.67 配置段（STAGE-277）：`party.players_enabled` 关闭时，除原来的六个请求外，`party_leader` 也回复 `WRONG_STATE`；规则没有变化，只是文字补全。

## 2026-10-23 · docs_rev bafdab79a1f7
bafdab79a1f7

- 澄清 `/protocol.md` §5.68（STAGE-277）：agent 在 `map`、`world` 的公开发言是第三级（先问），在 `party` 队伍频道的发言是第二级（做完汇报），与 `agent-guide.md` 第 6 节一致；规则没有变化。

## 2026-10-23 · docs_rev 35fe3565bd46

- 调整 `/protocol.md` §5.68「队伍频道」（STAGE-277）：分身队不能发队聊，和没有队伍一样回复 `NOT_IN_PARTY`（原先草稿写的是“分身队也能发、只回本人”）。对 agent 行为没有新要求：队聊只在真人队里能用，其余内容同下面 177c246e8742 一条。

## 2026-10-22 · docs_rev 177c246e8742

- 队伍协议先行（STAGE-277）：新增操作类请求 `party_leader {name}`（转让队长，见 `/protocol.md` §5.67，全队收到 `party_changed` 的 `leader_changed`）；公开聊天 `chat` 的 `channel` 新增 `party`（队伍聊天，§5.68「队伍频道」，发给本队真人成员，agent 连接收不到，不另加请求和推送）。新增错误码 0 个，复用已有的。agent 发队聊属第二级（做完汇报），转让队长属第三级（先问玩家），见 `agent-guide.md` 第 6 节。服务端和客户端尚未实现，此时 `party_leader` 与 `party` 频道仍会回复旧行为。

## 2026-10-23 · docs_rev eb6f5bcd088c

- 合并记录：STAGE-279（练级传送 §5.70/§5.71）同步 develop（含 STAGE-276 §5.69）后的修订号，无新增内容，见下面各条。

## 2026-10-23 · docs_rev 3482936a1600

- 编号调整（STAGE-279）：练级传送两节从 `/protocol.md` §5.69 / §5.70 改为 §5.70 `leveling_catalog` / §5.71 `leveling_start`，避开 STAGE-276 的 §5.69「社交设置」；内容没有变化，下面 8b0ff7551373 一条里的小节号以这里为准。

## 2026-10-23 · docs_rev 8b0ff7551373

- 新增 `/protocol.md` §5.70「练级传送目录」`leveling_catalog`（查看类，STAGE-279）与 §5.71「练级传送开战」`leveling_start`（操作类，STAGE-279）：目录按 10 级分段（1–10 到 121–130，共 13 段），不带参数返回摘要，带 `segment`（段下限）再返回该段的练级点；`leveling_start` 只带区域 ID，校验顺序与 5.63 相同。新增错误码 `LEVELING_SPOT_NOT_FOUND`。
- 对 agent 的影响：`leveling_start` 的 `once` / `infinite` 为第二级（做完汇报），`travel` 同 `warp`，见 `/agent-guide.md` 第 6 节第二级的新增项；服务端升级前没有这两条请求，行为不需要改。
- 服务端和客户端已在 STAGE-279 实现（服务端 `leveling_catalog` / `leveling_start`，客户端根层「练级传送」）；本条只登记协议，内容没有另外变化。

## 2026-10-22 · docs_rev 2ab9f81b1e7c

- 社交开关（STAGE-276，协议先行）：`/protocol.md` 新增 §5.69 `social_settings`（隐身 `visible`、拒绝邀请 `invite`，两个布尔值，都可省略）与推送 `social_changed`；`enter_world` 和 `status` 的响应带 `social` 字段；§5.67 的 `TARGET_REFUSES` 从这一版起返回。`agent-guide.md` 第 6 节：`social_settings` 第二级（做完汇报）。服务端和客户端还没实现，实现前不要发这个请求。

## 2026-10-22 · docs_rev 0403077c6ea8

- 合并记录：STAGE-274（真人组队 §5.67）同步 develop（含 STAGE-278 公开聊天 §5.68）后的修订号，无新增内容，见下面各条。

## 2026-10-22 · docs_rev 6361e65dc8f5

- 编号调整（STAGE-278）：公开聊天 `chat` 一节从 `/protocol.md` §5.67 改为 §5.68，避免和 STAGE-274 的「真人组队」§5.67 重号；内容没有变化，9.5、9.6 和 `agent-guide.md` 里的引用同步改成 5.68。

## 2026-10-22 · docs_rev 23e8c4cc3b2b

- `/protocol.md` 9.6 补一句例外（STAGE-278）：聊天类请求不写审计，但 agent 连接发出的公开聊天 `chat` 成功后每条写一条，注明由 agent 发出；玩家自己的发言不写。对 agent 行为没有新要求，其余内容同下面 2a8c2d2b60d5 条。

## 2026-10-22 · docs_rev 2a8c2d2b60d5

- 新增 `/protocol.md` §5.68「公开聊天」（STAGE-278，服务端和客户端尚未实现）：`chat` 请求发到本图（`map`）或世界（`world`），推送 `chat_message` 给同图或全服的玩家连接（含发送者自己的玩家连接，推送先于响应）。新增错误码 `CHAT_MUTED`、`CHAT_LEVEL_LOW`；`RATE_LIMITED` 在这里也会出现。`agent` 连接收不到任何公开聊天。
- 对 agent 的影响：服务端升级前没有可用的聊天请求，行为不需要改。公开发言属于第三级：先问玩家，再发 `chat`（`agent-guide.md` 第 6 节新增一行分级）；汇报和私聊仍用 `agent_say`。`protocol.md` 9.5 里“公开发言”的预留说明改成指向 5.68。

## 2026-10-22 · docs_rev 03e79e08785f

- 新增 `/protocol.md` §5.67「真人组队」（STAGE-274，协议先行，服务端尚未实现，功能默认关闭）：操作类请求 `party_invite`、`party_join`、`party_reply`、`party_cancel`、`party_leave`、`party_kick`；推送 `party_request`、`party_request_result`；`party_changed` 新增 `reason`（`member_joined`、`member_left`、`member_kicked`、`kicked`、`left`、`leader_changed`）和 `who` 字段；真人队的 `party` 为 `kind: "players"`；跟随时 `position_changed` 带 `reason: "follow"`；新增 14 个错误码。配置 `party.players_enabled` 默认关，关闭时组队请求回 `WRONG_STATE`。
- 对 agent 的影响：`party_invite`、`party_join`、`party_leave` 为第二级；`party_reply` 拒绝和 `party_cancel` 为第一级；`party_reply` 接受、`party_kick`、真人队的 `party_disband` 为第三级（见 `/agent-guide.md` 第 6 节的组队表）。队员的换图和遇敌请求会回复 `PARTY_FOLLOWER`，agent 应停止这些动作并等待队长行动。
- 改动既有消息：`players`（5.66）的 `party` 字段在有真人队时不再固定为 `null`，组队、离队、队长变化时推 `players_changed` 的 `update`。
- 用例：`party_players_invite`（两个账号）、`party_players_errors`（单账号）。这两份用例 `meta.stage` 是 STAGE-274，服务端未实现前不在回归测试里运行。

## 2026-10-22 · docs_rev dfa022f20b02

- 新增 `/protocol.md` §5.66「附近玩家」（STAGE-273）：`players` 查看同一张地图上其他在世界中的角色（名字、等级、转生次数、是否战斗中；队伍字段本期固定 `null`），最多 50 人；推送 `players_changed`（`enter` / `update` / `leave` 增量，同图 1 秒合并）。观察方也能查看。没有新的操作类请求，也没有改动既有消息。对 agent 的影响：可以用 `players` 看看同图有谁（不含自己）；推送不需要处理，忽略也不影响正确性，换图或重连后以 `players` 的整表为准。

## 2026-10-21 · docs_rev 8ba48b0557dc

- 合并记录：STAGE-266 与 develop（含 STAGE-268）的文档合在一起后的修订号，无新增内容，见下面各条（人物转生见 `/protocol.md` §5.65）。

## 2026-10-21 · docs_rev 1bd862448951

- 合并记录：STAGE-266 与 develop 的文档合在一起后的修订号，无新增内容，见下面各条（人物转生见 `/protocol.md` §5.65）。

## 2026-10-21 · docs_rev 93ac8786c3d8

- 新增 `/protocol.md` §5.65「人物转生：主城精灵王」（STAGE-266）：四个主城存档点旁各有一个精灵王（NPC `type` 为「转生」），等级 ≥ 80、转生次数 < 5 时 `talk` 走窗口流完成转生。没有新增请求或推送，窗口用现有的 `npc_window` / `npc_choose`。
- 对 agent 的影响：转生会把等级归 1、清空任务与事件进度（只保留少数白名单事件）、覆盖未分配的 `skill_points`，并赠一只宠物；做之前先问玩家（和宠物转生一样属于不可逆操作）。宠物栏满时会被拒绝。

## 2026-10-21 · docs_rev b15ad49f1381

- 0 号宠物栏位不再省略（STAGE-268，协议 10.7、10.9）：`battle_round` 的 `capture`、`swap_pet` 事件在栏位号为 0 时也带 `pet_slot`（`"pet_slot": 0`）；缺这个字段只表示“没有栏位”（抓取失败、资质放生），`swap_pet` 收回仍是 `"pet_slot": -1`。`battle_end.rewards.exp` 里宠物那一项的 `slot` 为 0 时也会出现，人物那一项没有 `slot`；`rewards.pets` 的 `slot` 一直如此。
- 对 agent 的影响：以前 0 号栏位拿不到栏位号，现在会明确给出；按 `pet_slot` / `slot` 是否存在判断“有没有栏位”仍然成立。第 6 节的分级表没有新增行：没有新增请求或参数。

## 2026-10-21 · docs_rev 28dfb5103150

- 宠位已满时抓宠判定失败（STAGE-267，协议 10.5、10.7）：随身宠位已有 5 只时，`battle_command` 的 `capture` 即使本来判定成功也不会抓到，`capture` 事件 `ok` 为 `false`，怪物留在场上（没有 `left`、`pet_slot`、`discarded`），`rewards.pets` 不含它，`pets` 列表不变。事件形状与普通抓取失败相同，客户端不用改；下一回合腾出栏位后可以重新抓。
- 对 agent 的影响：发出 `capture` 后不能只看指令回复成功就当抓到，要等 `battle_round` 的 `capture` 事件看 `ok`；`ok` 为 `false` 且怪物还在场上时，需要先放生或换栏位再抓。第 6 节的分级表没有新增行：`capture` 本来就是第三级。

## 2026-10-21 · docs_rev f98c799eba3e

- 合并记录：STAGE-253 与 develop 的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev 8c05ead06ceb

- 合并记录：STAGE-253 与 develop 的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev 1811f3144f32

- 新增 `/leveling.md`（STAGE-253）：agent 练级路线 1→120，按人物等级分 S0–S10 段，每段写明地图 floor、选区域的判据、换段与换层的条件；数据依据服务端的遇敌表和敌人表，已逐项核对。`/agent.md` §8 加了一条指向它。
- 对 agent 的影响：玩家没给安排时，按 `/leveling.md` 练级；协议没有变化，请求和字段都照 `/protocol.md`。

## 2026-10-21 · docs_rev c842cb8677de

- 合并记录：STAGE-264 与 develop（含 STAGE-269 注册上限、STAGE-260）的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev 9831e8d69448

- 抓宠可以按资质档位筛选（STAGE-264 B 段，服务端）：`battle_command` 的 `capture` 新增可选的 `keep` 对象（键 `vital`、`strength`、`toughness`、`dexterity`，值 −4 到 0 的整数，缺省为不限；AND 判断），协议 10.5。抓成功但资质不合格的宠物不入库、不占宠位，怪物照常离场，`capture` 事件 `ok` 为真、没有 `pet_slot`，改带 `discarded`（`name`、`level`、四项 `quality`、`reason`），协议 10.7。宠位已满时的行为不变（先看宠位，满了不看资质）。
- 对 agent 的影响：不带 `keep` 的捕捉与之前完全一样，不用改。要筛选的，`capture` 带上 `keep`；收到 `discarded` 时这只宠物不在 `pets` 里，也不在 `rewards.pets` 里，但战斗里仍算抓成功。`keep` 的类型或越界回复 `BAD_REQUEST`。分身方案（5.57）的 `capture.keep` 只存进方案，分身目前不自己捕捉，所以暂时不影响任何捕捉。第 6 节的分级表没有新增行：`capture` 本来就是第三级，带 `keep` 也先问。

## 2026-10-21 · docs_rev cafbe373f76b

- 同一 IP 注册频率上限由每小时 5 次放宽到 50 次（STAGE-269）：`register` 的 `RATE_LIMITED` 说明、协议第 7 节注册频率两处的数字改了，错误码和 `retry_after_seconds` 的语义不变。对 agent 没有影响（agent 用令牌登录，不走注册）。

## 2026-10-21 · docs_rev 31badb78dff1

- 合并记录：STAGE-262 与 develop 的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev da35c3ba5213

- 复活类道具可以在战斗中用（STAGE-262 A 段）：复活药(100/300/500)、回魂之玉、萨顿家的秘药、真诚的眼泪、铁壁之石对倒下的己方人物或宠物生效。这些道具的 `target_type` 是 `ally_down`（必须带 `target`，选活着的、敌方的回复 `INVALID_TARGET`），之前是 `none`、不能用。复活量同复活精灵，真诚的眼泪、铁壁之石复活后再挂吸收、无效的防御次数。世界里仍不能用（回复 `ITEM_UNUSABLE`），协议 5.31、10.5 已补充。
- 对 agent 的影响：背包里的复活类道具 `target_type` 变成 `ally_down`，按 10.5 带 `target` 即可；`agent-guide.md` 第 5 节的默认策略仍不自动复活，倒下的宠物要复活等玩家吩咐。

## 2026-10-21 · docs_rev a10f3c0228d0

- 合并记录：STAGE-251 与 develop（含 STAGE-255 美容师 5.64 等）的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev 61151de3adac

- 合并记录：STAGE-251 与 develop（含 STAGE-257、STAGE-258）的文档合在一起后的修订号，无新增内容，见下面 STAGE-251 的 `ed29803c887d`、STAGE-258 的 `24112cc1d4a5` 和 STAGE-257 的 `8ba52fb6ecf1` 几条。

## 2026-10-21 · docs_rev 350019ed800c

- 合并记录：STAGE-251 与 develop（含 STAGE-257）的文档合在一起后的修订号，无新增内容，见下面 `ed29803c887d`（STAGE-251）和 `8ba52fb6ecf1`（STAGE-257）两条。

## 2026-10-21 · docs_rev ed29803c887d

- 新增 1 级宠物目录（STAGE-251）：`pet_encounter_catalog`（5.62，查看类）一次返回全部 1 级可遇宠物和可遇点，`pet_encounter_start`（5.63，操作类）按宠物与区域跨图定位并开战；`mode` 除 `once` / `infinite` 外多一个 `travel`（只传送到代表落点，不开战）。新错误码 `PET_ENCOUNTER_NOT_FOUND`。协议用例 `pet_encounter.jsonl`、`pet_encounter_battle.jsonl`。
- 对 agent 的影响：已有请求和推送都不变；想用的话，目录从 `pet_encounter_catalog` 取，`catalog_version` 变了就重取。开战的状态字段、次数与停止语义同 `encounter_start`（5.50 / 5.51）。

## 2026-10-21 · docs_rev 74ac575d29c3

- 合并记录：STAGE-254 与 develop 的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev 06cce3bbf433

- 回血优先选精灵（STAGE-254）：`agent-guide.md` 第 5 节的默认战斗策略改为：己方人物 `hp` 低于 30% 时，先用回复类精灵（`effect` 为「回复自己 HP」「回复己方一个 HP」「回复己方全体 HP」，如「恩惠的精灵」，有全体的先选全体的），没有可用精灵再用回复道具，都没有才 `guard`。协议没有变动，`protocol.md` 不用重读。
- 对 agent 的影响：默认策略里，背包没药但身上有回复精灵且 MP 够时，不再直接 `guard`，改发 `battle_command` 的 `magic`。精灵的 `target_type` 为 `ally_all` / `self` 时不带 `target`，为 `ally` 时带宠物单位号。自己写的 agent 如果依赖旧的「道具优先」默认，要改成按新顺序处理。

## 2026-10-21 · docs_rev 73c459ebed8c

- 合并记录：STAGE-255 与 develop（含 STAGE-263 等）的文档合在一起后的修订号，无新增内容，见下面各条。

## 2026-10-21 · docs_rev e4e96156e7b6

- 美容师（STAGE-255，`protocol.md` 5.33 的类型列表和 5.64）：`talk` 美容师（`type` 为「美容师」，1789–1792 号，四张医院各一个）弹三步窗口，复用 `npc_window` / `npc_choose` / `npc_window_closed`，没有新消息和新字段。选「给我魅力」先关旧窗口再推报价，选「是」扣石币、魅力 +5（封顶 100），推 `status_changed`、`pets_changed`。
- 对 agent 的影响：美容师的窗口和护士、商店一样，按 `npc_window` 的正文和选项走即可；报价是 `1 是`、`2 否`；钱不够回 `NOT_ENOUGH_GOLD`，窗口留着。

## 2026-10-21 · docs_rev dc7cb3ff96cb

- `talk`（STAGE-263，NPC 对话前置）：同一张图上、角色离 NPC 超出它的对话距离时，服务端先把角色移到 NPC 旁边的空格（离角色最近的一格，不能是 NPC 所在格、出口触发格或被别人占着的格），推送 `position_changed`（先于 `npc_window`），再照常弹窗。目前只有宠物技能店（`npcgen_petskillshop`，2 格）有距离要求。找不到空格时角色不动，回复原来的距离错误。不同图的 NPC 不变，仍回复 `NPC_NOT_HERE`。
- 对 agent 的影响：`talk` 成功之后位置可能变了，会收到一条 `position_changed`（5.14），以它为准更新位置，不要假设位置不变；`npc_choose` 不移动角色。

## 2026-10-21 · docs_rev 24112cc1d4a5

- `maps`（STAGE-258 补充）：只带 `keyword` 不带 `parts` 时，按 `parts=["maps"]` 处理，只返回过滤后的平面表，不再附带整棵节点树（以前不声明 gzip 时会因节点树超过 64 KiB 收到 `RESPONSE_TOO_LARGE`）。取节点树（`parts` 含 `nodes`，原文约 81 KB）仍要先在 `hello` 里声明 gzip。完整 `maps` 与大岛 `encounter_areas` 的原文大小更正为约 100 KB。
- 对 agent 的影响：查地名直接发 `{"keyword":"村"}` 即可；需要节点树的，声明 gzip。

## 2026-10-21 · docs_rev d69750735af7

- 合并记录：STAGE-258 与 STAGE-257 的文档合在一起后的修订号，无新增内容，见下面 `82b32ac75c62`（STAGE-258）和 `8ba52fb6ecf1`（STAGE-257）两条。

## 2026-10-21 · docs_rev 82b32ac75c62

- 大帧压缩与超限提示（STAGE-258）：线上超过 64 KiB 的消息不再发出（生产经 CDN，CDN 限制单帧大小，以前超限会直接断连）。请求的响应超限时改发同 `id`、同 `type` 的 `RESPONSE_TOO_LARGE` 错误，推送超限直接丢弃，连接不断。`protocol.md` 第 1 节的 64 KiB 口径改为收发双向，第 4 节错误码表登记了 `RESPONSE_TOO_LARGE`。
- `maps` 新增可选参数 `parts`（`"maps"` / `"nodes"`，只返回选中的部分）和 `keyword`（按名字过滤平面表），见 5.42；`encounter_areas` 新增可选参数 `offset`、`limit`，响应多一个 `total`，见 5.50。不带参数时响应结构不变。
- 对 agent 的影响：完整 `maps` 和大岛的 `encounter_areas` 原文都约 88 KB，不声明 gzip 的连接会收到 `RESPONSE_TOO_LARGE`。参考脚本 `agent-connect.py` 已默认声明 gzip 并自动解压，不用改；自己写的客户端在 `hello` 里声明 `"compress":["gzip"]`，或用参数缩小范围，收到 `RESPONSE_TOO_LARGE` 不要原样重试。

## 2026-10-21 · docs_rev 8ba52fb6ecf1

- 骑乘战斗回血分账（STAGE-257）：战斗内回复 HP（治疗精灵、战斗中的道具）时，骑着宠物的人物的总回复量按缺口水位拆给人和骑宠，骑宠的那份写回它自己的血量（血量回到 1 以上则清倒下标记）。`protocol.md` 10.7 的 `heal` 一行和 `docs/ride.md` §5 已补说明。没有新增请求或推送，战报的 `heal` 效果仍只写人物。
- 对 agent 的影响：只读战报的 agent 不用改；骑宠的血量变化在 `battle_round` 的 `state` 和战斗结束的 `pets_changed` 里看。

## 2026-10-21 · docs_rev b009458803ef

- 骑宠血量下发（STAGE-250）：骑乘的人物单位带 `mount` 对象（`name`、`hp_pct`，己方另有 `hp`、`max_hp`），没骑乘时省略；`battle_start`、`joined` 和每回合 `battle_round` 的 `state` 都带，骑宠血量随分摊当回合更新。`protocol.md` 10.2、10.7 与 1765 行附近的说明已改。没有新增请求或推送。
- 对 agent 的影响：只读战斗单位的 agent 可以忽略这个字段；要显示骑宠血量的，`mount` 缺失即表示没骑乘。

## 2026-10-21 · docs_rev f9241fd85ccc

- 新增消息压缩（STAGE-246）：`hello` 请求可以声明 `"compress":["gzip"]`，服务端在响应里确认 `"compress":"gzip"` 之后，超过 10 KiB 的消息改发成一行 `{"z":"gzip","d":"<base64>"}`。`protocol.md` 第 1 节写清压缩格式、解压后 1 MiB 上限，以及 64 KiB 单行上限按线上字节数计算；第 5.1 节 `hello` 补了 `compress` 字段。没有声明的连接（包括 `agent-connect.py`）收到的仍是原样 JSON，现有请求和推送都没有变化。
- 对 agent 的影响：不声明压缩就不用改；想接收压缩消息的，见 `agent-guide.md` 末尾的「消息压缩（STAGE-246）」一节。

## 2026-10-21 · docs_rev e43930df3bfd

- 合并记录（新手发马祖棍棒合入 develop 后的组合）：协议只在建角色一节补了出生礼包说明，没有新增请求或推送，变更见下面已登记的条目。

## 2026-10-21 · docs_rev fb9397a691ef

- 合并记录（STAGE-239 合入 develop，与 STAGE-228/229 魔抗系状态、STAGE-237 合并后的组合）：没有新增协议内容，变更见下面已登记的条目。

## 2026-10-21 · docs_rev b16d00b24988

- 魔抗系状态（STAGE-228 的魔法防御、火/电/冰之精灵，STAGE-229 登记）：单位的 `status` 新增 `magic_resist` 魔抗、`fire_resist` 火抗、`thunder_resist` 电抗、`ice_resist` 冰抗（10.2）；挂上和解除、互斥、回合末递减见 10.7。火、电、冰抗只占状态槽，不改数值。协议用例 `battle_magic_resist.jsonl` 登记在 10.13。客户端卡片标记为 魔、火、电、冰。

## 2026-10-21 · docs_rev 27a9f8c4bccd

- 合并记录（STAGE-237、STAGE-239 合入 STAGE-238 分支的组合）：`pet_talk` 的协议内容沿用 STAGE-239 的登记；STAGE-237 转生后删灵石设事件 71 只改服务端，不改协议。没有新增协议内容，各需求的变更见下面已登记的条目。

## 2026-10-21 · docs_rev 0c1492d1dfd8

- 合并记录（STAGE-239 合入 develop 后的组合）：`pet_talk` 的协议章节从 5.60 改为 5.61（5.60 已被 STAGE-236 宠物转生占用），没有其他新增内容，变更见下面已登记的条目。

## 2026-10-21 · docs_rev 1ecdab20b1f6

- 合并记录（STAGE-231、STAGE-236 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-21 · docs_rev c56dd0161df3

- 合并记录（STAGE-209、STAGE-230 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev 9a9cac9567a2

- 装备耐久（STAGE-209，原版“道具损坏”）：背包里有耐久的装备（最大耐久 > 0）带 `durability`、`max_durability`、`crush_level`（5.17 / 5.18）；战斗里挨打、出手打中时事件带 `crush` 提示（10.7）。耐久耗尽的装备在战斗结束后从背包删掉。客户端可以先忽略这些字段，界面显示另开需求。

## 2026-10-20 · docs_rev 770a6ffe9cbf

- `magic_use`（5.51，STAGE-230）：请求多了 `to`（`self` 自己、`pet` 宠物、`member` 队友），`target` 是对象栏位；不写 `to` 时的老写法不变（不带 `target` 是自己、带了是宠物，变幻的精灵照旧）。
- 战斗外的治愈、滋润、恩惠能回血，响应是 `heal`（`amount`、`to`，宠物还带 `target`）；复活、解状态、防御类扣 MP、没有效果，`heal` 为 `null`。
- 对象无效（宠物栏位没有宠物、队友）回复 `INVALID_TARGET`，这时 MP 已经扣了（照原版先扣后判），这是唯一一种“出错但扣 MP”的情况。
- 其他精灵战斗外的提示改成“这个精灵不能在战斗外使用”（原来的提示里写了“只做了变幻的精灵”）。

## 2026-10-20 · docs_rev 6f64dddadaed

- STAGE-236 宠物转生窗口（精灵王 `npc_transmigration`，新 NPC 类型「宠物转生」）：`talk` 按身上的玛蕾分流，`npc_window` 的选项走 PETTRANS 的步骤（下一步、是/否、选宠、确认），`npc_choose` 回应。协议见 `protocol.md` 5.60；没有新增请求类型，`agent-guide.md` 第 6 节不用改（`talk`、`npc_choose` 原有分级）。
- 玩家不用做任何事：宠物转生只改宠物（删玛蕾、转生被选中的宠物），名字和栏位不变。

## 2026-10-20 · docs_rev 272dbbd981a9

- 新增操作类请求 `pet_talk`（和宠物对话，`protocol.md` 5.61，STAGE-239）：请求 `slot`，回复宠物的台词 `text` 和个体等级上限是否被改写 `limit_changed`。
  只能对出战或待命的宠物说话；战斗中、宠物休息或骑乘中不能说（分别回复 `IN_BATTLE`、`WRONG_STATE`）。改了上限会推送 `pets_changed`，发了道具会推送 `inventory_changed`。
- `agent-guide.md` 第 6 节把 `pet_talk` 放在第二级（做完汇报）：它会永久改宠物的上限，也可能给道具。

## 2026-10-20 · docs_rev 8fd391ae2ecf

- 合并记录（STAGE-207、STAGE-218 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev 87dd612e7203

- 合并记录（STAGE-207 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev 76845a23b986

- 攻击精灵的作用范围与魔法闪避（STAGE-218，第 5.8、10.5、10.7 节）：`target_type` 不变（`enemy` 要选一个中心目标、`enemy_all` 不用选），但攻击精灵的实际命中可能不止一个，范围由原版 `attmagic.bin` 的 3×5 掩码决定（目标码 9 在选中的人周围取格、10 打选中敌人所在的一排、11 遍历敌方两排；熔岩爆发和暴风冰刃虽然不用选，也只打一排 5 个）。每个被打的目标先判一次魔法闪避，闪过时该目标的 `effect` 是 `dodge`、伤害为 0。没有新增请求或字段，客户端按 `effects` 显示实际目标即可；逐精灵对照见 `docs/magic-range.md`。

## 2026-10-20 · docs_rev 1049274b7706

- 合并记录（STAGE-194、STAGE-202、STAGE-196、STAGE-215、STAGE-216 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev f596128fc20e

- 宠物的成长率和五档素质（STAGE-216，第 5.19、5.20 节）：`pets` 响应和 `pets_changed` 推送里每只宠物新增两个字段，没有新请求，已有字段不变，多出的字段忽略即可。`growth`：成长率的依据 `basis`（`raised` 从 1 级起养、`caught` 捕获或获得时已有等级、`unknown` 历史宠物）、起始等级 `from_level`，以及 `rates`（`hp`、`attack`、`defense`、`agility` 每升一级的平均涨幅，两位小数；刚获得还没升过级、或历史宠物时四项都是 `null`）。`quality`：体力、腕力、耐久力、速度四项的素质点 `points` 和档位 `tier`（0 为满，正常范围 −4 到 0）。模板资质仍不发给客户端。

## 2026-10-20 · docs_rev a53091405410

- 合并记录（STAGE-194、STAGE-202、STAGE-196 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev 8666bb28ed23

- 合并记录（STAGE-194、STAGE-202 合入 develop 后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev b6bb0b51d616

- 召唤龙阵与精灵版宠物技能（STAGE-202，第 10.5、10.7 节，5.8 的 `effect` 与 `target_type`）：奔龙阵、强恩阵、奔雷阵、GM 烟火是新的精灵函数 `dragon`，有魔法闪避（`dodge`），不反击；强恩阵实际等同敌方全体（原版行为）。精灵版宠物技能（`attskill`）让人物用宠物技能，事件只有 `skill`，没有 `magic` 事件。没有新请求，客户端不用改协议处理，只多了 `magic.func` 的取值和 `dragon`、`attskill` 两种精灵的文字。

## 2026-10-20 · docs_rev faed99dcc85a

- 战斗失败的协议细节同上一条（STAGE-194，第 10.9 节 `lose` 的 `in_place`）；这一版补充了协议用例的说明（第 10 章用例列表：`battle_lose_inplace`、`battle_fly_encounter`），agent 的行为没有变化。

## 2026-10-20 · docs_rev 35ea65d4edb0

- 战斗失败分两种（STAGE-194，第 10.9 节 `lose`）：被打死（人物倒下但没被打飞）原地以 1 血复活，不传送，`penalty.respawn` 是当前位置，`penalty.in_place` 为 `true`；被打飞才送回记录点或出生地（省略 `in_place`，之后推送 `position_changed`）。
- 遇敌开关（第 5.29、5.30 节）：打输且被打飞传走才自动关闭（原因 `lose`）；原地复活时开关保持开着，隔一段时间开下一场。

## 2026-10-19 · docs_rev ef556b864343

- 合并记录（STAGE-196 同步 develop（STAGE-197、199、200、201、195、203 已在 develop）后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-20 · docs_rev 43d3b443a736

- 加工、料理会合成（STAGE-207）：`pet_skill` 的加工（200）、料理（201）不再回复 `SKILL_UNUSABLE`，改为合成。请求多一个 `items`（背包格号 5–19，2–15 件，加工和料理必填）；结果是 `created`（合成出产物）、`fallback`（匹配失败，还回一件材料）、`cooldown`（冷却中，还回一件材料）或 `illegal`（食物与道具混合，没有产物）（第 5.55 节）。不论哪种结果，参与的背包材料都会被扣掉；成功后推送 `inventory_changed`。规则见 `docs/synthesis.md`。

## 2026-10-19 · docs_rev 75bc97b3d1fe

- 合并记录（STAGE-203 合入 develop 后的组合（STAGE-195 已在 develop））：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-19 · docs_rev 6e6605453954

- 合并记录（STAGE-203 同步 develop（STAGE-197、199、200、201 已在 develop）后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-19 · docs_rev a3eaf39681e2

- 合并记录（STAGE-195 同步 develop（STAGE-197、199、200、201 已在 develop）后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-19 · docs_rev 5418f2721f1b

- 合并记录（STAGE-197 合入 develop 后的组合（STAGE-199、200、201 已在 develop））：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-19 · docs_rev 3f0b087eb21c

- 任务 NPC 收宠物遇到骑乘中的宠物（STAGE-197，第 5.20 节、`docs/ride.md`）：按原版先自动下骑再收走，不回复 `PET_RIDING`；这是 `PET_RIDING` 规则的唯一例外，其余请求（放生、寄放、卖掉、改状态）仍然拒绝骑乘中的宠物。

## 2026-10-19 · docs_rev cb5f276c53f9

- 任务 NPC 的交易可能收走或送出宠物（STAGE-197，第 5.20 节 `pets_changed`）：交易成功后推 `pets_changed`，消息栏的 `notice` 每只宠物一条（“交出XX。”“拿到XX。”）。收不成（数量不够、战斗中）时什么都不推；宠物栏满时只推 `PetFullMsg` 台词。没有新请求，客户端不用改。

## 2026-10-19 · docs_rev af34255549f4

- 合并记录（STAGE-201 同步 develop（STAGE-199、200 已在 develop）后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-19 · docs_rev 34480283ae42

- 合并记录（STAGE-200 同步 develop（STAGE-199 已在 develop）后的组合）：没有新增内容，各需求的变更见下面已登记的条目。

## 2026-10-19 · docs_rev 2efb3ba652b0

- 防御精灵（STAGE-199）：光、镜子、守护（`defend`）与复活光、复活镜、复活守护（`revive_defend`）。目标类型分别是 `ally`（缺省落在自己身上）和 `ally_down`（第 5.8 节、第 10.5 节）。防御次数标记 `absorb`（吸收）、`reflect`（反射）、`vanish`（无效）进入单位的 `status`（第 10.2 节），次数用完去掉；结算结果 `result` 新增 `absorb`（被打者回血）、`reflect`（伤害落在攻击者身上，`intent` 指回弹开它的一方）、`vanish`（这一下伤害为 0），只有物理直接攻击才会触发，精灵伤害和中毒不触发（第 10.7 节）。

## 2026-10-19 · docs_rev f1b4d6e4f347

- 合并记录（STAGE-200 的协议用例 `battle_magic_field.jsonl` 登记进文档，内容无新增）：场地属性与属性反转的字段、事件和目标类型已在下面两条登记（第 10.2、10.4、10.5、10.7、10.11 节）。

## 2026-10-19 · docs_rev 3e6cdb0cf841

- 场地属性与属性反转（STAGE-200）：`battle_start`、`battle_turn` 和 `battle` 查询在场地有属性时带 `field: {attr, pow, left}`（无属性时省略）；大地、水、火炎、疾风、调和的精灵施放后发 `field` 事件（`attr` 为 `none` 表示无属性），回合末场地恢复也发一个。彩虹、极光的效果是 `none`，标志用 `status_add` / `status_remove` 带上 `reversed`（单位 `status` 的新取值，第 10.2、10.5、10.7 节）。

## 2026-10-19 · docs_rev 03032d022fb4

- 目标类型新增 `any`（STAGE-200）：己方或敌方任意一个活着的单位，必须带 `target`，给倒下的或空格子回复 `INVALID_TARGET`。只有属性反转精灵（彩虹、极光）是这个取值（第 10.5 节“目标类型”）。场地属性精灵是 `none`，不用选。

## 2026-10-19 · docs_rev 074fd91d171d

- 精灵效果文字的状态名加上虚弱、魔障、沉默（STAGE-201，第 5.8 节 `magic[].effect`）：状态精灵和解状态精灵的文字照常写，状态名与 10.2 的 `weaken`、`barrier`、`nocast` 对应。上一条登记的其他内容不变。

## 2026-10-19 · docs_rev 17fcc02dc5bd

- 状态精灵新增魔障、沉默、虚弱（STAGE-201，原版规则）：单位的 `status` 新增 `weaken`（虚）、`barrier`（障）、`nocast`（默）。魔障让单位不能行动（`skip` 的 `reason` 为 `barrier`），也不替主人挡刀；沉默让人物不能交 `magic` 指令（回复 `MAGIC_UNUSABLE`），宠物不中；虚弱和魔障挂上后回合末冻结（不会自己消失，只能靠净化精灵解，第 10.7 节）。虚弱的说明文字写了防御、敏捷降 20%，原版源码没有实现，这里也不做。净化魔障、沉默、虚弱（404–409、411）能解掉对应的状态（第 10.5 节）。

## 2026-10-19 · docs_rev 75e30f28e14a

- 变幻的精灵（STAGE-203）：新请求 `magic_use`（第 5.51 节），战斗外使用精灵。目前只有变幻的精灵能在战斗外用：`target` 是宠物栏位，变身成那只宠物；不带 `target` 是变回自己。变身只改外观，`status.metamo`（第 5.8 节）记录宠物名、图像编号和到期时间；到期、卸下身体装备（装备格 1）自动变回；变身中不能穿远距离武器。战斗中 ``magic`指令`仍回复 `MAGIC_UNUSABLE`，`magic_use` 回复 `IN_BATTLE`。

## 2026-10-19 · docs_rev b9b1f2edc11c

- 分身队带自动战斗方案（STAGE-195）：`party_clone` 可以带 `auto_profile`（客户端的自动战斗方案，外加 `"version": 1`），响应多一个 `auto_profile`：`applied`（收下，分身按方案出招）或 `ignored`（版本不对、字段不合法，不报错，分身走默认的普通攻击）；不带字段时响应里没有这个字段。方案随队伍存在内存里，解散就丢，之后改方案不会自动重推（第 5.57 节）。
- 分身按方案出招的规则写在第 5.57 节的“战斗”段：目标（合击顺序 / 随机）、自动捕捉回避、人物补血（精灵）、首回合、常规动作（精灵、宠物技能）、不做的部分。

## 2026-10-19 · docs_rev ee0018df4833

- 宠物 `transmigration` 字段（STAGE-196）：`pets` 列表和 `pets_changed` 里每只宠物带 `transmigration`（转生次数，新宠物和旧宠物都是 0，第 5.19 节）。骑乘检查 `RIDE_TRANS_LIMIT` 生效：转生次数超过 2 次的宠物上骑被拒（第 5.47 节第 8 条）。宠物转生本身还没做，次数现在只能由测试用的管理命令 `stoneage-admin setpettrans` 设定。

## 2026-10-18 · docs_rev 71b527915929

- 合并记录（STAGE-192 合入 develop 后的组合修订号），内容无新增：商店买入按空位截断（STAGE-192）已在下面一条登记。

## 2026-10-18 · docs_rev 7078554d1606

- 商店买入按空位截断（STAGE-192，原版行为）：背包空格比买的数量少时，买得下几件就买几件，不再整单回复 `BAG_FULL`。报价时 `trade.count`、`total` 就是截断后的数，窗口正文写明“背包只剩 N 格，按空位买 N 件”；一格都没有时选数量那一步回复 `BAG_FULL`（第 5.38 节）。

## 2026-10-18 · docs_rev 25f4e02fa700

- 合并记录（STAGE-189 与 STAGE-190 合入 develop 后的组合修订号），内容无新增：战斗中加点、解散队伍、组分身队（STAGE-189）与装备精灵计入 `status.magic`（STAGE-190）都已在下面两条登记。

## 2026-10-18 · docs_rev 49c355f3d4bb

- 装备自带的精灵计入 `status.magic`（STAGE-190）：身上穿的装备带精灵（效果文字里的「滋润的精灵 Lv1」这类）时，同号装备位（0–4）就是一个精灵栏位，战斗里可用 `magic` 指令按这个 `slot` 施放；`mp` 以装备上的为准。穿脱装备后服务端会推送新的 `magic`（第 5.8 节 `status.magic`）。

## 2026-10-18 · docs_rev 0b65edba88e4

- 战斗中也能解散队伍、组成分身队（STAGE-189，接上一条的加点）：`party_clone`、`party_disband` 在战斗中照常成功，不再回复 `IN_BATTLE`。正在打的这一场分身打完为止（不会中途从战场上拿掉），战后按结算后的队长重建（等级、属性跟着队长），新分身从下一场起上场（第 5.57 节）。
- 遇敌开着时组队不再需要先关遇敌（STAGE-187 的做法取消）：连场的下一场也不挡组队和解散。上一条“`party_clone`、`party_disband` 在战斗中仍回复 `IN_BATTLE`”已不再成立。

## 2026-10-18 · docs_rev 9642b673e3a5

- 战斗中可以加点（STAGE-189）：`addpoint` 不再回复 `IN_BATTLE`。开着无限遇敌连场打时，不用先关遇敌，升级后直接加点即可。加的点从下一场起生效（当前这场的战斗数值是开战时的快照）；战斗中推送的 `status_changed` 里 `hp`、`mp` 是战斗里的实时值（第 5.21、10.1 节）。
- `party_clone`、`party_disband` 在战斗中仍回复 `IN_BATTLE`，要重组分身队还是先关遇敌（第 5.57 节）。

## 2026-10-18 · docs_rev 8b603182ff03

- 遇敌开着时组分身队（STAGE-187）：`party_clone`、`party_disband` 在遇敌开关开着时回复 `IN_BATTLE`。先发 `encounter` `{"set": "off"}` 关掉遇敌，再组队（第 5.57 节）。
- 分身队战斗可以指定攻击目标（STAGE-186）：`battle_turn.optional` 列出可以指定的分身，`battle_command` 的 `actor` 为 `clone:N` 或 `clone_pet:N`，只能 `attack`（第 10.4、10.5 节）。
- `hello` 响应新增 `docs_rev`（协议和手册的修订号）和 `server_version`（服务端版本）。协议版本号仍是 1，多出的字段忽略即可（第 5.1 节）。
- agent 每次登录（`hello` 之后）都要比对 `docs_rev`：和上次记下的不一致，就先读这里新增的条目，再继续行动（手册第 2 节第 6 条）。

## 2026-10-18 · docs_rev 5d158b89e805

- 分身队战斗可以指定攻击目标（STAGE-186）：`battle_turn.optional` 与 `battle_command` 的 `clone:N` / `clone_pet:N`（第 10.4、10.5 节）。
- `hello` 响应新增 `docs_rev` 和 `server_version`（第 5.1 节）。agent 每次登录比对 `docs_rev`，不一致先读本记录（手册第 2 节第 6 条）。
- 参考脚本 `agent-connect.py` 记住上次的 `docs_rev`，变了就先输出 `script_docs_updated` 事件，列出新增的条目，然后才是 `hello` 的响应。

