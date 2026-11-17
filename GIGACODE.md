# GIGACODE.md — Project Creator

## Команды

Фронтенд использует Bun 1.4.4 как менеджер пакетов и для `next build`; Node.js 26 остаётся для тестов и production-сервера.


**Go (из корня):**
```
go test -p 1 ./...     # интеграционные тесты — строго последовательно (общая блокировка модели)
go vet ./...
gofmt -l cmd contracts graph internal
go run ./cmd/codegen   # генерация graph/models_gen.go из SDL
```

**Frontend (из `frontend/`):**
```
bun install --frozen-lockfile
bun run test                # unit (Node, без LLM)
bun run codegen         # генерация lib/generated.ts из graph/schema.graphqls
bun run typecheck
bun run build
bun run e2e             # Playwright — требует E2E_DATABASE_URL и отдельную БД
bun run test:ux         # Playwright UX-регрессии
bun run format:check    # Biome lint
```

**Локальная разработка (хост):**
```
# Бэкенд
docker compose --env-file .env -f compose.postgres.yml up -d --wait postgres
set -a && . ./.env && set +a
go run ./cmd/server

# Фронтенд
cd frontend && bun install --frozen-lockfile
export PROJECT_CREATOR_TOKEN='…'
bun run dev -- -H 127.0.0.1
```

**Docker Compose (весь стек):**
```
cp .env.example .env && chmod 600 .env   # задать пароль, токен, MODEL/DIGEST
docker compose up -d --wait postgres ollama
docker compose exec ollama ollama pull bge-m3:567m
docker compose exec ollama ollama pull qwen2.5:7b
docker compose up -d --build --wait
```

**Smoke-тест (отдельный стек, тестовая заглушка Ollama):**
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

## Архитектура

- **Go backend** (`cmd/server/`) — GraphQL (graph-gophers), PostgreSQL/pgvector, SSE invalidation, фоновая индексация, анализ пар локальной LLM (Ollama / OpenVINO NPU)
- **Frontend** (`frontend/`) — Next.js 16, TypeScript, GraphQL Code Generator, Playwright E2E
- **Общий контракт** — SDL `graph/schema.graphqls` → Go codegen + TS codegen; JSON-контракты `contracts/*.json` для валидации на обеих сторонах
- **i18n** — `contracts/i18n/{ru,en}.json`, общие словари Go/TypeScript
- **Runtime** — Python CLI (`runtime/setup/`) для установки/настройки, NPU runtime (`runtime/npu/`)
- **Миграции** — SQL-файлы в `migrations/` (автоматический apply при старте backend)
- **ADR** — `docs/adr/001–010`

## Тесты

- **Go unit/integration:** `go test -p 1 ./...` — параллельный запуск блокирует общую advisory lock модели; без `TEST_DATABASE_URL` интеграционные тесты пропускаются
- **Frontend unit:** `bun run test` — Node, детерминированные стабы
- **E2E:** `bun run e2e` — требует `E2E_DATABASE_URL`, отдельная БД, Go + Next.js серверы
- **UX:** `bun run test:ux` — Playwright desktop/narrow регрессии
- **Smoke:** `scripts/compose-smoke.mjs` — полный HTTP-путь через Docker Compose со стабом Ollama
- **Реальные модели:** только явно через `REAL_MODEL_SMOKE=1` или hardware smoke-тесты; не включены в обычный CI
- **Hooks:** `node --test scripts/git-hooks.test.mjs`

## Конвенции

- **TDD Red→Green→Refactor.** Сначала падающий тест, потом реализация, потом рефакторинг.
- **Go:** модуль `github.com/VolkovDani/project-creator`, Go 1.26.10. Пакеты: `internal/api`, `internal/analysis`, `internal/rag`, `internal/project`, `internal/requirement`, `internal/storage`, `internal/codegen`, `graph`, `contracts`, `cmd/`
- **Frontend:** Biome (`frontend/biome.jsonc`) — single quotes, double jsx quotes, semicolons, trailing commas. Scope: `frontend/**/*.{ts,tsx,json,jsonc}`. Исключены: `.next`, `lib/generated.ts`, `node_modules`
- **Codegen:** `go run ./cmd/codegen` (SDL → `graph/models_gen.go`), `bun run codegen` (SDL → `frontend/lib/generated.ts`). Сгенерированные файлы не редактировать вручную
- **Валидация:** общий контракт `contracts/{project,requirement}.json` + fixtures. Go и TS проверяют один набор примеров. Ошибки: `path`/`code`/`messageKey`/`params`
- **i18n:** все пользовательские строки через ключи из `contracts/i18n/`. Пользовательский контент не переводится
- **SSE:** `cache.invalidate` с тегами `projects`, `project:<id>`, `project:<id>:requirements`. workspace-изоляция до выдачи событий
- **RAG/Analysis:** локальные модели приоритет. Ollama (CPU/GPU/Intel Arc) и экспериментальный OpenVINO NPU. Веса в volume `ollama_data`, БД в `postgres_data`
- **GPU-профили:** `compose.yml` (CPU), `compose.gpu.yml` (NVIDIA), `compose.intel.yml` (Arc/Vulkan), `compose.amd.yml` (ROCm), `compose.npu.yml` (Intel NPU), `compose.smoke.yml` (тестовая заглушка), `compose.postgres.yml` (только БД)
- **Logs:** корневой `logs/` (исключён из Git и Docker), NPU — `logs/npu/`
- **Git hooks:** `.githooks/pre-commit` через `bun run hooks:install` в `frontend/`

## CI (`.github/workflows/ci.yml`)

Два job: `checks` (ubuntu-24.04 + PostgreSQL service) и `containers` (ubuntu-latest, Docker build). Free-tier: стандартные runners, GHA cache (10 GiБ лимит), no extra registries.
