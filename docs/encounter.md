# 野外遇敌（机制与查法）

一个坐标点会遇到什么敌人、多少级、几只、怎么站位，全部由三张数据表决定，**没有随机等级补偿，也不看玩家等级**。
本文是 `gmsv/char/char_walk.c`、`gmsv/char/encount.c`、`gmsv/char/enemy.c` 的行为整理，附一个只读查询工具。

- 源码：`original-data/gmsv/char/{char_walk,encount,enemy}.c`、`battle/battle.c`、`configfile.c`
- 数据：`original-data/gmsv/data/{encount,group1,enemy,enemybase}.txt`（**GBK**）
- 新服务端：`server/battle/encounter.go`、`server/battle/seat.go`、`server/gamedata/encounters.go`（STAGE-40 实现）
- 查询：`tools/encounter-lookup.py`

## 1. 结论

1. **三级查表**：坐标 → 遇敌区域（`encount.txt`）→ 敌群（`group1.txt`）→ 敌人变体（`enemy.txt`）→ 模板（`enemybase.txt`）。
2. **遇敌不是固定概率**，是 CEP 累积：不遇敌就 `+1`，遇敌后重置。同一步的概率取决于你在这张图走了多久。
3. **等级只由 `enemy.txt` 的 `LV_MIN`/`LV_MAX` 决定**，在区间里均匀随机；普通遇敌的 `baselevel = 0`，与玩家等级无关。
   两只同种怪可以是不同等级。
4. **数量** = `1 .. min(区域队伍上限, Σ该敌群各变体的同场上限)`。
5. **队形** = 抽取顺序即站位；前 5 只进 15–19 号格，第 6–10 只进 10–14 号格。大型怪全场最多 5 只且优先前排。
6. **所有"概率"都是相对权重**，不是百分比，也不是千分比。求和后按区间落点抽。
7. 表里引用了但分组表里不存在的敌群，原版**仍参与抽签，抽中即空场**（不报错、直接不出怪）。

## 2. 判定链路

```text
玩家走一格（CHAR_WALKSUCCESSED，落到新格）
  │  CHAR_TYPEPLAYER 才判；战斗中、走到有 warp 事件的格 → 不判
  ▼
CEP 判定（char_walk.c:528）
  │  rand() % (120 × enemyact) < cep  →  开战
  │  否则 cep++（封顶区域上限）
  ▼
找区域（ENCOUNT_getEncountAreaArray）
  │  同 floor + 矩形包含该点 + zorder > 0，取 zorder 最大
  ▼
抽敌群（ENEMY_getEnemy，enemy.c:1267）
  │  区域 10 个槽位，跳过空槽 / 表里缺失 / 道具条件不满足
  ▼
抽敌人（同一函数）
  │  敌群 10 个槽位，随机替换展开，同场上限封顶
  ▼
定等级与站位（ENEMY_createEnemy，enemy.c:991 + battle.c:2188）
```

## 3. 三张数据表

### 3.1 `encount.txt` —— 遇敌区域（33 列）

| 列 | 字段 | 说明 |
|---|---|---|
| 1 | `index` | 区域编号 |
| 2 | `floor` | 地图编号 |
| 3–6 | `x1,y1,x2,y2` | 矩形两角，读入时排成小/大，`width = max - min` |
| 7 | `encountprob_min` | **CEP 下限** |
| 8 | `encountprob_max` | **CEP 上限**（读入时也会排成小/大） |
| 9 | `enemymaxnum` | 一场最多的敌人数，**必须 1–10**，否则整行跳过 |
| 10 | `zorder` | 区域优先级；**≤ 0 的行在查找时被忽略** |
| 11–20 | `groupid[10]` | 敌群号，空 = -1；**有重复则整行被跳过**（`checkRedundancy`） |
| 21–30 | `createprob[10]` | 敌群权重 |
| 31–33 | `event_now` / `event_end` / `enemy_group` | `_ADD_ENCOUNT` 扩展：任务事件期间换用另一套敌群。当前表里基本全空 |

查找规则（`ENCOUNT_getEncountAreaArray`）：

