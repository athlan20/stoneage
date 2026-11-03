# 石器时代 2.5 运维手册

本目录是运维操作手册（`docs/` 是开发约定，这里是运维照着做的步骤）。目前收录：**批量发放物资**（`stoneage-admin grant`，STAGE-241；发石币 STAGE-310）、**重启服务端要优雅退出**（第 10 节，STAGE-290）。

所有命令在仓库根目录执行。下面的命令默认读取 `.env`（即当前指向的库）。

## 0. 先知道的几件事

- **先确认 `.env` 指向哪个库**（开发库还是正式库），再执行任何带 `--yes` 的命令。
- 批量发放**没有自动回滚**，发出去的东西只能人工扣回，见第 7 节。
- 发放命令不需要服务端在运行。角色在线时会收到推送当场到账，离线的下次进入世界可见，见第 6 节。
- 发放记录写在 `runtime/grants/<批次号>.jsonl`（`runtime/` 不入库）。它就是这次发放的凭据，不要删。
- 名单文件、账号清单都用 UTF-8 保存，一行一个。

## 1. 发放前检查清单

- [ ] 有审批单或工单，写明物资、数量、人群和原因；把批次号写进工单。
- [ ] 批次号只用字母、数字、`-`、`_`，最长 64 个字符。**一个批次号只对应一种物资和一个数量**，不要拿同一个批次号发别的东西（命令会拒绝，但别指望它替你记）。
- [ ] 物资编号已经查到并核对过名字（第 2 节）。
- [ ] 人群名单已生成，先预演，看人数和前 20 个名字是否符合预期。
- [ ] 确认 `.env` 指向的库。
- [ ] 同一个批次号不要在两个终端同时执行。

## 2. 物资编号去哪里查

编号一律以 `server/gamedata/` 下的表为准（UTF-8，第一行是注释，列以 Tab 分隔）。

| 物资 | 参数 | 查表 | 说明 |
| --- | --- | --- | --- |
| 物品 | `--kind item --id <编号> [--count N]` | `server/gamedata/itemset.tsv`：第 1 列编号，第 2 列名字 | 数量默认 1，放进第一个空的背包格（5–19 号，共 15 格）。 |
| 宠物 | `--kind pet --id <编号> [--level N]` | `server/gamedata/petbase.tsv`：第 1 列编号，第 2 列名字，第 5 列 PETFLG，第 7 列等级上限 | PETFLG 为 0 的不是宠物，不能发；等级上限为 0 表示不限。等级默认 1，最高 160，超过模板上限报错。放进第一个空的宠物栏位。 |
| 精灵 | `--kind magic --id <编号>` | `server/gamedata/magic.tsv`：第 1 列编号，第 2 列名字 | 每次发 1 只，放进第一个空的精灵栏（共 5 栏）。 |
| 能力点 | `--kind point --points N` | 没有编号 | 发到的是“未分配点数”，玩家自己用 `addpoint` 加到能力上。 |

按名字查（把关键字换掉）：

```sh
awk -F'\t' '$2 ~ /斧/ {print $1, $2}' server/gamedata/itemset.tsv
awk -F'\t' '$2 ~ /乌力/ && $5 != 0 {print $1, $2, "上限", $7}' server/gamedata/petbase.tsv
awk -F'\t' '$2 ~ /岩石/ {print $1, $2}' server/gamedata/magic.tsv
```

同名不同编号很常见（如 `1 贝诺美斯Lv1斧头`、`2 威金尼尔Lv1斧头`），以第 2 列的完整名字为准，不要只看关键字。

## 3. 三种人群的命令样例

名单文件放在 `runtime/lists/`（不入库）。下面的样例假设已经建好名单。

### 3.1 指定角色名清单

```sh
mkdir -p runtime/lists
cat > runtime/lists/comp-01.txt <<'EOF'
# 每行一个角色名，# 开头是注释
角色名1
角色名2
EOF

go run ./cmd/stoneage-admin grant --kind item --id 0 --count 1 --batch comp-01 --names-file runtime/lists/comp-01.txt          # 预演
go run ./cmd/stoneage-admin grant --kind item --id 0 --count 1 --batch comp-01 --names-file runtime/lists/comp-01.txt --yes    # 正式
```

