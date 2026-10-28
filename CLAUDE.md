# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meta2Cloud is a one-stop media management platform for 115 网盘 (cloud drive). It covers the full chain: 资源检索/订阅 (mikan + anibt) → 115 云下载 (offline tasks) → 自动整理 (filename parsing → TMDB matching → standardized folders) → 播放 (STRM 302 redirect for Emby/Jellyfin, or direct-connect players) → 观看同步 (Plex ↔ Trakt). Everything is operated through the Web UI / WebAPI — the CLI only starts the server.

The backend is written in Go (rewritten from Python); the frontend is React 19 + TypeScript + react-router + Tailwind CSS v4 (frontend code under `frontend/`).

## Development Commands

```bash
# Run WebUI server (port 38765, serves frontend + API + background watchers).
# `serve` is the ONLY subcommand — organizing is triggered via POST /api/pipeline/trigger
# or POST /trigger (webhook), never via CLI.
go run ./cmd/meta2cloud serve

scripts/dev.sh        # start backend + frontend together
docker logs -f meta2cloud   # first-boot initial password appears here
```

Standard `go build/test/vet ./...` and `npm run dev/build/lint` (in `frontend/`) apply.

## Key Patterns

### Storage Provider Pattern
All cloud storage implementations implement `cloudfile.Provider` (in `internal/cloudfile`). The `CloudFile` struct provides a unified interface for files/folders across platforms. Use `storage.GetProvider(name, cfg)` to instantiate the correct provider. Google Drive support was removed during the Go rewrite — only `pan115` is registered.

### Runtime Singletons (internal/webui/runtime.go)
`RT()` holds config/provider/u115-client/TMDB-client/store singletons, auto-invalidated when config file mtimes change. NOTE: guards use non-reentrant `sync.Mutex` — internal code must call the `*Locked` variants while holding the lock.

### API Authentication
Backend uses JWT (HS256, `golang-jwt/jwt/v5`). Frontend stores token in localStorage, sends `Authorization: Bearer` header. Middleware exempts `/api/auth:login`, `/trigger*`, `/strm/redirect` (protected by optional `url_secret` instead), and non-`/api/` paths.

### Routing Quirk
Go's ServeMux doesn't allow a suffix after a wildcard segment (`{id}:check` is invalid). Such routes register the bare wildcard and dispatch on the suffix inside the handler (see `handleSubscriptionPostOp`).

### Configuration
Four YAML files under `config/`: `config.yaml` (main), `secrets.yaml` (敏感键，0600，值优先于主配置), `parser-rules.yaml` (识别规则), `categories.yaml` (媒体分类). All are editable via Web UI; missing files are auto-generated with defaults on first boot. WebUI 密码只存 bcrypt 哈希（`webui.password_hash`，旧版明文 `webui.password` 登录时自动迁移）；敏感键集合见 `config.SecretKeys`，保存配置时自动拆入 secrets.yaml。u115 目录配置存目录名（WebUI 目录选择器直选网盘目录），不再手填 cid。`META2CLOUD_CONFIG_DIR` env var is not used; the server resolves `config/` under `--root` (default: current directory).

### TMDB Caching
TMDB responses are cached in the `tmdb_media` table of `library.db` (entity-keyed by media_type+tmdb_id, seasons/episodes nested in JSON columns). Pipeline and WebUI share the same cache.

### SQLite
All stores use `modernc.org/sqlite` (pure Go, no CGO — don't suggest golang.org/x/sqlite or CGO-based drivers) with WAL mode and `SetMaxOpenConns(1)`. Table schemas are identical to the Python version, so existing `library.db` files work as-is.

## Important Notes

- **115 API QPS 限制为 3**：手动测试 115 接口时必须串行且间隔 ≥1s，密集调用会触发 429 并进入约 3600s 冷却；新增批量网盘操作须走全项目统一的 QPS 节拍
- Frontend dev server proxies `/api` to backend on port 38765
- Database files live in `config/data/` at runtime
- Test files live alongside code (`*_test.go`); utility scripts go in `scripts/`
- The mediaparser reimplements cn2an/zhconv/Pinyin2Hanzi/anitopy subsets natively (no Go equivalents existed — don't suggest external Go ports)
