# 石器时代 2.5 · 命令行版

在终端里玩的石器时代 2.5：Rust（ratatui）命令行客户端 + Go 重写的服务端，两者之间使用自定义的 JSON Lines 协议，账号与角色数据存 PostgreSQL。

> 本项目是个人的非商业研究与学习作品，与原游戏的版权方没有任何关联。仓库不包含原版的美术素材、数据表和源码；需要它们的脚本和测试由使用者自备数据（见 [AGENTS.md](AGENTS.md)“原版数据与素材”）。

<img src="docs/screenshots/battle.jpg" alt="战斗界面：敌我双方、血量、指令栏和战斗日志都在一个终端窗口里" width="900">

**只想玩**：不用编译，按 [docs/play.md](docs/play.md) 里的三步装好客户端即可（macOS / Linux / Windows）。下面是给想自己编译、跑服务端或改代码的人看的。

## 目录

| 目录 | 内容 |
| --- | --- |
| `client/` | 命令行客户端（Rust + ratatui）与模拟服务端 |
| `server/` | 游戏服务端（Go） |
| `cmd/` | 服务端、管理工具、协议测试客户端、MCP 服务、石器百科等入口 |
| `internal/` | 认证、MCP 会话层、石器百科等内部包 |
| `docs/` | 协议、界面、交互规范与各玩法的设计文档（从 `docs/protocol.md` 读起） |
| `demo/ratatui-demo/` | 界面风格基准（ratatui 官方 demo） |
| `scripts/` | 服务端启停、冒烟测试、客户端一键启动 |
| `tools/` | 数据生成与校验脚本 |

## 快速开始

需要 Go、Rust 工具链和本机的 PostgreSQL。

```sh
cp .env.example .env            # 改 STONEAGE_DATABASE_URL 指向你的库
scripts/server-start.sh         # 编译并后台启动服务端（默认 127.0.0.1:7065）
scripts/client.sh               # 编译并运行客户端，默认连本机服务端
```

只想看界面、不起服务端：

```sh
cd client
cargo run --bin mock-server -- --case register_ok
cargo run --bin stoneage        # 另开终端
```

更多命令、测试方法和代码结构见 [AGENTS.md](AGENTS.md)、[docs/server.md](docs/server.md)、[docs/client.md](docs/client.md)。

## 许可证

尚未选定。