### 3.2 指定账号清单（展开为这些账号下的所有角色）

```sh
cat > runtime/lists/comp-acc.txt <<'EOF'
# 每行一个账号名，账号名按小写比较
账号1
账号2
EOF

go run ./cmd/stoneage-admin grant --kind point --points 3 --batch comp-acc-01 --accounts-file runtime/lists/comp-acc.txt          # 预演
go run ./cmd/stoneage-admin grant --kind point --points 3 --batch comp-acc-01 --accounts-file runtime/lists/comp-acc.txt --yes    # 正式
```

### 3.3 全部老角色（可按注册日期、等级过滤）

```sh
go run ./cmd/stoneage-admin grant --kind item --id 0 --count 1 --batch old-30-01 --all --registered-before 2026-06-01 --min-level 30          # 预演
go run ./cmd/stoneage-admin grant --kind item --id 0 --count 1 --batch old-30-01 --all --registered-before 2026-06-01 --min-level 30 --yes    # 正式
```

- `--registered-before YYYY-MM-DD`：账号注册时间早于该日 0 点（本机时区）。
- `--min-level N`：角色等级不低于 N。
- 两个过滤都不写，就是全部角色。
- 过滤条件不会排除禁用的账号。需要排除的话，先用 `go run ./cmd/stoneage-admin accounts` 看清楚再决定。

### 3.5 发石币（`--kind gold`，STAGE-310）

用 `--amount` 给石币数量，人群参数与其他物资相同（名单 / 账号 / 全体）。在线、离线角色都能发：在线角色收到通知后，石币立即增加，不用等下次进入世界。

```sh
go run ./cmd/stoneage-admin grant --kind gold --amount 5000000 --batch comp-gold-01 --names-file runtime/lists/comp-gold.txt          # 预演
go run ./cmd/stoneage-admin grant --kind gold --amount 5000000 --batch comp-gold-01 --names-file runtime/lists/comp-gold.txt --yes    # 正式
```

- 石币有持有上限，统一 1000 万（与商店、交易、战斗奖励同一口径，不随转生变化）。超过上限的部分不发，记录里该角色那一行的 `note` 写明“石币封顶：应发 X，实发 Y（持有上限 Z）”，状态仍是 `ok`；已经在上限的角色实发 0，同样写 `note`。
- 预演只列人数和名字，不查每人还能收多少；封顶情况看正式执行后的记录。
- `--retry` 沿用记录里的数量重试；同一批次号已成功的角色不会重复发。
- 石币不接受 `--count`、`--level`、`--points`。
- 已知限制：在线角色的发放与服务端自动存盘之间有毫秒级窗口，恰好此时存盘会把旧石币整行写回，这笔发放会丢，记录仍是 `ok`。大批量发放后可抽查几名在线角色的石币。

## 4. 流程：预演 → 正式 → 核对 → 失败重试

1. **预演**：命令不加 `--yes`。只打印人数、前 20 个名字和警告（账号不存在、账号下没有角色），**不改库，也不写记录文件**。核对无误再继续。
2. **正式**：同一条命令加 `--yes`。角色逐个独立处理：成功或失败都会立刻写入记录文件，失败的不影响后面的人。结束时打印成功、失败、跳过的数量和每个失败的原因。
3. **核对**：
   ```sh
   grep -c '"status":"ok"' runtime/grants/comp-01.jsonl        # 成功的行数
   grep '"status":"failed"' runtime/grants/comp-01.jsonl       # 失败的明细（含原因）
   ```
   抽查几个角色是否真的拿到了：物品、宠物、精灵在角色在线时可以直接看游戏里；离线角色用库查（库连接串在 `.env` 的 `STONEAGE_DATABASE_URL`）。
4. **重试失败的**：修好原因（清背包、改清单里的名字等）后，用**同一个批次号**重试：
   ```sh
   go run ./cmd/stoneage-admin grant --batch comp-01 --retry                # 预演：只列出记录里失败的角色
   go run ./cmd/stoneage-admin grant --batch comp-01 --retry --yes          # 正式
   ```
   `--retry` 只处理记录里最后状态是失败的角色，物资和数量沿用记录，不能和 `--kind`、`--id`、人群参数同用。
