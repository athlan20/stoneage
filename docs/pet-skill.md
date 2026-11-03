# 宠物技能清点（STAGE-144）

战斗外使用宠物技能（协议 5.50 `pet_skill`）之前，先把 `petskill.txt` 里所有技能按能用的场合分类。本文是清点的结论：哪些能在战斗外用、各自的目标和原版数值，以及这一单做什么、不做什么。

数据来源：`original-data/gmsv/data/petskill.txt`，共 32 条。新服务端 `server/gamedata/petskill.tsv` 与之一致（场合和目标码已逐条核对）。

## 判断依据

- **场合**（`petskill.txt` 第 8 列，原版 `PETSKILL_FIELD`，`include/pet_skill.h` 的 `PETSKILL_FIELDTYPE`）：0 战斗和战斗外都能用，1 只能在战斗中用，2 只能在战斗外用。
- **用法位**（第 10 列，原版 `PETSKILL_USETYPE`）：`battle/pet_skill.c` 的 `PETSKILL_Use` 对带“战斗”位（0x02）的技能，要求主人处于战斗中，否则返回失败。两列的结论一致：30 条场合为 1 的用法位为 2，加工、料理场合为 2 的用法位为 1。
- 原版 `lssproto_PS_recv`（`gmsv/callfromcli.c`）收到客户端的宠物技能请求直接调用 `PETSKILL_Use`，所以“战斗外能不能用”只看这两列。
- **学习价格**（第 11 列，原版 `PETSKILL_COST`）：`npc/npc_petskillshop.c` 用它作为宠物技能店的收费。它不是使用技能时的消耗，使用时不扣东西。
- **目标**（第 9 列，原版 `PETSKILL_TARGETTYPE`）：换算成协议的 `target_type` 见 `server/gamedata/target.go` 的 `BattleTarget`；场合不是战斗的技能一律是 `none`。

## 清点结果

| 分类 | 条数 | 技能 |
| --- | --- | --- |
| 战斗中可用（场合 1） | 30 | 待机、攻击、防御等，见下表 |
| 战斗外可用（场合 2） | 2 | 加工（200）、料理（201） |
| 都可用（场合 0） | 0 | 原版数据里没有 |

### 战斗外可用的技能

- **加工**（200，`PETSKILL_Merge`）：原版调用 `ITEM_mergeItem`，把背包里的材料按配方合成一件物品，合成的结果有随机成分，宠物的属性会影响合成（`ITEM_merge_getPetFix`）。**消耗**是合成用掉的背包材料，不是技能本身。目标：无。学习价格 1500。
- **料理**（201，`PETSKILL_Merge`）：同一个合成函数。原版 `ITEM_mergeItem` 看第一件材料是不是食物（`ITEM_DISH`），是的话换成食物的随机表，并且不许食物和道具混在一起合。消耗同上。目标：无。学习价格 1000。

两者都依赖合成功能。STAGE-207 起服务端实现了合成（`pet_skill` 的 `items` 参数，协议 5.55；规则见 `docs/synthesis.md`），处理决定见下面的表。

### 全部 32 条

