# STAGE-275 调研：真人队伍同场战斗（对照原版 2.5）

> 只读调研，未改任何代码。原版源码：`original-data/gmsv/`（下文简写 `gmsv/`）。
> 行号是 GBK 源文件里 `grep -n` 的行号（读取时用 `iconv -f GBK -t UTF-8`）。文中标“约”的行号是按分段读取估计的，**动手前请以 grep 为准**。
> 设计依据：`docs/social-players.md` 第 2.6 节。文件为 `docs/social-players.md`。
> 置信度标注：**已查实**＝读过对应代码；**未查实**＝没读到或读到的结构拿不准，原文写明。

## 0. 编译开关与常量（决定上面哪些分支生效）

- 已启用（`gmsv/include/version.h`）：`_BATTLECOMMAND_TIME`（出手超时）、`_BATTLE_GOLD`（战后金钱）、`_GET_BATTLE_EXP`（经验倍率）、`_ITEM_ADDEQUIPEXP`（装备 EXPUP）、`_ITEM_ADDEXP`、`_PET_TALK`（结算顺序从 0 到 9 升序）、`_PET_LIMITLEVEL`。
- 启用了 `_BATTLE_PK` 与 `_AUTO_PK`，只影响 PvP，本需求不涉及。
- `gmsv/include/battle.h`：`BATTLE_ENTRY_MAX 10`（每方 10 格）、`BATTLE_PLAYER_MAX 5`（玩家格 0–4）、`SIDE_OFFSET 10`（敌方编号 = 10+格号）、`BATTLE_TIME_LIMIT (60*60)`（整场战斗 3600 秒上限，行 10）。
- `gmsv/include/char_base.h`：`CHAR_PARTYMAX 5`（队伍 5 人含队长）。
- 两种战斗都是 `BATTLE_TYPE_P_vs_E`（玩家 vs 敌群），本需求只看这一类。

## 1. 经验分配

**结论（已查实）：经验不分摊、不按人头平分。每次“出手造成击杀”时，只有那一次出手的角色（攻击方，若是组合技则为组合成员）拿经验，每人拿到的是该敌人的完整经验（按等级差折算后）。队伍里没出手的队员拿 0。**

机制：
1. 每次出手结束调用 `BATTLE_AddProfit(battleindex, aAttackList)`，`aAttackList` 只含本次出手者：
   - 普通攻击：`aAttackList = {attackNo}`（`battle.c:5981`，调用点 `6471`、`6506`；反击 `6506` 也是单人）。
   - 组合技：`aAttackList` = 同一 combo 组内所有存活成员（`battle.c:7263-7291`）。
   - 魔法等：`battle_magic.c:1244-1247` 同样单人列表。
2. `BATTLE_AddProfit`（`battle.c:4882`）在非对战（`dpbattle` 为 0）时调用 `BATTLE_AddExpItem`（`4657`）。
3. `BATTLE_AddExpItem` 对本次新倒下的每个敌人（`4657` 起）：
   - 只在“玩家方出手、对方是敌群”时（`proflg==1 && side!=j`，约 `4700-4746`）发经验和战利品；
   - 对列表中每个出手者（`for k`，约 `4760-4770`）：`CHAR_WORKGETEXP += nowexp`；
   - 敌人经验记完即清零：`CHAR_setMaxExp(enemyindex,0)`（`battle.c:4805`），同一敌人不会被二次计经验。
4. 等级折算（`battle.c` 约 `4755-4768`，`EXPGET_MAXLEVEL 5`、`EXPGET_DIV 15`，行 4740 附近的 `#define`）：
   - `d = 出手者等级 - 敌人等级`；`d <= 5`：全额 `exp`；
   - `d > 5`：`b = min(15, 20 - d)`；`b <= 0` 则 1，否则 `exp*b/15`；最低 1。
   - 即差 6 级约 14/15，差 20 级及以上只有 1 点。
5. 战后结算 `BATTLE_GetExp`（`battle.c:2880`，启用 `_ITEM_ADDEQUIPEXP` 的那一版）：
   - 玩家：`addexp = getexp + getexp*modexp/100`（修饰来自道具 ADDEXP，`2892`）；
   - 乘 `getBattleexp()`（`2915`，经验倍率）；装备 EXPUP 按“人/宠/骑”加成（`2920-2945`）；
   - 满级为 0（`2956`）；最后 `CHAR_AddMaxExp`。
