# 魔法抗性体系设计（STAGE-211）

状态：设计稿，只描述方案，未改任何代码、协议和用例。

口径：原版 `original-data/gmsv`。原版 `include/version.h` 已开启 `_ATTACK_MAGIC`（:55）、`_EQUIT_DEFMAGIC`（:134）、`_FIX_MAGICDAMAGE`（:135）、`_MAGIC_DEFMAGICATT`（:161）、`_OTHER_MAGICSTAUTS`（:163）、`_MAGICSTAUTS_RESIST`（:310），所以下文原版行为都是“实际生效”的分支。`_FIX_MAGICDAMAGE` 没有 `#else` 时的旧公式在源码里是 `#ifndef` 分支，不生效。

## 推荐方案

1. 人物、宠物、怪物都补齐四元素抗性字段（地水火风），人物另加“抗魔装备”加成；怪物按原版 `LV×0.5` 现算，不存字段。
2. 攻击精灵伤害**整体换成原版 `_FIX_MAGICDAMAGE` 公式**（熟练度、抗性、等级、施法者属性都进公式）。
3. 换公式的同时必须带上**熟练度与抗性成长**（原版 `Magic_ComputeAttExp` / `Magic_ComputeDefExp`），否则新角色熟练度为 0 时精灵伤害接近 0（见第 3 节）。
4. 魔抗状态按原版算：只让被打方的四元素抗性 ×(1+N%)，不影响闪避，不叠加。
5. 火/电/冰抗精灵照原版：只占用魔法状态槽、有动画，**源码里没有数值效果**（见第 2 节，待决定第 1 条）。
6. 拆成 4 个需求，依赖顺序：抗性字段 → 伤害公式（含熟练度成长） → 8 个精灵 → 协议与客户端。

## 1. 抗性字段从哪来

| 单位 | 原版字段 | 来源 | 原版源码位置 |
|---|---|---|---|
| 人物 | `CHAR_EARTH_RESIST` / `WATER_` / `FIRE_` / `WIND_RESIST`（持久化） | 初值 0（`include/defaultPlayer.h:143–146`）；被攻击时由 `Magic_ComputeDefExp` 成长 | `include/char_base.h:416–419`；`battle_magic.c:3102` |
| 人物 | `CHAR_EQUITDEFMAGIC_E/WA/F/WI`（workint） | 穿戴装备时读道具参数 `EA/WA/FI/WI`（±100，单件限幅），`QU` 写 `CHAR_EQUITQUIMAGIC` | `item/item_event.c:2922–2945`（`ITEM_MagicEquitWear`） |
| 人物 | `CHAR_EARTH_EXP[4]`（熟练度）、`CHAR_EARTH_ATTMAGIC_EXP[4]`、`CHAR_EARTH_DEFMAGIC_EXP[4]` | 初值 0；由 `Magic_ComputeAttExp` / `Magic_ComputeDefExp` 成长 | `include/char_base.h:412` 起；`battle_magic.c:3060`、`3102` |
| 宠物 | 攻击时读 `CHAR_EARTH_EXP`，防守时读 `CHAR_EARTH_RESIST` | 源码里**没有任何地方给宠物这些字段赋值**（`grep` 只在 `battle_magic.c`、`char_base.h`、`defaultPlayer.h` 出现），按 0 处理；通用整数表读入的情况待确认 | `battle_magic.c:1081–1100` |
| 怪物 | 不读字段 | 攻击熟练 `LV×0.9`，防守抗性 `LV×0.5` | `battle_magic.c:1036–1040`（攻）、`1085–1090`（守，怪物分支，行号按上下文估） |

新服务端现状：`server/battle/unit.go:22` 的 `Elements` 只有地水火风四个属性（属性百分比，不是抗性）；`Unit` 里没有抗性、熟练度、抗魔装备字段；`StatusSet`（`unit.go:171`）只有 `Weaken/Barrier/Nocast` 等异常状态，没有魔法状态。

