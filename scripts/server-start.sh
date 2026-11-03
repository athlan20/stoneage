#!/usr/bin/env bash
# 编译并在后台启动新服务端。数据库用 .env 里 STONEAGE_DATABASE_URL 指向的库（自己准备，见 docs/server.md），
# 服务端启动时会自动建表、升级表结构。
# 用法: scripts/server-start.sh [--protocol-log] [--foreground]    （或设置 STONEAGE_PROTOCOL_LOG=1）
#   --foreground  不放到后台：服务端留在当前终端运行，日志同时显示在终端和 server.log，Ctrl+C 停止。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/server-common.sh"

want_protocol_log="${STONEAGE_PROTOCOL_LOG:-0}"
foreground=0
for arg in "$@"; do
  case "${arg}" in
    --protocol-log) want_protocol_log=1 ;;
    --foreground) foreground=1 ;;
    *) die "未知参数: ${arg}" ;;
  esac
done

mkdir -p "${log_dir}"
ensure_env

if pid="$(server_pid)"; then
  echo "服务端已在运行 (pid ${pid})；要让新代码或新配置生效，请先 scripts/server-stop.sh 再启动"
  exit 0
fi

build_server
load_db_parts
echo "数据库: ${DB_USER}@${DB_HOST}:${DB_PORT}/${DB_NAME}"

args=(-env "${env_file}")
if [[ "${want_protocol_log}" =~ ^(1|true|TRUE|yes|on)$ ]]; then
  args+=(-protocol-log "${protocol_log}")
  echo "协议日志: ${protocol_log}（密码和令牌显示为 ***）"
fi
cd "${project_root}"
if [[ "${foreground}" == 1 ]]; then
  # 前台运行：输出同时写终端和 server.log；pid 文件照常写，别的终端的 server-stop.sh 也能停它。
  "${server_bin}" "${args[@]}" > >(tee -a "${server_log}") 2>&1 &
  pid=$!
  echo "${pid}" > "${pid_file}"
  trap 'kill -TERM "${pid}" 2>/dev/null || true' INT TERM
  echo "服务端 pid ${pid}（前台运行，Ctrl+C 停止）"
  status=0
  wait "${pid}" || status=$?
  wait "${pid}" 2>/dev/null || status=$?   # 被信号打断的第一次 wait 不代表服务端已退出
  rm -f "${pid_file}"
  exit "${status}"
fi
log_lines_before="$(wc -l < "${server_log}" 2>/dev/null || echo 0)"
cd "${project_root}"
nohup "${server_bin}" "${args[@]}" >> "${server_log}" 2>&1 < /dev/null &
echo $! > "${pid_file}"
disown
pid="$(cat "${pid_file}")"

# 等服务端开始监听。连不上数据库时服务端会重试 30 秒后退出。
for _ in $(seq 1 80); do
  if ! kill -0 "${pid}" 2>/dev/null; then
    rm -f "${pid_file}"
    tail -n 20 "${server_log}" >&2
    die "服务端启动失败（数据库是否在运行、连接串是否正确？），日志: ${server_log}"
  fi
  if tail -n +"$((log_lines_before + 1))" "${server_log}" | grep -q "服务端已启动"; then
    break
  fi
  sleep 0.5
done
tail -n +"$((log_lines_before + 1))" "${server_log}" | grep "服务端已启动" || die "服务端没有在 40 秒内启动，日志: ${server_log}"
echo "服务端 pid ${pid}，日志: ${server_log}"
