# Repository Guidelines

## Project Structure & Module Organization

This repository is split into a Go backend and a Vite/React frontend. Backend entry points live in `backend/cmd`; application code is under `backend/internal` in `handlers`, `connectors`, `database`, `scheduler`, `middleware`, `observability`, and `syncstate`. SQL migrations are stored in root-level `migrations` as paired `*.up.sql` and `*.down.sql` files. Frontend code lives in `frontend/src`, with UI in `components`, screens in `pages`, shared utilities in `lib`, hooks in `hooks`, and static assets in `public` or `src/assets`. Kubernetes manifests are under `k8s/base` and `k8s/overlays/production`.

## Build, Test, and Development Commands

- `docker compose up --build`: start Postgres, backend, and frontend locally.
- `cd backend && go build ./...`: compile all backend packages.
- `cd backend && go test ./...`: run backend unit tests.
- `cd backend && go vet ./...`: run Go static checks used by CI.
- `cd frontend && npm ci`: install frontend dependencies from `package-lock.json`.
- `cd frontend && npm run dev`: run the Vite dev server.
- `cd frontend && npm run build`: type-check and build the frontend.
- `cd frontend && npm run lint`: run ESLint for TypeScript and React files.

## Coding Style & Naming Conventions

Format Go code with `gofmt`; use short, lowercase package names aligned with the existing `internal` modules. Keep HTTP handlers in `backend/internal/handlers` and external API clients in `backend/internal/connectors`. Frontend components and pages use PascalCase filenames such as `Dashboard.tsx`; hooks use camelCase names prefixed with `use`. Prefer typed helpers in `frontend/src/lib`.

## Testing Guidelines

Backend tests use Go's standard test runner and live beside implementation files as `*_test.go`. Add focused tests for handler behavior, connector parsing, scheduling policy, and migration-sensitive logic. The frontend currently has lint and build checks but no test runner.

## Commit & Pull Request Guidelines

Use Conventional Commits, matching recent history such as `feat: add income categories to finance` and `fix: align finance accounts card height`. Keep commit subjects concise and exactly 50 characters when committing from this workspace. Pull requests should describe the change, list verification commands, link related issues, and include screenshots for visible frontend updates.

## Security & Configuration Tips

Copy `.env.example` to `.env` for local secrets and never commit real tokens. Connector credentials are supplied through environment variables in `docker-compose.yml`. Keep production secrets sealed under `k8s/overlays/production`.