6. 战后结算入口 `BATTLE_GetExpGold`（`battle.c:3103`）：死亡的玩家直接返回 0（`3122`），**不发经验、不发战利品、不发金钱**；存活玩家调用一次 `BATTLE_GetExp`（`3134`）。

**宠物经验（已查实的部分）**
- 宠物有自己的 `WORKGETEXP`，击杀时它若是出手者就拿到（`4769` 同一条路径）。
- 骑宠：出手者骑着宠时，骑宠额外拿 **60%**（`battle.c:4776-4795`，`nowexp *= 0.6`，`4790`）。
- 宠物在战后经 `BATTLE_GetExpGold` 的宠物循环（`3189-3200`）结算；循环内的 `BATTLE_GetExp(petindex, charaindex)` 调用包在 `if(WORKGETEXP<=0)` 的条件里，**大括号结构未查实**（`3192` 的 `if` 与 `3200` 的调用之间的 `#endif` 与括号关系没完全看清），因此“宠物经验是否被结算两次”未查实。

**队伍成员之间的差别只在“谁出手”。** 同一敌人被 5 个人依次打死，只有最后那次出手的人拿到该敌人经验；被组合技打死则组合成员各拿一份完整经验。

## 2. 战利品（掉落）

**结论（已查实）：战利品不是每人独立掷，也不是全队共享。敌人身上的每件道具，在它被击倒的那一刻，随机分给本次出手的“列表”中的一个人。**

- 敌人道具在 `CHAR_STARTITEMARRAY` 起的 10 格（`battle.c:4672-4690` 附近）。
- 分配：`k = RAND(0, allnum-1)`（`battle.c:4726`），`allnum` 是本次出手列表的人数（同一出手列表，见第 1 节）。
- 落到该人的 `getitem[3]` 槽位（`4727-4735`）；满了则随机替换一格或直接销毁（`4737-4753`）。
- 结算时（`BATTLE_GetExpGold` 的道具段，`battle.c` 约 `3250-3300`）：只有存活的玩家能拿到自己 `getitem` 里的道具；死亡玩家的 `getitem` 不会被发放（因为 `3122` 直接返回）。**死亡玩家身上暂存的战利品是否回收，未查实。**
- 宠物的战利品落到宠物的主人那条 entry（`BATTLE_AddExpItem` 开头为出手列表建立 `pEntryPlayer`：宠物格取 `subnum-5` 即主人格，约 `4680-4700`）。

## 3. 逃跑判定

**结论（已查实）：逃跑是个人行为，只让逃跑的那一个人（及其宠物）退出战斗，队伍里其他人照常战斗；成功率公式没有队伍人数或队伍因素。**

- 指令入口：`battle_command.c:101-106`（`"E"` → `CHAR_WORKBATTLECOM1 = BATTLE_COM_ESCAPE`，扣 20 MP）。
- 执行：`battle.c:6683` 调用 `BATTLE_Escape(battleindex, attackNo, 0)`；强制离开（超时踢出用）`6692` 传 `flag=1`。宠物不能逃（`6682` 的类型判断）。
- `BATTLE_Escape`（`battle_event.c:3569`）：
  - 失败：仅 `Entry[].escape++`（同一格的逃跑次数累计，`3580`）。
  - 成功：`CHAR_setWorkInt(...BATTLE_CHARMODE_FINAL)`，`CHAR_DischargePartyNoMsg(attackindex)`（`3613`），`BATTLE_Exit(attackindex)`（`3614`）。
- 成功率（`BATTLE_EscapeCheck`，`battle_event.c:3480`）：
  - 对战类型为 P_vs_P 直接成功（`3486` 附近）；
  - 幸运 `luck = clamp(WORKFIXLUCK, 1, 5)`（敌人用稀有度映射，`3495-3505`）；
  - `enemylevel` = **所有还留在敌方格里的敌人**的平均等级（`ABIO` 标记的 -100 修正，`3510-3525`），注意这里没有过滤已倒下的敌人；
  - `escape_cnt = Entry[].escape + 1`（本次是这个格子的第几次逃跑）；
  - `Esc = 基数(luck) * escape_cnt - 2*(平均敌等级 - 自身等级)`，基数：luck≥5 为 95，4 为 60，3 为 50，2 为 40，1 为 30（`3539-3553`）；
  - `Esc < 1` 时取 1；`RAND(1,100) < Esc` 成功（`3561`）。