建议新增（都放在单位快照里，人物的熟练度和抗性要持久化）：
- `Unit.MagicResist [4]int`：人物 = 持久化抗性 + 抗魔装备（运行时合算）；宠物 = 0（待确认）；怪物 = `LV*0.5` 现算，不存。
- `Unit.MagicExp [4]int`（攻击熟练度）、`Unit.MagicDefExp [4]int`（防守经验）、`Unit.MagicAttExp [4]int`（攻击经验）：只给人物。
- `Unit.EquipMagicDef [5]int`：抗魔装备（地水火风、全）加成，±100。
- `Unit.DefMagicNums int`、`Unit.DefMagicTurns int`：魔抗状态（见第 2 节）。

数据来源：`server/gamedata` 里的道具参数需要能读到 `EA/WA/FI/WI/QU`；待确认新服务端道具表是否已有这些参数（`tools/gen-itemset.py` 生成的表，未检查）。

## 2. 魔抗状态怎么作用

### 2.1 精灵表（`data/magic.txt`，GBK）

字段顺序（`magic/magic_base.c` 读取，`include/magic_base.h` 的 `MAGIC_DATAINT`）：名称、说明、函数名、参数（`MAGIC_OPTION`），然后是整数字段 `MAGIC_ID`（即表里的“编号”，如 392）、`MAGIC_FIELD`（1 = 战斗）、`MAGIC_TARGET`（目标码）、`MAGIC_TARGET_DEADFLG`、`MAGIC_IDX`（攻击精灵才有）。“编号”指第 5 个整数列，**不是文件行号**（`magic.txt` 第 153 行的编号是 392）。

| 精灵 | magic.txt 行 | 编号 | 函数 | 参数（状态\|回合\|数值\|单/全） | 目标码 | 范围 |
|---|---|---|---|---|---|---|
| 魔法防御 Lv1 | 153 | 392 | `MAGIC_MagicStatusChange` | `魔抗\|3\|20\|单` | 1 | 我方单体 |
| 魔法防御 Lv2 | 154 | 393 | `MAGIC_MagicStatusChange` | `魔抗\|3\|20\|全` | 8 | 我方全体 |
| 冰之精灵 Lv1 | 176 | 417 | 同上 | `冰抗\|3\|10\|单` | 1 | 单体 |
| 电之精灵 Lv1 | 177 | 418 | 同上 | `电抗\|3\|10\|单` | 1 | 单体 |
| 火之精灵 Lv1 | 178 | 419 | 同上 | `火抗\|3\|10\|单` | 1 | 单体 |
| 冰之精灵 Lv2 | 179 | 420 | 同上 | `冰抗\|3\|10\|全` | 8 | 一方全体 |
| 电之精灵 Lv2 | 180 | 421 | 同上 | `电抗\|3\|10\|全` | 8 | 一方全体 |
| 火之精灵 Lv2 | 181 | 422 | 同上 | `火抗\|3\|10\|全` | 8 | 一方全体 |

- 目标码 1 = `MAGIC_TARGET_OTHER`，8 = `MAGIC_TARGET_WHOLEOTHERSIDE`（`include/magic_base.h` 的枚举按顺序数出，注释是乱码；8 是否就是“我方全体”待确认）。
- 第三列 `MAGIC_FIELD` 都是 1（战斗）；`DEADFLG` 都是 0。
- 精灵的 `Success` 字段在这 8 个精灵里**没有**，参数里也没有“成”：`MAGIC_MagicStatusChange_Battle` 不掷成功率（见 2.3）。

### 2.2 施放与挂载（原版）

1. `MAGIC_MagicStatusChange_Battle`（`battle/battle_magic.c:2480`）解析参数：状态名按 `MagicStatus[]` 匹配（`battle/battle_event.c:50–56`，`_MAGICSTAUTS_RESIST` 下为 `魔抗/铁壁/火抗/电抗/冰抗`），第 2 段是回合，第 3 段是数值（写入 `CHAR_OTHERSTATUSNUMS`），第 4 段含“单”且目标码 ≥20（整体）时**直接失败**。
2. `BATTLE_MultiMagicStatusChange`（`battle_magic.c:1798`）对目标列表逐个处理：只要目标已有任一魔法状态（`MagicTbl[1..5]` 中有 >0 的），就**跳过**，不覆盖、不叠加、不刷新回合；否则写 `MagicTbl[status] = turn` 和 `CHAR_OTHERSTATUSNUMS = nums`。
3. `MagicTbl = { -1, CHAR_DEFMAGICSTATUS, CHAR_MAGICSUPERWALL, CHAR_MAGICFIRE, CHAR_MAGICTHUNDER, CHAR_MAGICICE }`（`battle_event.c:52`），`MAXSTATUSTYPE = 6`（`include/battle_event.h:50`）。
4. 挂载不掷骰。原版其他状态（毒、麻痹等）走 `BATTLE_MultiStatusChange`（`battle_magic.c:1733`）和成功率，魔法状态没有。

