# GIGACODE.md — Project Creator

[Русская версия](GIGACODE.md)

## Commands

The frontend uses Bun 1.4.4 as its package manager and to run `next build`; Node.js 26 remains in use for tests and the production server.

**Go (from the repository root):**
```
go test -p 1 ./...     # integration tests run sequentially (shared model lock)
go vet ./...
gofmt -l cmd contracts graph internal
go run ./cmd/codegen   # generate graph/models_gen.go from SDL
```

**Frontend (from `frontend/`):**
```
bun install --frozen-lockfile
bun run test                # unit tests (Node, no LLM)
bun run codegen             # generate lib/generated.ts from graph/schema.graphqls
bun run typecheck
bun run build
bun run e2e                 # Playwright — requires E2E_DATABASE_URL and a separate database
bun run test:ux             # Playwright UX regressions
bun run format:check        # Biome lint
```

**Local development (host):**
```
# Backend
docker compose --env-file .env -f compose.postgres.yml up -d --wait postgres
set -a && . ./.env && set +a
go run ./cmd/server

# Frontend
cd frontend && bun install --frozen-lockfile
export PROJECT_CREATOR_TOKEN='…'
bun run dev -- -H 127.0.0.1
```

**Docker Compose (full stack):**
```
cp .env.example .env && chmod 600 .env   # set password, token, MODEL/DIGEST
docker compose up -d --wait postgres ollama
docker compose exec ollama ollama pull bge-m3:567m
docker compose exec ollama ollama pull qwen2.5:7b
docker compose up -d --build --wait
```

**Smoke test (separate stack, Ollama test stub):**
```
(
  set -e
  export COMPOSE_FILE=compose.yml:compose.smoke.yml
  export COMPOSE_PROJECT_NAME=project-creator-smoke
  export COMPOSE_ENV_FILE=.env.smoke.example
  export APP_PORT=3200
  docker compose --env-file "$COMPOSE_ENV_FILE" up -d --build --wait --wait-timeout 180
  node scripts/compose-smoke.mjs
)
```

## Architecture

- **Go backend** (`cmd/server/`) — GraphQL (graph-gophers), PostgreSQL/pgvector, SSE invalidation, background indexing, and local LLM pair analysis (Ollama / OpenVINO NPU)
- **Frontend** (`frontend/`) — Next.js 16, TypeScript, GraphQL Code Generator, Playwright E2E
- **Shared contract** — SDL `graph/schema.graphqls` → Go codegen + TS codegen; JSON contracts `contracts/*.json` for validation on both sides
- **i18n** — `contracts/i18n/{ru,en}.json`, shared Go/TypeScript dictionaries
- **Runtime** — Python CLI (`runtime/setup/`) for installation/configuration, NPU runtime (`runtime/npu/`)
- **Migrations** — SQL files in `migrations/` (automatically applied when the backend starts)
- **ADR** — `docs/adr/001–010`

## Tests

- **Go unit/integration:** `go test -p 1 ./...` — parallel execution contends on the shared model advisory lock; integration tests are skipped without `TEST_DATABASE_URL`
- **Frontend unit:** `bun run test` — Node, deterministic stubs
- **E2E:** `bun run e2e` — requires `E2E_DATABASE_URL`, a separate database, and Go + Next.js servers
- **UX:** `bun run test:ux` — Playwright desktop/narrow regressions
- **Smoke:** `scripts/compose-smoke.mjs` — full HTTP path through Docker Compose with an Ollama stub
- **Real models:** only explicitly via `REAL_MODEL_SMOKE=1` or hardware smoke tests; not included in ordinary CI
- **Hooks:** `node --test scripts/git-hooks.test.mjs`

## Conventions

- **TDD Red→Green→Refactor.** Add a failing test first, then implement, then refactor.
- **Go:** module `github.com/VolkovDani/project-creator`, Go 1.26.10. Packages: `internal/api`, `internal/analysis`, `internal/rag`, `internal/project`, `internal/requirement`, `internal/storage`, `internal/codegen`, `graph`, `contracts`, `cmd/`
- **Frontend:** Biome (`frontend/biome.jsonc`) — single quotes, double JSX quotes, semicolons, trailing commas. Scope: `frontend/**/*.{ts,tsx,json,jsonc}`. Excludes: `.next`, `lib/generated.ts`, `node_modules`
- **Codegen:** `go run ./cmd/codegen` (SDL → `graph/models_gen.go`), `bun run codegen` (SDL → `frontend/lib/generated.ts`). Do not edit generated files manually.
- **Validation:** shared contract `contracts/{project,requirement}.json` + fixtures. Go and TS check the same examples. Errors: `path`/`code`/`messageKey`/`params`
- **i18n:** all user-facing strings use keys from `contracts/i18n/`. User content is not translated.
- **SSE:** `cache.invalidate` with tags `projects`, `project:<id>`, `project:<id>:requirements`. Workspace isolation is enforced before events are sent.
- **RAG/Analysis:** local models are preferred. Ollama (CPU/GPU/Intel Arc) and experimental OpenVINO NPU. Weights are in the `ollama_data` volume, database in `postgres_data`.
- **GPU profiles:** `compose.yml` (CPU), `compose.gpu.yml` (NVIDIA), `compose.intel.yml` (Arc/Vulkan), `compose.amd.yml` (ROCm), `compose.npu.yml` (Intel NPU), `compose.smoke.yml` (test stub), `compose.postgres.yml` (database only)
- **Logs:** root `logs/` (excluded from Git and Docker), NPU — `logs/npu/`
- **Git hooks:** `.githooks/pre-commit` via `bun run hooks:install` in `frontend/`

## CI (`.github/workflows/ci.yml`)

Two jobs: `checks` (`ubuntu-24.04` + PostgreSQL service) and `containers` (`ubuntu-latest`, Docker build). Free tier: standard runners, GHA cache (10 GiB limit), no extra registries.