- 先按 `floor` 过滤，矩形用**闭区间**包含判断；
- 只考虑 `zorder > 0` 的块，重叠时取 `zorder` 最大；**平局取文件里靠前的那一行**。

### 3.2 `group1.txt` —— 敌群（24 列）

⚠️ `setup.cf:377` 的 `groupfile` 指向 **`group1.txt`**，不是 `group.txt`。改数据别改错文件。

| 列 | 字段 | 说明 |
|---|---|---|
| 1 | 名字 | 仅注释用，程序不读（如 `sai_e_113_1`） |
| 2 | `GROUP_ID` | 敌群编号，被 `encount.txt` 第 11–20 列引用 |
| 3 | `GROUP_APPEARBYITEMID` | **持有**这个物品编号才出现；-1 = 无条件 |
| 4 | `GROUP_NOTAPPEARBYITEMID` | **持有**这个物品编号就不出现；-1 = 无条件 |
| 5–14 | `ENEMY_ID1..10` | 敌人变体编号（`enemy.txt` 第 1 列） |
| 15–24 | `CREATEPROB1..10` | 群内生成权重 |

### 3.3 `enemy.txt` —— 敌人变体（34 列）

字段顺序见 `include/enemy.h` 的 `ENEMY_DATAINT` 枚举。关键项：

| 枚举 | 含义 |
|---|---|
| `ENEMY_ID` | 变体编号，被 `group1.txt` 引用 |
| `ENEMY_TEMPNO` | 模板编号 → `enemybase.txt`（决定名字、属性、图像、体型、技能） |
| `ENEMY_LV_MIN` / `ENEMY_LV_MAX` | **等级区间** |
| `ENEMY_CREATEMAXNUM` | 同场这只最多几份（还要乘以它在敌群里占的槽位数） |
| `ENEMY_TACTICS` | 战术模式（1 = `BATTLE_ai_normal`） |
| `ENEMY_EXP` | -1 表示按公式算（`ENEMY_getExp`） |
| `ENEMY_STYLE` | 武器样式 1–7，影响反击系数与暴击 |
| `ENEMY_ITEM1..10` / `ENEMY_ITEMPROB1..10` | 掉落槽位与概率 |

**同名不同级是常态**：同一种怪在表里有多个变体（等级段不同），被不同敌群引用。比如萨伊那斯的
「邦洛洛克斯」既有 `Lv1` 变体（敌群 118）也有 `Lv10` 变体（敌群 119）。

**随机替换**：变体 945–956、964–969 不是具体怪，而是"随机怪组"。`ENEMY_RandomEnemyArray` 会按候选表
（`RandomEnemyTbl`，如 945 → 739/742/745/748）换成一个具体变体再入库。共 18 个这类变体。

### 3.4 模板 `enemybase.txt` / `petbase.tsv`

`enemybase.txt`（952 行）是怪物模板：名字、地水火风属性、`E_T_INITNUM`/`E_T_LVUPPOINT`（成长）、
`E_T_SIZE`（**0 = 普通，1 = 大型**）、抗性、技能等。新服务端已经把它转成 `server/gamedata/petbase.tsv`（UTF-8），
查询工具读的是这份。

## 4. 遇敌频率：CEP 累积

原版不是"每格 1/N 概率"，而是一个**逐格增长的计数器**（`CONNECT_getCEP(fd)`，每个连接一个，跨地图保留）：

```c
cep = clamp(cep, minep, maxep);            // 进入区域时夹到本区域区间
if (rand() % (120 * getEnemyAction()) < cep) {
    开战; cep = minep;                      // 触发后回到下限
} else if (cep < maxep) {
    cep++;                                  // 没触发就抬一档
}
```

- `getEnemyAction()` 来自 `setup.cf` 的 `ENEMYACTION`（`configfile.c:2416`），缺省 / 0 / 非法都取 **1**；
  配 2 则遇敌率直接减半。`_ENEMY_ACTION` 宏在 `version.h` 里是**开着**的。
- CEP 上下限还会被"避敌"类道具按 `CHAR_WORK_TOHELOS_CUTRATE` 放大后夹到 0–100。

