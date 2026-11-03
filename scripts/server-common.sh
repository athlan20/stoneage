#!/usr/bin/env bash
# 新服务端脚本的公共部分：路径、读取 .env、进程。由 server-*.sh 引用，不单独运行。
# 数据库由开发者自己准备（本机安装的 PostgreSQL 即可），脚本不管理数据库。说明见 docs/server.md。

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="${project_root}/.env"
runtime_dir="${project_root}/runtime"
log_dir="${runtime_dir}/logs"
pid_file="${runtime_dir}/server.pid"
server_log="${log_dir}/server.log"
protocol_log="${log_dir}/protocol.log"
server_bin="${project_root}/build/stoneage-server"

die() { echo "错误: $*" >&2; exit 1; }

build_server() {
  mkdir -p "${project_root}/build"
  (cd "${project_root}" && go build -o "${server_bin}" ./cmd/stoneage-server) || die "编译服务端失败"
}

# 首次启动时没有 .env：从模板复制一份，让开发者填好自己的数据库连接串后再启动。
ensure_env() {
  [[ -f "${env_file}" ]] && return
  [[ -f "${project_root}/.env.example" ]] || die "找不到 .env.example"
  cp "${project_root}/.env.example" "${env_file}"
  chmod 600 "${env_file}"
  die "已从 .env.example 生成 .env，请把 STONEAGE_DATABASE_URL 改成你本机数据库的连接串后再启动（见 docs/server.md）"
}

# 用服务端程序自己解析配置，保证脚本和服务端对 .env 与环境变量优先级的理解一致。
load_db_parts() {
  { read -r DB_USER; read -r DB_PASS; read -r DB_NAME; read -r DB_HOST; read -r DB_PORT; } \
    < <("${server_bin}" -env "${env_file}" -print-db) || die "无法解析 STONEAGE_DATABASE_URL"
  [[ -n "${DB_USER}" ]] || die "无法解析 STONEAGE_DATABASE_URL"
}

server_pid() {
  [[ -f "${pid_file}" ]] || return 1
  local pid
  pid="$(cat "${pid_file}")"
  [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null && ps -p "${pid}" -o command= 2>/dev/null | grep -q stoneage-server && echo "${pid}"
}
