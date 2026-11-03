#!/usr/bin/env bash
# 停止新服务端：先给所有连接推送 SERVER_SHUTDOWN 再退出。数据库不受影响。
# 用法: scripts/server-stop.sh
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/server-common.sh"

[[ $# -eq 0 ]] || die "未知参数: $*"

if pid="$(server_pid)"; then
  kill -TERM "${pid}"
  for _ in $(seq 1 60); do
    kill -0 "${pid}" 2>/dev/null || break
    sleep 0.25
  done
  if kill -0 "${pid}" 2>/dev/null; then
    echo "服务端 15 秒内没有退出，强制结束"
    kill -KILL "${pid}"
  fi
  echo "服务端已停止 (pid ${pid})"
else
  echo "服务端没有在运行"
fi
rm -f "${pid_file}"