期望步数（照上式解马尔可夫链，`enemyact = 1`）：

| CEP | 期望格数/场 | 全表区域数 |
|---|---|---|
| 0 → 0 | **永不遇敌** | 81 |
| 0 → 1 | 121.0 | 6 |
| 1 → 2 | 60.5 | 34 |
| 1 → 3 | 41.0 | 175 |
| 1 → 4 | 31.5 | 16 |
| 1 → 5 | 26.0 | 211 |
| 1 → 6 | 22.5 | 25 |
| 1 → 7 | 20.1 | 108 |
| 2 → 6 | 21.6 | 2 |
| 2 → 7 | 19.2 | 1 |
| 1 → 8 | 18.4 | 1 |
| 1 → 10 | 16.2 | 1 |
| 100 → 100 | **1.2**（几乎每格都打） | 2 |
| 1 → 1 | 120.0 | 1 |
| 字段不全 | — | 1 |

（合计 665 个区域行；区域编号 430、639 各出现两次，不影响）

> 直觉上"上限越小遇敌越密"是反的：上限同时是概率分子的封顶值，`cep/120` 才是每格概率。
> 所以 1→3 比 1→7 **更稀**（41 格 vs 20 格）。

不判定的情形（`char_walk.c`）：

- 不在 `BATTLE_CHARMODE_NONE`（已经在战斗里）；
- 落脚格上有 `CHAR_EVENT_WARP` 事件（`entflag = 0`）——传送点不会把你扔进战斗；
- `getNoenemy(fd)` 非 0，或装备的 `getEqNoenemy` 达到阈值（Ra's amulet 这类，按"每 100 步"分级免疫）；
- `_Item_MoonAct`（月神的守护）判定未通过。

另两条非走路触发：道具/诅咒石的 `stayencount`（`net.c` 原地定时遇敌），以及客户端发 `EN` 包
（`lssproto_EN_recv`，旧协议入口）。

## 5. 抽敌群

`ENEMY_getEnemy` 第一段（`enemy.c:1298` 起）：

1. 遍历区域的 10 个 `(groupid, 权重)` 槽位，`groupid = -1` 跳过；
2. 取敌群；**表里没有这个编号时**：原版不剔除，权重照算（后续取到空表 → 这一场没有敌人）；
3. 检查道具条件：`APPEARBYITEMID != -1` 时背包里必须有它；`NOTAPPEARBYITEMID != -1` 时背包里不能有它；
4. `r = RAND(0, Σ权重 - 1)`，从头累加权重落点选中。**最后一项是兜底**（累计都没中时取最后一项）。

所以权重是纯相对的：`50 / 50 / 10 / 10` 合计 120，分别占 41.7% / 41.7% / 8.3% / 8.3%。

## 6. 抽敌人、定数量

第二段（`enemy.c:1358` 起）：

1. 遍历敌群的 10 个 `(enemyid, 权重)` 槽位；变体属于随机怪组时先展开替换；
2. 累加 `createenemynum += 该变体的 ENEMY_CREATEMAXNUM`；
3. `enemyentrymax = min(区域 enemymaxnum, createenemynum)`；
4. **本次总数** `entrymax = RAND(1, enemyentrymax)` —— 所以是 `1 .. 上限` 的均匀随机，不是"抽满"；
5. 循环 `entrymax` 次，每次按权重抽一个变体，并检查两个约束：
   - 同一变体已出现的只数 `< CREATEMAXNUM × 该变体在敌群里出现的槽位数`，超了就重抽；
   - **大型怪**（模板 `E_T_SIZE = 1`）全场最多 5 只。
6. 循环最多 100 次（`loopcounter < 100`），抽不出来就少几只。

## 7. 队形（站位）

- 抽取顺序就是站位顺序：**前 5 只 → 15–19 号格，第 6–10 只 → 10–14 号格**
  （`server/battle/battle.go` 的 `firstPetNo = 5`、`unitsPerSide = 10`；老服务端在 `battle.c:2188` 把前后
  5 个 `Entry` 整体对调，效果一致）。
