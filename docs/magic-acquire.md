# 精灵取得途径与战斗外使用（STAGE-204 研究）

> 性质：研究与拆分文档，只写结论，不改代码、协议（`docs/protocol.md`）和用例。
> 依据：原版服务端 C 源码 `original-data/gmsv`（下称“原版”，只读）；新服务端现状见 `server/`。
> 源码中文注释是 GBK，核对时已转 UTF-8。台词与数字均从源码和数据文件现取。

## 一、结论

1. **原版的“精灵”就是装备上的魔法 ID**：道具表 `itemset.txt` 第 55 列（`ITEM_MAGICID`）非 0 的装备，穿在装备位 0–4 上时，这个装备位就是一个精灵栏位。原版没有 `CHAR_EQUMAGIC` 这个字段，“5 栏位”就是 `CHAR_EquipPlace` 的前五项（头、身、手、装饰1、装饰2）。原版也没有“人物自己的精灵”列表。
2. **原版取得途径，有源码支撑的只有两条**：①穿上带魔法 ID 的装备；②战斗外“精工”（镶宝石，宠物技能 `PETSKILL_Inslay`）把材料道具的魔法 ID 覆盖到武器或防具上。NPC 教学、战斗中学习、任务和转生奖励，在源码里没有对应代码，记为“原版无”（见第二节 2.2）。
3. **新服务端**：`givemagic` 管理命令写入的“人物自己的精灵”（`characters.magics`）是新服务端自己加的，**原版没有**；装备自带的精灵（`equipmagic.go`）与原版一致。“精工”在新服务端**未实现**。
4. **战斗外使用**：原版走 `MU` 请求 → `MAGIC_Use`。战斗外真正有效的只有四类：自己回血（治愈）、单人回血（滋润）、一方全体回血（恩惠，战斗外实际只回自己）、变幻（变身）。复活、解状态、防御类战斗外**扣 MP 后无效果、不提示**；攻击、状态、场地等战斗外不生效。新协议 5.51 目前只做了变幻，其余回 `MAGIC_UNUSABLE`。
5. **拆分**：见第六节，建议 5 个小需求，第一条是“战斗外治疗（自己、宠物、队友）”，它决定新 `magic_use` 请求的对象字段。

## 二、原版取得途径

### 2.1 数据与栏位

- **魔法表** `gmsv/data/magic.txt`（181 行）：每行 `名称,说明,函数名,参数,魔法ID,…`，例如第 1 行 `治愈的精灵 Lv1,耐久力回复 80 (自己),MAGIC_Recovery,80,0,…`，第 3 列是函数名，第 4 列是威力（回复量或复活量），第 5 列是魔法 ID。
- **装备魔法属性**：`itemset.txt` 第 55 列 `ITEM_MAGICID`，第 57 列 `ITEM_MAGICUSEMP`（MP 消耗），列号见 `tools/gen-itemset.py:44`。抽样：`itemset.txt` 第 1323 行“大地的耳饰”魔法 ID 20，第 2222 行“大地之羽”魔法 ID 50，MP 都是 0。
- **栏位**：`include/char_base.h:64-69` 的 `CHAR_EquipPlace` 为 HEAD(0)、BODY(1)、ARM(2)、DECORATION1(3)、DECORATION2(4)，EQUIPPLACENUM(5)。`char/char.c:510-518`：装备位 `i` 上的道具若 `ITEM_canuseMagic` 为真（`item/item.c:1662`，魔法 ID 在魔法表里能查到），就发送状态 `J<i>`。这就是协议 5.8 “装备位号即精灵号”的依据。
- **饰品进哪一位**：耳饰、戒指等饰品进装饰位（3 或 4），具体取决于道具的装备位字段，本次未逐条核对（见第七节）。

### 2.2 取得途径逐条

| 途径 | 原版 | 条件 | 源码 | 说明 |
|---|---|---|---|---|
| A. 穿上带魔法 ID 的装备 | 有 | 装备位与道具的装备位一致 | `char/char.c:510-518`，`item/item.c:1662` | 精灵就是装备本身，卸下或换掉即失去 |
| B. 战斗外“精工”（镶宝石） | 有（`_ITEM_INSLAY` 已开，`include/version.h:144`） | 角色不在战斗中（`BATTLE_CHARMODE_NONE`，`include/battle.h:43`），见 2.3 | `callfromcli.c:1621-1633`（PS 请求），`battle/pet_skill.c:1546-1632`，`item/item_gen.c:1254-1331` | 覆盖，不叠加 |
| C. NPC 教学 | **原版无** | — | `npc/*.c` 里没有魔法学习代码；`data/` 里搜不到任何精灵名 | 待确认：NPC 脚本是否只用道具编号发放（见第七节） |
| D. 道具“学习” | **原版无** | — | `item/item_event.c:1074-1103` 的 `ITEM_useMagic` / `ITEM_useMRecovery` 只在战斗中有效，战斗外分支是空的 | 这两个是战斗中用道具放魔法，不是学习 |
| E. 战斗中学习 | **原版无** | — | `battle/battle_magic.c` 里的“魔法熟练度/抗性”是属性熟练度（`att_magic_lv`），不给精灵 | 不要和精灵混淆 |
| F. 任务、转生奖励 | **原版无（源码未找到）** | — | 未在 `npc/`、`char/` 里找到发放魔法 ID 的代码 | 待确认：数据脚本是否发过带魔法的装备 |
| G. 道具来源（商店、掉落、NPC 发放） | 有，但本次未逐条核对 | — | 候选：`data/bmitem.txt`、`data/gambleitems.txt`、`data/npc/` | 待确认，见第七节 |

