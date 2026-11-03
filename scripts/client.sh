#!/usr/bin/env bash
# 一键启动命令行客户端：编译（有改动才会重新编译）并运行，参数原样传给客户端。
# 用法: scripts/client.sh [客户端参数...]
#   scripts/client.sh                                   连本机服务端（127.0.0.1:7065）
#   scripts/client.sh --account alice                   预填账号
#   scripts/client.sh --server 127.0.0.1:7067           换服务端地址
#   scripts/client.sh --account alice --character 1     登录后自动进入角色
# 密码只能通过环境变量 STONEAGE_PASSWORD 传入。
set -euo pipefail
project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="${HOME}/.cargo/bin:${PATH}"
command -v cargo >/dev/null || { echo "错误: 找不到 cargo，请先安装 Rust 工具链" >&2; exit 1; }

# 目标是本机默认地址、而服务端没在运行时，先提醒一声（客户端自己也会提示连不上）。
if [[ ! -f "${project_root}/runtime/server.pid" ]] && ! printf '%s ' "$@" | grep -q -- '--server'; then
  echo "提示: 本机服务端好像没在运行，先执行 scripts/server-restart.sh" >&2
fi

cd "${project_root}/client"
cargo build -q --bin stoneage
exec ./target/debug/stoneage "$@"
