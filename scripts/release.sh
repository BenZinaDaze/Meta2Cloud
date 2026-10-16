#!/usr/bin/env bash

# meta2cloud 一键发版脚本
# 用法: scripts/release.sh [--dry-run] [--skip-checks] [-y] <版本号>
# 示例: scripts/release.sh 2.2.0          # 发布 v2.2.0
#       scripts/release.sh --dry-run 2.2.0  # 只演练，不做任何修改
#
# 流程: 预检 → 测试构建 → 同步版本号 → 更新 CHANGELOG → release 提交 → 打 tag → 推送
# 推送后 CI 自动完成: Docker 镜像(docker-publish.yml) + GitHub Release(release.yml)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

DRY_RUN=0
SKIP_CHECKS=0
ASSUME_YES=0
VERSION=""

for arg in "$@"; do
    case "$arg" in
        --dry-run)    DRY_RUN=1 ;;
        --skip-checks) SKIP_CHECKS=1 ;;
        -y|--yes)     ASSUME_YES=1 ;;
        -h|--help)
            sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        -*)
            echo "❌ 未知参数: $arg（--help 查看用法）" >&2
            exit 1
            ;;
        *)  VERSION="$arg" ;;
    esac
done

[ -n "$VERSION" ] || { echo "❌ 缺少版本号参数（--help 查看用法）" >&2; exit 1; }

# 版本号规范化：允许 2.2.0 / v2.2.0 两种写法
VERSION="${VERSION#v}"
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "❌ 版本号须为 X.Y.Z 格式（语义化版本），收到: $VERSION" >&2
    exit 1
fi
TAG="v$VERSION"

# ---------- 发版预检（准备工作清单） ----------
echo "🔍 发版预检..."

BRANCH="$(git branch --show-current)"
[ "$BRANCH" = "main" ] || { echo "❌ 必须在 main 分支发版，当前: $BRANCH" >&2; exit 1; }

# 已跟踪文件的未提交变更会进入发版，必须先清理
if [ ! -f CHANGELOG.md ]; then
    echo "❌ 缺少 CHANGELOG.md（发版流程以它为更新日志源头）" >&2
    exit 1
fi
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "❌ 工作区有未提交变更，请先提交或暂存:" >&2
    git status --short >&2
    exit 1
fi
# 未跟踪文件（本地 db 等）不进版本库，仅提示
UNTRACKED="$(git status --porcelain | grep '^??' || true)"
[ -n "$UNTRACKED" ] && echo "⚠️  存在未跟踪文件（不影响发版）: $(echo "$UNTRACKED" | head -5 | tr '\n' ' ')"

# 与远端同步，避免基于过期 main 发版
git fetch origin main --quiet
LOCAL_HEAD="$(git rev-parse HEAD)"
REMOTE_HEAD="$(git rev-parse origin/main)"
[ "$LOCAL_HEAD" = "$REMOTE_HEAD" ] || { echo "❌ 本地 main 与 origin/main 不一致，请先 pull/push" >&2; exit 1; }

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
    echo "❌ tag $TAG 已存在" >&2
    exit 1
fi

LAST_TAG="$(git tag --sort=-version:refname | grep -E '^v[0-9]' | head -n1)"
# sort -V 后最小者仍为 LAST_TAG ⟺ 新版本大于 LAST_TAG
if [ "$(printf '%s\n%s\n' "$LAST_TAG" "$TAG" | sort -V | head -n1)" = "$LAST_TAG" ]; then
    : # 新版本 > 当前最新，通过
else
    echo "❌ 新版本 $TAG 不大于当前最新版本 $LAST_TAG" >&2
    exit 1
fi
echo "   $LAST_TAG → $TAG ✅"

# ---------- 测试与构建 ----------
if [ "$SKIP_CHECKS" = "1" ]; then
    echo "⏭️  跳过测试与构建 (--skip-checks)"
else
    echo "🧪 后端测试..."
    go build ./...
    go vet ./...
    go test ./internal/... -count=1

    echo "🧪 前端构建..."
    (cd frontend && npm run build >/dev/null)
    echo "   ✅ 全部通过"
fi

# ---------- 同步前端版本号 ----------
echo "🔢 同步 frontend/package.json → $VERSION"
run() {
    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] $*"
    else
        "$@"
    fi
}
run node -e "
const fs = require('fs');
const p = 'frontend/package.json';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.version = '$VERSION';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
"

# ---------- 生成 CHANGELOG 段落 ----------
echo "📝 生成 $TAG 更新日志（$LAST_TAG..HEAD）"
TODAY="$(date +%F)"
TMPDIR_REL="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_REL"' EXIT

