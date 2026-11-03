# 装备耐久度（原版“道具损坏”）

STAGE-209 的研究与实现说明。原版依据全部来自原版服务端 C 源码（只读参考），行号以 `original-data/gmsv/battle/battle_event.c` 为准。

## 1. 范围与结论

- 原版开关：`gmsv/include/version.h:139 #define _TAKE_ITEMDAMAGE`（已定义，实际编译的是这个分支）。
- 只实现 `#ifdef _TAKE_ITEMDAMAGE` 分支；`#else` 分支（`ITEM_MERGEFLG` 随机损坏，`gItemCrushRate`）是死代码，不实现。
- 所有人物装备都按耐久度损坏（不只是合成品），宠物和怪物不损坏。
- 属性下降（`ItemBreakPos` 循环）、改名（`ITEM_SECRETNAME`）两段在原版都是注释掉的，不实现。

## 2. 原版规则（已逐条对照源码）

### 2.1 挑哪件装备（`BATTLE_ItemCrushCheck`，317–355 行）

- 攻击方（flg=2）：只看武器栏 `CHAR_ARM`，有装备就是它，没有则不损坏（返回 -1）。
- 防守方（flg=1）：`rand()%100`（注意是 C 的 `rand()`，不是 `RAND` 宏）：
  - `<50` 身体 `CHAR_BODY`；`50–66` 头 `CHAR_HEAD`；`67–83` 饰品1 `CHAR_DECORATION1`；`>=84` 饰品2 `CHAR_DECORATION2`。
  - 选中的栏没装备，则 `rndwork = (rndwork+1)%5` 往后找，最多 5 次；**跳过 `CHAR_ARM`**（代码条件 `rndwork != CHAR_ARM`），所以防守方的武器永远不会被挑中。找不到返回 -1。
- 栏位编号（`include/char_base.h:64–69`）：`CHAR_HEAD=0, CHAR_BODY=1, CHAR_ARM=2, CHAR_DECORATION1=3, CHAR_DECORATION2=4, CHAR_EQUIPPLACENUM=5`。
  我们的 `gamedata.SlotHead=0, SlotBody=1, SlotHand=2, SlotAccessory1=3, SlotAccessory2=4` 与之一一对应，可直接照用。
- 防守方的“往后找”顺序（以 `CHAR_BODY` 起头为例）：BODY(1) → ARM(2，跳过) → DEC1(3) → DEC2(4) → HEAD(0)。

### 2.2 谁会损坏、何时调用（`BATTLE_ItemCrushSeq`，622–646 行）

```
flg=1: crushindex = ItemCrushCheck(defender, 1); 若 >=0 且 defender 是玩家 → ItemCrush(defender, crushindex, Damages, 1)
flg=2: crushindex = ItemCrushCheck(attacker, 2); 若 >=0 且 attacker 是玩家 → ItemCrush(attacker, crushindex, Damages, 2)
return iRet   （iRet 恒为 FALSE）
```

- 防守方先、攻击方后；两者用同一个伤害值 `Damages`。
- 返回值恒为 FALSE，所以各调用点里 `== TRUE` 的分支（`BCF_CRUSH` 标记）在 `_TAKE_ITEMDAMAGE` 下不会执行，原版也不会发“装备受到损伤”的战斗字符串。
- 调用点（`battle_event.c` 行号 → 所在函数）：

| 行号 | 所在函数 | 攻击类型 | 调用前的条件 | 我们的对应（待第 4 步核对） |
|---|---|---|---|---|
| 2443 | `BATTLE_Attack` | 普通攻击 | `damage > 0` | `rules.go` `attack` → `attackSeq` → `strike` → `physHurt` |
| 2694 | `BATTLE_Attack_FIREKILL` | 火焰击杀类技能 | `damage > 0` | `skills.go` / `skills_strong.go` 中的物理技能 |
| 3045 | `BATTLE_Counter` | 反击 | `damage > 0` | `actions.go` `counter` → `physHurt` |
| 3951 | `BATTLE_S_GBreak` | 技能（G 击/破防类） | `damage > 0` | `skills_strong.go` |
| 4249 | `BATTLE_S_GBreak2` | 技能（同上，第二版） | `damage > 0` | `skills_strong.go` |
| 4600 | `BATTLE_Combo` | 合击 | `damage > 0`（在循环内，每个攻击者结算） | `combo.go`（用 `landSeq`，要在它之后接） |
| 5275 | `BATTLE_S_FallGround` | 技能（落石类） | `damage > 0` | `skills_strong.go` / `skills.go` |
| 5888 | `BATTLE_S_AttackDamage` | 通用技能伤害 | `damage > 0` | `skills.go` `skillAttack` 一类 |
| 6713 | `BATTLE_BattleModel_ATTACK` | 战斗模型里的攻击 | **没有** `damage > 0` 判断，只判“目标没死” | 待核对（见 §5 第 3 条） |

