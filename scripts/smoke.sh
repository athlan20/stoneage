#!/usr/bin/env bash
# 冒烟测试（STAGE-5，说明见 docs/smoke.md）：每次发版前跑一遍。
#
#   scripts/smoke.sh            自动部分：起一个真实的服务端（数据库里建临时 schema，不碰开发库），
#                               用真实的客户端二进制在伪终端里走完整流程（TCP 一遍，WebSocket 再一遍），
#                               再检查数据库和日志里没有明文密码
#   scripts/smoke.sh --full     再加上：空闲 75 秒验证心跳，以及客户端的联调测试（cargo test --test e2e）
#
# 需要：Go、Rust（cargo）、python3、psql（只用来检查数据库里没有明文密码，没有就跳过那一项）；
# 数据库连接串取自 .env 或环境变量 STONEAGE_DATABASE_URL。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/server-common.sh"
export PATH="${HOME}/.cargo/bin:${PATH}"

full=0
[[ "${1:-}" == "--full" ]] && full=1

work="$(mktemp -d)"
server_pid_smoke=""
agent_pid=""
cleanup() {
  [[ -n "${agent_pid}" ]] && kill "${agent_pid}" 2>/dev/null || true
  [[ -n "${server_pid_smoke}" ]] && kill -TERM "${server_pid_smoke}" 2>/dev/null && wait "${server_pid_smoke}" 2>/dev/null || true
  rm -rf "${work}"
}
trap cleanup EXIT

echo "== 编译服务端、客户端和管理命令"
build_server
(cd "${project_root}/client" && cargo build -q --bin stoneage) || die "编译客户端失败"
admin_bin="${project_root}/build/stoneage-admin"
(cd "${project_root}" && go build -o "${admin_bin}" ./cmd/stoneage-admin) || die "编译管理命令失败"

db_url="${STONEAGE_DATABASE_URL:-}"
if [[ -z "${db_url}" && -f "${env_file}" ]]; then
  db_url="$(grep '^STONEAGE_DATABASE_URL=' "${env_file}" | head -1 | cut -d= -f2-)"
fi
[[ -n "${db_url}" ]] || die "没有数据库连接串：设置 STONEAGE_DATABASE_URL，或在 .env 里配置"

echo "== 启动真实服务端（临时 schema，随机端口）"
STONEAGE_DATABASE_URL="${db_url}" STONEAGE_LISTEN="127.0.0.1:0" STONEAGE_WS_LISTEN="127.0.0.1:0" \
  "${server_bin}" -env /nonexistent.env -temp-schema -protocol-log "${work}/protocol.log" \
  > "${work}/server.log" 2>&1 < /dev/null &
server_pid_smoke=$!
addr=""
for _ in $(seq 1 100); do
  addr="$(grep -o ' listen=[0-9.:]*' "${work}/server.log" 2>/dev/null | head -1 | cut -d= -f2 || true)"
  [[ -n "${addr}" ]] && break
  kill -0 "${server_pid_smoke}" 2>/dev/null || { cat "${work}/server.log"; die "服务端启动失败"; }
  sleep 0.1
done
[[ -n "${addr}" ]] || die "服务端没有在 10 秒内启动"
ws_addr="$(grep -o 'ws_listen=[0-9.:]*' "${work}/server.log" | head -1 | cut -d= -f2 || true)"
[[ -n "${ws_addr}" ]] || die "服务端没有开 WebSocket 入口（STONEAGE_WS_LISTEN）"
echo "   服务端 ${addr}，WebSocket ws://${ws_addr}/ws"
# 临时 schema 的名字在日志里：管理命令（give 发物品）要用同一个 schema 才碰得到这次的数据。
schema="$(grep -o 'schema=e2e_[0-9_]*' "${work}/server.log" | head -1 | cut -d= -f2 || true)"
admin_db=""
if [[ -n "${schema}" ]]; then
  sep='?'
  [[ "${db_url}" == *\?* ]] && sep='&'
  admin_db="${db_url}${sep}search_path=${schema}"
else
  echo "   注意：日志里没有临时 schema 名，跳过背包发物品这一段"
fi

# 分身队（STAGE-162，docs/smoke.md）：新版界面的一段独立流程（场景 → Esc 菜单 → 社交页组队、解散、命令），
# 换一个账号和角色名；放在完整流程之前，这一段只依赖真实客户端和真实服务端。
echo "== 分身队：社交页组成分身队 → 动作菜单解散（默认取消、再确认）→ /party 命令"
python3 "${project_root}/tools/smoke-pty.py" --client "${project_root}/client/target/debug/stoneage" \
  --server "${addr}" --account "smoke_party_${RANDOM}" --name Hero3 --password 'Zq9-plain-secret' --party-only