| 编号 | 名字 | 函数 | 场合 | 协议目标 `target_type` | 学习价格 |
| --- | --- | --- | --- | --- | --- |
| 0 | 待机 | `PETSKILL_None` | 战斗中 | `none` | 500 |
| 1 | 攻击 | `PETSKILL_NormalAttack` | 战斗中 | `enemy` | 1000 |
| 2 | 防御 | `PETSKILL_NormalGuard` | 战斗中 | `none` | 1000 |
| 3 | 破除防御 | `PETSKILL_GuardBreak` | 战斗中 | `enemy` | 1500 |
| 10 | 连续攻击 | `PETSKILL_ContinuationAttack（参数 2）` | 战斗中 | `enemy` | 2000 |
| 11 | 三段攻击 | `PETSKILL_ContinuationAttack（参数 3）` | 战斗中 | `enemy` | 5000 |
| 12 | T四段攻击 | `PETSKILL_ContinuationAttack（参数 4）` | 战斗中 | `enemy` | 15000 |
| 13 | T五段攻击 | `PETSKILL_ContinuationAttack（参数 5）` | 战斗中 | `enemy` | 25000 |
| 20 | 忠犬 | `PETSKILL_Guardian（参数 攻%-20  COM:攻击）` | 战斗中 | `enemy` | 2500 |
| 30 | 突击 | `PETSKILL_ChargeAttack（参数 1 攻%+90）` | 战斗中 | `enemy` | 4000 |
| 31 | 双重突击 | `PETSKILL_ChargeAttack（参数 2 攻%+110）` | 战斗中 | `enemy` | 8000 |
| 40 | 一击必杀 | `PETSKILL_Mighty（参数 倍2 回避30）` | 战斗中 | `enemy` | 2500 |
| 41 | T一击必杀改 | `PETSKILL_Mighty（参数 倍3 回避15）` | 战斗中 | `enemy` | 25000 |
| 50 | 背水之战其之１ | `PETSKILL_PowerBalance（参数 攻%+25 防%-35）` | 战斗中 | `enemy` | 2000 |
| 51 | 背水之战其之２ | `PETSKILL_PowerBalance（参数 攻%+45 防%-55）` | 战斗中 | `enemy` | 2500 |
| 52 | T背水之战之其３ | `PETSKILL_PowerBalance（参数 攻%+80 防%-50）` | 战斗中 | `enemy` | 30000 |
| 60 | 毒攻击 | `PETSKILL_StatusChange（参数 毒 turn 3  攻%-30）` | 战斗中 | `enemy` | 2500 |
| 61 | 猛毒攻击 | `PETSKILL_StatusChange（参数 毒 turn 5  攻%-50）` | 战斗中 | `enemy` | 4000 |
| 80 | 石化攻击 | `PETSKILL_StatusChange（参数 石 turn 3  攻%-30）` | 战斗中 | `enemy` | 2300 |
| 90 | 混乱攻击 | `PETSKILL_StatusChange（参数 乱 turn 3 攻%-30）` | 战斗中 | `enemy` | 2000 |
| 100 | 泥醉攻击 | `PETSKILL_StatusChange（参数 醉 turn 3 攻%-30）` | 战斗中 | `enemy` | 2000 |
| 110 | 催眠攻击 | `PETSKILL_StatusChange（参数 眠 turn 3  攻%-30）` | 战斗中 | `enemy` | 2500 |
| 120 | 地球一周 | `PETSKILL_EarthRound（参数 攻%+90）` | 战斗中 | `enemy` | 2000 |
| 130 | 旅程伙伴 | `PETSKILL_Abduct` | 战斗中 | `enemy` | 3000 |
| 140 | 偷窃 | `PETSKILL_Steal` | 战斗中 | `enemy` | 0 |
| 150 | 不防守战法 | `PETSKILL_NoGuard（参数 回避%+30 反击%+50 会心%+20）` | 战斗中 | `none` | 3000 |
| 151 | T不防守战法2 | `PETSKILL_NoGuard（参数 回避%+40 反击%+60 会心%+30）` | 战斗中 | `none` | 15000 |
| 152 | T不防守战法3 | `PETSKILL_NoGuard（参数 回避%+50 反击%+70 会心%+40）` | 战斗中 | `none` | 25000 |
| 200 | 加工 | `PETSKILL_Merge` | 战斗外 | `none` | 1500 |
| 201 | 料理 | `PETSKILL_Merge` | 战斗外 | `none` | 1000 |
| 210 | 落马术 | `PETSKILL_FallGround（参数 攻%-30）` | 战斗中 | `enemy` | 3000 |
| 211 | 捐献 | `PETSKILL_StealMoney` | 战斗中 | `enemy` | 3500 |

## 处理决定

| 项目 | 决定 | 理由 |
| --- | --- | --- |
| 清点 | 做（本文） | 需求第 1 步 |
| 战斗外使用宠物技能的协议（5.50 `pet_skill`） | 做 | 入口、校验顺序和错误分支已有协议用例（`docs/protocol/cases/pet_skill_outside.jsonl`）和单元测试 |
| 加工、料理的合成效果 | 做（STAGE-207） | 原版是完整的合成系统（配方、原子、随机结果、宠物加成、材料扣除）。服务端合成的规则见 `docs/synthesis.md`，协议见 5.55；合成品标记的效果（STAGE-209）和客户端界面（STAGE-208）另立 |
| 战斗中的技能 | 已对齐原版（STAGE-291） | 已在 STAGE-49、75、76、78 完成；STAGE-291 按原版修正连续攻击、破除防御、落马术，补上偷窃、捐献、旅程伙伴，怪物的忠犬、背水之战、不防守战法在回合开始时登记 |
| 客户端的 `pet skill <编号> <栏位> [背包格号…]` 命令 | 做 | 与协议一一对应，错误原样显示；带格号时是合成（STAGE-208） |
| 宠物页的菜单入口 | 不做（本单） | 入口属于客户端的后续需求（`docs/synthesis.md` 第 9 节第 4、5 步） |
| 宠物技能店：花钱给宠物学技能（STAGE-143） | 做 | 原版 `npc_petskillshop.c` 的收费（学习价格 × 店的 `skill_rate`）和上架规则（行首 T 的非法技能不上架；0 号待机合法，参数里有就照常上架）；入口是村里的宠物技能店 NPC，见协议 5.51。学到的技能覆盖栏位里原来的技能，不另外扣别的东西 |
| 宠物技能店里的加工、料理 | 上架，学得到，用的时候是合成 | 原版店里有这两个技能（参数里 200、201），学习不需要合成功能；用的时候按合成规则（5.55）处理 |
| 骑乘中的宠物用战斗外技能 | 待定 | 等 STAGE-123 确定后再对齐（见 `docs/ride.md`） |
