# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meta2Cloud is a media organization tool that automatically organizes movies and TV shows from cloud drives (115网盘). It parses filenames, queries TMDB for metadata, generates NFO files, downloads posters, and organizes files into a proper media library structure.

The backend is written in Go (rewritten from Python); the frontend is React 19 + TypeScript + Tailwind CSS v4.

## Development Commands

### Backend (Go)
```bash
# Build
go build ./...

# Run WebUI server (port 38765, serves frontend + API + background watchers)
go run ./cmd/meta2cloud serve

# Run organization pipeline (equivalent of the old `python -m core`)
go run ./cmd/meta2cloud pipeline              # production run
go run ./cmd/meta2cloud pipeline --dry-run    # preview without changes
go run ./cmd/meta2cloud pipeline --no-tmdb    # skip TMDB lookup
go run ./cmd/meta2cloud pipeline --no-images  # skip image downloads

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
docker logs -f meta2cloud             # View logs
docker exec meta2cloud /app/meta2cloud pipeline --dry-run  # Run pipeline in container
```

## Architecture

### Backend Structure (Go, module `meta2cloud`)
- `cmd/meta2cloud/` — CLI entry: `serve` (WebUI server) and `pipeline` (organize run) subcommands
- `internal/cloudfile/` — `CloudFile` unified data model + `Provider` interface (the storage abstraction every layer depends on)
- `internal/storage/` — Pan115Provider + provider registry + minimal OSS multipart upload (hand-rolled V1 signing)
- `internal/u115/` — 115 open-platform client (OAuth PKCE, rate limiting/cooldown, retries, files, upload hashes, share receive, offline downloads)
- `internal/config/` — YAML config loading (`config/config.yaml`, `config/parser-rules.yaml`)
- `internal/mediaparser/` — Filename parsing library (video + anime parsers, Chinese numeral/trad-simp/pinyin utilities, release-group matcher, streaming-platform table)
- `internal/tmdb/` — TMDB v3 client (search/detail/name matching, 429 retry, cache interface)
- `internal/nfo/` — Plex/Kodi/Infuse-compatible NFO XML generation
- `internal/organizer/` — Media folder organizer (idempotent `Title (Year)/Season NN` chains)
- `internal/pipeline/` — Full pipeline (scan→parse→TMDB→NFO→folders→images→move), subtitle matcher, image uploader
- `internal/scraper/` — RSS spiders (mikan + anibt) with factory/registry
- `internal/webui/` — HTTP server (Go 1.22 ServeMux), JWT auth, runtime singletons, all `/api/*` handlers, background watchers (115 auto-organize + RSS polling)
- `internal/webui/store/` — SQLite stores: library entities (tmdb_media/library_media/app_state), TMDB entity cache adapter, ingest history, app logs, RSS subscriptions, 115 auto-organize state
- `internal/jsonx/` — dynamic-JSON read helpers (Python `dict.get` semantics)

### Frontend Structure
- `frontend/src/App.tsx` - Main app with routing and state management
- `frontend/src/api.ts` - Axios API client
- `frontend/src/components/` - React components (TypeScript)
  - Page components: `LibraryPage`, `DownloadsPage`, `ConfigPage`, `SubscriptionsPage`, etc.
  - Modal components: `DetailModal`, `SubscriptionModal`, `ParseTestModal`
- Uses Tailwind CSS v4 with `@tailwindcss/vite` plugin
- Uses Radix UI primitives and shadcn-style components

## Key Patterns

### Storage Provider Pattern
All cloud storage implementations implement `cloudfile.Provider` (in `internal/cloudfile`). The `CloudFile` struct provides a unified interface for files/folders across platforms. Use `storage.GetProvider(name, cfg)` to instantiate the correct provider. Google Drive support was removed during the Go rewrite — only `pan115` is registered.

### Runtime Singletons (internal/webui/runtime.go)
`RT()` holds config/provider/u115-client/TMDB-client/store singletons, auto-invalidated when config file mtimes change. NOTE: guards use non-reentrant `sync.Mutex` — internal code must call the `*Locked` variants while holding the lock.

### API Authentication
Backend uses JWT (HS256, `golang-jwt/jwt/v5`). Frontend stores token in localStorage, sends `Authorization: Bearer` header. Middleware exempts `/api/auth:login`, `/trigger*`, and non-`/api/` paths.

### Routing Quirk
Go's ServeMux doesn't allow a suffix after a wildcard segment (`{id}:check` is invalid). Such routes register the bare wildcard and dispatch on the suffix inside the handler (see `handleSubscriptionPostOp`).

### Configuration
Config stored in YAML files (`config/config.yaml`, `config/parser-rules.yaml`). Editable via Web UI or directly. `META2CLOUD_CONFIG_DIR` env var is no longer used; the server resolves `config/` under `--root` (default: current directory).

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