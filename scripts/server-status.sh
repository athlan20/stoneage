#!/usr/bin/env bash
# 查看新服务端是否在运行，以及它使用的数据库和表结构版本。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/server-common.sh"

status=0
if pid="$(server_pid)"; then
  echo "服务端: 运行中 (pid ${pid})，日志 ${server_log}"
  grep "服务端已启动" "${server_log}" 2>/dev/null | tail -n 1 | sed 's/^/        /'
else
  echo "服务端: 未运行"
  status=1
fi

if [[ ! -f "${env_file}" ]]; then
  echo "数据库: 还没有配置（复制 .env.example 为 .env 并填好连接串）"
  exit ${status}
fi
[[ -x "${server_bin}" ]] || build_server
load_db_parts
echo "数据库: ${DB_USER}@${DB_HOST}:${DB_PORT}/${DB_NAME}（连接串见 .env）"
if command -v psql >/dev/null; then
  url="$(grep -E '^STONEAGE_DATABASE_URL=' "${env_file}" | cut -d= -f2-)"
  url="${STONEAGE_DATABASE_URL:-${url}}"
  if v="$(psql "${url}" -X -q -tAc 'select coalesce(max(version),0) from schema_migrations' 2>/dev/null)"; then
    echo "        表结构版本: ${v}"
  elif psql "${url}" -X -q -tAc 'select 1' >/dev/null 2>&1; then
    echo "        可以连接，还没有建表（服务端首次启动时自动建表）"
  else
    echo "        连接失败，请检查数据库是否在运行、连接串是否正确"
    status=1
  fi
fi
exit ${status}