# 按 conventional commit 类型分拣到临时文件（subject 在前，便于 awk 按行首锚定分类）
# 注：macOS 自带 awk 不解释 -F'\t' 转义，用 printf 传真实制表符
git log --no-merges --format='%s%x09%h' "$LAST_TAG"..HEAD | awk -F "$(printf '\t')" -v tmp="$TMPDIR_REL" '
    function put(f, s, h) { print "- " s " (`" h "`)" > (tmp "/" f) }
    { s = $1; sub(/^[a-z]+(\([^)]*\))?!?:[[:space:]]*/, "", s) }
    /^release:/ { next }
    /^feat!/    { put("01-feat", "**[破坏性]** " s, $2); next }
    /^feat/     { put("01-feat", s, $2); next }
    /^fix/      { put("02-fix",  s, $2); next }
    /^refactor|^perf/ { put("03-imp", s, $2); next }
    /^style/    { put("04-style", s, $2); next }
    /^docs?/    { put("05-doc",  s, $2); next }
    /^(chore|build|ci|test)/ { put("06-chore", s, $2); next }
    { put("07-other", s, $2) }
' || true
# 注：不用 $(case ...) 内联——bash 3.2(macOS 默认)在 pipefail 下对其解析有 bug
label_for() {
    case "$1" in
        01-feat)  echo '✨ 新增' ;;
        02-fix)   echo '🐛 修复' ;;
        03-imp)   echo '♻️ 优化重构' ;;
        04-style) echo '🎨 样式' ;;
        05-doc)   echo '📝 文档' ;;
        06-chore) echo '🧹 杂项' ;;
        07-other) echo '其他' ;;
    esac
}
for f in 01-feat 02-fix 03-imp 04-style 05-doc 06-chore 07-other; do
    [ -s "$TMPDIR_REL/$f" ] || continue
    printf '### %s\n\n' "$(label_for "$f")" >> "$TMPDIR_REL/section.md"
    cat "$TMPDIR_REL/$f" >> "$TMPDIR_REL/section.md"
    printf '\n' >> "$TMPDIR_REL/section.md"
done

if [ ! -s "$TMPDIR_REL/section.md" ]; then
    echo "❌ $LAST_TAG..HEAD 之间没有任何提交，无需发版" >&2
    exit 1
fi

# 插入到 CHANGELOG.md 头部说明之后、第一个版本段落之前
{ echo "## [$TAG] - $TODAY"; echo; cat "$TMPDIR_REL/section.md"; } > "$TMPDIR_REL/block.md"

if [ "$DRY_RUN" = "1" ]; then
    echo "   [dry-run] 将插入以下段落到 CHANGELOG.md 顶部:"
    sed 's/^/   | /' "$TMPDIR_REL/block.md"
else
    awk -v block="$TMPDIR_REL/block.md" '
        !inserted && /^## \[/ { while ((getline line < block) > 0) print line; print ""; inserted = 1 }
        { print }
    ' CHANGELOG.md > "$TMPDIR_REL/CHANGELOG.new"
    mv "$TMPDIR_REL/CHANGELOG.new" CHANGELOG.md
    echo "   ✅ 已写入，请检查内容后继续（可手动润色 CHANGELOG.md）"
fi

# ---------- 提交 + 打 tag ----------
if [ "$DRY_RUN" = "1" ]; then
    echo "🏃 [dry-run] 将执行:"
    echo "   git add CHANGELOG.md frontend/package.json"
    echo "   git commit -m 'release: $TAG'"
    echo "   git tag -a $TAG -m 'Meta2Cloud $TAG'"
    echo "   git push origin main --follow-tags"
    echo "✅ 演练结束，未修改任何文件"
    exit 0
fi

git add CHANGELOG.md frontend/package.json
git commit -m "release: $TAG"
git tag -a "$TAG" -m "Meta2Cloud $TAG"

echo "📦 已创建提交与 tag $TAG"
# 推送确认：-y 跳过询问；交互终端询问 y/N；非交互环境（如 CI/脚本调用，stdin 关闭）
# 自动确认——read 遇 EOF 会非零退出，在 set -e 下会静默中止导致漏推送，故需显式分支
push_confirmed=0
if [ "$ASSUME_YES" = "1" ]; then
    push_confirmed=1
elif [ -t 0 ]; then
    read -r -p "🚀 推送到 origin（触发 Docker 镜像 + GitHub Release CI）？[y/N] " reply || reply=""
    case "$reply" in
        [yY]*) push_confirmed=1 ;;
        *) ;;
    esac
else
    echo "ℹ️  非交互环境，自动确认推送（只演练请使用 --dry-run）"
    push_confirmed=1
fi
if [ "$push_confirmed" != "1" ]; then
    echo "已停止。稍后可手动执行: git push origin main --follow-tags"
    exit 0
fi

git push origin main --follow-tags
echo "🎉 $TAG 发布完成！"
echo "   - Docker 镜像: https://github.com/${GITHUB_REPOSITORY:-BenZinaDaze/Meta2Cloud}/actions"
echo "   - GitHub Release: https://github.com/${GITHUB_REPOSITORY:-BenZinaDaze/Meta2Cloud}/releases"
