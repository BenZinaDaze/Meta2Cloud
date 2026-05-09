# Repository Guidelines

## Project Structure & Module Organization
Meta2Cloud is split between a Python backend and a Vite frontend. Backend modules live in `core/` (organizing pipeline), `webui/` (FastAPI app, routes, services, schemas), `storage/`, `drive/`, `u115pan/`, and `scraper/`. Configuration templates are in `config/`, scripts in `scripts/`, and tests in `test/`. The React UI lives in `frontend/src/`; reusable components use `frontend/src/components/`, shared utilities use `frontend/src/lib/` and `frontend/src/utils/`, and static assets go in `frontend/public/`.

## Build, Test, and Development Commands
Use `uv` for Python tasks and `npm` inside `frontend/` for UI work.

- `uv sync` installs backend dependencies from `pyproject.toml`.
- `uv run uvicorn webui.app:app --host 0.0.0.0 --port 38765 --reload` starts the FastAPI server.
- `uv run python -m core --dry-run` previews organizer changes without moving files.
- `uv run python -m core` runs the organizer for real.
- `uv run pytest test` runs the backend test suite.
- `cd frontend && npm install` installs frontend dependencies.
- `cd frontend && npm run dev` starts the Vite dev server.
- `cd frontend && npm run build` type-checks and builds the frontend bundle.
- `cd frontend && npm run lint` runs ESLint on `ts` and `tsx` files.

## Coding Style & Naming Conventions
Follow the existing code style rather than introducing a new formatter. Use 4 spaces in Python, `snake_case` for modules, functions, and variables, and keep FastAPI route/service files focused by feature. In the frontend, use TypeScript, PascalCase for React components (`BackToTopButton.tsx`), `useX` for hooks, and camelCase for helpers. Keep imports explicit and avoid mixing unrelated concerns in the same module.

## Testing Guidelines
Backend tests use `pytest` and live under `test/` with names like `test_parser.py` and `test_storage.py`. Add or update tests alongside behavior changes, especially for parser rules, storage integrations, and Web UI services. There is no stated coverage gate, so contributors should at minimum run `uv run pytest test` and `cd frontend && npm run build` before opening a PR.

## Commit & Pull Request Guidelines
Recent history uses Conventional Commit prefixes such as `feat:`, `fix:`, and `refactor:`; keep that format and write concise, descriptive subjects. Pull requests should explain the user-visible change, note config or migration impact, link related issues, and include screenshots for frontend changes. Do not commit real credentials; start from `config/*.example.yaml` and keep local secrets out of version control.