- 结论：队伍人数不影响；只影响“同一个人连续逃跑”的概率（次数越多越容易成功）和敌人平均等级。

**逃跑与队伍的副作用（已查实）**：逃跑成功调用 `CHAR_DischargePartyNoMsg`（`3613`）。原版 `CHAR_DischargePartySub`（`char/char_party.c:331`）：**若逃跑者是队长，整个队伍解散（队长分支遍历全部队员，`335` 起）；若是队员，只离开队伍（`CLIENT` 分支）。** 也就是说，队长战斗中逃跑会解散队伍，其他队员留在战斗里但已不在队内。

## 4. 回合内指令：是否等全员、超时怎么办

**是否等全员（已查实）**：`BATTLE_CommandWait`（`battle.c:2805`）按格遍历己方，只要还有存活玩家处于 `BATTLE_CHARMODE_C_WAIT`（未下指令），本回合就不推进（`2826-2861` 中 `iRet = FALSE`）。`INIT`、`RESCUE`、`WATCHINIT` 不阻塞；`C_OK` 计入 `BeOk`。`BATTLE_Command`（`battle.c:3797`，`3805-3808`）双方都满足才执行回合。

**超时（已查实）**：
- 第一条指令下达后启动 120 秒计时：`battle.c:2869-2870`（`PartTime = time(NULL)+120`）。
- 计时到期（`2841` 的 `TimeOut`）时，仍处于 `C_WAIT` 的玩家被**强制退出战斗**，而不是执行默认指令：
  - 发送“超过时间未下指令，强迫离开战斗！”（`2839`）；
  - `CHAR_DischargeParty(charaindex, 0)`（`2847`，注释写作“解散团队”）；
  - 设为 `BATTLE_COM_COMPELESCAPE` 并 `BATTLE_Exit`（`2851`）；给客户端发 `BU`（`2853`）。
  - 注意：`CHAR_DischargeParty` 对队长是整队解散，对队员只是离队（见第 3 节）。所以“超时的人如果是队长，全队解散”。
- 整场战斗时限（`BATTLE_TimeOutCheck`，`battle.c:3717`，`BATTLE_TIME_LIMIT=3600s`，`include/battle.h:10`）：所有仍 `C_WAIT` 的玩家被 `BATTLE_Exit` 并 `CHAR_DischargePartyNoMsg`（`3740-3741`），发“时间到，结束战斗”。
- **原版没有“默认指令”**（没有自动防御/自动攻击）。设计文档 2.6 写的“超时的按已有的超时默认指令处理”与原版不符，见第 8 节。

## 5. 战斗中掉线

**普通掉线（已查实）**：下线清理函数（`char/char.c:1488-1512`）对战斗中的角色：
1. `battleindex >= 0` 时调用 `BATTLE_EscapeDpSend`（只对 PvP 生效，`battle.c:4426` 起）；
2. `BATTLE_Exit(charindex, battleindex)`（`char.c:1505`）：该角色和他的出战宠物（`_BATTLE_Exit` 中 `pEntry[i+5]`，`battle.c:1072` 起）立即退场；**不调用 `BATTLE_GetProfit`，因此不结算经验、战利品、金钱**；已累计的 `WORKGETEXP` 随之丢弃；
3. 之后 `CHAR_dropItemAtLogout`，再 `CHAR_DischargeParty(charindex,0)`（`char.c` 约 `1507`，即队伍规则同第 3 节：队长掉线 = **整队解散**）。
- 战斗中的其他人继续战斗（战斗本身不结束，除非己方没有存活玩家）。
- 血量：`_BATTLE_Exit` 对存活角色保留当前 HP；对死亡角色清除 ISDIE 并把 HP 设为 1（`battle.c:1124-1131` 附近），即死亡的人退场后是 1 血。

**队长掉线（已查实）**：没有“顺位接任”。队长 `CHAR_DischargeParty` 直接解散全队（`char_party.c` 的队长分支）。战斗中队员的队伍关系随之消失，但战斗实体（格位）不立即被移除，只是不再属于队伍。“是否影响战斗中其余队员的格位”未单独查实，按代码推断：格位不变，只是不再是队友。

**设计对比**：设计 2.6 写“战斗中成员掉线 = `BATTLE_Exit`，不给奖励、不判负、血量照存”，**与原版一致**；但“队伍随即在战斗结束后移除该成员”和“队长掉线则战斗结束后顺位接任”与原版不符（见第 8 节）。