echo "== 真实客户端走完整流程（注册 → 登录 → 建角 → 进入游戏 → 离开 → 再进入 → 退出）"
extra=()
(( full )) && extra=(--idle-seconds 75)
give_args=()
[[ -n "${admin_db}" ]] && give_args=(--admin "${admin_bin}" --admin-db "${admin_db}")
python3 "${project_root}/tools/smoke-pty.py" --client "${project_root}/client/target/debug/stoneage" \
  --server "${addr}" --password 'Zq9-plain-secret' --token-file "${work}/token" \
  --script-token-file "${work}/script-token" \
  "${give_args[@]+"${give_args[@]}"}" "${extra[@]+"${extra[@]}"}"

# WebSocket 用例：同一个服务端，换一个账号和角色名，走一遍完整流程（不带管理命令和 agent 段，
# 数据库里的 Hero1 相关数据由 TCP 那一遍负责）。
echo "== 同一个服务端，经 WebSocket 再走一遍（注册 → 登录 → 建角 → 进入游戏 → 离开 → 再进入 → 退出）"
python3 "${project_root}/tools/smoke-pty.py" --client "${project_root}/client/target/debug/stoneage" \
  --server "ws://${ws_addr}/ws" --account "smoke_ws_${RANDOM}" --name Hero2 --password 'Zq9-plain-secret' --no-agent

token="$(cat "${work}/token")"
[[ "${token}" == sat_* ]] || die "没有拿到 agent 令牌"
script_token="$(cat "${work}/script-token")"
[[ "${script_token}" == sat_* ]] || die "没有拿到参考脚本用的令牌"

# 参考脚本（STAGE-118，docs/agent-guide.md 第 7 节）：用它的令牌经 WebSocket 登录、进入世界、查状态、离开。
echo "== 参考脚本 agent-connect.py：令牌登录 → 进入世界 → 查状态 → 离开"
agent_in="${work}/agent.in"
mkfifo "${agent_in}"
STONEAGE_AGENT_TOKEN="${script_token}" python3 "${project_root}/docs/agent-connect.py" --url "ws://${ws_addr}/ws" \
  --input "${agent_in}" > "${work}/agent.out" 2>&1 &
agent_pid=$!
wait_agent() {
  for _ in $(seq 1 100); do
    grep -qF -- "$1" "${work}/agent.out" 2>/dev/null && return 0
    kill -0 "${agent_pid}" 2>/dev/null || break
    sleep 0.1
  done
  cat "${work}/agent.out"
  die "参考脚本没有等到: $1"
}
wait_agent '"type": "agent_login", "ok": true'
echo '{"type": "enter_world", "data": {"slot": 0}}' > "${agent_in}"
wait_agent '"type": "enter_world", "ok": true'
echo '{"type": "status", "data": {}}' > "${agent_in}"
wait_agent '"type": "status", "ok": true'
echo '{"type": "leave_world", "data": {}}' > "${agent_in}"
wait_agent '"type": "leave_world", "ok": true'
kill "${agent_pid}" 2>/dev/null || true
wait "${agent_pid}" 2>/dev/null || true
agent_pid=""
grep -qF -- "${script_token}" "${work}/agent.out" && die "参考脚本的输出里出现了令牌原文"
echo "   进入世界、查状态、离开：通过"

echo "== 检查数据库和日志里没有明文密码和令牌原文"
for secret in 'Zq9-plain-secret' "${token}" "${script_token}"; do
  if grep -qF -- "${secret}" "${work}/server.log" "${work}/protocol.log"; then
    die "服务端日志或协议日志里出现了明文密码或令牌"
  fi
done
[[ -s "${work}/protocol.log" ]] || die "协议日志是空的，检查没有意义"
echo "   服务端日志、协议日志：没有"
if command -v psql >/dev/null; then
  schema="$(psql "${db_url}" -Atc "select schema_name from information_schema.schemata where schema_name like 'e2e_%' order by 1 desc limit 1")"
  hits=0
  for t in accounts auth_audit login_locks characters agent_tokens agent_audit; do
    for secret in 'Zq9-plain-secret' "${token}" "${script_token}"; do
      hits=$((hits + $(psql "${db_url}" -Atc "select t::text from ${schema}.${t} t" | grep -cF -- "${secret}" || true)))
    done
  done
  (( hits == 0 )) || die "数据库里出现了明文密码或令牌"
  echo "   数据库（accounts、auth_audit、login_locks、characters、agent_tokens、agent_audit）：没有"
  echo "   密码列：$(psql "${db_url}" -Atc "select left(password_hash, 30) from ${schema}.accounts limit 1")…  令牌列：$(psql "${db_url}" -Atc "select left(token_hash, 12) from ${schema}.agent_tokens limit 1")…（SHA-256）"
  echo "   agent 审计（${schema}.agent_audit）："
  psql "${db_url}" -At -F ' | ' -c "select kind, request, result, role, token_id from ${schema}.agent_audit order by id" | sed 's/^/     /'
else
  echo "   没有 psql，跳过数据库检查"
fi

if (( full )); then
  echo "== 客户端联调测试（挤下线、关服、锁定、名字被拒、删除角色……）"
  (cd "${project_root}/client" && STONEAGE_TEST_DATABASE_URL="${db_url}" cargo test -q --test e2e)
fi
echo "冒烟测试通过"