### 2.3 回合与清除（原版）

- 每回合 tick：`BATTLE_MagicStatusSeq`（`battle/battle.c:7584`）对每个存活单位的 `MagicTbl[1..5]` 做 `--cnt`，减到 0 清零。调用点：`battle.c:6000`（`BATTLE_Battling` 的回合循环，每单位一次）和 `battle.c:7278`（连击循环里又调一次，连击时同一单位可能多 tick，待确认）。
- 战斗结束或状态清空：`BATTLE_BadStatusAllClr`（`battle.c:80–90`）把 `MagicTbl[1..5]` 和 `CHAR_OTHERSTATUSNUMS` 清零。
- 挂上 `turn=3` 后当回合是否计数、实际能作用几次行动：源码阅读无法确定，待实测（待决定第 3 条）。

### 2.4 魔抗的实际效果

`BATTLE_MultiAttMagic`（`battle_magic.c:1104–1113`）：当 `CHAR_DEFMAGICSTATUS > 0` 时，对被打方四个元素的抗性（`def_magic_resist[j]`，且 >0 才加）乘 `(1 + nums/100)`。`nums` 就是精灵参数里的第 3 段（魔抗 20 即 +20%）。

- 只影响**被打方**，只在攻击精灵的伤害公式里生效。
- 不进闪避：`BATTLE_MagicDodge`（`battle_magic.c:886` 起）用的是未加成的 `CHAR_EARTH_RESIST × 0.15` 和装备值，不读 `DEFMAGICSTATUS`。
- 另一处相同逻辑在 `BATTLE_MultiAttMagic_Fire`（`battle_magic.c:3277` 起，3389 行），它是 `battle.c:6349` 的固定 200 火伤害路径。

### 2.5 火/电/冰抗精灵：没有数值效果

- `CHAR_MAGICFIRE` / `CHAR_MAGICTHUNDER` / `CHAR_MAGICICE` 在整个 `gmsv` 只出现在 `include/char_base.h:829–831` 和 `battle/battle_event.c:52`（即 `MagicTbl`）。
- `CHAR_OTHERSTATUSNUMS`（数值 10）写进去以后，只在 `CHAR_DEFMAGICSTATUS > 0` 的分支里读，火/电/冰抗永远走不到那里。
- 它们对战斗的唯一效果：占用魔法状态槽（互斥、清除、回合递减都会作用到它们）、播放动画（`battle_magic.c:2517–2524`，图号 101802/101804/101803）。
- 精灵描述写的是“火抗性提升 10”，原版没有实现。新服务端要不要照做，见待决定第 1 条。

## 3. 精灵伤害要不要换成 `_FIX_MAGICDAMAGE` 公式

### 3.1 两套公式（伪代码）

非 FIX（`#ifndef`，不生效）：
```
attvalue = BATTLE_AttrAdjust(攻方属性, 守方, Power)        // 攻方自身属性 × Power，四属性相克
if 守方是玩家:
    temp = (攻熟练 - 守抗) / 守抗 / 100                       // 守抗为 0 时除零
    attvalue = Power + Power*temp + attvalue
else 守方是宠物:
    temp = (攻熟练 - 守等级) / 守等级 / 100
    attvalue = Power + Power*temp + attvalue
attvalue += ±(1..10)
```

FIX（`#ifdef`，生效，`battle_magic.c:1127–1155`）：
```
K    = max(0, 攻熟练[F]*1.4 - 守抗[F])                      // 怪物攻熟练 = LV*0.9，守抗 = LV*0.5
M    = max(1, 攻熟练[F])
A    = K*K / (M*M) + rand%20/100
APower = int(Power * (1 + MagicLv/10.0) * A)                // MagicLv 来自精灵参数第 3 段
attvalue = BATTLE_getMagicAdjustInt(攻方, 守方, MagicLv, F, APower)
           // 见下：把攻方属性换成“精灵属性 × 等级”后做四属相克
if 施法失败(rand%100 > 攻熟练[F]): attvalue = int(attvalue * 0.7)
```

