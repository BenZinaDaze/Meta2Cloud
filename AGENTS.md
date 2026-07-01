# Repository Guidelines

## Project Structure & Module Organization
Meta2Cloud combines a Python backend with a Vite/React frontend. Backend code lives in `core/` for organizing workflows, `webui/` for the FastAPI app, routes, services, and schemas, plus integrations in `storage/`, `drive/`, `u115pan/`, `mediaparser/`, `nfo/`, and `scraper/`. Configuration templates and runtime data belong under `config/`. Helper scripts are in `scripts/`. Backend tests live in `test/`. Frontend source is under `frontend/src/`, with reusable UI in `frontend/src/components/`, shared helpers in `frontend/src/lib/` and `frontend/src/utils/`, and public assets in `frontend/public/`.

## Build, Test, and Development Commands
Use `uv` for Python tasks and run npm commands from `frontend/`.

- `uv sync` installs backend dependencies from `pyproject.toml` and `uv.lock`.
- `uv run uvicorn webui.app:app --host 0.0.0.0 --port 38765 --reload` starts the FastAPI backend from the repository root.
- `uv run python -m core --dry-run` previews organizer changes without moving files.
- `uv run python -m core` runs the organizer.
- `uv run pytest test` runs backend tests.
- `cd frontend && npm install` installs frontend dependencies.
- `cd frontend && npm run dev` starts the Vite dev server.
- `cd frontend && npm run build` type-checks and builds the frontend.
- `cd frontend && npm run lint` runs ESLint.

## Coding Style & Naming Conventions
Follow the existing style in nearby files. Python uses 4-space indentation, `snake_case` for modules, functions, and variables, and focused FastAPI route/service modules by feature. Frontend code uses TypeScript, PascalCase React components such as `BackToTopButton.tsx`, `useX` hook names, and camelCase helpers. Keep imports explicit and avoid mixing unrelated concerns in one module.

## Testing Guidelines
Backend tests use `pytest` and are named `test_*.py` under `test/`, for example `test_parser.py` and `test_storage.py`. Add or update tests for parser rules, storage behavior, Web UI services, and integration-specific changes. There is no stated coverage gate; at minimum run `uv run pytest test` and `cd frontend && npm run build` before submitting changes that touch both sides.

## Commit & Pull Request Guidelines
Recent commits use Conventional Commit prefixes such as `feat:`, `fix:`, and `refactor:`. Keep subjects concise and describe the user-visible change or bug fixed. Pull requests should include a short summary, testing performed, configuration or migration impact, linked issues when applicable, and screenshots for frontend changes. Never commit real credentials; start from `config/*.example.yaml` and keep local secrets out of version control.
