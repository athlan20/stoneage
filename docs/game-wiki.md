# 石器百科

`/wiki` 是公开的静态百科：页面、目录、搜索索引与详情全部是预先生成的静态文件，搜索在浏览器 Worker 内执行；服务端没有百科查询 API，例外是 `/wiki/q`，它是给 agent 用的纯文本接口（STAGE-118，挂在服务端的 WebSocket 端口上，见 `docs/server.md`）。百科不建立游戏连接，也不读取账号、角色、存档或聊天。

数据快照编译进二进制（`internal/gamewiki/snapshot/`），客户端的游戏内百科页（`client/src/wiki/`）也在编译期嵌入同一份快照。

## 资料范围

宠物/生物模板、装备与物品、宠物技能、敌人配置、NPC 放置记录、战斗阵容、地图与传送、野外遭遇，以及任务攻略摘要。任务摘要是重新归纳的事实，不转载原文；收录不表示已通过本服完整流程验收。

## 数据口径与重新生成

`cmd/stoneage-wiki-index` 只在本地或构建环境读取原版数据表；运行中的服务端不读取游戏表、不构建目录、不重建百科。首页仅加载 `catalog.json` 分类目录；进入分类后加载对应 `category-*.json`，只有主动全站搜索才加载 `index.json`。Worker 至多保留一份搜索索引；详情按 SHA-256 前缀分为 256 片，只保留最近查看的一片。索引及详情路径包含内容版本，使用长缓存；gzip 在本地预先生成。

原版数据表不随仓库提供。把自备的数据放到 `original-data/gmsv/data`（已被 `.gitignore` 忽略）后重新生成：

```sh
go run ./cmd/stoneage-wiki-index -data original-data/gmsv/data -output internal/gamewiki/snapshot
```

`-media` 参数可选，指向图片清单；没有时词条不带图片引用。旧版本的详情分片确认不再使用后可清理。改了构建代码就要重新生成快照，不能只改 `snapshot/` 里的文件。

怪物 HP 范围按原版的建怪公式计算：等级成长整数处理、四属性独立 ±2 扰动，以及十点共享分配。结果是创建时、装备修正前的基础满血范围，不是实时剩余 HP。掉落表显示物品生成概率，不等于玩家最终获奖概率。静态关联不能保证每条交互在本服都可执行。

## 图片

百科页面本身不带图片。部署时设置 `STONEAGE_WIKI_CDN`（或 `cmd/stoneage-wiki` 的 `-cdn`）为你自己的 HTTPS 图片源，页面才会按固定路径加载图片和地图；留空则不加载任何外部资源，内容安全策略也不会放行外部源。

## 验证

```sh
go test ./internal/gamewiki ./internal/aiknowledge ./internal/gamecatalog
node --test internal/gamewiki/site_test.js internal/gamewiki/search_worker_test.js
```

依赖原版数据表的测试在 `original-data/gmsv/data`（或环境变量 `STONEAGE_ORIGINAL_DATA` 指向的目录）不存在时自动跳过。

## agent 查询（/wiki/q，STAGE-118）

- **搜索**：`GET /wiki/q?q=关键词&kind=分类&limit=20`。关键词不区分大小写，匹配名称和检索文本；`kind` 可选，取值同分类 id（`quest`、`pet`、`equipment`、`item`、`skill`、`battle_npc`、`enemy`、`map`、`npc`、`encounter`、`server_task`、`npc_template`、`guide`）；`limit` 默认 20，最多 50。返回每行一条：编号、分类、名称、位置、简介，制表符分隔。
- **详情**：`GET /wiki/q?key=编号`，例如 `key=map:1001`，返回摘要、字段、表格、备注和相关词条的纯文本；不存在时 404。
- **实现**：`internal/gamewiki/query.go` 在创建 `Handler` 时读入嵌入快照当前版本的 `index.json.gz`（约 2.2 万行，堆内存约 10MB，是对“百科不常驻内存”原则的有意放宽）；详情按需读对应分片，不常驻。`query_text.go` 负责渲染。
- **测试**：`go test ./internal/gamewiki -run TestWikiQuery`。

## 快照与 Agent 分类（STAGE-118）

- **Agent 分类**：`internal/gamewiki/agent.go` 收录一条指向 `/agent.md` 的说明（分类 `agent`，站点里叫「Agent」）。改了构建代码就要重新生成快照，不能只改 `snapshot/` 里的文件。
- **重新生成**：见上文“数据口径与重新生成”。STAGE-118 验证过：重新生成后原有 22034 行的内容和图片列完全一致，只多出 8 条本服任务验证。
- **快照版本**：`catalog.json.gz` 指向当前版本；旧版本的目录保留在 `snapshot/` 里，没有删除。