### 2.3 精工（途径 B）的规则

- **入口**：客户端 PS 请求 → `lssproto_PS_recv`（`callfromcli.c:1621`）→ `PETSKILL_Use`；`PETSKILL_Inslay` 注册在 `battle/pet_skill.c:54`，实现在 `pet_skill.c:1546`。哪一个宠物技能条目指向它，**待确认**（`data/petskill.txt` 里搜不到 `Inslay`）。
- **前置**：宠物有效；人物 `CHAR_WORKBATTLEMODE` 必须是 `BATTLE_CHARMODE_NONE`（不在战斗中）。
- **输入**：`data` 是 `|` 分隔的道具栏号，最多 4 件（`MAXINSLAY`，`pet_skill.c:1552`）。必须恰好有 1 件武器或防具（道具的 `ITEM_TYPECODE` 含 `INSLAY`），其余是材料。
- **台词**（`pet_skill.c` 原文，白色之外为黄色提示）：
  - 超过 4 件：「精工每次只能选择四个物品。」
  - 选了 2 件以上武器防具：「精工每次只能选择一个武器或防具。」
  - 没选武器防具：「精工必须选择一个武器或防具。」
  - 材料类型码为 `NULL`：「这样物品不适合精工。」
  - 武器防具镶满（`MAXCANINSLAY` = 3 格）：「武器或防具已经镶满了。」（`item_gen.c:1270-1300`）
- **更正（STAGE-232，详见 `docs/inlay.md` 一、二）**：“最多 4 件”是含武器防具在内共 4 件（即最多 3 件材料）；精工除了魔法覆盖，还累加八项数值、覆盖物品函数与参数、重写效果文字；原版数据（`itemset.txt` 第 4、5 列）里没有任何物品启用精工。
- **魔法转移**：材料的 `ITEM_MAGICID > 0` 时，武器防具的 `ITEM_MAGICID` 和 `ITEM_MAGICUSEMP` 被**覆盖**为材料的值（`item/item_gen.c:1325-1331`）。`ITEM_MAGICID` 排在 `worktyp` 最后（`item_gen.c:1265`），攻、防、敏等能力的求和循环不含它（`i < arraysizeof(worktyp)-1`），所以魔法 ID 不会相加。
- **结束**：材料逐件 `CHAR_DelItem`，刷新道具和能力（`pet_skill.c` 约 1616–1630）。

### 2.4 装备栏位的规则（换装、覆盖、卸下）

- **一格一个**：每个装备位最多一个精灵，就是这件装备的魔法 ID。5 个装备位（0–4）最多 5 个精灵。
- **换装 = 覆盖**：同一装备位换上另一件带魔法的装备，栏位里的精灵换成新装备的。
- **卸下 = 消失**：装备位空了，`ITEM_canuseMagic` 不再发 `J<i>`，精灵消失。原版没有“卸下限制”。
- **背包里的带魔法道具不占栏位**：只有穿在身上的才显示为精灵（但见 4.1 第 3 点，`MAGIC_Use` 本身不校验穿没穿）。
- **精工覆盖**：武器防具上原有的魔法 ID 与 MP 直接被替换，不保留。

## 三、新服务端现状

| 项目 | 现状 | 位置 |
|---|---|---|
| `givemagic` 管理命令 | 有：写 `characters.magics` 并发 PostgreSQL 通知；**原版无对应结构** | `cmd/stoneage-admin/main.go:35`、`:204`；`server/character/magics.go:26`（GiveMagic），`:70`（reloadMagics） |
| 精灵清单（栏位合并） | 有：人物栏位优先，否则看同号装备位；同号时人物精灵占住 | `server/character/equipmagic.go:10`（magicSlot）、`:17`（magicSlots） |
| 精灵模板表 | 有：由 `tools/gen-magic.py` 从 `magic.txt` 生成 | `server/gamedata/magic.go`（`MagicFuncHeal` 等常量从 `:29` 起）、`magic.tsv` |
| 战斗外使用 | 只做变幻，其余回 `MAGIC_UNUSABLE` | `server/character/magic_use.go:68-69`（`t.Func != gamedata.MagicFuncMetamo`）；协议 `docs/protocol.md:1296` 起（5.51） |
| 精工 | **未实现**（`server/`、`docs/` 里搜不到 `inslay` / 精工） | — |
| 光精、暗精 MP 打折 | 未实现（新服务端无 family 精灵字段） | 原版 `magic/magic.c:26-66` |

对照：新服务端 `tools/gen-magic.py:53-64` 把原版函数名映射成 `heal`（`MAGIC_Recovery`）、`heal_other`（`MAGIC_OtherRecovery`）、`revive`（`MAGIC_Ressurect`）、`revive_defend`（`MAGIC_ResAndDef`）等。映射本身没问题，问题在于战斗外的分支（第四节）没有照原版做。

## 四、战斗外使用（原版 MU）

### 4.1 请求与分派