5. **同一批次号再执行一遍**（不加 `--retry`）：已成功的跳过不重发，失败的重试。这是防重发的保险，所以**同一批次号可以放心重跑**。改了名单（比如把错名字改对）后也用同一批次号重跑，已成功的不会再发。

## 5. 常见错误与处理

| 报错（原文） | 原因 | 处理 |
| --- | --- | --- |
| `找不到角色 X` | 名字写错、角色没建、或改过名 | 核对名字，修正名单后用同一批次号重跑。这个失败记录会留在记录里，但不影响其他人。 |
| `X 的背包已满（15 格都有物品）` | 背包没有空格 | 让玩家清理背包，然后 `--retry --yes`。 |
| `X 的精灵栏位已满（5 个）` | 精灵栏满 | 同上。 |
| `X 的宠物栏位已满（N 只）` | 宠物栏满 | 同上。 |
| `编号 X（名字）不能当宠物（PETFLG 为 0）` | 编号不是宠物 | 回第 2 节重查编号。这类报错在执行前就会停下，不写记录。 |
| `宠物等级要在 1–160 之间` / `X 的等级上限是 N` | 等级超出范围或超过模板上限 | 调整 `--level`。同上，执行前就停。 |
| `物品编号 X 不存在` / `精灵编号 X 不在魔法表里` | 编号错了 | 回第 2 节重查。同上，执行前就停。 |
| `批次号已经用于 …，对不上` | 这个批次号已经发过别的物资或别的数量 | 换一个新的批次号。不要手改记录文件。 |
| `批次 X 没有发放记录` | `--retry` 时找不到这个批次的记录文件 | 确认批次号和 `--dir` 是否写对。 |
| `账号 X 不存在` / `账号 X 下没有角色` | 账号名写错，或账号下还没有角色 | 修正账号清单。这些账号会被跳过，不影响其他账号。 |
| `--registered-before 要写成 YYYY-MM-DD` | 日期格式错 | 改成 `2026-01-01` 这种格式。 |
| `写发放记录 … 失败，已处理到 X` | 磁盘满、目录没权限等 | **命令会停下。** 最后处理的那个角色可能已经发成功、但记录没写进去。先用库查 X 有没有拿到，再决定：已拿到的把记录补上（或从名单里去掉后重跑），没拿到的直接重跑。直接重跑会在没有记录的角色上重复发放，务必先查。 |
| `N 个角色失败（见上面明细）…` | 正式执行时有失败的角色 | 按明细处理，再用 `--retry` 重试。这是正常结果，不是命令坏了。 |

## 6. 在线与离线到账

- **在线**（角色在世界中）：发放的同一个事务里发出通知，运行中的服务端重读角色数据并推送给玩家。物品推送 `inventory_changed`，宠物推送 `pets_changed`，精灵、能力点和石币推送 `status_changed`。玩家当场在背包、宠物、状态里看到。
- **离线**（角色不在世界中）：只写库，下次进入世界时可见。
- 批量发放期间不需要停服，玩家在线也不受影响。
- `stoneage-admin level`、`stoneage-admin move` 只写库，角色在线时要先离开世界，这两条不在批量发放里。

## 7. 回滚与追回

- **没有自动回滚。** 批量发放的东西只能人工扣回。
- 记录文件只记录“发了什么”，扣回的动作要另外记在工单里：扣了谁、扣了什么、扣了多少、为什么。
- 扣回前先让角色离开世界（下线），避免服务端内存里的旧数据在保存时把扣回覆盖回去。
- 下面的 SQL 在 `psql` 里执行（连接串取 `.env` 里的 `STONEAGE_DATABASE_URL`）。**先 SELECT 确认，再 DELETE 或 UPDATE。** 建议放在事务里。

