# 发版手册

本文档描述 Panoply 的完整发版流程。日常发版只需一条命令：

```bash
scripts/release.sh 2.2.0        # 发布 v2.2.0
```

## 流程总览

```
本地准备 ──► 一键发版 ──► push tag ──► CI 自动化 ──► 发布完成
             (release.sh)              │
                                       ├─ docker-publish.yml → Docker Hub 多平台镜像
                                       └─ release.yml        → GitHub Release（正文取自 CHANGELOG）
```

推送 tag 后一切自动化，无需在 GitHub 网页上手工填写 Release 说明。

## 发版前准备清单

| # | 检查项 | 说明 |
|---|--------|------|
| 1 | 待发内容已全部合入 `main` | 确认没有遗漏的分支/未推送提交 |
| 2 | 工作区干净 | 脚本会拦截未提交变更（未跟踪的本地 db 文件不影响） |
| 3 | 本地 `main` 与 `origin/main` 一致 | 避免基于过期代码发版，脚本会校验 |
| 4 | 版本号确定 | 规则见下文「版本号规范」 |
| 5 | `go vet` / `go test` / 前端 build 通过 | 脚本自动执行（`--skip-checks` 可跳过，不建议） |
| 6 | CHANGELOG 内容可读 | 脚本自动生成草稿，commit 前可手工润色 |

## 版本号规范

遵循[语义化版本](https://semver.org/lang/zh-CN/) `vMAJOR.MINOR.PATCH`：

- **MAJOR**：不兼容的配置/接口/数据变更（如 v1 → v2 的 Go 重构）
- **MINOR**：新增向后兼容的功能（如 v2.1.0 的 UI 重构）
- **PATCH**：bug 修复（如 v2.0.1、v2.0.2）

提交信息遵循 Conventional Commits（`feat:` / `fix:` / `feat!:` 破坏性等），
CHANGELOG 草稿按此自动分类。

## 一键发版

```bash
scripts/release.sh 2.2.0               # 标准发版
scripts/release.sh --dry-run 2.2.0     # 演练：只打印动作，不修改任何文件
scripts/release.sh --skip-checks 2.2.0 # 跳过 go/npm 检查（应急用）
scripts/release.sh -y 2.2.0            # 跳过 push 前的确认
```

脚本依次执行：

1. **预检** — main 分支、工作区干净、与远端同步、tag 不存在、版本号递增
2. **检查** — `go build` / `go vet` / `go test ./internal/...` / `npm run build`
3. **同步版本号** — `frontend/package.json` 的 `version` 字段
4. **生成 CHANGELOG** — 将 `上一个 tag..HEAD` 的提交按类型分组，
   插入 `CHANGELOG.md` 顶部（此时可手工润色，脚本会等待确认后再提交）
5. **提交 + 打 tag** — `release: vX.Y.Z` 提交 + annotated tag
6. **推送** — 确认后 `git push origin main --follow-tags`

> 注意：macOS 自带 bash 3.2 与新版 bash 均兼容；脚本无 gawk 依赖。

## 手工发版（不依赖脚本）

1. 确认工作区干净、`main` 与远端同步
2. 更新 `frontend/package.json` 的 `version` 字段
3. 在 `CHANGELOG.md` 顶部插入新版本段落（格式对齐现有条目）：

   ```markdown
   ## [vX.Y.Z] - YYYY-MM-DD

   ### 新增
   - ... (`commit-hash`)

   ### 修复
   - ... (`commit-hash`)
   ```

4. 提交并打 tag：

   ```bash
   git add CHANGELOG.md frontend/package.json
   git commit -m "release: vX.Y.Z"
   git tag -a vX.Y.Z -m "Panoply vX.Y.Z"
   git push origin main --follow-tags
   ```

## CI 自动化

推送 `v*` tag 后两个 workflow 并行执行：

### Docker 镜像（docker-publish.yml）

构建 `linux/amd64` + `linux/arm64` 多平台镜像并推送 Docker Hub：

- `benz1/panoply:vX.Y.Z`
- `benz1/panoply:latest`

镜像内通过 `APP_VERSION` 注入版本号（Go `main.version` + 前端 `VITE_APP_VERSION`）。

### GitHub Release（release.yml）

- 从 `CHANGELOG.md` 提取 `## [vX.Y.Z]` 段落作为 Release 正文
- 段落缺失时回退为自动生成的简短说明（并给出 `docker pull` 命令）
- 用 `gh release create --verify-tag` 创建，tag 必须真实存在

## 发布后验证

```bash
gh run list --limit 4                          # 两个 workflow 均应绿色
gh release view vX.Y.Z                         # Release 存在且正文完整
docker manifest inspect benz1/panoply:vX.Y.Z | grep architecture   # amd64 + arm64
docker pull benz1/panoply:vX.Y.Z            # 本地拉取冒烟
```

用户视角验证：WebUI 侧边栏/关于页显示的版本号应为 `vX.Y.Z`。

## 异常处理

- **CI 失败（镜像构建）**：修复后无需重发版，手动触发 `docker-publish.yml`
  的 `workflow_dispatch`，它会自动取最新 tag 重新构建推送
- **Release 正文有误**：`gh release edit vX.Y.Z --notes-file <文件>` 或网页编辑
- **彻底撤回一个版本**（慎用，镜像可能已被拉取）：

  ```bash
  gh release delete vX.Y.Z --yes
  git push origin :refs/tags/vX.Y.Z
  ```
