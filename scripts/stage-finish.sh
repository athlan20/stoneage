#!/usr/bin/env bash
# 测试通过后收尾：把需求分支本身（不是 test）合进 develop，然后删掉它的 worktree 和分支。
# 用法: scripts/stage-finish.sh [--keep] [--force] [需求号或分支名]
#   scripts/stage-finish.sh STAGE-29          合进 develop 并清理
#   scripts/stage-finish.sh --keep STAGE-29   只合并，保留 worktree 和分支
#   --force                                   需求分支的最新提交还没进 test 时也照样合（跳过测试，慎用）
# 合并在检出 develop 的那个 worktree（一般是主仓库）里进行。有冲突时停下，解决并提交后再运行一次即可。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/stage-common.sh"

keep=false force=false
while [[ "${1:-}" == --* ]]; do
  case "$1" in
    --keep) keep=true ;;
    --force) force=true ;;
    *) die "未知参数: $1" ;;
  esac
  shift
done
[[ $# -le 1 ]] || die "用法: scripts/stage-finish.sh [--keep] [--force] [需求号或分支名]"

branch="$(resolve_branch "${1:-}")"
wt="$(worktree_of "${branch}")" || wt=""
if [[ -n "${wt}" ]]; then
  require_clean "${wt}" "${branch} 的 worktree"
  if ! ${keep}; then
    [[ "$(pwd -P)/" != "$(cd "${wt}" && pwd -P)/"* ]] || die "当前目录在要删除的 worktree 里，换到主仓库再运行"
    untracked="$(git -C "${wt}" ls-files --others --exclude-standard)"
    [[ -z "${untracked}" ]] || die "${wt} 里有未跟踪的文件，提交或删掉后再运行：$(echo ${untracked} | cut -c1-200)"
  fi
fi
dev_wt="$(worktree_of "${base_branch}")" || die "没有检出 ${base_branch} 的 worktree，先在主仓库 git switch ${base_branch}"

if is_merged "${branch}" "${base_branch}"; then
  echo "${branch} 已经全部在 ${base_branch} 里"
else
  if ! ${force} && ! { branch_exists "${test_branch}" && is_merged "${branch}" "${test_branch}"; }; then
    die "${branch} 的最新提交还没合进 ${test_branch}，先 scripts/stage-to-test.sh $(id_of "${branch}") 测试；确定要跳过测试加 --force"
  fi
  require_clean "${dev_wt}" "${base_branch} 的 worktree"
  git -C "${dev_wt}" merge --no-ff "${branch}" -m "${branch}：合入 ${base_branch}" ||
    die "合并有冲突：到 ${dev_wt} 解决并提交后，再运行一次 scripts/stage-finish.sh $(id_of "${branch}")"
  echo "已把 ${branch} 合进 ${base_branch}（${dev_wt}）"
fi

if ${keep}; then
  echo "按 --keep 保留分支和 worktree"
else
  if [[ -n "${wt}" ]]; then
    g worktree remove "${wt}"
    echo "已删除 worktree ${wt}"
  fi
  # git branch -d 比的是脚本运行位置当前检出的分支（主仓库常停在 main），会把已合进 develop 的分支误报成“未合并”；
  # 这里自己确认已进 develop，再用 -D 删除
  is_merged "${branch}" "${base_branch}" || die "${branch} 还没全部进 ${base_branch}，不删分支"
  g branch -D "${branch}"
fi

cat <<EOF

接下来:
  1. 云效里把 $(id_of "${branch}") 需求改成「已完成」，缺陷改成「已关闭」
  2. 需要时推送: git push origin ${base_branch}
EOF