1. **请求**：客户端发 `MU`，`lssproto_serv.c:806-826` 解析为 `x, y, array, toindex`，调用 `lssproto_MU_recv`（`callfromcli.c:1312` 起）。
2. **位置校正**：先把人物位置改回服务端记录的位置（防瞬移，`callfromcli.c:1300-1311` 同款写法），再取目标。
3. **道具**：`array` 是道具栏号，`MAGIC_Use` 用 `CHAR_getItemIndex(array)` 取道具（`magic/magic.c:26-36`）。**源码不校验道具是否穿在身上**，背包里的带魔法道具也能发 MU（**待确认**：客户端是否只对装备发送）。
4. **目标**（`callfromcli.c:52-70`，`Callfromcli_Util_getTargetCharaindex`）：`toindex` = 0 是自己；1–5 是自己的宠物 0–4（`CHAR_getCharPet(toindex-1)`）；6–10 是队伍成员 0–4（`CHAR_getPartyIndex(toindex-6)`）；其他或对象不存在为 -1。
5. **MP**：`MAGIC_Use` 取道具的 `ITEM_MAGICUSEMP`（不是魔法表推算值）。光精（`CHAR_FMINDEX ≥ 1` 且 `CHAR_FMSPRITE == 0`，魔法号 0–31、105–107）或暗精（`FMSPRITE == 1`，魔法号 32–81）的 MP 乘 `MP_RATE = 0.7`（`magic.c:23` 定义，`:47-57` 使用）。新服务端没有这套 family 字段。
6. **分派**：`MAGIC_getMagicFuncPointer`（`magic/magic_base.c:422`）按魔法表第 3 列的函数名查出函数指针，然后 `func(charaindex, toindex, marray, mp)`（`magic.c:61`）。
7. **回包**：`lssproto_MU_recv` 是 `void`，源码里没有 MU 的应答发送（**待确认**：客户端是否靠道具变化或消息栏感知结果）。

### 4.2 各函数的战斗外行为

“扣 MP”指 `MAGIC_*` 在判断战斗之前就把 MP 扣掉了。

| 函数（魔法表第 3 列） | 例（魔法表原文） | 战斗外行为（原版） | 扣 MP | 源码 |
|---|---|---|---|---|
| `MAGIC_Recovery` | 治愈的精灵 Lv1：`耐久力回复 80 (自己)`；恩惠的精灵 Lv1：`耐久力回复 50 (一方全体)` | 回自己；**恩惠战斗外也只回自己**（`toindex` 没用到，只校验它有效） | 先扣 | `magic.c:132-154`，`magic_field.c:20-80` |
| `MAGIC_OtherRecovery` | 滋润的精灵 Lv1：`耐久力回复 65 (单人)` | 回 `toindex` 的血，对象可以是自己的宠物或队友（原版未核对是否限定己方） | 先扣 | `magic.c:156-176`，`magic_field.c:82-149` |
| `MAGIC_Ressurect` | 复活精灵 Lv1：`气绝状态回复 100 (单人)` | **无效**：扣 MP 后 `return FALSE`，无台词 | 扣 | `magic.c:324-352`（战斗外分支 `:349` 起） |
| `MAGIC_StatusRecovery` | 净化精灵(毒) Lv1：`毒状态回复 (单人)` | **无效**，同上 | 扣 | `magic.c:275-302` |
| `MAGIC_MagicDef` | 光、镜子、守护的精灵 | **无效**，同上 | 扣 | `magic.c:304-322` |
| `MAGIC_Metamo` | 变幻的精灵 | 变身（新服务端 STAGE-203 已做） | 扣 | `magic.c:448-546` |
| `MAGIC_FieldAttChange`、`MAGIC_AttMagic`、`MAGIC_StatusChange`、`MAGIC_MagicStatusChange`、`MAGIC_StatusChange2`、`MAGIC_AttReverse`、`MAGIC_ResAndDef`、`MAGIC_Barrier`、`MAGIC_Nocast`、`MAGIC_Weaken`、`MAGIC_ToCallDragon`、`MAGIC_AttSkill` | 攻击、状态、场地、召唤龙阵等 | 战斗外 `return FALSE`，**不生效** | 只核对了 `FieldAttChange` 先扣（`magic.c:193-196`）；其余待确认 | `magic.c:178-546` 各函数的 `IsBATTLING` 分支 |

### 4.3 回复量、台词、错误情形

**回复量**（`magic_field.c`）：
- `power = RAND(p×0.9, p×1.1)`，`p` 是魔法表第 4 列（如治愈 Lv1 为 80），结果取整。
- 加到当前 HP 上，封顶为最大 HP（`CHAR_WORKMAXHP`）。
- `include/version.h:137` 定义了 `_MAGIC_REHPAI`，所以**不乘**回复率 `GetRecoveryRate`。

**中毒限制**（`include/version.h:189` 的 `_TYPE_TOXICATION` 已开）：
- 施放者（不是对象）中毒、`CHAR_CanCureFlg(施放者, "HP")` 为假时，自己回血（`MAGIC_Recovery_Field`）直接 `return TRUE`，**无回血、无台词**；
- 单人回血（`MAGIC_OtherRecovery_Field`）则 `return FALSE`。
- 两者都在 MP 扣完之后判断，**MP 已扣**。