- 5645 行 `BATTLE_S_ToothCrushe`（咬碎类技能）**不调用** `BATTLE_ItemCrushSeq`，它自己只读防守方的 `BATTLE_ItemCrushCheck(def,1)` 去改“耐久比例”的数值效果，没有扣耐久。我们不接它。
- 魔法伤害（`BATTLE_MagicAttack` 一类，`magicDamage`）在原版没有任何 `ItemCrushSeq` 调用，**魔法不损坏装备**。我们也不接。
- 宠物（`CHAR_TYPEPLAYER` 检查）和怪物不损坏：攻击方是宠物时 `ItemCrushCheck(attacker,2)` 找不到武器，防守方是宠物/怪物时进 `ItemCrush` 前就被 `CHAR_TYPEPLAYER` 拦住。
- 注意：原版对非玩家防守方仍会调用 `rand()%100`（只是之后不扣），会消耗随机数。我们不这样做（见 §5）。

### 2.3 扣多少（`BATTLE_ItemCrush`，389–480 行）

1. 物品没有（`ITEM_CHECKINDEX` 失败）返回 -1。
2. 最大耐久 `ITEM_MAXDAMAGECRUSHE < 1` 直接返回 -1（不损坏）。
3. 扣减基数：
   - 武器（`ItemEquip == CHAR_ARM`）：`breadnums = (Damages-50)/40`，`<=0` 取 1（C 整数除法）。
   - 防具：`if (Damages >= 200) breadnums = 1;` 随后 `breadnums = (200-Damages)/20`，`<0` 取 1。
     **原版那一句 `if(Damages>=200) breadnums=1;` 被下一句覆盖，是死代码**：伤害为 200 时 `(200-200)/20 = 0`，扣减为 `RAND(0, 0)`（见下条），即不扣。我们原样照搬，不“修正”。
4. 实扣：`crushenum = crushenum - RAND(breadnums, breadnums*1.4)`。
   `RAND` 是宏（`include/util.h:79`）：`((x-1)+1+(int)((double)(y-(x-1))*rand()/(RAND_MAX+1.0)))`。
   **注意**：`y` 不是先截断再传入的，而是宏展开后在 double 里算。结果是 `b + floor((0.4b+1)*u)`，`u∈[0,1)`，取值区间 `[b, b+ceil(0.4b)]`：
   - `b=1` → `{1,2}`；`b=2` → `{2,3}`；`b=3` → `{3,4,5}`；`b=5` → `{5,6,7}`；`b=10` → `{10,…,14}`。
   - 任务书原文写的“`breadnums*1.4` 截断成 int”不准确，以宏为准（见 §5）。
5. 扣后 `crushenum <= 0`：
   - 置 0，系统消息 `"%s因过度损坏而消失。\n"`（黄色，`CHAR_COLORYELLOW`），写物品日志，`CHAR_DelItem` 删掉这件装备，`crushlevel=4`（只是局部变量）。
6. 否则按剩余比例 `defs = crushenum*100/maxcrushenum` 定等级：
   - `>=70` → 0 完好；`50–69` → 1 受损；`30–49` → 2 毁坏；`<30` → 3 不堪使用。
   - 只有 `level != crushlevel && defs < 70` 时才：写 `ITEM_CRUSHLEVEL`，发 `"%s是%s的。"`（红色，`CHAR_COLORRED`，文字表 `aszCrushTbl` = 完好/受损/毁坏/不堪使用/碎片）。
7. 最后 `CHAR_sendItemDataOne` + `CHAR_complianceParameter`（重算属性）+ 状态刷新。

### 2.4 初始耐久与数据来源

- 物品表 `gmsv/data/itemset.txt`（GBK、逗号分隔、无表头）：
  - **第 30 列（0 起）= `dmce` = `ITEM_DAMAGECRUSHE`（当前耐久），第 31 列 = `mdmce` = `ITEM_MAXDAMAGECRUSHE`（最大耐久）。**
  - 依据：`gmsv/include/item.h` 的 `ITEM` 枚举从 `ITEM_ID`（列 16）起按顺序排：`ID16 BASEIMAGE17 COST18 TYPE19 ABLEUSE20 TARGET21 LEVEL22`，`DAMAGEBREAK23`（`dib`，数据里该列为 -1，吻合），`USEPILENUMS24 CANBEPILE25 NEEDSTR26 NEEDDEX27 NEEDTRANS28 NEEDPROFESSION29`，`DAMAGECRUSHE30 MAXDAMAGECRUSHE31`。
  - 数据验证：第 30、31 列两列值相等且 `>=1000` 的行恰好 1547 条，与下面的统计一致；小斧头那一行第 30、31 列是 `200000,200000`。
  - 解析表 `gmsv/item/item.c:60–61`（`{"dmce",…}`、`{"mdmce",…}`）与 `ITEM_setintdata` 的名字对应。
