#!/usr/bin/env bash
# 需求开发流程脚本的公共部分：找主仓库、按需求号找分支和 worktree、检查工作区干净。
# 由 stage-*.sh 引用，不单独运行。流程说明见 AGENTS.md「开发流程」。

die() { echo "错误: $*" >&2; exit 1; }

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
git -C "${script_dir}" rev-parse --git-dir >/dev/null 2>&1 || die "不在 git 仓库里"

# 所有 worktree 共用同一个仓库；主 worktree 是 git worktree list 的第一项。
main_root="$(git -C "${script_dir}" worktree list --porcelain | sed -n '1s/^worktree //p')"
base_branch="develop"
test_branch="test"
test_dir="${main_root}-test"

g() { git -C "${main_root}" "$@"; }

branch_exists() { g show-ref --verify --quiet "refs/heads/$1"; }

# 打印检出了指定分支的 worktree 目录，没有则返回失败。
worktree_of() {
  g worktree list --porcelain | awk -v ref="refs/heads/$1" '
    /^worktree / { dir = substr($0, 10) }
    $0 == "branch " ref { print dir; found = 1; exit }
    END { exit !found }'
}

# 需求号（STAGE-29）或完整分支名（STAGE-29-背包穿脱整理）→ 分支名。
# 不传参数时取当前所在 worktree 的分支。
resolve_branch() {
  local arg="${1:-}" matches
  if [[ -z "${arg}" ]]; then
    arg="$(git rev-parse --abbrev-ref HEAD 2>/dev/null)" || die "请给出需求号，例如 STAGE-29"
    [[ "${arg}" != "${base_branch}" && "${arg}" != "${test_branch}" ]] || die "当前在 ${arg} 上，请给出需求号，例如 STAGE-29"
  fi
  if branch_exists "${arg}"; then echo "${arg}"; return; fi
  matches="$(g for-each-ref --format='%(refname:short)' "refs/heads/${arg}-*")"
  [[ -n "${matches}" ]] || die "找不到 ${arg} 对应的分支"
  [[ "$(printf '%s\n' "${matches}" | wc -l | tr -d ' ')" == 1 ]] || die "${arg} 对应多个分支：$(echo ${matches})"
  echo "${matches}"
}

# 已跟踪文件没有未提交的改动（未跟踪文件不影响合并，不算）。
require_clean() {
  local dir="$1" what="$2"
  git -C "${dir}" diff --quiet && git -C "${dir}" diff --cached --quiet ||
    die "${what}（${dir}）有未提交的改动，先提交或还原"
}

# 分支名开头的需求号：STAGE-29-背包穿脱整理 → STAGE-29。
id_of() { echo "$1" | sed -E 's/^([A-Z]+-[0-9]+).*/\1/'; }

is_merged() { g merge-base --is-ancestor "$1" "$2"; }