**台词**（`magic_field.c`，白色，`CHAR_talkToCli(..., CHAR_COLORWHITE)`）：
- 自己回血：`耐久力回复%d`
- 单人回血，施放者看到：`%s的耐久力回复%d`（`%s` 是对象名）
- 单人回血，对象看到：`藉由%s耐久力回复%d`（`%s` 是施放者名）
- `%d` = `min(回复后血量, 最大血量) − 回复前血量`，可能为 0，照样显示。

**错误情形**：
- MP 不够（`CHAR_MP < mp`）：`return FALSE`，无台词。
- 战斗状态为 `BATTLE_CHARMODE_INIT`：`return FALSE`，MP 不扣。
- **对象无效**：`MAGIC_Recovery` 与 `MAGIC_OtherRecovery` 都是先扣 MP，再判断 `CHAR_CHECKINDEX(toindex)`，无效就 `return FALSE`，**MP 白扣**（`magic.c:145-151`、`:169-171` 附近）。
- 道具栏里没有道具或道具没有魔法 ID：`MAGIC_Use` 直接 `return FALSE`（`magic.c:33-37`）。
- 台词里**没有**“不能使用”之类的提示，失败都是静默的。

**距离与对象限制**：源码里未见 MU 对目标的距离校验，也未见单人回血对倒下宠物（HP 0）的限制（**待确认**，见第七节）。

### 4.4 精工与战斗外的其他说明

- `battle/battle_item.c:974` 的注释写明“复活（光、镜、守）精灵 …但只能在战斗之使用，用完就没了”，即道具版复活只在战斗中生效。这和 4.2 里精灵版复活战斗外无效是一致的。

## 五、对新协议的建议（只写建议，不改 `docs/protocol.md`）

- **请求沿用 `magic_use`（5.51）**，并把对象扩展成原版的三类：自己、宠物、队友。
  - 现在的 `target` 是宠物栏位（metamo 用），建议保留，给治疗加一个明确的对象字段，例如 `{"slot": 0, "to": "pet", "index": 2}` / `{"slot": 0, "to": "member", "index": 1}` / `{"slot": 0}`（自己）。风格上与 5.17 道具、5.19 宠物技能的 `target_type` 对齐：`target_type` 取 `self`、`ally`（`ally` 在战斗外指“宠物或队友”）。具体取值由 `docs/protocol.md` 的修订决定。
- **错误码**沿用已有的：`BAD_REQUEST`、`CHARACTER_DEAD`、`IN_BATTLE`、`MAGIC_UNUSABLE`、`NOT_ENOUGH_MP`、`INVALID_TARGET`、`REQUIREMENT_NOT_MET`、`CONTROLLED_BY_*`。对象无效建议回 `INVALID_TARGET` 且**不扣 MP**（与原版“先扣后判”不同，需要拍板，见第七节 2）。
- **响应**：沿用 metamo 的写法，`data` 里加 `{"heal": {"amount": n, "target": ...}}`；HP 变化靠 `status_changed`（10.10）推送，宠物 HP 变化靠 5.19 的宠物信息推送。台词用原版原文（见 4.3），以 `notice` 推送。
- **复活、解状态、防御类的战斗外行为**：按原版是“扣 MP、无效果、无台词”。这不是好的玩家体验，所以建议**先由产品确认**，再决定是照搬，还是改成 `MAGIC_UNUSABLE` 且不扣 MP（见第七节 2）。

## 六、拆分建议

按依赖顺序，每条一个需求，都是技术或玩法小需求：

1. **战斗外治疗精灵（协议 + 服务端）**：治愈（自己）、滋润（单人）、恩惠（战斗外只回自己，照原版）。内容：扩展 `magic_use` 的对象字段；回复量 `RAND(p×0.9, p×1.1)` 封顶；中毒时自己回血无效（按原版）；台词按 4.3 原文。验收：用 `docs/protocol/cases/` 新增用例；服务端 `go test ./server/character/`；对象无效的行为按拍板结果。依赖：无。
2. **复活、解状态、防御的战斗外行为（拍板 + 实现）**：依赖 1 的协议。内容：按原版（扣 MP、无效果）或改为拒绝；需要先定。验收：按拍板结果写用例。依赖：1 与产品确认。
3. **客户端角色页精灵清单加“使用”**（单独一条，用户要求）：`docs/ui.md` 角色页 C 的精灵清单上加“使用”动作，流程是选精灵 → 选对象（自己 / 宠物 / 队友）→ 发 `magic_use`；空栏位、变幻以外的精灵给出 `MAGIC_UNUSABLE` 的提示。验收：客户端场景用例 + 模拟服务端。依赖：1。
4. **精工（镶宝石，PETSKILL_Inslay）**：新协议指令（或复用 pet_skill 的 PS 形式），规则按 2.3 并以 `docs/inlay.md`（STAGE-232）为准：一件武器防具加最多 3 件材料（共 4 件）、3 格永久上限、数值累加与魔法 ID/MP 覆盖、战斗中拒绝。依赖：道具模型与装备位（已有）。验收：协议用例覆盖 4 条台词和覆盖规则。依赖：先确认 2.3 的入口（待确认 1）。
5. **取得途径盘点（研究，无代码）**：核对道具来源（商店、掉落、NPC 发放）与 `itemset.txt` 第 55 列非 0 的装备清单，确认饰品进哪个装备位，并回答第七节的待确认项。依赖：无。可以与 1 并行。