`BATTLE_getMagicAdjustInt`（`battle_magic.c:3009`）：`Mlv = MagicLv*10`；精灵属性 `F` 的分量 = `Mlv + Mlv*(攻属性[F]/50)`（整数除法），其余三个元素清零，**无属性分量保留攻方自身的值**（只清了三个元素，`At_none` 没清，这是原版设计，照抄）；各分量乘 `APower`，再用 `BATTLE_AttrCalc`（`battle_magic.c:795`，与新服务端 `rules.go:395` 的矩阵一致）按守方属性相克，`/10000`。

熟练度与抗性成长（只在 FIX 路径里）：
- 攻方：`battle_magic.c:1261–1262`，`if (施法失败 && 攻方是玩家) Magic_ComputeAttExp(...)`。原版是**失败时**涨攻击熟练（`addEx = MagicLv*3*命中数`，满 100 升 1 级，上限 100）。这个条件看上去反直觉，但就是源码，见待决定第 4 条。
- 守方（玩家或宠物）：`battle_magic.c:1155`，伤害 ≥200 时 `Magic_ComputeDefExp`（`3102`）：`addEx = (伤害/20)*(MagicLv*2)`，满 100 对应抗性 +1（上限 100）；同时对“相克”的下一个元素扣 2 点经验，经验为负时抗性 −1。

### 3.2 数值对比（手算，源码公式逐步算出；rand 取 0，即 A 不加随机项）

| 例子 | 攻方 | 精灵 | 守方 | 原版 FIX | 现服务端（精灵属性 100%，基数，未含 ±10%） |
|---|---|---|---|---|---|
| E1 | 人物，火熟练 50，无属性 | 火球 Power 100 L1 | 怪物，守抗 10，无属性 | **181** | 150 |
| E2 | 人物，火熟练 80，无属性 | 火焰连弹 Power 150 L2 | 玩家，火抗 30，无属性 | **245** | 225 |
| E3 | 人物，火熟练 60，火 40 / 无 60 | 熔岩爆发 Power 200 L3 | 玩家，水 30 / 无 70，火抗 0 | **456** | 246 |
| E4 | 同 E2，守方魔抗 +20% → 火抗 36 | 同 E2 | 同 E2 | **210**（不开魔抗 245，少 14%） | — |
| E5 | 人物，火熟练 0，无属性 | 火球 Power 100 L1 | 无属性守方，抗 0 | 0–20（A 只剩随机项，失败再 ×0.7） | 150 |

要点：
- E5 是换公式的最大风险：新角色熟练度 0 时，精灵伤害只剩 `APower = int(Power*(1+L/10)*rand)`，约 0–20。原版靠“失败涨熟练”自举，所以初值 0 在原版是自洽的；新服务端如果只换公式不做熟练度成长，攻击精灵基本没伤害。
- E3 说明：原版 FIX 把施法者自身属性带进来（火 40），单纯“精灵属性 100%”的现公式会低估约一半。

### 3.3 方案比较

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| A 全换（推荐） | 攻击精灵整体用 FIX，同时补熟练度成长、抗性成长、抗魔装备 | 与原版一致；公式只有一套；魔抗、装备、熟练度都有处用 | 改动大；所有攻击精灵数值变化（E1–E3 增加 20%–90%）；需要人物持久化字段 |
| B 有抗性才用 FIX | 守方有抗性（人物/宠物）走 FIX，怪物和无抗性走现公式 | 怪物战数值不变 | 两套公式并存，怪物与玩家伤害规则不一致，原版没有这种分支 |
| C 保留现公式，只让魔抗乘伤害 | 现公式里守方抗性 ×(1+N%) | 最省事，不改伤害数值 | 与原版偏离大：熟练度、施法者属性、等级都不进公式；魔抗在这里的效果会远小于原版 |

**推荐 A**，但拆两步上线：先做抗性字段、熟练度字段和成长（不改伤害），再切 FIX 公式，避免 E5 那种“上线即无伤害”。

## 4. 会变的现有测试和用例

以下只按文件名和“是否涉及精灵伤害”判断，**具体断言要实现时逐个确认**：

