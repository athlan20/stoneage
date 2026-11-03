# STAGE-152 遇敌区域与按区域遇敌（协议 5.50 / 5.51）

## encounter_areas（查看，in_world，player / agent 均可）

只读角色当前地图。请求 `{}`；响应 `{"floor":100,"areas":[...],"server_time_ms":0}`。
`floor` 是读取时的地图。不属于任何可选区域的格子不报错——就是 `areas` 为空。

每项字段：

| 字段 | 含义 |
|---|---|
| `id` | 稳定区域 ID（地图-原编号-矩形，同键重复追加序号）。跨会话稳定，不是列表序号 |
| `number` | 原版 `encount.txt` 的区域编号（不同地图上可能重复） |
| `rect` | `{x1,y1,x2,y2}` 原始矩形，已排成小/大、并裁到地图边界 |
| `anchor` | 代表落点 `{x,y}`：区域内、确实由本区域生效、且不是普通出口触发格，按 x 再 y 取第一个 |
| `available` | 现在能不能开战 |
| `unavailable_reason` | 不能开战的原因（可用时为 null） |
| `max_enemies` | 一场的有效人数上限 = min(区域上限, 该图最佳敌群的同场上限之和) |
| `level_min` / `level_max` | 该区域所有候选敌人的等级覆盖范围 |
| `enemies` | 候选敌人，见下 |
| `required_items` / `forbidden_items` | 道具条件（没有时为空数组） |
| `level1_pets` | 本区域能遇到的 1 级宠物名（STAGE-286，只读新增）。数据来自 1 级宠物目录（5.62 的可遇点，同区域其他 1 级宠物也算），按目录顺序去重；没有宠物点的区域为空数组 `[]`，不是 null。列表标记和详情用它，不影响 `available`、开战和人数 |

`enemies` 每项：`{"id":119,"name":"昆伊","level_min":1,"level_max":1,"create_max":2,"weight":50,"group_id":88}`。
`weight` 是「区域给该敌群的权重 × 敌群给该敌人的权重」，只用来判断谁更常见，**不是百分比**。
同名不同等级是不同变体，分别列出；随机怪组已展开成实际候选。列表不承诺每场候选都会出现。

## 什么算一个可选区域

- 数据链 `encount → group1 → enemy → 模板`。区域必须有正 `zorder`、有效地图与整数坐标。
- 生效范围按闭区间包含、最大 `zorder`、平局文件靠前计算，再裁到地图边界（尺寸见 `server/gamedata/mapsizes_gen.go`）。
  **高优先级的安全区也参与覆盖判断**：被它盖住的下层怪区不会因为安全区被过滤而重新暴露。
- 完全被更高优先级盖住、或裁完为空、或找不到代表落点的区域不列出。
- **CEP 上限 0 的原始安全区**（城镇、室内）不列出，`encounter on` 也不能在它里面开战。
- 必须能展开到正权重、有效变体、正同场上限、存在的模板；缺失的敌群按新服务端规则剔除并重新归一权重，
  一个都剩不下就排除。敌人变体的模板缺失时跳过该变体。
- 道具条件让候选暂时为空时**仍然列出**，`available=false`，原因写明缺少/不得持有哪个物品。
- 预览与开战共用同一套候选筛选，不会出现“列表里能打、开战时没敌人”。
- 人数是随机的 1..有效上限，`max_enemies` 只是封顶，不是每场满员。
- 原始区域被高优先级切碎后仍算**一个**选项。

## encounter_start（操作，in_world，必须拥有控制权）

请求 `{"source_floor":100,"area_id":"100-21-582-399-783-560","mode":"once"}`，`mode` 取 `once` / `infinite`。
`source_floor` 必填（整数，就是读取 `encounter_areas` 时所在的 floor），缺省回复 `BAD_REQUEST`，不会按 0 层去比对。
客户端不能指定坐标；落点、敌人、数量全部由服务端解析。

一次角色操作锁内检查：控制权、非战斗、当前地图、区域归属本图、区域当前可用（敌人 + 道具条件）、代表落点仍由该区域生效。
成功时**先把角色定位到代表落点**，再立刻开第一场战斗：

推送顺序固定为 `position_changed` → `battle_start` → `battle_turn` → `encounter_changed` → 响应。
玩家和 agent 收到一致的位置、战斗与计划状态。同图定位要关掉旧 NPC 窗口，旧操作上下文失效。

失败没有任何副作用（位置、窗口、原计划、自动战斗都不变）：
`BAD_REQUEST`（参数/模式错）、`POSITION_CHANGED`（地图变了）、`ENCOUNTER_AREA_NOT_FOUND`（本图没有这个 ID）、
`ENCOUNTER_AREA_UNAVAILABLE`（当前不可用，message 说明）、`IN_BATTLE`（战斗中拒绝切区/换次数，但 `encounter off` 仍然允许）、
控制权沿用 `CONTROLLED_BY_AGENT` / `CONTROLLED_BY_PLAYER`。

## 次数与停止

- `once`：以**成功开启的战斗**计数，不按回合、敌人数或胜负。第一场建立即消耗额度，之后无论赢、逃、捕获还是输都不再开下一场。
- `infinite`：沿用每场结束后的默认间隔（2 秒，配置 `STONEAGE_ENCOUNTER_INTERVAL_MS`），在所选区域按原数据随机抽敌。
  连续遇敌不会顺带打开自动战斗。
- 两场之间再发 `encounter_start` 是**原子替换**：先取消旧计划的计时器，再开新的一场；`once` 替换 `infinite` 时不残留旧计时。
- `encounter off` 停止：只取消后续场次，当前这场照常打完，角色不回原坐标。
- 打输回城、任何位置改变（含同图传送）、离开世界、角色离线、控制权转交 → 清除计划与旧计时器；
  控制权转交同时关掉开关（推 `encounter_changed`，`reason: "control"`，STAGE-311），当前这场照常打完。
  旧 timer 即使已排队也不得开新战。断线重连不自动恢复玩家此前发起的计划。
- 每场开打前复核区域、位置与道具条件；抽不出敌人时关闭并推 `reason: "no_enemy"`。

## 状态字段（encounter_changed / status / enter_world）

`encounter` 布尔字段保留，新增并列的权威字段，`null` 表示没有计划：

| 字段 | 含义 |
|---|---|
| `encounter_area_id` | 当前计划的区域 ID |
| `encounter_area_number` | 原编号 |
| `encounter_mode` | `once` / `infinite` |
| `encounter_continues` | 之后还会不会再开场（`once` 打完第一场即 false） |

`encounter_changed` 的 `reason`：`lose` / `warp` / `no_enemy` / `manual`（手动 off）/ `once_done`（单次打完）/ `area_unavailable`。
自动关闭同时推一条 `notice`（`manual` 与 `once_done` 除外）。

## 刷新

进入世界、换图、重连、同图定位后重查；背包变化后重查受道具条件影响的可用性；`F5` 重查焦点列。
响应按请求 id + 地图 + 位置/连接世代隔离，A→B→A 的迟到响应不能污染列表。操作前服务端总是重校验。
