# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meta2Cloud is a media organization tool that automatically organizes movies and TV shows from cloud drives (Google Drive, 115网盘, 夸克网盘). It parses filenames, queries TMDB for metadata, generates NFO files, downloads posters, and organizes files into a proper media library structure.

## Development Commands

### Backend (Python/FastAPI)
```bash
# Install dependencies (using uv)
uv sync

# Run development server
uv run uvicorn webui.app:app --host 0.0.0.0 --port 38765 --reload

# Run pipeline (media organization)
uv run python -m core                    # Production run
uv run python -m core --dry-run          # Preview without changes
uv run python -m core --storage pan115   # Use 115 cloud drive
uv run python -m core --no-tmdb          # Skip TMDB lookup
uv run python -m core --no-images        # Skip image downloads

# Run tests
uv run pytest test/                      # All tests
uv run pytest test/test_parser.py -v     # Single test file with verbose
```

### Frontend (React/Vite)
```bash
cd frontend
npm install
npm run dev      # Development server at http://localhost:5173 (proxies API to :38765)
npm run build    # Production build
npm run lint     # ESLint
```

### Docker
```bash
docker compose up -d                  # Start container
docker logs -f meta2cloud             # View logs
docker exec meta2cloud python -m core --dry-run  # Run pipeline in container
```

## Architecture

### Backend Structure
- `webui/app.py` - FastAPI application entry point, mounts all routers
- `webui/routes/` - API endpoints (auth, library, config, subscriptions, aria2, etc.)
- `webui/services/` - Business logic layer
  - `media_actions.py` - TMDB lookups, media detail fetching
  - `library_data.py` - Library queries and statistics
  - `subscriptions.py` - RSS subscription handling
  - `watcher.py` - Background tasks
- `webui/library_store.py` - SQLite database for library state
- `webui/tmdb_cache.py` - TMDB API response caching

### Frontend Structure
- `frontend/src/App.tsx` - Main app with routing and state management
- `frontend/src/api.ts` - Axios API client
- `frontend/src/components/` - React components (TypeScript)
  - Page components: `LibraryPage`, `DownloadsPage`, `ConfigPage`, `SubscriptionsPage`, etc.
  - Modal components: `DetailModal`, `SubscriptionModal`, `ParseTestModal`
- Uses Tailwind CSS v4 with `@tailwindcss/vite` plugin
- Uses Radix UI primitives and shadcn-style components

### Storage Layer
- `storage/base.py` - Abstract `StorageProvider` interface with `CloudFile` data model
- `storage/google_drive.py` - Google Drive implementation
- `storage/pan115.py` - 115网盘 implementation
- `storage/quark.py` - 夸克网盘 implementation
- `u115pan/` and `uquark/` - Low-level API clients for respective platforms

### Media Parsing
- `mediaparser/metainfo.py` - Main entry point: `MetaInfo()` for strings, `MetaInfoPath()` for file paths
- `mediaparser/meta_video.py` - Video file parsing logic
- `mediaparser/meta_anime.py` - Anime-specific parsing (uses anitopy)
- `mediaparser/tmdb.py` - TMDB API client

### NFO Generation
- `nfo/generator.py` - Generates Plex/Kodi/Infuse compatible NFO XML
- `nfo/image_uploader.py` - Downloads and uploads poster/fanart images

### Scraper (RSS Subscriptions)
- `scraper/core/base_spider.py` - Abstract spider interface
- `scraper/strategies/mikan_spider.py` - Mikan Project RSS scraper for anime

### Pipeline Flow
`core/pipeline.py` orchestrates: Scan → Parse → TMDB → NFO → Images → Move files
- Supports subtitle matching and moving via `core/subtitle_matcher.py`
- Empty folder cleanup after processing

### Scripts
- `scripts/organize_subtitles.py` - Standalone subtitle organization utility
- `scripts/upload.example.sh` - Example upload script with `/trigger` webhook

## Key Patterns

### Storage Provider Pattern
All cloud storage implementations inherit from `StorageProvider` ABC in `storage/base.py`. The `CloudFile` dataclass provides a unified interface for files/folders across platforms. Use `get_provider(name, cfg)` from `storage/__init__.py` to instantiate the correct provider.

### Frontend Modal Pattern
Modals use React portals (`createPortal`) to render at `document.body`. They typically include:
- Animation state (`show` with `requestAnimationFrame` for entry animation)
- Escape key handler
- Body scroll lock when open

### API Authentication
Backend uses JWT tokens. Frontend stores token in localStorage, includes in `Authorization: Bearer` header. Unauthorized responses trigger redirect to login.

### Configuration
Config stored in YAML files (`config/config.yaml`, `config/parser-rules.yaml`). Can be edited via Web UI or directly. Environment variable `META2CLOUD_CONFIG_DIR` overrides config directory location.

### TMDB Caching
WebUI uses SQLite cache (`webui/tmdb_cache.py`) to store TMDB responses. Pipeline writes to this cache, WebUI reads from it to avoid redundant API calls.

## Important Notes

- Frontend uses React 19 with TypeScript
- Tailwind CSS v4 syntax (uses `@tailwindcss/vite`, not `tailwind.config.js`)
- Backend runs on port 38765
- Frontend dev server proxies `/api` to backend
- Database files stored in `data/` directory at runtime
- Project uses uv for Python package management (migrated from pip)
- Test files go in `test/`, utility scripts go in `scripts/`