- **大型怪优先前排**：数组前 5 位优先放大型怪；第 6 只及以后抽到大型怪时，会去前 5 位里找一只普通体型的
  **顶替**（普通怪被挤到后排）。前 5 位全是大型时，新来的大型怪直接**不出场**（`entrymax--`）。
- 结果是：打大怪基本都在前排，普通怪被推到后排；一场最多 10 只但大型怪最多 5 只。

## 8. 实例：萨伊那斯（地图 100）

| 项 | 值 |
|---|---|
| 区域行数 | **45**（编号 #21–#265） |
| 真正会遇敌 | **44** |
| 不遇敌的那一块 | #36，`(1,1)-(799,799)` 全图兜底，`zorder=1`，**CEP 0→0** |
| zorder 分档 | 5×1、10×1、20×8、30×5、40×4、50×5、60×1、100×11、110×2、120×2、200×1、500×3 |
| 队伍上限分布 | 2 只×4、3 只×11、4 只×5、5 只×12、6 只×1、8 只×2、10 只×9 |
| 等级覆盖 | Lv1 – Lv90 |
| 会出 1 级怪的区域 | 11 个 |

**44 是块数，不是同时生效数**：这些矩形互相嵌套，走到某点只有 zorder 最大的那块生效。
例：`(380,460)` 命中 #37（z=60），同时被 #33（z=40）和 #36（z=1）覆盖，实际用 #37。

`(650,500)` 的完整结果：

| 项 | 值 |
|---|---|
| 区域 | #21，`(582,399)-(783,560)`，zorder 40，CEP 1→5（约 26 格/场） |
| 队伍规模 | 1–2 只 |
| 敌群 88 权重 50 → 22.7% | 昆伊 Lv1（同场上限 2） |
| 敌群 91 权重 50 → 22.7% | 凯比特 Lv1（同场上限 2） |
| 敌群 89 权重 10 → 4.5% | 昆伊 Lv2-3（同场上限 10） |
| 敌群 92 权重 10 → 4.5% | 凯比特 Lv2-3（同场上限 10） |
| 敌群 1230 权重 100 → **45.5%** | **分组表里没有** → 原版这一场空场 |

## 9. 查询工具

`tools/encounter-lookup.py`（只读）：读 `server/gamedata/*.tsv`（`gen-encounters.py` 生成物，UTF-8），
CEP 上下限回原始 GBK 表按区域号取。

```sh
python tools/encounter-lookup.py --floor 100 --x 650 --y 500   # 这个点会遇到什么（含概率、等级、数量、队形提示）
python tools/encounter-lookup.py --floor 100                   # 一张图的全部区域概览
python tools/encounter-lookup.py --name 乌力                    # 反查：哪里能遇到它
python tools/encounter-lookup.py --lv 1                        # 反查：哪里有 1 级怪
```

改了 `encount.txt` / `group1.txt` / `enemy.txt` 之后，要先 `python tools/gen-encounters.py` 重新生成 TSV，
工具和服务端才会看到新数据。

## 10. 新服务端现状（STAGE-40）与偏差

现有实现：`encounter on|off` 开关（`docs/protocol.md` 5.29），打开后按固定间隔一场接一场，
间隔由 `STONEAGE_ENCOUNTER_INTERVAL_MS` 控制（默认 500ms）。

| 项 | 原版 | 新服务端 |
|---|---|---|
| 触发 | 走路 + CEP 累积 | 开关式，固定 2 秒间隔，**无 CEP** |
| 缺失敌群 | 参与抽签，抽中即空场 | 直接从抽签里剔除，权重集中到存在的敌群 |
| 模板缺失 | 这一只不入场 | 跳过这只，其余照常开战 |
| 等级 | `RAND(LV_MIN, LV_MAX)` | 同左（`enemyLevel`） |
| 数量 / 大型怪 / 换位 | 见第 6、7 节 | 同左（`placeEnemy`） |

偏差是有意为之（`server/battle/encounter.go` 顶部注释），但**走图手感完全不同**：要还原「走几十格遇一次」，
得把 CEP 那套搬进新服务端。