不拆的：光精、暗精的 MP 打折（原版 family 系统，新服务端没有，需要单独立项，目前不建议）。

## 七、待确认

> STAGE-233 已逐条核对，结论与源码依据见第八节 8.1。

1. **精工的入口**：`PETSKILL_Inslay` 对应哪个宠物技能条目（`data/petskill.txt` 搜不到），客户端在哪里发 PS 请求。
2. **对象无效时的 MP**：原版是先扣后判（MP 白扣）。新协议建议先判后扣，还是照搬？需要拍板。复活、解状态的战斗外行为也一起拍板（见第五节）。
3. **背包中的魔法道具能否 MU**：源码不校验穿戴，客户端是否限制未知。
4. **MU 有没有应答**：`lssproto_MU_recv` 无返回值，未见 MU 应答发送。
5. **饰品进哪个装备位**：取决于道具装备位字段，未核对 `itemset.txt` 的装备位列。
6. **战斗外先扣 MP 的函数**：只核对了 `Recovery`、`OtherRecovery`、`FieldAttChange`、`MagicDef`、`StatusRecovery`、`Ressurect`，`StatusChange` 系列、`AttMagic` 等未逐个核对。
7. **对倒下的宠物（HP 0）用单人回血**：原版未检查，HP 增加后宠物是否恢复为可战斗，取决于宠物状态机，待新服务端确认。
8. **MU 的距离校验**：源码里未见，待确认是否有其他地方限制。
9. **新服务端恩惠的 `target_type`**：`gen-magic.py` 生成的恩惠是否是“全体”语义；若是，战斗外应只回自己（与原版一致），需要在实现时核对。
10. **原版 NPC 教学与任务、转生奖励**：`npc/`、`char/` 里没找到，`data/npc/` 的脚本没有逐条核对，记为“原版无（源码未找到）”，不是已证明不存在。

## 八、STAGE-233 核对结论（第七节待确认项）

> 性质：源码与数据核对，未改代码和协议。路径相对 `original-data/gmsv/`（下称 gmsv/）。GBK 注释已转 UTF-8 读取。
> 核对范围：第七节 1–10 条逐条给结论；第 8.2 节列出 `itemset.txt` 第 55 列非 0 的装备及来源；第 8.3 节是本次用到的入口索引。

### 8.1 第七节逐条结论

1. **精工入口（PETSKILL_Inslay）**
   - 注册：`battle/pet_skill.c:53-54`（`#ifdef _ITEM_INSLAY`，`{"PETSKILL_Inslay", PETSKILL_Inslay, 0}`），实现在 `pet_skill.c:1546`。
   - 入口链路：客户端 PS 请求 → `callfromcli.c:1621-1632` `lssproto_PS_recv` → `pet_skill.c:505` `PETSKILL_Use`，按宠物技能 ID 查 `data/petskill.txt` 的函数名列，再取函数指针。
   - 结论：`data/petskill.txt`（32 行）里**没有任何一行引用 `PETSKILL_Inslay`**（`grep` 0 命中）。原版数据里没有挂接精工的宠物技能条目，精工是代码存在、数据未启用的功能。哪只宠物带这个技能、客户端哪个按钮发 PS，原版数据和源码里都没有，**待确认**（客户端不在本仓库）。

2. **对象无效时的 MP**
   - `magic/magic.c:142-149`（`MAGIC_Recovery` 战斗外）：先判 MP（142），先扣 MP（143-144），之后才判 `toindex`（149）。`magic.c:165-171`（`MAGIC_OtherRecovery`）同样。
   - 结论：原版“先扣后判”属实，对象无效时 MP 白扣、无台词。复活、解状态、防御类沿用 4.2 的结论（前次已核对）。裁定已按原版走，本条只补源码位置。

3. **背包中的魔法道具能否 MU**
   - `callfromcli.c:1297-1318` `lssproto_MU_recv` 只把客户端坐标改写为服务端坐标（不拒绝），然后把客户端给的道具栏号 `array` 原样交给 `MAGIC_Use`。
   - `magic/magic.c:33` 用 `CHAR_getItemIndex(charaindex, haveitemindex)` 取道具，不区分装备位 0–4 与背包栏；`magic.c:35-36` 只看 `ITEM_MAGICID`。
   - 结论：**原版服务端不校验穿没穿**，背包里的带魔法道具也能 MU。客户端是否限制：原版 2.5 客户端不在本仓库，未知。

4. **MU 有没有应答**
   - `lssproto_MU_recv` 返回 void（`callfromcli.c:1297`），gmsv 中搜不到 `lssproto_MU_send`（0 命中）。
   - 结论：原版**不发 MU 应答**，结果靠状态推送和台词。

