#!/usr/bin/env bash
# 把需求分支合进 test 分支，供测试。test 检出在主仓库同级的 stoneage-test，第一次运行时从 develop 建出来。
# 用法: scripts/stage-to-test.sh [--reset] [需求号或分支名]
#   scripts/stage-to-test.sh STAGE-29          合进 test
#   scripts/stage-to-test.sh                   在需求的 worktree 里运行时，合当前分支
#   scripts/stage-to-test.sh --reset STAGE-29  先把 test 重置成 develop 再合（test 上其他需求要重新合）
# test 只用来测试，不往外合；测试通过后用 scripts/stage-finish.sh 把需求分支本身合进 develop。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/stage-common.sh"

reset=false
[[ "${1:-}" == "--reset" ]] && { reset=true; shift; }
[[ $# -le 1 ]] || die "用法: scripts/stage-to-test.sh [--reset] [需求号或分支名]"

branch="$(resolve_branch "${1:-}")"
if wt="$(worktree_of "${branch}")"; then require_clean "${wt}" "${branch} 的 worktree"; fi

if ! branch_exists "${test_branch}"; then
  g branch "${test_branch}" "${base_branch}"
  echo "已从 ${base_branch} 建出 ${test_branch} 分支"
fi
if ! test_wt="$(worktree_of "${test_branch}")"; then
  [[ ! -e "${test_dir}" ]] || die "目录已存在但不是 test 的 worktree: ${test_dir}"
  g worktree add "${test_dir}" "${test_branch}"
  [[ -f "${main_root}/.env" ]] && cp -p "${main_root}/.env" "${test_dir}/.env"
  test_wt="${test_dir}"
fi
require_clean "${test_wt}" "test 的 worktree"

if ${reset}; then
  git -C "${test_wt}" reset --hard "${base_branch}"
  echo "已把 ${test_branch} 重置为 ${base_branch} $(g rev-parse --short "${base_branch}")"
fi

if is_merged "${branch}" "${test_branch}"; then
  echo "${branch} 已经全部在 ${test_branch} 里，没有新提交要合"
else
  git -C "${test_wt}" merge --no-ff "${branch}" -m "${branch}：合入 test" ||
    die "合并有冲突：到 ${test_wt} 解决后提交。同样的冲突合进 develop 时还会出现，最好在需求分支上先合 develop 解决"
fi

cat <<EOF

${branch} 已合进 ${test_branch}（${test_wt}）
接下来:
  1. 在 ${test_wt} 里测试：go test -race ./server/...、cd client && cargo test、scripts/smoke.sh
  2. 云效里需求改成「测试中」（技术类需求改成「已完成」），缺陷改成「已修复」
  3. 测试通过后 scripts/stage-finish.sh $(id_of "${branch}")；不通过就回需求分支修改，再运行本脚本
EOF