## 11. 易错点

1. **权重不是百分比**。区域 21 的权重合计 220，`50` 只占 22.7%。
2. **缺失敌群会吃掉整场**。区域 21 有 45.5% 的概率抽到不存在的敌群 1230 → 空场，看起来像"漏了一次遇敌"。
3. **`setup.cf` 用的是 `group1.txt`**，同目录的 `group.txt` 不被读取。
4. **`zorder ≤ 0` 整行作废**，不是"最低优先级"。
5. 这些表是 **GBK**，用 UTF-8 编辑器直接改会把中文名写坏；新服务端侧一律读生成的 UTF-8 TSV。
6. `enemymaxnum` 超过 10、或 `groupid` 有重复，会让**整行被跳过**（只在启动日志里留一句语法错误）。
7. 等级和玩家的等级、人数、队伍都无关——只有 NPC 陪同战斗（`mode = 2`）才用 NPC 的等级当 `baselevel`。

## 12. 按区域遇敌（STAGE-152）

场景页的「遇敌」列不再只依赖 `encounter on`，而是先列出本图可选的区域，选中后定位过去直接开战。
协议见 [protocol/encounter-areas.md](protocol/encounter-areas.md)。

**什么算一个可选区域**

- 有正 `zorder`、有效地图与整数坐标，矩形裁到地图边界（`server/gamedata/mapsizes_gen.go`）；
- 生效范围按闭区间 + 最大 zorder + 文件靠前计算，**高优先级安全区也参与覆盖判断**；
- 完全被盖住、裁完为空、找不到代表落点的不列；
- **CEP 上限 0 的原始安全区不列**（城镇、室内），`encounter on` 也不在里面开战；
- 必须能展开到正权重、有效变体、正同场上限、存在的模板；缺失敌群按新服务端规则剔除并重新归一权重；
- 道具条件让候选暂时为空时仍然列出，标为不可用并写明缺什么。

**代表落点**：区域内、确实由本区域生效（`EncounterAreaAt` 命中自己）、且不是普通出口触发格，
按 x 再 y 升序取第一个。不取矩形中心，也不随机。

**两种次数**：`once` 以成功开启的战斗计数，第一场打完就停；`infinite` 按默认 2 秒间隔连场。
两场之间再选一次是原子替换（先取消旧计时器）。被打飞传走、传送、离线、控制权转交都会清掉计划与旧计时器；控制权转交还会关掉开关（`encounter_changed` 的 `reason` 为 `control`，STAGE-311），当前这一场照常打完但不再开下一场。打输被打死原地复活（STAGE-194）不清，照常排下一场。

数据侧新增：`encounts.tsv` 保留 CEP 上下限、原始行号与序号、稳定区域 ID；
`mapsizes_gen.go` 提供 696 张地图的格子尺寸。生成器对越界敌人数、重复敌群、完全重复记录给诊断，
判定与原版加载器一致（整行跳过）。

## 1 级宠物目录口径（STAGE-251）

目录（协议 5.62 / 5.63，客户端“快捷传送 → 1级宠物”）列的是**能在野外遇到的 1 级宠物**，每只宠物列出它能遇到的区域。生成器是 `tools/gen-pet-encounters.py`，产物 `server/gamedata/pet_encounters_gen.go`，报告 `build/pet-encounter-report.md`（剔除项逐条原因、无俗称/系列的宠物、数据指纹）。