5. **饰品进哪个装备位**
   - `item/item.c:1214-1252` `ITEM_getEquipPlace` 按道具类别（`itemset.txt` 第 19 列，枚举见 `include/item.h:11-43`）决定装备位：
     - 手镯 8、乐器 9、项链 10、戒指 11、腰带 12、耳环 13、鼻环 14、护身符 15 → `CHAR_DECORATION1`（3）。
     - 头盔 6 → HEAD（0）；铠甲 7 → BODY（1）；斧 1、棍 2、矛 3、拳 0、回旋镖 17、投掷斧 18、投掷石 19、弓 4 → ARM（2）。
     - 盾牌 5 的分支在 `item.c:1223` 被注释掉，返回 -1，即**当前代码不能穿盾**（按代码如此，未在客户端验证）。
   - `CHAR_DECORATION2`（4）`ITEM_getEquipPlace` 从不直接返回。第二个饰品位由穿戴逻辑选择：双击穿戴 `char/char_item.c:597-631`，拖到指定格 `char_item.c:372-400`。装饰 1 已占且类别不同时放进装饰 2；**同类别不能同时戴**（`char_item.c:384-396`，CANNOTEQUIP2 / CANNOTEXCHANGE）。
   - 结论：饰品默认进装饰 1（3）；与装饰 1 类别不同的第二件饰品进装饰 2（4）。

6. **战斗外先扣 MP 的函数**（`magic/magic.c` 行号）
   - `MAGIC_StatusChange`（209-222）：战斗外先扣 MP（216），再 `return FALSE`（220）。结论：扣 MP、无效果、无台词。
   - `MAGIC_AttMagic`（425-441）：玩家先扣 MP（435，宠物不扣），战斗外 `return FALSE`（441）。结论同上。
   - `MAGIC_Metamo`（448-469）：战斗外不返回，继续变身（与 4.2 一致：变幻战斗外有效）。
   - `MAGIC_Recovery`（132-153）、`MAGIC_OtherRecovery`（156-175）：战斗外有效（回血）。
   - 补读（STAGE-233 第二轮，战斗外分支逐个核对，均为“扣 MP、无效果、无台词”，除非注明）：
     - `MAGIC_FieldAttChange`（178-205）：扣 MP（195-196），战斗外 `return FALSE`（203）。
     - `MAGIC_MagicStatusChange`（225-241）：扣 MP（235），战斗外 `return FALSE`（239）。
     - `MAGIC_StatusRecovery`（275-302）：扣 MP（292-293），战斗外 `return FALSE`（300）。
     - `MAGIC_MagicDef`（304-320）：扣 MP（314），战斗外 `return FALSE`（318）。
     - `MAGIC_Ressurect`（324-351）：扣 MP（341-342），战斗外 `return FALSE`（349）。复活战斗外无效，与 4.2 一致。
     - `MAGIC_AttReverse`（354-381）：扣 MP（371-372），战斗外 `return FALSE`（379）。
     - `MAGIC_ResAndDef`（386-413）：扣 MP（403-404），战斗外 `return FALSE`（411）。
     - `MAGIC_AttSkill`（547-587）：**例外**，`IsBATTLING` 为假时直接 `return FALSE`（555），不扣 MP；只有技能函数成功后才扣（584）。
     - `MAGIC_Barrier`（590-661）：扣 MP（616-617），战斗外 `return FALSE`（659）。
     - `MAGIC_Nocast`（664-738）：扣 MP（690-691），战斗外 `return FALSE`（736）。
     - `MAGIC_Weaken`（742-768）：扣 MP（759-760），战斗外 `return FALSE`（766）。
     - `MAGIC_ToCallDragon`（772-792）：扣 MP（786），战斗外 `return FALSE`（791）。
   - 小结：战斗外真正有效的只有 `MAGIC_Recovery`（自己，见第 9 条）、`MAGIC_OtherRecovery`（单人）、`MAGIC_Metamo`（变幻）。其余 14 个函数战斗外都是扣 MP 后无效果（`MAGIC_AttSkill` 例外，它不扣）。除 `MAGIC_AttSkill` 外，函数开头都先判 `BATTLE_CHARMODE_INIT`，然后判 MP，再扣 MP。

7. **对倒下宠物（HP 0）用单人回血**
   - `magic/magic_field.c:82-111` `MAGIC_OtherRecovery_Field` 不检查 `CHAR_ISDIE`，直接把 HP 加上回复量并封顶 `WORKMAXHP`（101-102）；`magic.c:156-175` 顶层也没有 ISDIE 判断。
   - 补充（宠物“倒下”状态）：
     - `CHAR_ISDIE` 的置位：`char/char_event.c:405-466` `CHAR_die` 置 1（463）。它在 gmsv 里没有 C 代码直接调用，只在脚本函数表里以 `core_Dying` 注册（`function.c:126`），所以宠物何时被置 1，**取决于脚本调用，待确认**。战斗内只有玩家在 HP≤0 时置 1（`battle/battle_magic.c:1250-1253`，条件 `CHAR_TYPEPLAYER`）。
     - `CHAR_ISDIE` 的清除：`battle/battle_magic.c:600-619`（复活类魔法，对 ISDIE 为真的对象回血并清 ISDIE，619）；`char/char_event.c:391-400` `CHAR_playerresurrect` 清 0（395），但它没有 C 代码调用者。
     - 读取处：`battle/battle.c:1336-1337`（宠物出战只看 `!ISDIE` 且 `HP>0`）；`char/pet.c:395`（设为默认宠物时只看 ISDIE）；`battle/battle_command.c:845`（ISDIE 时菜单关闭）。
     - 结论：`MAGIC_OtherRecovery_Field` 加血后 HP>0；若宠物 ISDIE 为假，就能出战（`battle.c:1336-1337`）。若 ISDIE 为真，则要靠复活类魔法清除，单人回血救不了。宠物 ISDIE 的实际置位路径取决于脚本调用，**待确认**。