## 6. 捕捉到的宠物归谁

**结论（已查实）：归“出手捕捉”的那名成员。** `BATTLE_Capture`（`battle_event.c:3336`）中 `attackindex = BATTLE_No2Index(battleindex, attackNo)` 即出手者，宠物由 `PET_createPetFromCharaIndex(attackindex, defindex)` 创建（`battle_event.c:3372`，`char/pet.c:307`）；`LogPet` 也记在出手者名下。捕捉指令入口 `battle_command.c:213`（`BATTLE_COM_CAPTURE`），执行 `battle.c:6652-6657`。捕捉前还有道具检查（`BATTLE_CaptureItemCheck`）和成功率（`BATTLE_CaptureCheck`，`battle_event.c:3091`，未展开细节）。

## 7. 我方站位

**结论（已查实，与设计一致）：**
- 玩家格：`BATTLE_NewEntry`（`battle.c:945`）对 `CHAR_TYPEPLAYER` 取 `iEntryFirst=0`、`iEntryMax=BATTLE_PLAYER_MAX(5)`（`975`），即玩家占 0–4。
- 队伍入场顺序：`BATTLE_PartyNewEntry`（`battle.c:1574`）先入队长（`1579`），再按 `CHAR_WORKPARTYINDEX1` 的顺序入 1–4 号队员（`1592-1596`），已在战斗中的队员（`WORKBATTLEMODE != 0`）跳过。
- 宠物格：`CHAR_TYPEPET`（`battle.c:985-994`）的格位 = 主人格位 + 5（`work += 5`），即队员 k 的宠物在 k+5；每人最多一只出战宠。
- 实际调用：`BATTLE_CreateVsEnemy`（`battle.c:2078`）中队伍入场是 `battle.c:2182`（`2114` 那一处被注释掉了）。
- 原版“队伍入场”只发生在遇敌建战时，中途不能加入（`BATTLE_CreateVsEnemy` 对已在战斗中的人报“二重遭遇”，`2086`）。

**注意**：`BATTLE_NewEntry` 的 `iEntryMax` 为 5，满了返回错误并中断队员入场（`1596`）。所以队伍第 6 人不可能进入，但我们的队伍上限是 5，不会触发。

## 8. 与设计 2.6 的出入：建议调整

1. **经验分配（设计写“每人按参战情况结算，细节沿用原版”）**：原版是“出手者拿完整经验，不分摊；组合技组内每人一份完整经验”。建议写入协议：经验仅计给本次出手（含组合）的角色，队员不出手就没有经验；队伍不因人数增加或减少单个敌人的经验。
2. **战利品（设计没写细节）**：原版是“每件道具随机分给本次出手的一个人”。建议：只在“这次击杀的出手列表”里随机分配；死亡的人拿不到已分得的道具，其道具的处理（回收或丢弃）需要定。
3. **超时（设计写“按已有超时默认指令处理”）**：原版没有默认指令，超时的人直接被强制退出战斗（`COMPELESCAPE`），且会按 `CHAR_DischargeParty` 解散队伍（队长超时 = 全队解散）。建议：要么照搬（超时踢出，队长超时解散），要么明确定义默认指令（如防御）；二者不能混用。第一条指令后 120 秒计时、整场 3600 秒上限建议一并写进文档。
4. **逃跑（设计写“沿用原版”，但 2.1 写“战斗中不能离队”）**：原版个人逃跑成功就退出战斗，并且通过 `CHAR_DischargePartyNoMsg` 把自己移出队伍（队长则解散全队）。这与设计 2.1“战斗中不能离队 / 解散 / 踢人（`PARTY_IN_BATTLE`）”冲突。建议二选一：(a) 允许战斗中逃跑，成功即离队（队长则解散），或 (b) 逃跑也受 `PARTY_IN_BATTLE` 约束，即战斗中禁止逃跑。原版公式可直接采用，只是没有队伍因素。
5. **掉线（设计写“队长掉线则战斗结束后顺位接任”）**：原版队长掉线直接解散整队（`char.c` 的 `CHAR_DischargeParty`），没有顺位接任。建议改为“与原版一致：队长掉线即解散”，或在设计里明确这是新规则并说明理由。设计 2.3 的“顺位接任”只适用于离队（不在战斗中）。
6. **掉线的队员的队伍归属**：设计写“队伍随即在战斗结束后移除该成员”；原版是立即离队（`CHAR_DischargeParty(charindex,0)` 在 `BATTLE_Exit` 后同步执行）。建议改为即时。
7. **整场失败判定（设计未写）**：原版的“一方战败” = 该方没有任何存活的**非宠物**玩家（`BATTLE_OnlyRescue` 返回 0，`battle.c:3663-3716`，`3848-3855`）。宠物再多也不算。建议在设计里明写。
8. **强制离开的副作用（设计未写）**：超时踢出、逃跑、掉线都会走 `BATTLE_Exit`，该玩家的宠物也一起退出（`battle.c:1150` 附近）。建议写明“宠物随主人退场”。
9. **死亡与退场的血量（设计未写）**：原版死亡角色战后 HP=1、不再是倒地状态。建议确认我们是否沿用。
10. **中途不能加入（设计已写，一致）**：保持。
11. **单位号（设计已写，一致）**：队员 0–4，宠物 5–9（宠物 = 主人格位 + 5）。建议把“宠物号 = 对应队员号 + 5”写明，而不是只写“5–9”。