- **1 级可遇**：敌人变体 `LV_MIN ≤ 1 ≤ LV_MAX`、同场上限 > 0，随机怪组先展开成候选再看（同 `encounter_areas`）。
- **验收口径（STAGE-251）**：搜「人龙」命中的是实际有可遇点的人龙系成员，当前 3 只（扬奇洛斯、邦奇诺、利则诺顿）。邦浦洛斯、布鲁顿的唯一 1 级区域被 300 图的先行区域 300-311 完全覆盖（同 zorder，文件靠前者胜），服务端也取不到落点，按硬规则不进目录；多利诺布斯、小乌乌、小坦坦同理（见 build/pet-encounter-report.md 单列的一节）。
- **必须是宠物**：`petbase` 的 `PETFLG = 1`。1 级可遇的非宠物（矿石、木、深蓝色石头等）不进目录。
- **区域要真能遇敌**：`zorder > 0`、CEP 上限 > 0、裁到地图边界后非空、有有效敌人、同场上限 ≥ 1，并且能算出代表落点（区域内、本区域生效、不是出口触发格）。被更高 zorder 完全覆盖的区域取不到落点，不进目录；**同 zorder 的先行区域**（文件靠前）同样覆盖后面的区域，服务端也是这样判（ENCOUNTER_AREA_UNAVAILABLE）。
- **地图必须在快捷传送目录里**（`travel_catalog`，5.42）：997、11304、11305 不在目录，这些地图上的点一律不列。
- **道具条件**：敌群有 `GROUP_APPEARBYITEMID` / `NOTAPPEARBYITEMID` 的，这一期不进（这些点不稳定可遇），报告里逐条列出。
- **一只宠物一个模板**：同一模板的多个 1 级变体合并成一行，所有可遇点都列出。
- **筛选与服务端共用同一套口径**：生成器的 Python 镜像与服务端的 `encounter_areas` 对照，`server/character/pet_encounter_test.go` 把目录里的每个可遇点放进服务端的 `buildAreaView` 再校验（落点、矩形、同场上限、宠物模板都要对上）。
- **约多少格一场**：直接用本文件前面的 CEP 期望步数（`tools/encounter-lookup.py` 的 `steps()`），取整。

### 俗称与系列

- `server/gamedata/pet_aliases.tsv`：俗称人工审核清单，覆盖全部宠物模板。俗称挂在**同名的所有 PETFLG=1 模板**上（不挂 PETFLG=0 的），备注列写来源。查不到通用叫法的留空，不编造。
- `server/gamedata/pet_series.tsv`：系列表，系列名只作搜索标签，不做分类层。初版只收人龙系、毒蛙系；其余系列等对照官方 31 系后再补。
- 两份表是数据变更：改了要重跑 `tools/gen-pet-encounters.py`，不需要重跑遇敌数据，也不改协议。
- 同一俗称对应多只宠物时全部列出（如「大舌头」指贝洛恩一族），不做智能消歧。

## 练级传送口径（STAGE-279）

练级传送目录（协议 5.70 / 5.71）与 1 级宠物目录用同一套区域判定（`buildAreaView` 与 `collectAreaEnemies`），只在“怎么归段、怎么合并”上不同：

1. **敌人**：每个区域展开全部敌群与变体（随机怪组展开成候选，同开战），模板名取 `enemybase` 的名字。
2. **道具条件**：`GROUP_APPEARBYITEMID`（必须持有道具才出现）的敌群不进目录。`NOTAPPEARBYITEMID`（持有道具时不出现）的敌群**进目录**：不持有道具时它就是常态敌群，这一点和 1 级宠物目录（`pet_encounter`）不同，以下面的 323 条数据为准，见 `map.md`。
3. **Lv1 变体**：等级区间恰为 1–1 的变体不显示（抓宠看 1 级宠物目录）；只剩这种敌人的区域不产生条目。
4. **分段与裁切**：变体的等级区间 `[lo, hi]` 与每段 `[s, e]` 求交，交集非空则在该段列出，显示的是交集（`max(lo,s)`–`min(hi,e)`）。跨段的怪两段都列。
5. **合并**：同段内“怪物名 + 段内等级区间”（去重、按名字和等级排好后）完全相同的区域合成一条，条目保留全部落点。
6. **落点与剔除**：与 1 级宠物目录相同（`zorder ≤ 0`、CEP 上限 0、地图不在快捷传送目录、裁边后为空、没有有效敌人、同场上限为 0、找不到代表落点都剔除）。
7. **校验**：生成器产出的落点要经过服务端 `buildAreaView` 复核（`server/character/leveling_test.go`）；数据表变了重跑 `python3 tools/gen-leveling-spots.py`。