8. **MU 的距离校验**
   - `lssproto_MU_recv`（`callfromcli.c:1297-1318`）只有位置改写，没有目标距离判断；`magic/magic.c` 里也没有距离判断。目标由 `Callfromcli_Util_getTargetCharaindex` 取得，其内部**未读**。
   - 补读：`callfromcli.c:52-71` `Callfromcli_Util_getTargetCharaindex`：`toindex` 为 0 取自己；1–5 取宠物栏（`CHAR_getCharPet(fd, toindex-1)`，无效则 -1）；6–10 取队伍成员（`CHAR_getPartyIndex(fd, toindex-6)`）；其他值返回 -1。全程没有距离、地图、战斗状态判断。
   - 结论：在 gmsv 的 MU 处理链路里**没有距离限制**：`lssproto_MU_recv`（1297-1318）没有距离判断，`MAGIC_OtherRecovery`（156-175）和 `MAGIC_Recovery`（142-151）也没有。`CHAR_getPartyIndex` 内部是否限制地图未读，**待确认**。目标为 -1 时，`MAGIC_OtherRecovery_Field` 在检查目标前已扣 MP（171），与 2.2 同样是“先扣后判”。

9. **新服务端恩惠的 target_type**
   - 原版一方全体回血在 `magic.txt` 中是 `MAGIC_Recovery` 且第 7 列 target 为 8（共 7 行）。战斗中走 `MAGIC_Recovery_Battle`（`magic.c:145-147`）；战斗外走 `MAGIC_Recovery_Field(charaindex, marray)`（`magic.c:150`，`magic_field.c:20`），只回自己。
   - 新服务端：`tools/gen-magic.py:163` 把 target 8 / 11 生成为 `kind = "all"`，这是战斗语义。**战斗外实现要按上面的分支只回自己，不能直接套 `all`。**

10. **NPC 教学、任务、转生奖励**
    - 全部源码中读取 `ITEM_MAGICID` 的地方只有 `char/char.c:2418`（显示/状态）、`item/item_gen.c:1265,1321-1325`（精工覆盖的字段列表和条件）；写入只有精工覆盖（`item_gen.c:1325-1331`）。没有“把魔法 ID 写给角色”的代码路径（搜 `MAGICID`、`ITEM_MAGIC` 与 `npc/`、`char/`、`item/`、`battle/`）。
    - 结论：原版 NPC、任务、转生**只能通过发道具间接给精灵**，见 8.2 的来源表。“原版无”成立（不是“未找到”）。

### 8.2 `itemset.txt` 第 55 列非 0 的装备清单与来源

- 口径：`data/itemset.txt` 第 55 列（`ITEM_MAGICID`，编号从 0 起，`gen-itemset.py` 同口径）非 0 且非空，共 **7874 件**，涉及 **145 个魔法 ID**。第 16 列为物品 ID，第 19 列为类别。
- 按类别（第 19 列）计数：

| 类别（第 19 列） | 件数 | 装备位（8.1-5） |
|---|---|---|
| 0 拳、1 斧、2 棍、3 矛、4 弓 | 594 / 592 / 599 / 596 / 591 | ARM（2） |
| 6 头盔 | 1188 | HEAD（0） |
| 7 铠甲 | 1737 | BODY（1） |
| 8 手镯 / 9 乐器 / 10 项链 / 11 戒指 / 15 护身符 | 7 / 109 / 3 / 37 / 48（饰品共 204） | DECORATION1（3），第二件进 DECORATION2（4） |
| 16 其他 | 1 | 不穿戴 |
| 17 回旋镖 / 18 投掷斧 / 19 投掷石 | 591 / 591 / 590 | ARM（2） |

- 来源（按 `data/` 与 `npc/` 脚本里的字面 ID 核对，即物品 ID 在来源里出现过；一件装备可能同时出现在多个来源）：

| 来源 | 数据/代码位置 | 带精灵的装备件数 |
|---|---|---|
| 商店售卖 | `data/npc/**` 的 `ItemList:`（genout 等）→ `npc/npc_itemshop.c:569-572` | 346 |
| 商店特价 | `data/npc/**` 的 `special_item:` | 5 |
| 随机给（宝箱类） | `data/npc/**` 的 `GetRandItem:` → `npc_eventaction.c:336-340`（`NPC_ActionTreasureRandItemGet` :2041）、`npc_exchangeman.c:2285-2289`（`NPC_RandItemGet` :2487） | 204 |
| 脚本直接给 | `data/npc/**` 的 `AddItem:` → `npc_eventaction.c:240-241`（`NPC_ActionAddItem` :1151） | 0（7 个 ID 都不带精灵） |
| 游乐场积分兑换 | `data/gambleitems.txt`（格式 `名称 ID 积分 类别`），`configfile.c:752-770`、`npc/npc_gamblebank.c:456-486` | 7 |
| 敌人身上的道具（掉落） | `enemy.txt` ITEM1–10，`char/enemy.c:1109-1123`；战后分给玩家 `battle/battle.c:4715-4745`、领取 `battle.c:3241-3262` | 0（409 个 ID 中无带精灵的） |
| 初始装备（角色创建） | `char/char_data.c:805-819`（按技能发）、`char_data.c:832`（法师、祭司另发 802） | 6 个 ID 带精灵：601、101、201、501、17、802 |
| NPC 直接发放 | `npc/npc_newnpcman.c:424`（ID 1292 真．灵力铠，精灵 24） | 1 个 ID |
| `bmitem.txt`（“请拿…来换”交换表，36 行） | 代码里没有任何读取它的地方（grep 无引用） | 奖励 ID 中 0 |

