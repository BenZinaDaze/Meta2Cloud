# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meta2Cloud is a one-stop media management platform for 115 网盘 (cloud drive). It covers the full chain: 资源检索/订阅 (mikan + anibt) → 115 云下载 (offline tasks) → 自动整理 (filename parsing → TMDB matching → standardized folders) → 播放 (STRM 302 redirect for Emby/Jellyfin, or direct-connect players) → 观看同步 (Plex ↔ Trakt). Everything is operated through the Web UI / WebAPI — the CLI only starts the server.

The backend is written in Go (rewritten from Python); the frontend is React 19 + TypeScript + react-router + Tailwind CSS v4.

## Development Commands

### Backend (Go)
```bash
# Build
go build ./...

# Run WebUI server (port 38765, serves frontend + API + background watchers).
# This is the ONLY subcommand — organizing is triggered via POST /api/pipeline/trigger
# or POST /trigger (webhook), never via CLI.
go run ./cmd/meta2cloud serve

# Run tests
go test ./internal/... -count=1               # all tests
go test ./internal/mediaparser/ -v            # single package
go vet ./...                                  # static analysis
```

### Frontend (React/Vite)
```bash
cd frontend
npm install
npm run dev      # Development server at http://localhost:5173 (proxies API to :38765)
npm run build    # Production build
npm run lint     # ESLint
```

Or use `scripts/dev.sh` to start both together.

### Docker
```bash
docker compose up -d                  # Start container
docker logs -f meta2cloud             # View logs (first-boot initial password appears here)
docker exec meta2cloud /app/meta2cloud serve     # server runs under compose by default
```

## Architecture

### Backend Structure (Go, module `meta2cloud`)
- `cmd/meta2cloud/` — CLI entry: `serve` subcommand only (host/port/root flags)
- `internal/cloudfile/` — `CloudFile` unified data model + `Provider` interface (the storage abstraction every layer depends on)
- `internal/storage/` — Pan115Provider + provider registry + minimal OSS multipart upload (hand-rolled V1 signing)
- `internal/u115/` — 115 open-platform client (OAuth PKCE + QR scan, rate limiting/cooldown, retries, files, upload hashes, share receive, offline downloads)
- `internal/config/` — YAML config loading; `EnsureDefaultFiles` generates default `config.yaml`/`parser-rules.yaml`/`categories.yaml` (with random initial WebUI password, printed to the startup log) on first boot
- `internal/mediaparser/` — Filename parsing library (video + anime parsers, Chinese numeral/trad-simp/pinyin utilities, release-group matcher, streaming-platform table)
- `internal/tmdb/` — TMDB v3 client (search/detail/name matching, 429 retry, cache interface)
- `internal/organizer/` — Media folder organizer (idempotent `Title (Year)/Season NN` chains)
- `internal/pipeline/` — Organize flow (scan→parse→TMDB→folders→move), subtitle matcher; NFO/cover upload was removed in v2.4.0 — metadata is left to the player
- `internal/scraper/` — Resource-site spiders (mikan + anibt) with factory/registry
- `internal/trakt/` — Trakt client (device-code OAuth, history/sync endpoints)
- `internal/plexserver/` — Plex Media Server client (PIN auth, sessions, SSE realtime push)
- `internal/syncengine/` — Bidirectional watched-state sync engine (Trakt ↔ Plex, union merge on first sync, Trakt wins on conflict)
- `internal/watched/` — Shared watched-record domain types
- `internal/webui/` — HTTP server (Go 1.22 ServeMux), JWT auth, runtime singletons, all `/api/*` handlers, background watchers (115 云下载自动整理 + RSS 订阅轮询), STRM 302 redirect endpoint, WebSocket live logs
- `internal/webui/store/` — SQLite stores: library entities (tmdb_media/library_media/app_state), TMDB entity cache adapter, ingest history, app logs, RSS subscriptions, 115 auto-organize state, STRM sync runs/playback, Trakt sync runs/ignores, media categories, title pins
- `internal/jsonx/` — dynamic-JSON read helpers (Python `dict.get` semantics)

### Frontend Structure
- `frontend/src/App.tsx` - Router setup (react-router; page navigation is route-driven, not state-driven)
- `frontend/src/api.ts` - Axios API client
- `frontend/src/components/` - React components (TypeScript)
  - `layout/` — Sidebar / MobileNav / Topbar (nav: 仪表盘、媒体库、新番列表、资源检索、订阅列表、云下载、文件管理、入库记录、整理名称、STRM 同步、播放记录、Trakt 同步、日志、配置)
  - `pages/` — Page components matching the routes above (e.g. `DashboardPage`, `LibraryPage`, `CalendarPage`, `ScraperSearch`, `SubscriptionsPage`, `U115OfflinePage`, `FileManagerPage`, `TitlePinsPage`, `StrmSyncPage`, `StrmPlaybackPage`, `WatchedSyncPage`, `ConfigPage`)
  - `modals/` — `DetailModal`, `SubscriptionModal`, `ParseTestModal`, `ReidentifyModal`, etc.
- Uses Tailwind CSS v4 with `@tailwindcss/vite` plugin, Radix UI primitives and shadcn-style components

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
Three YAML files under `config/`: `config.yaml` (main), `parser-rules.yaml` (识别规则), `categories.yaml` (媒体分类). All are editable via Web UI; missing files are auto-generated with defaults on first boot. u115 目录配置存目录名（WebUI 目录选择器直选网盘目录），不再手填 cid。`META2CLOUD_CONFIG_DIR` env var is not used; the server resolves `config/` under `--root` (default: current directory).

### TMDB Caching
TMDB responses are cached in the `tmdb_media` table of `library.db` (entity-keyed by media_type+tmdb_id, seasons/episodes nested in JSON columns). Pipeline and WebUI share the same cache.

### SQLite
All stores use `modernc.org/sqlite` (pure Go, no CGO) with WAL mode and `SetMaxOpenConns(1)`. Table schemas are identical to the Python version, so existing `library.db` files work as-is.

## Important Notes

- **115 API QPS 限制为 3**：手动测试 115 接口时必须串行且间隔 ≥1s，密集调用会触发 429 并进入约 3600s 冷却；新增批量网盘操作须走全项目统一的 QPS 节拍
- Frontend uses React 19 with TypeScript
- Tailwind CSS v4 syntax (uses `@tailwindcss/vite`, not `tailwind.config.js`)
- Backend runs on port 38765
- Frontend dev server proxies `/api` to backend
- Database files live in `config/data/` at runtime
- Dependencies: `yaml.v3`, `regexp2` (lookaround regex), `goquery` (HTML), `go-qrcode`, `golang-jwt/jwt/v5`, `modernc.org/sqlite`
- The mediaparser reimplements cn2an/zhconv/Pinyin2Hanzi/anitopy subsets natively (no Go equivalents existed)
- Test files live alongside code (`*_test.go`); utility scripts go in `scripts/`