- 新物品的当前耐久 = 表值（数据里 `dmce` 直接写入，`item.c` 里没有额外设置）；当前耐久等于最大耐久。
- `char.c:6226–6227`：物品 `1292`、`19646` 被特殊设成 `MAX=0, DAMAGECRUSHE=0`（即不损坏），这两个编号在我们表里同样是 max=0。
- 合成/修理相关：`item_gen.c:1405–1440` 的修理函数（`_PETSKILL_FIXITEM`）按 `crushes >= maxcrushes*0.80` 拒绝、`maxcrushes < 500` 不可修、修后 `max*0.85` 并清 `CRUSHLEVEL`。修理是另一个功能，不在本需求内。
- 合成产出（STAGE-207）：`item_gen.c` 的合成产出处没有设置耐久，按产出物品的表值（见 §3.4）。

## 3. 我们的实现方案（Go）

### 3.1 数据

- `tools/gen-itemset.py` 新增两列：`DAMAGECRUSHE=30`、`MAXDAMAGECRUSHE=31`，生成 `server/gamedata/itemset.tsv` 的两个新字段（当前耐久、最大耐久），`server/gamedata/items.go` 的 `ItemTemplate` 增加 `Durability`、`MaxDurability`。
- 只改 tsv 中新增的两列，其他列不变（`git diff --stat` 只多两列）。

### 3.2 持久化

- 迁移（编号见 §3.5）给物品表加 `durability`、`max_durability`、`crush_level`（int，默认 0）。
- 老数据不回填：老物品 `max_durability=0` 即不损坏，只对之后新创建的物品生效。
- 物品在创建、移动、丢弃、交易、合成时都要带上这三列（照 `merged` 标记随物品走的做法，`0022_item_merged.sql` 的写法）。

### 3.3 战斗接入（已实现）

- 纯函数在 `server/battle/item_crush.go`：`pickDefenseSlot`（防守方选栏，rnd(100) 的 50/17/17/16 分布，跳过武器栏，空栏 (栏+1)%5 往后找）、`pickAttackSlot`（攻击方只看武器栏）、`crushBreadths`（扣减基数，整数除法向零截断）、`crushRand`（RAND 宏的区间）、`crushLevelOf`（70/50/30 阈值）、`applyCrush`（扣减、消失、等级提示）。
- 接线在 `server/battle/item_crush_hook.go`：`physHurt`（`magic_def.go`）里伤害落地且 `dmg > 0` 时调用 `itemCrushSeq`，先扣防守方的一件防具，再扣攻击方的武器（同一个伤害值，与原版 `BATTLE_ItemCrushSeq` 的顺序一致）。Go 里所有物理直接伤害（普攻、反击、合击、物理技能、回旋镖）都经过 `physHurt`，魔法伤害不经过，与原版的调用范围一致。
- 只有人物本人（单位号 0，`humanUnit`）的装备会损坏：`Unit.crushOwner` 为真才结算。宠物、怪物和分身的装备不结算。
- 装备快照 `SideInput.Equips[0..4]` 由 `character/battle_apply.go` 的 `sideInput` 从背包的装备栏读出；没有耐久的装备也放进去（挑栏时它仍占着格子，与原版一致），只是扣减结果为 0 不变。
- 耐久用**独立随机源** `Battle.crushRnd`，按战斗编号播种（`newCrushRand`），不占用战斗主随机源 `Rnd`，所以既有战斗的随机序列和协议用例的结果不变（原版共用 `rand()`，这是有意的偏离，见 §5）。
- 提示放在事件的 `crush` 数组里（`Event.Crush`，协议 10.7）：消失为黄色，等级变了为红色。
- 战斗结束（`finishBattle`）：`applyItemCrush` 把耐久和等级写回 `r.Items`，耗尽的从背包移除；`saveItemCrush` 写库（耗尽的删行，其余更新 `durability`、`crush_level`）；有变化时推送 `inventory_changed`。胜、负、逃跑都写回。

### 3.4 合成产出

- 合成（`server/character/merge_plan.go`）产出的新物品耐久按产出物品的表值，不额外设置。

### 3.5 迁移编号

- 写迁移前看 test 分支上的最大号（`../stoneage-test/server/store/migrations`，当时为 `0022_item_merged.sql`），取比它和 develop 都大的号。

## 4. 数据统计（itemset.txt，12622 行）

- 有耐久（最大耐久 >0）的物品：**1547 条**，全部满足 当前 = 最大；没有“最大为 0 但当前非 0”的行。
- 按物品类型（第 19 列 TYPE）分布，括号内是该类型总数：