- `server/battle/formulas_test.go`：`TestDamageCalcHighAttack` / `LowAttack` / `ElementClash` / `AttrAdjustNoneElement`。如果只改精灵的 `magicDamage`，属性相克函数 `attrAdjustWith` 不变，这些测试不应变；待确认。
- `server/battle/magic_test.go`：`TestStatusSpell*`、`TestCureSpell*` 是状态精灵，不走伤害公式，应不变；如果状态精灵的成功率要改成原版（不掷骰），`TestStatusSpellMissesWhenRollFails` 会变（待决定第 2 条）。
- `server/battle/magic_attskill_test.go`、`magic_range_test.go`、`magic_dragon_test.go`、`magic_dodge_test.go`、`battle_test.go`、`auto_plan_test.go`：含攻击精灵，待确认是否断言了具体伤害值。
- `docs/protocol/cases/` 里涉及攻击精灵的用例：`battle_target_type.jsonl`、`battle_clone_target.jsonl`、`party_battle.jsonl`、`pet_skill_merge.jsonl`、`world_item_use.jsonl`（这几个只是 `magic` 字样的初筛，是否断言伤害待确认）。
- 新增：魔抗相关的协议用例（状态名、解状态、推送）要新建，`docs/protocol/cases/` 里还没有 “魔抗”。
- 客户端：状态名显示（`client/` 里的状态栏、战斗界面）需要认 “魔抗、火抗、电抗、冰抗”。

## 5. 实现需求拆分建议

| 序 | 需求 | 内容 | 验收口径 | 依赖 |
|---|---|---|---|---|
| 1 | 单位抗性字段与来源 | 人物持久化 `MagicResist/MagicExp/MagicDefExp/MagicAttExp`；`EquipMagicDef` 从道具参数读 EA/WA/FI/WI/QU；怪物 `LV*0.5`、`LV*0.9` 现算；宠物按 0 | 单元测试：装备加减、限幅 ±100、怪物抗性随等级变；人物快照里能读回 | 无 |
| 2 | 精灵伤害公式 | `magicDamage` 改为 FIX 公式（A 值、`getMagicAdjustInt`、失败 ×0.7）；Magic_ComputeAttExp / DefExp 成长；rand 走 `b.Rnd` 可注入 | E1–E4 数值用例（rand 注入为 0）；E5 在熟练度 0 时数值与原版同量级；成长用例 | 1 |
| 3 | 魔抗与 8 个精灵 | 精灵表加 392/393/417–422（先确认 `tools/gen-magic.py` 不过滤）；`StatusSet` 增魔抗、火抗、电抗、冰抗及数值、回合；挂载规则（互斥、不掷骰，待决定第 2 条）；魔抗乘守方抗性；回合 tick | 魔抗 E4 对照；互斥用例；回合递减用例；火/电/冰抗“只占槽不改数值”（若按第 1 条决定） | 1、2 |
| 4 | 协议与客户端显示 | `docs/protocol.md` 状态名（新增 魔抗/火抗/电抗/冰抗）和用例；`gamedata/magic_effect.go:10` 的状态名表；客户端状态栏、精灵说明文字；`docs/agent-guide.md` 第 6 节分级表 | `go test ./server/webpages`；`cargo test --test <状态相关文件>`；协议用例回放 | 3 |

## 待决定

