#!/usr/bin/env bash
# 开始一个需求或缺陷：以 develop 为基础，新建分支「需求号-短标题」和对应的 worktree。
# 用法: scripts/stage-start.sh <需求号> <短标题>
#   scripts/stage-start.sh STAGE-29 背包穿脱整理
# 分支: STAGE-29-背包穿脱整理；worktree: 主仓库同级的 stoneage-STAGE-29（目录只用需求号，保持纯 ASCII）。
# 短标题只能用汉字、字母、数字、- 和 _，不超过 20 个字，不带空格、【】和标点。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/stage-common.sh"

[[ $# -eq 2 ]] || die "用法: scripts/stage-start.sh <需求号> <短标题>，例如 scripts/stage-start.sh STAGE-29 背包穿脱整理"
id="$1" title="$2"

[[ "${id}" =~ ^[A-Z]+-[0-9]+$ ]] || die "需求号格式应为 STAGE-29 这样的「大写字母-数字」：${id}"
# 原版用 perl 按 Unicode 属性校验；本机 Git Bash 没装 perl，改成等价的纯 bash 检查
# （长度按字符数，显式排除空格与常见中英文标点；git check-ref-format 还会再兜一层）。
[[ ${#title} -ge 1 && ${#title} -le 20 ]] ||
  die "短标题只能用汉字、字母、数字、- 和 _，1–20 个字：${title}"
[[ "${title}" != *[[:space:]]* ]] || die "短标题不能带空格：${title}"
probe="${title//[-_]/}"
[[ "${probe}" != *[[:punct:]]* ]] || die "短标题只能带 - 和 _ 两种标点：${title}"
case "${title}" in
  *[【】「」『』、，。：；！？（）《》]*)
    die "短标题不能带全角标点：${title}" ;;
esac

branch="${id}-${title}"
dir="${main_root}-${id}"
g check-ref-format --branch "${branch}" >/dev/null || die "不是合法的分支名: ${branch}"
branch_exists "${base_branch}" || die "找不到 ${base_branch} 分支"
existing="$(g for-each-ref --format='%(refname:short)' "refs/heads/${id}" "refs/heads/${id}-*")"
[[ -z "${existing}" ]] || die "${id} 已经有分支: $(echo ${existing})"
[[ ! -e "${dir}" ]] || die "目录已存在: ${dir}"

g worktree add -b "${branch}" "${dir}" "${base_branch}"

# .env 不进 Git，复制一份让新 worktree 里的服务端脚本能直接用。
[[ -f "${main_root}/.env" ]] && cp -p "${main_root}/.env" "${dir}/.env"

cat <<EOF

已创建分支 ${branch}（基于 ${base_branch} $(g rev-parse --short "${base_branch}")）
worktree: ${dir}

接下来:
  1. 云效里把 ${id} 改成「开发中」
  2. cd ${dir} 开发、提交（提交信息以 ${id} 开头）
  3. 自测通过后 scripts/stage-to-test.sh ${id}
EOF