- 合计（STAGE-233 第二轮修正，按结构化来源重新归类，四类互不重叠，按顺序取第一个命中的类）：

| 归类 | 件数 | 判断依据 |
|---|---|---|
| 合成产物 | 6621 | `itemset.txt` 第 83 列（`canmergeto`）非 0。合成入口 `battle/pet_skill.c:1262-1272` `PETSKILL_Merge` → `item/item_gen.c:1042` `ITEM_mergeItem_merge` → `item_gen.c:1155` `ITEM_makeItemAndRegist(ret)`。`data/petskill.txt` 有 2 行 `PETSKILL_Merge`。配方在第 84–93 列（原子名与数值）。 |
| 结构化来源（不是产物） | 479 | 即上表的商店、宝箱、游乐场、敌人、初始装备、NPC 发放（不含已归入合成产物的 6 件）。 |
| 来源未定 | 774 | 不是产物，也不在上表任何来源里。其中名字为“精灵名前缀 + Lv1 + 基础武器或防具”的占大多数（特洛西 155、特洛金 147、米鲁菲 80、提欧 56、迪所斯 25、贝所金 25、莱伊萨 21、贝洛萨 21、威金娜斯 19、贝诺美斯 17、莱伊卡 17、芙雷雅 15 等，前 12 个前缀共 618 件）；另有 19646–19695 一段（如 地震、火球、变幻的衣服 Lv1–3），见下。 |
| 小计 | 7874 | 6621 + 479 + 774 = 7874 |

- 来源未定的候选与核对结果（均未找到能发放这些 ID 的代码或数据）：
  - NPC 模板 `haveitem`：源码有 `npc/npcgen.c:285-300` 的发放逻辑，但模板读取只认 `itm=` 行（`npc/npctemplate.c:869-903`）。`data/npc` 下 0 行 `itm=`，所以随附数据没有启用这一来源。
  - 攻击魔法的占位道具：`battle/pet_skill.c:1320` 的注释写“magic id 预设为火魔法等级一，item index 预设为 19659”，说明 19646–19671 这一段是战斗用的占位道具编号，不是可取得的物品（**待确认**）。
  - `item/item_event.c` 的变身道具：`变幻的衣服 Lv1–3`（19693–19695）在 `item_event.c` 里有字面引用，判断是变身功能的道具，**待确认**是否可取得。
  - `npc/npc_newnpcman.c:421-424`：道具 19646（灵力铠占位）在 NPC 处理中被换成 1292，见 8.2 的 NPC 直接发放。
  - 结论：这 774 件在原版源码和随附数据里**找不到取得途径**，按“来源未定”记录，不要当作可取得的装备。

- 完整清单：本表只给汇总。逐件清单用下面的命令生成（只读，不落盘）：

```python
# 在仓库根目录运行；输出：物品 ID、名称、类别、魔法 ID、MP
import sys
rows = open("original-data/gmsv/data/itemset.txt", "rb").read().decode("gbk", "replace").splitlines()
for line in rows:
    f = line.split(",")
    if len(f) > 57 and f[55].strip() not in ("", "0") and not line.startswith("#"):
        print(f[16].strip(), f[0].strip(), f[19].strip(), f[55].strip(), f[57].strip(), sep="\t")
```

### 8.3 本次用到的入口索引（与第二节 2.2 的 C、F、G 行对应）

- 魔法函数分布（`data/magic.txt`，181 行）：`MAGIC_StatusChange` 55、`MAGIC_AttMagic` 25、`MAGIC_StatusRecovery` 23、`MAGIC_FieldAttChange` 21、`MAGIC_Recovery` 13、`MAGIC_MagicStatusChange` 8、`MAGIC_OtherRecovery` 6、`MAGIC_AttSkill` 5、`MAGIC_Metamo` 5、`MAGIC_Ressurect` 4、`MAGIC_ToCallDragon` 4、`MAGIC_MagicDef` 3、`MAGIC_ResAndDef` 3、`MAGIC_AttReverse` 2、`MAGIC_Weaken` 2、`MAGIC_Barrier` 1、`MAGIC_Nocast` 1。
- 本地未发现的入口：`data/bmitem.txt` 未被代码读取；`VipItem`（`npc_eventaction.c:245`）在 `data/npc` 中 0 处使用。
- 本次**未核对**：`item/item_event.c` 礼物表（2547、2624）与宝箱分支的完整数据；`CHAR_getPartyIndex` 的地图限制；`CHAR_die` 被脚本调用的位置（`core_Dying`）；客户端的穿戴与 MU 限制。

## 裁定（用户：需要拍板的都尽量按原版走）

- 第七节 2：对象无效时照原版“先扣 MP 后判断”；复活、解状态、防御类战斗外照原版“扣 MP、无效果、无台词”。因此第六节的 1 和 2 合并为一个需求实现，不再单独拍板。
- 其余待确认项（1、3–10）是源码事实核对，由对应需求动手时对照原版确认，结论补进该需求评论或本文档。