1. **火/电/冰抗精灵是否照原版做成“只占槽、无数值”。** 原版没有数值效果，精灵描述写的是“提升抗性 10”。推荐：第一期照原版，描述里写明“原版未生效”，等用户决定是否补数值。
2. **魔法状态的挂载规则。** 原版：不掷骰、只与其他魔法状态互斥（`battle_magic.c:1810`）；现服务端 `magicStatus`（`magic.go:140`）走 `statusCheck`（`status.go:56`），有成功率，并且只要身上有任何异常状态就挂不上（`anyBad`）。推荐照原版：魔抗系不掷骰、只和魔法状态互斥。
3. **回合数语义。** 原版每回合 tick 一次（`battle.c:6000`），连击还可能多 tick（`battle.c:7278`）；挂上 turn=3 当回合是否计数，源码阅读无法确定。现服务端魔障/虚弱用 `turn+1`（`magic_debuff.go:11`），这个经验是否适用于魔抗待确认。
4. **攻击熟练度只在施法失败时涨（`battle_magic.c:1261`）。** 这是原版源码，看上去像是条件写反了，但照抄能保证与原版一致。需要用户确认是“照抄”还是“修正”。
5. **宠物的抗性来源。** 源码里没有对宠物抗性或熟练度的赋值，按 0 处理，宠物作为攻击方时（`battle_magic.c:1060` 的 else 分支）伤害会接近 0。待确认是不是原版也如此。
6. **怪物抗性用 `LV×0.5`。** 新服务端怪物表是否有等级字段可以直接用，待确认。
7. **玩家闪避含抗性。** 原版 `BATTLE_MagicDodge` 对玩家加 `抗性×0.15`（`battle_magic.c:896`），这一项要不要保留，影响闪避平衡。
8. **FIX 公式里施法者的无属性分量。** `getMagicAdjustInt` 只清零三个元素，无属性分量照旧保留（`battle_magic.c:3009`），这是原版设计，推荐照抄，但会让无属性攻方的精灵伤害偏大（E1 中无属性分量占大头）。
9. **“全”的目标码 8。** `MAGIC_TARGET_WHOLEOTHERSIDE` 的注释是乱码，需要确认它就是“我方全体”（393、420–422 依赖它）。
10. **`tools/gen-magic.py` 是否会过滤 392、393、417–422。** 按名称搜 `server/` 和 `tools/` 没有“魔法防御”“冰之精灵”，生成脚本第 126–146 行有若干过滤条件，待实现时确认。
11. **FIX 的随机数要可注入。** `rand()%20` 与 `rand()%100`（施法成败）都要接到 `b.Rnd`，否则 E1–E4 无法做确定性用例。

### STAGE-228 核对结果（第 3、6、9、10 条）

- **第 3 条（回合数）**：原版 `MAGIC_MagicStatusChange_Battle` 把 turn 原样写进 `MagicTbl`，不 +1（`gmsv/battle/battle_magic.c:2480` 起，`BATTLE_MultiMagicStatusChange` 在 `battle_magic.c:1816`）。回合末递减 `BATTLE_MagicStatusSeq`（`battle.c:7584`）与 `BATTLE_StatusSeq` 在同一处调用（`battle.c:6000–6003`，连击在 `battle.c:7278–7280`），魔抗与普通状态同速递减。Go 对魔抗不 +1（`server/battle/magic_resist.go`），回合末统一结算（`server/battle/resolve.go` 的 statusTick）。残留差异：原版是每个单位行动前结算，Go 是回合末结算，这是已有模型的差异，本次未改。
- **第 6 条（怪物抗性）**：原版 FIX 分支对 `CHAR_TYPEENEMY` 取 `CHAR_LV*0.5`（`battle_magic.c` 约 1080–1083 行）。Go 的 `NewMonster` 取 `level/2`（`server/battle/unit.go`），结果一致，无需改。
- **第 9 条（目标码 8）**：`MAGIC_TARGET_WHOLEOTHERSIDE` 是 `include/magic_base.h` 第 16–24 行枚举的第 9 项，值为 8。`MAGIC_isTargetValid`（`magic/magic_base.c:460–470`）把它对应到 toindex 20/21（一方）。魔抗全体取施法者一方，依据 `data/magic.txt:153–154` 的说明“我方全体”。
- **第 10 条（生成器过滤）**：`tools/gen-magic.py` 原先的 funcs 表里没有 `MAGIC_MagicStatusChange`，第 143 行 `if fn not in funcs: continue` 会把这 8 个精灵过滤掉。STAGE-228 已加入 funcs 与专用分支；修改前生成器逐字节复现了原 TSV。

## 裁定（用户：需要拍板的都尽量按原版走）

全部“待决定”按原版处理，实现时以原版源码为准：

- 第 1 条：火/电/冰抗只占槽、无数值（原版如此）；第 2 条：魔抗系不掷骰，只与其他魔法状态互斥。
- 第 4 条：熟练度成长照抄原版（含“施法失败才涨”）；第 5 条：宠物抗性/熟练度按 0；第 7、8 条：闪避含抗性、无属性分量照原版保留。
- 第 11 条（随机数可注入）是实现约束，照做。
- 第 3、6、9、10 条是源码事实核对，不是取舍：实现需求动手时先对照原版确认，确认结果写进该需求的评论或本文档。
