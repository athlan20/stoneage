#!/usr/bin/env bash
# 发布客户端新版本：校验版本号，给当前提交打 tag 并推到 GitHub，tag 推上去就触发 GitHub Actions 构建并创建 Release。
# 用法: scripts/release-github.sh v0.1.0
#   前提：要发布的提交已经推到 origin；client/Cargo.toml 的 version 与 tag 一致；CHANGELOG.md 最上面有这一版。
#   tag 已经存在就用它（指向哪个提交由它自己决定），不存在就打在当前 HEAD 上。
set -euo pipefail

die() { echo "错误: $*" >&2; exit 1; }

[[ $# -eq 1 ]] || die "用法: scripts/release-github.sh v0.1.0"
tag="$1"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "tag 格式应为 v0.1.0（预发布可带后缀，如 v0.2.0-rc.1）：$tag"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# 1. 远端上不能已经有这个 tag（有了就不重复发布；要重发请先在 GitHub 上删除 tag 和对应的 Release）
if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
  die "origin 上已经有 ${tag}。要重发请先在 GitHub 上删除这个 tag 和对应的 Release"
fi

# 2. tag 指向的提交里，client/Cargo.toml 的版本要和 tag 一致，CHANGELOG 里要有这一版
target="HEAD"
if git rev-parse --verify --quiet "refs/tags/$tag" >/dev/null; then target="refs/tags/$tag"; fi
version="$(git show "$target:client/Cargo.toml" | sed -n 's/^version = "\(.*\)"/\1/p' | head -1)"
[[ "$tag" == "v$version" ]] || die "tag $tag 与 client/Cargo.toml 的 version（${version}）不一致"
git show "$target:CHANGELOG.md" | grep -q "^## ${tag}\b" || die "CHANGELOG.md 里没有 \"## ${tag}\" 这一节"

# 3. 要发布的提交必须已经在 origin 的某个分支上，否则构建出来的版本别人查不到源码
git fetch --quiet origin
commit="$(git rev-parse "$target^{commit}")"
git branch -r --contains "$commit" | grep -q . || die "提交 ${commit:0:8} 还没推到 origin，先推送再发布"

# 4. 打 tag（已有就跳过）并推送，推上去就触发 GitHub Actions
if [[ "$target" == "HEAD" ]]; then git tag "$tag"; fi
git push --quiet origin "refs/tags/$tag"
echo "已推送 ${tag}（提交 ${commit:0:8}）。GitHub Actions 开始构建：https://github.com/athlan20/stoneage/actions"