| 类型 | 有耐久 / 总数 | 例 | 备注 |
|---|---|---|---|
| 7 | 233 / 2062 | 铠 | 身体防具 |
| 6 | 226 / 1432 | 兜 | 头部防具 |
| 2 | 116 / 716 | 棍棒 | 武器 |
| 3 | 111 / 714 | 枪 | 武器 |
| 1 | 110 / 712 | 斧 | 武器 |
| 0 | 108 / 709 | 爪 | 武器（拳套类） |
| 19 | 105 / 705 | 投掷石 | 武器 |
| 4 | 104 / 706 | 弓箭 | 武器 |
| 17 | 103 / 703 | 回旋标 | 武器 |
| 18 | 103 / 703 | 投掷斧头 | 武器 |
| 9 | 49 / 111 | 贝铃 | 饰品（栏位待核对） |
| 10 | 46 / 404 | 项链 | 饰品（栏位待核对） |
| 11 | 36 / 322 | 戒指 | 饰品（栏位待核对） |
| 8 | 34 / 249 | 手环 | 饰品（栏位待核对） |
| 15 | 29 / 58 | 护身符 | 饰品（栏位待核对） |
| 13 | 28 / 278 | 耳饰 | 饰品（栏位待核对） |
| 16 | 4 / 1096 | 戒指/爪（混杂） | 少量 |
| 27 | 2 / 2 | 皮甲手套 | 防具 |

- 常见耐久值：200000、300000、800000、1200000、2500000；武器（斧/棍/枪/爪…）多为 200000 或 800000。
- 具体装备栏由 `server/gamedata/equip.go` 的类型表决定（武器 → 手栏，防具 → 头/身，饰品 → 饰品1/2）。

## 5. 与原版的差异与待定项

1. **RAND 取值**：任务书写“`breadnums*1.4` 截断成 int 传入”，源码的 `RAND` 是宏，`y` 在 double 里运算，实际区间是 `[b, b+ceil(0.4b)]`（§2.3 第 4 条）。我们按宏的真实语义实现，测试里按这个区间断言。
2. **防具 `Damages>=200` 行**：原版被覆盖的死代码，我们照搬结果（伤害 200 时防具扣 0，武器不受影响），不修正。
3. **6713 路径**（`BATTLE_BattleModel_ATTACK`）原版没有 `damage > 0` 判断。我们统一要求 `dmg > 0` 才结算（保守：不会比原版多扣）。这是有意偏离（负责人决定：统一 dmg>0）。
4. **非玩家防守方不摇随机数**：原版对宠物/怪物防守方也会摇 `rand()`（只是不扣）。我们不摇（负责人决定）。耐久用独立随机源（见 §3.3），所以战斗主随机序列不受影响。
5. **魔法**：原版魔法伤害不调用，我们也不接。
6. **咬碎类技能（5645 行）**：原版不扣耐久，我们不接。
7. **属性下降、改名**：原版注释掉，不实现。
8. **老数据**：不回填，`max_durability=0` 不损坏。
9. **消息**：原版用 `CHAR_talkToCli` 直接发给人物本人；我们放在事件的 `crush` 数组里（协议 10.7），颜色按原版（消失=黄，降级=红）。客户端的显示另开需求。
10. **RNG 来源**：原版的装备损坏与战斗共用 `rand()`；我们用独立随机源（按战斗编号播种），结果可重现，但具体数值与原版不同（分布一致）。

## 6. 验证

- 单测：`server/battle/item_crush_test.go`（选栏边界与回退、扣减基数、RAND 区间、等级阈值、消失、武器、最大耐久 0）；`server/battle/item_crush_hook_test.go`（physHurt 集成：怪物打人扣防具、人打怪扣武器、伤害 0 不扣、等级提示、消失、怪物不损坏）。
- 搬运与视图：`server/character/battle_item_crush_test.go`（战斗快照带上耐久、背包视图只对有耐久的物品下发）。
- 数据：`server/gamedata/items_durability_test.go`（1547 条）、`items_test.go`（小斧头两列）。
- 数据库：迁移 0023 与读写在 `server/store`、`server/character` 的数据库测试里执行（用隔离 schema）。
- 协议用例：`docs/protocol/cases/world_item_durability.jsonl`（背包视图的耐久字段）。

## 7. 参考

- 原版：`original-data/gmsv/battle/battle_event.c`（317–355、389–480、622–646、2443/2694/3045/3951/4249/4600/5275/5645/5888/6713 行）。
- 栏位：`gmsv/include/char_base.h:64–69`；开关：`gmsv/include/version.h:139`；RAND：`gmsv/include/util.h:79`；枚举：`gmsv/include/item.h:85–86`；解析表：`gmsv/item/item.c:60–61`；修理：`gmsv/item/item_gen.c:1405–1440`。
- 数据：`original-data/gmsv/data/itemset.txt`（第 30、31 列）。
