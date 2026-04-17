# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meta2Cloud is a media organization tool that automatically organizes movies and TV shows from cloud drives (Google Drive, 115网盘, 夸克网盘). It parses filenames, queries TMDB for metadata, generates NFO files, downloads posters, and organizes files into a proper media library structure.

## Development Commands

### Backend (Python/FastAPI)
```bash
# Install dependencies
pip install -r requirements.txt

# Run development server
uvicorn webui.app:app --host 0.0.0.0 --port 38765 --reload

# Run pipeline (media organization)
python pipeline.py                    # Production run
python pipeline.py --dry-run          # Preview without changes
python pipeline.py --storage pan115   # Use 115 cloud drive
python pipeline.py --no-tmdb          # Skip TMDB lookup
python pipeline.py --no-images        # Skip image downloads
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
docker exec meta2cloud python pipeline.py --dry-run  # Run pipeline in container
```

## Architecture

### Backend Structure
- `webui/app.py` - FastAPI application entry point, mounts all routers
- `webui/routes/` - API endpoints (auth, library, config, subscriptions, aria2, etc.)
- `webui/services/` - Business logic layer
  - `media_actions.py` - TMDB lookups, media detail fetching
  - `library_data.py` - Library queries and statistics
  - `config.py` - Configuration management
  - `subscriptions.py` - RSS subscription handling
  - `watcher.py` - Background tasks
- `webui/library_store.py` - SQLite database for library state
- `webui/tmdb_cache.py` - TMDB API response caching

### Frontend Structure
- `frontend/src/App.jsx` - Main app with routing and state management
- `frontend/src/api.js` - Axios API client
- `frontend/src/components/` - React components (no TypeScript)
  - Page components: `LibraryPage`, `DownloadsPage`, `ConfigPage`, `SubscriptionsPage`, etc.
  - Modal components: `DetailModal`, `SubscriptionModal`, `ParseTestModal`
- Uses Tailwind CSS v4 with `@tailwindcss/vite` plugin

### Storage Layer
- `storage/base.py` - Abstract `StorageProvider` interface
- `storage/google_drive.py` - Google Drive implementation
- `storage/pan115.py` - 115网盘 implementation
- `storage/quark.py` - 夸克网盘 implementation

### Media Parsing
- `mediaparser/metainfo.py` - Main entry point for filename parsing
- `mediaparser/meta_video.py` - Video file parsing logic
- `mediaparser/meta_anime.py` - Anime-specific parsing
- `mediaparser/tmdb.py` - TMDB API client

### Pipeline Flow
`pipeline.py` orchestrates: Scan → Parse → TMDB → NFO → Images → Move files

## Key Patterns

### Frontend Modal Pattern
Modals use React portals (`createPortal`) to render at `document.body`. They typically include:
- Animation state (`show` with `requestAnimationFrame` for entry animation)
- Escape key handler
- Body scroll lock when open

### API Authentication
Backend uses JWT tokens. Frontend stores token in localStorage, includes in `Authorization: Bearer` header. Unauthorized responses trigger redirect to login.

### Configuration
Config stored in YAML files (`config/config.yaml`, `config/parser-rules.yaml`). Can be edited via Web UI or directly. Environment variable `META2CLOUD_CONFIG_DIR` overrides config directory location.

## Important Notes

- Frontend uses React 19 with JSX (no TypeScript)
- Tailwind CSS v4 syntax (uses `@tailwindcss/vite`, not `tailwind.config.js`)
- Backend runs on port 38765
- Frontend dev server proxies `/api` to backend
- Database files stored in `data/` directory at runtime