```sql
-- 物品：先找到槽位，再删
SELECT ci.slot, ci.item_id, ci.count FROM character_items ci
  JOIN characters c ON c.id = ci.character_id WHERE c.name = '角色名';
DELETE FROM character_items WHERE character_id = (SELECT id FROM characters WHERE name = '角色名')
  AND slot = <槽位> AND item_id = <编号>;

-- 能力点（未分配的点数）：玩家已经 addpoint 用掉的扣不回来，这种情况走人工补偿
UPDATE characters SET skill_points = skill_points - <点数> WHERE name = '角色名' AND skill_points >= <点数>;

-- 精灵：PostgreSQL 数组下标从 1 开始，array_position 找不到时返回 NULL，要先确认能找到
UPDATE characters SET magics[array_position(magics, <编号>)] = -1
  WHERE name = '角色名' AND array_position(magics, <编号>) IS NOT NULL;

-- 宠物：先查栏位和模板编号，再删
SELECT p.slot, p.template_id FROM character_pets p JOIN characters c ON c.id = p.character_id WHERE c.name = '角色名';
DELETE FROM character_pets WHERE character_id = (SELECT id FROM characters WHERE name = '角色名')
  AND slot = <栏位> AND template_id = <编号>;
```

## 8. 记录文件格式

`runtime/grants/<批次号>.jsonl`，每行一个 JSON，字段：

| 字段 | 含义 |
| --- | --- |
| `batch` | 批次号 |
| `character` | 角色名 |
| `kind` | `item` / `pet` / `magic` / `point` / `gold` |
| `id` | 物品、宠物、精灵编号；能力点为 0 |
| `qty` | 物品个数、宠物等级、能力点数、石币数量（应发）；精灵固定为 1 |
| `status` | `ok` 或 `failed` |
| `error` | 失败原因，成功时为空 |
| `note` | 成功但有说明时才有：石币封顶时写明应发、实发和持有上限 |
| `time` | 处理时间 |

同一个角色可能有多行（失败后重试），**以最后一行为准**。不要手工删改记录文件，有问题先停下来问。

## 9. 命令速查

```sh
go run ./cmd/stoneage-admin grant -h        # 参数说明
go run ./cmd/stoneage-admin accounts        # 看账号（包括禁用的）
```

`grant` 的参数：`--kind`、`--id`、`--count`（物品）、`--level`（宠物）、`--points`（能力点）、`--amount`（石币）、`--batch`、`--names-file`、`--accounts-file`、`--all`、`--registered-before`、`--min-level`、`--retry`、`--yes`、`--dir`（记录目录，默认 `runtime/grants`）。

## 10. 重启服务端要优雅退出（STAGE-290）

角色的等级、经验、HP/MP、金币和位置在内存里，写库有三个时机：**离开世界时**（下线、被踢、关服）、**每次战斗结束**、**定时存盘**（默认每 3 分钟，`.env` 的 `STONEAGE_AUTOSAVE_SECONDS` 调整，`0` 关闭）。所以服务端被强杀、崩溃、机器断电时，玩家最多回退到最近一次存盘；定时存盘关着的话，会一直回退到这次登录之前（玩家看到的就是“回档”）。

**重启一律走脚本，不要 `kill -9`：**

```sh
scripts/server-restart.sh        # 停掉、重新编译、后台启动，最后显示状态
scripts/server-stop.sh           # 只停
```

- 脚本给服务端发 SIGTERM：服务端通知所有连接 `SERVER_SHUTDOWN`，逐个保存角色后再退出。最多等 15 秒，超时才 `kill -KILL`（强杀）。
- 在线的人多、数据库慢时 15 秒可能不够。重启前先看 `scripts/server-status.sh`；不急的话等在线人数少的时候再重启。
- **不要**用 `kill -9`、`pkill -9`、直接关机或关终端来停服务端；用 systemd、Docker、supervisor 托管时，把停止信号设成 SIGTERM，停止等待时间设成不少于 30 秒，否则默认的超时会把还在保存的服务端杀掉。

**确认是不是优雅退出**，看 `runtime/logs/server.log`：

- 正常：`收到退出信号，正在通知所有连接并关闭`，之后没有错误，下一条是新的 `服务端已启动`。
- 有问题：出现 `保存角色失败` / `存盘失败`，或者 `服务端 15 秒内没有退出，强制结束`（脚本的输出）。出现任何一条，当次重启时在线的玩家可能回档，记下重启时间。

**玩家反馈回档时**：先查上面的日志确认是不是强杀或存盘失败；确认后可用数据库备份（如果有）核对，或按第 3、7 节的办法补发经验、物品。回档不会影响账号和已落库的数据（背包物品、宠物表、任务进度在操作时就写库了，不依赖离开世界的存盘）。