## 未查实清单

- `BATTLE_GetExpGold` 宠物循环（`battle.c:3189-3200`）的大括号结构：宠物经验是否会因 `_ITEM_ADDEQUIPEXP` 分支被结算两次。需要对照 `battle.c` 第 3189 行起逐行看括号。
- 状态致死（中毒等）的 `BATTLE_AddProfit`（`battle.c:7446`）时 `aAttackList` 的内容（是否沿用上一次的出手者）：未查实，可能导致状态致死的敌人没有经验。
- `BATTLE_CaptureCheck`（`battle_event.c:3091`）的成功率细节：未展开。
- 死亡玩家的 `getitem` 是否在退场时回收：未查实。
- 原版 `BATTLE_Exit` 是否在队员全部倒下后对宠物做额外处理：未查实。
- 战斗 NPC 对话（`BATTLE_CreateVsEnemy` 的 `npcindex` 分支）是否同样让队伍入场：`battle.c:2182` 对所有 `BATTLE_CreateVsEnemy` 调用都执行，结论按此推断，未逐一核对调用方。
- 敌群数量是否随队伍人数变化：`BATTLE_CreateVsEnemy` 中敌群由遇敌表（`EnemyList`）决定，未见按人数缩放的逻辑；但遇敌表本身（`char/` 或 `map/` 下的遇敌生成）未读。

## 证据位置速查

| 主题 | 文件:行 |
|---|---|
| 经验结算入口 | `battle/battle.c:2880`（BATTLE_GetExp）、`3103`（BATTLE_GetExpGold）、`3330`（BATTLE_GetProfit） |
| 击杀分配（经验/战利品） | `battle/battle.c:4657-4822`（BATTLE_AddExpItem），战利品 `4726`，经验 `4765-4770`，清零 `4805` |
| 出手列表 | `battle/battle.c:5981`、`6471`、`6506`、`7263-7301`、`7446` |
| 结算与退场 | `battle/battle.c:3348-3495`（BATTLE_Finish，`3478-3479`），`1072`（_BATTLE_Exit） |
| 指令等待与超时 | `battle/battle.c:2805`（BATTLE_CommandWait），`2839-2853`（强制退出），`2869-2870`（120 秒），`3717`（整场时限） |
| 整场推进与胜负 | `battle/battle.c:3797`（BATTLE_Command），`3663`（BATTLE_OnlyRescue） |
| 逃跑 | `battle/battle_command.c:101-106`，`battle/battle.c:6683-6692`，`battle/battle_event.c:3480`（EscapeCheck）、`3569`（Escape）、`3613`（离队） |
| 入场与站位 | `battle/battle.c:945`（NewEntry）、`975`、`994`，`1574`（PartyNewEntry），`2078`（CreateVsEnemy），`2182`（队伍入场） |
| 捕捉 | `battle/battle_command.c:213`，`battle/battle.c:6652-6657`，`battle/battle_event.c:3336`，`3372` |
| 下线 | `char/char.c:1488-1512` |
| 队伍解散 | `char/char_party.c:331`（CHAR_DischargePartySub），`536`（CHAR_DischargeParty），`541`（NoMsg） |
| 常量 | `include/battle.h:10`（TIME_LIMIT）、`BATTLE_ENTRY_MAX`、`BATTLE_PLAYER_MAX`；`include/char_base.h`（CHAR_PARTYMAX） |
