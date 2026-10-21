# Repository Guidelines

## Project Structure & Module Organization
Meta2Cloud combines a Go backend with a Vite/React frontend. Backend code lives in `cmd/meta2cloud/` (CLI entry: `serve` subcommand) and `internal/`: `cloudfile/` (storage abstraction), `storage/` (Pan115 provider + registry), `u115/` (115 open-platform client), `config/` (YAML loading), `mediaparser/` (filename parsing), `tmdb/`, `organizer/`, `pipeline/` (organize flow, subtitle matcher), `scraper/` (mikan + anibt spiders), `trakt/`, `plexserver/`, `syncengine/` (Plex↔Trakt watched-state sync), `watched/`, `webui/` (HTTP server, JWT auth, runtime singletons, API handlers, background watchers), and `webui/store/` (SQLite stores). Configuration templates and runtime data belong under `config/`. Helper scripts are in `scripts/`. Backend tests live next to the code as `*_test.go`. Frontend source is under `frontend/src/`, with reusable UI in `frontend/src/components/`, shared helpers in `frontend/src/lib/` and `frontend/src/utils/`, and public assets in `frontend/public/`.

## Build, Test, and Development Commands
Use standard Go toolchain from the repository root; run npm commands from `frontend/`.

- `go build ./...` builds the backend.
- `go run ./cmd/meta2cloud serve` starts the WebUI backend (port 38765) from the repository root.
- All organizing runs through the WebAPI (`POST /api/pipeline/trigger` or the WebUI「一键整理」button) — there is no pipeline CLI subcommand.
- `go test ./internal/... -count=1` runs backend tests.
- `go vet ./...` runs static analysis.
- `cd frontend && npm install` installs frontend dependencies.
- `cd frontend && npm run dev` starts the Vite dev server.
- `cd frontend && npm run build` type-checks and builds the frontend.
- `cd frontend && npm run lint` runs ESLint.
- `scripts/dev.sh` starts backend + frontend together for development.

## Coding Style & Naming Conventions
Follow the existing style in nearby files. Go uses tabs via `gofmt` (run `gofmt -w .` before committing), package-level doc comments in Chinese matching existing files, `snake_case` JSON field tags matching the frontend API contract. Frontend code uses TypeScript, PascalCase React components such as `BackToTopButton.tsx`, `useX` hook names, and camelCase helpers. Keep imports explicit and avoid mixing unrelated concerns in one module.

## Testing Guidelines
Backend tests are Go tests colocated with packages (`*_test.go`), e.g. `internal/mediaparser/metainfo_test.go` and `internal/webui/store/library_test.go`. HTTP handlers are tested end-to-end via `httptest.Server`; external APIs (115, TMDB, mikan) are mocked with `httptest` servers. Add or update tests for parser rules, storage behavior, Web UI services, and integration-specific changes. There is no stated coverage gate; at minimum run `go test ./internal/... -count=1` and `cd frontend && npm run build` before submitting changes that touch both sides.

## Commit & Pull Request Guidelines
Recent commits use Conventional Commit prefixes such as `feat:`, `fix:`, and `refactor:`. Keep subjects concise and describe the user-visible change or bug fixed. Pull requests should include a short summary, testing performed, configuration or migration impact, linked issues when applicable, and screenshots for frontend changes. Never commit real credentials; start from `config/*.example.yaml` and keep local secrets out of version control.
