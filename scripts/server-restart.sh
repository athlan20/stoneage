#!/usr/bin/env bash
# 一键重启新服务端：停掉正在运行的（没有在运行也不报错），重新编译，再后台启动，最后显示状态。
# 默认打开协议日志（runtime/logs/protocol.log），黑盒测试时方便对照；不需要时加 --no-protocol-log。
# 用法: scripts/server-restart.sh [--no-protocol-log] [--foreground]
#   --foreground  不放到后台：服务端留在当前终端运行（日志直接显示），Ctrl+C 停止。
set -euo pipefail
dir="$(dirname "${BASH_SOURCE[0]}")"

protocol_flag=(--protocol-log)
foreground_flag=()
for arg in "$@"; do
  case "${arg}" in
    --no-protocol-log) protocol_flag=() ;;
    --foreground) foreground_flag=(--foreground) ;;
    *) echo "错误: 未知参数: ${arg}" >&2; exit 1 ;;
  esac
done

"${dir}/server-stop.sh"
if [[ ${#foreground_flag[@]} -gt 0 ]]; then
  exec "${dir}/server-start.sh" ${protocol_flag[@]+"${protocol_flag[@]}"} --foreground
fi
"${dir}/server-start.sh" ${protocol_flag[@]+"${protocol_flag[@]}"}
echo
"${dir}/server-status.sh"
