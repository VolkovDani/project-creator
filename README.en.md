# Project Creator

[Русская версия](README.md)

## One-command setup

On native Linux x86_64 with Python 3.12 and Docker Engine/Compose:

```bash
./project-creator setup --device cpu  # or arc, nvidia, amd, amd-vulkan, npu
./project-creator up
./project-creator doctor
./project-creator down
```

`setup` explicitly downloads local models, prepares the NPU when that device is
selected, starts the application, and checks a real AI workflow. Existing
`.env`, the Compose project name, data, and model weights are preserved. System
prerequisites, diagnostics, resuming after interruption, and hardware limitations
are described in the [CLI guide](docs/setup-cli.md). Driver installation is done
separately.

A product knowledge system and business-requirement collision checker.
See [GOAL.md](GOAL.md) for project goals and [ARCHITECTURE.md](ARCHITECTURE.md)
for architecture contracts.

## MVP status

Project cards and business requirements, a GraphQL API with PostgreSQL, and a
Next.js interface are implemented. Background indexing with local embeddings and
explicit semantic search within a project are available. Explicit analysis by a
local LLM checks selected pairs for possible collisions and preserves source
versions.

Implementation order:

1. [Card model and validation](https://github.com/VolkovDani/project-creator/issues/2).
2. [GraphQL API and storage](https://github.com/VolkovDani/project-creator/issues/3).
3. [Next.js and project cards](https://github.com/VolkovDani/project-creator/issues/4).
4. [Business requirements](https://github.com/VolkovDani/project-creator/issues/5).
5. [PostgreSQL + pgvector](https://github.com/VolkovDani/project-creator/issues/14).
6. [SSE invalidation](https://github.com/VolkovDani/project-creator/issues/6).
7. [Local RAG](https://github.com/VolkovDani/project-creator/issues/7).
8. [Collision analysis](https://github.com/VolkovDani/project-creator/issues/8).
9. [Docker deployment and CI](https://github.com/VolkovDani/project-creator/issues/9).

Preliminary Laya classification and the two-model pipeline are enabled explicitly;
configuration and length limits are described in the [Laya guide](docs/laya-pipeline.md).
Hardware acceptance #48 remains open.

## Run the complete MVP with Docker Compose

Docker Engine with Compose v2 (the `docker compose` command), or Docker Desktop
with Linux containers, is required. Go and Node.js are not needed on the host to
run the application. Run commands from the repository root. CPU is used by
default; a GPU is optional, but analysis may exceed the timeout on a slow CPU.

The configuration pins Go 1.26.10, Node.js 24.22.1 on Debian Bookworm,
PostgreSQL 16 with pgvector 0.8.0, and Ollama 0.40.16. Models are downloaded
separately:

| Purpose | Model | Weight download size |
| --- | --- | --- |
| Multilingual embeddings, 1024 dimensions | `bge-m3:567m` | about 1.2 GB |
| Requirement-pair analysis, Q4_K_M | `qwen2.5:7b` | about 4.7 GB |

The sizes are published on the official [BGE-M3](https://ollama.com/library/bge-m3)
and [Qwen2.5 7B](https://ollama.com/library/qwen2.5:7b) model pages. Model tags
may change; the application checks the full digest of the installed model. The
BGE-M3 page lists MIT, and Qwen2.5 7B lists Apache 2.0; other Qwen sizes may have
a different license.

For planning, allocate 24–32 GiB RAM for the CPU version of the whole stack and
at least 20 GiB of free disk space for images, weights, and builds, plus space
for the database and backups. For the optional GPU version, plan for 12–16 GiB
VRAM plus RAM for the application/database. This is an approximate budget, not a
measured minimum: the generator requests a 32768-token context, and memory use
depends on the KV cache, simultaneous model loading, driver, and runtime. Weight
file size is not the same as RAM/VRAM consumption. Check actual usage with
`docker stats` and `docker compose exec ollama ollama ps`.

### First run and model download

```bash
cp .env.example .env
chmod 600 .env
```

Edit `.env`: replace `POSTGRES_PASSWORD` and `PROJECT_CREATOR_TOKEN` with
different random values; the token must be at least 32 characters. For example,
generate each value with a separate call to `openssl rand -hex 24`. Use hex without
URL-special characters for the Compose password. Do not commit `.env`. The
`PROJECT_CREATOR_DATABASE_URL` and `POSTGRES_PORT` variables are needed to run the
API on the host; the full Compose setup supplies the internal database address
itself.

```bash
docker compose up -d --wait postgres ollama
docker compose exec ollama ollama pull bge-m3:567m
docker compose exec ollama ollama pull qwen2.5:7b
docker compose --profile tools run --rm model-info
```

The last command prints the local `/api/tags` JSON. Find the entries with the
required `name` and copy their **full** `digest`, in the form `sha256:` followed
by 64 hexadecimal characters; the short ID from `ollama list` is not sufficient.
Add these values to `.env`:

```dotenv
PROJECT_CREATOR_EMBEDDING_MODEL=bge-m3:567m
PROJECT_CREATOR_EMBEDDING_DIGEST=sha256:full-digest-of-installed-embedding-model
PROJECT_CREATOR_GENERATOR_MODEL=qwen2.5:7b
PROJECT_CREATOR_GENERATOR_DIGEST=sha256:full-digest-of-installed-generator
```

```bash
docker compose up -d --build --wait
docker compose ps
```

Open [http://127.0.0.1:3000](http://127.0.0.1:3000). If the port is occupied,
set `APP_PORT`, for example `APP_PORT=3100`, in `.env` and use the new port. Only
`127.0.0.1:${APP_PORT:-3000}` is published to the host. PostgreSQL, backend, and
Ollama are available on the Compose network and do not publish ports. Weights are
stored in the `ollama_data` volume; projects, requirements, indexes, and analyses
are stored in `postgres_data`. The Compose project name is part of the actual
volume names, so keep it when updating.

Weights are not downloaded automatically at startup. You can run the full stack
without AI by leaving both `MODEL`/`DIGEST` pairs unset: CRUD works, while
indexing and analysis report that the provider is not configured. Setting only
one variable in a pair is insufficient; invalid configuration prevents the
backend from starting. The full Compose setup sets
`PROJECT_CREATOR_OLLAMA_URL=http://ollama:11434`. After changing a model/digest,
recreate the backend with `docker compose up -d --wait backend`. Changing the
embedding profile requires reindexing requirements for semantic search; full
pair analysis does not depend on embeddings. Changing the generator requires a
new analysis; existing vectors remain usable.

Health checks verify PostgreSQL, that the Ollama process is available, and a real
authenticated request through backend/Next.js. The backend applies migrations
before accepting requests. `healthy` does not confirm that weights are loaded or
that the LLM is high quality. For diagnostics, use
`docker compose logs --tail=100 backend frontend ollama` and inspect the request
result (the `errors` field should be absent):

```bash
curl -fsS http://127.0.0.1:3000/api/graphql \
  -H 'Content-Type: application/json' \
  --data '{"query":"{projects(limit:1,offset:0){id}}"}'
```

Store local logs and diagnostic reports in the root `logs/` directory. NPU
runtime and CLI setup reports go in `logs/npu/`. The directory is excluded from
Git and the Docker build context. Container logs are available through
`docker compose logs`; to save them to a file, use `mkdir -p logs` and
`docker compose logs --no-color > logs/compose.log`.

Manual acceptance: create a project with a goal, then add the requirements
“Payment is allowed by card only” and “Payment is allowed in cash only.” Explicitly
index both, search, and start an analysis. Check the links, versions, and source
snapshots; a person evaluates any possible collision. If data is insufficient or
an error occurs, check the model and index, then retry. After
`docker compose restart postgres backend`, wait for `healthy` and confirm that the
project, requirements, index, and result remain. Edit one requirement: the
previous analysis should become stale. Failure to find a collision is not proof
of consistency.

### Intel Arc / Core Ultra laptops

For integrated Intel Arc, including Core Ultra 7 268V, `compose.intel.yml` is
provided: Ollama with Vulkan and the Intel Mesa driver inside the container.
Ubuntu preparation, first run, checking actual GPU offload, and returning to CPU
are described in the [Intel Arc guide](docs/intel-arc.md).

### Experimental Intel NPU (separate branch)

An explicitly enabled host runtime using OpenVINO GenAI and `compose.npu.yml` is
prepared for testing Core Ultra 7 268V. Generation runs only on the NPU; BGE-M3
remains in local Ollama. This option requires driver and IR-weight preparation.
After fixes to the CLI and link normalization, fixed hardware examples, including
insufficient data and source reordering, passed on 2026-10-30. Full acceptance
with CPU/GPU comparison remains a separate task. CLI `setup --device npu`, stop,
repeat `up`, and `doctor` were checked while preserving the database and prepared
weights. Commands, results, and the way back to Ollama are in the
[NPU guide](docs/intel-npu.md).

The `.npu/` directory is created separately by NPU setup commands, excluded from
Git and Docker builds, and is not created automatically by ordinary
`docker compose up`. It is not needed for the CPU/Arc Ollama option; another NPU
user must prepare their own local environment, weights, and profile following the guide.
The CLI automation for these steps is not implemented yet and is described in
[issue #25](https://github.com/VolkovDani/project-creator/issues/25).

### Optional NVIDIA GPU

Using the CLI: `./project-creator setup --device nvidia`. If there are multiple
NVIDIA GPUs, set `--gpu-id` to a UUID from `nvidia-smi -L` (or an index; the CLI
stores the UUID). The selection is retained for `up`, `down`, and `doctor`.

On Linux, first install a compatible NVIDIA driver and the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html),
configure it for Docker, and check that containers can access the GPU. Then run:

```bash
docker compose -f compose.yml -f compose.gpu.yml up -d --wait
```

The override requests a GPU for Ollama; the other services do not need one. Use
the same pair of `-f` options for subsequent GPU configuration changes. Docker
Desktop on macOS does not provide the Metal GPU to this Linux container; use the
CPU option or a separately and deliberately configured local runtime. Official
options are described in the [Ollama Docker documentation](https://docs.ollama.com/docker).

### GPU and integrated graphics: startup matrix

The CLI supports native Linux x86_64, Python 3.12, and Docker Engine/Compose 2.20+.
A profile indicates configuration support; compatibility with specific hardware
is confirmed by a separate real `setup`/`up` run.

| CLI mode | Device and runtime | Setup / status |
| --- | --- | --- |
| `cpu` | Ollama 0.40.16 on CPU | Baseline option; no GPU required. |
| `arc` | Intel Arc dGPU/iGPU, including Core Ultra 7 268V; Ollama 0.40.16/Vulkan | Intel Linux graphics, access to `/dev/dri`; [Arc guide](docs/intel-arc.md). Intel ICD is isolated from AMD/NVIDIA. New selection logic checked with tests without a GPU. |
| `nvidia` | Supported NVIDIA GPU; Ollama 0.40.16/CUDA | NVIDIA driver and Container Toolkit, `nvidia-smi`; hardware check of the new CLI integration not yet completed. |
| `amd` (ROCm) | Only AMD dGPU/iGPU in the Ollama 0.40.16 matrix; `ollama/ollama:0.40.16-rocm` | ROCm 7, `rocminfo`, `/dev/kfd` and `/dev/dri`. Strict GFX target check; hardware acceptance not performed. |
| `amd-vulkan` (RADV) | AMD dGPU/iGPU with working Vulkan; Ollama 0.40.16 + Mesa RADV | `amdgpu`, available `/dev/dri/renderD*`, and Vulkan support in the driver. ROCm and `/dev/kfd` are not required. Broader, but **not universal**; hardware acceptance not performed. |
| `npu` | Intel NPU; OpenVINO GenAI, embeddings on CPU | Fixed hardware examples pass on Core Ultra 7 268V; results and limitations are in the [NPU guide](docs/intel-npu.md). |

**Operation on every AMD GPU or Ryzen APU cannot be guaranteed.** For ROCm,
check the model, OS, and driver in the [AMD matrix](https://rocm.docs.amd.com/en/latest/compatibility/compatibility-matrix.html)
and [Ollama 0.40.16 matrix](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx).
Even if a newer ROCm version supports an iGPU, that does not automatically extend
the capabilities of the pinned Ollama container. ROCm example:
`./project-creator setup --device amd --gpu-id GPU-...`, where the UUID comes from
`rocminfo`. The CLI accepts targets `gfx908`, `gfx90a`, `gfx942`, `gfx950`,
`gfx1030`, `gfx1100`, `gfx1101`, `gfx1102`, `gfx1150`, `gfx1151`, `gfx1200`,
`gfx1201` from the [Ollama v0.40.16 matrix](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx).
This is the upstream matrix, not a list of video cards tested by this project.
Unknown targets are rejected; `HSA_OVERRIDE_GFX_VERSION` is not used.

For another AMD GPU/iGPU, you can **try Vulkan**:
`./project-creator setup --device amd-vulkan`. The Mesa RADV driver runs without
ROCm, and the Vulkan ICD is isolated from Intel/NVIDIA. With multiple AMD render
devices, set `--gpu-id 0` (or another **Vulkan index from the Ollama logs**, not
a UUID from `rocminfo` or a `renderD*` number). If Vulkan, the model, or available
memory is incompatible, setup fails; it does not automatically switch to ROCm or
CPU. Integrated graphics use shared system memory, so working Vulkan does not
guarantee both models will load.

To change the device or adapter, run `setup` again with `--switch-device`. Data
and pinned models are preserved. `setup`/`up` check GPU offload separately for
embeddings and the generator; zero offload is an error instead of a successful
CPU fallback. The displayed GPU memory bytes allow partial offload and do not
prove full execution on the GPU. `doctor` does not run inference and reports that
no measurement is available for an unloaded model. More about selection, manual
startup, and hardware checks is in the [CLI guide](docs/setup-cli.md#gpu-и-выбор-адаптера).

### Stop, update, and back up

`docker compose stop` stops processes; `docker compose down` also removes
containers and the network. Both commands preserve named volumes. To resume, run
`docker compose up -d --wait`. Do not change the project name or Compose file path
without migrating the volumes, or you may get a separate empty database.

Before an update, stop writes and create a dump:

```bash
mkdir -p backups
chmod 700 backups
docker compose stop frontend backend
umask 077
docker compose exec -T postgres pg_dump -U project_creator -d project_creator -Fc > backups/project-creator.dump
docker compose up -d --wait
```

Store the dump outside the application machine and test restoring it in a separate
Compose project. It contains projects, requirements, vectors, source versions,
and analysis history, but not Ollama weights. Keep a protected copy of `.env` and
the pinned digests separately from git. To preserve the downloaded weights
specifically, stop Ollama and copy its directory:

```bash
docker compose stop ollama
docker compose cp ollama:/root/.ollama backups/ollama
docker compose up -d --wait
```

To restore weights, stop Ollama and copy the saved directory contents into its
`/root/.ollama`, then start the service and check the full digests again with
`model-info`. Alternatively, download the weights again and verify the digest; an
updated tag does not guarantee the same result.

Restoring the database replaces data in the selected Compose project. First back
up its current database; use compatible PostgreSQL/pgvector and application
versions. PostgreSQL must be running and the application stopped:

```bash
docker compose up -d --wait postgres
docker compose stop frontend backend
docker compose exec -T postgres dropdb -U project_creator --if-exists project_creator
docker compose exec -T postgres createdb -U project_creator project_creator
docker compose exec -T postgres pg_restore -U project_creator -d project_creator --exit-on-error < backups/project-creator.dump
docker compose up -d --wait
```

After restoring, check readiness, project list, requirements, and index status. A
password error after changing `.env` means the password in the existing
PostgreSQL volume is still the old one; change it using the PostgreSQL role rather
than deleting the volume. To update, first save a backup, review migrations and
version changes, get the repository changes, then run
`docker compose up -d --build --wait`. Migrations run at startup; downgrades are
not supported. A PostgreSQL major-version update requires `pg_upgrade` or
dump/restore, not changing the tag on an old volume. Update weights as a separate
decision: changing the embedding profile requires reindexing for search; changing
the generator requires new pair analyses.

To **permanently delete all local data and weights** intentionally, run
`docker compose down --volumes`. This removes both named volumes for the selected
project; backups outside the volumes remain. It is not needed for ordinary shutdown.

### Container smoke and CI

The regular smoke test starts a temporary Compose project with a test Ollama stub.
It checks building both images, migrations, and the complete HTTP path through
Next.js: project → requirements → indexing → collision with source versions,
then data persistence after restarting PostgreSQL/backend. No weights or GPU are
needed. The test stub is included only through explicit `compose.smoke.yml`; it
is not in the production Compose setup.

Node.js 24 is additionally required to run the smoke script on the host. The
commands below run in a subshell so test variables do not affect normal startup.
The test requires an empty database and refuses to add data to a non-empty one:

```bash
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

The script does not create or delete volumes: it checks an already-running stack
and restarts its PostgreSQL/backend. After the check, remove only the test stack:

```bash
docker compose --env-file .env.smoke.example -p project-creator-smoke -f compose.yml -f compose.smoke.yml down --volumes
```

Do not substitute the name of your working Compose project. Before repeating the
smoke test, remove only its test volumes so the database is empty. If a failure
happens before cleanup, save the output of `docker compose ... logs` for diagnosis.

A real-model smoke test requires a separate project and its own weight volume.
Copy `.env.example` to `.env.real-smoke`, set a separate random password and
token, and initially leave model settings disabled. Prepare the runtime and weights:

```bash
(
  set -e
  export COMPOSE_FILE=compose.yml
  export COMPOSE_PROJECT_NAME=project-creator-real-smoke
  export COMPOSE_ENV_FILE=.env.real-smoke
  docker compose --env-file "$COMPOSE_ENV_FILE" up -d --wait postgres ollama
  docker compose --env-file "$COMPOSE_ENV_FILE" exec ollama ollama pull bge-m3:567m
  docker compose --env-file "$COMPOSE_ENV_FILE" exec ollama ollama pull qwen2.5:7b
  docker compose --env-file "$COMPOSE_ENV_FILE" --profile tools run --rm model-info
)
```

Save the full digests in `.env.real-smoke` following the first-run instructions,
then explicitly check the real model:

```bash
(
  set -e
  export COMPOSE_FILE=compose.yml
  export COMPOSE_PROJECT_NAME=project-creator-real-smoke
  export COMPOSE_ENV_FILE=.env.real-smoke
  export REAL_MODEL_SMOKE=1
  export APP_PORT=3201
  docker compose --env-file "$COMPOSE_ENV_FILE" up -d --build --wait --wait-timeout 180
  node scripts/compose-smoke.mjs
)
```

After the check, this command removes only this test project's data and weights:

```bash
docker compose --env-file .env.real-smoke -p project-creator-real-smoke -f compose.yml down --volumes
```

This run does not assess statistical LLM quality and may fail on a slow CPU; it
is not part of ordinary CI. Standalone checks of real adapters and test database
requirements are described below. GitHub Actions separately runs
unit/integration tests, SDL/operation validation and codegen, frontend tests,
TypeScript checking, Next.js build, Playwright, and container smoke. Passing
ordinary CI confirms integration with stubs, not accuracy of the actual local model.

Checks of the completed technical MVP boundaries:

| Scenario | Check and boundary |
| --- | --- |
| Create a project and manage requirements without chat | GraphQL/Playwright; shared validation and optimistic locking |
| Data isolation | Check workspace/project before retrieval and model calls; local token instead of a full role model |
| Keep two clients current | SSE, reconnect/reset, repeated events, and read/invalidation races |
| Search and known collision with sources | Deterministic integration/container tests; real weights checked separately |
| Provenance of outputs | IDs, versions, and source snapshots; stale state when data/profile changes |
| Local operation and controlled changes | No cloud keys/fallback; the model does not modify requirements on its own |
| Restart and recovery | Container smoke checks restart; test backup/restore with your own backups |

Full roles/participants, functional blocks, knowledge graph, chat, and history of
all edits remain part of the broader roadmap in GOAL, not the completed technical
slice. Passing the test smoke does not mean that LLM quality on real requirements
has been accepted or that the public multi-user service is ready.

## Local checks

Go 1.26.10, a C compiler for SQLite import/testing, Node.js 24, and PostgreSQL 16
with pgvector 0.8.0 are required. Ordinary tests do not require an LLM or cloud
APIs. Integration tests use separate schemas in the database from
`TEST_DATABASE_URL`. Without this variable they are skipped; such a run does not
check storage.

```bash
export TEST_DATABASE_URL='postgresql://project_creator:your-password@127.0.0.1:5432/project_creator_test?sslmode=disable'
go test -p 1 ./...
go vet ./...
gofmt -l cmd contracts graph internal
```

The test role must be allowed to create and drop schemas and install the `vector`
extension. Use a separate test database, not the production one. Packages run
sequentially (`-p 1`): separate test schemas do not isolate the shared model lock
in PostgreSQL. Parallel packages may block each other's stub startup and cause
timeouts. Development follows TDD: failing behavior test → implementation → refactor.

## Project card

`internal/project.Input`: `name`, `goal`, `description`.
`project.Validate` returns normalized input and a list of errors. Errors contain
`path`, `code`, `messageKey`, `params`; successful validation returns an empty
list. Validation does not create an ID or save data. The server adds ID, workspace,
and timestamps when saving.

The source of constraints is `contracts/project.json`, embedded in the Go binary.
The frontend reads the same file; shared examples are in
`contracts/project.fixtures.json`. Do not copy constraints into TS. An unknown
contract version, field, or mode causes a loading error.

- Name: required, maximum 120 Unicode code points.
- Goal: required, maximum 2000 code points.
- Description: optional, maximum 10000 code points.
- Before validation, only ASCII SPACE, TAB, LF, and CR are trimmed from the ends
  of each field. NBSP, zero-width characters, internal spaces, and line breaks are retained.
- Length is measured after trimming in Unicode code points (not bytes, UTF-16
  units, or grapheme clusters). The equivalent TypeScript counter is
  `Array.from(value).length`. Unicode NFC/NFKC normalization is not performed.
  Invalid UTF-8 is rejected; the TS adapter also rejects unpaired surrogates before sending.
- All field errors are collected in one call, in the order name, goal, description.

Shared dictionaries are `contracts/i18n/ru.json` and `en.json`. They contain UI
text and validation errors; `params.max` is passed separately from the text. The
server and UI use the key and do not parse translated strings.

## PostgreSQL and running the API

The storage decision is documented in [ADR-001](docs/adr/001-postgresql-pgvector.md).
`compose.postgres.yml` pins PostgreSQL 16 and pgvector 0.8.0. It provides only a
database for development with processes on the host. To run the whole application,
use the main `compose.yml` as described above. SQLite is no longer the server's
working storage.

First run:

```bash
cp .env.example .env
# Edit .env: set a random password and token, and make the password in the URL match.
docker compose --env-file .env -f compose.postgres.yml up -d --wait
set -a
. ./.env
set +a
go run ./cmd/server
```

Special characters in a URL password must be percent-encoded. `.env` must not be
committed to git. Compose publishes PostgreSQL only on `127.0.0.1`; the API listens
on `127.0.0.1:8080` by default. `sslmode=disable` is intended for local loopback;
use a verified TLS connection for a remote database.

`PROJECT_CREATOR_DATABASE_URL` is required and accepts a PostgreSQL URL;
`PROJECT_CREATOR_DB` is no longer used. `PROJECT_CREATOR_TOKEN` must contain at
least 32 characters. POST `/graphql` requires
`Authorization: Bearer <PROJECT_CREATOR_TOKEN>`. One token grants access to one
workspace—this is a local session, not full multi-user authentication. Do not
expose the interface or API to the internet without separate protection.

Versioned SQL migrations are applied automatically at startup. The server first
checks the connection and pgvector availability; if either check fails, it does
not start. Initially running `CREATE EXTENSION vector` requires permission to
install extensions (usually DBA/superuser). An administrator can install it in
the target database in advance; the application role needs permissions on tables
and the schema changes being applied. Back up before application updates and
migrations. An unknown newer schema version is rejected; automatic downgrade is
not supported.

Check readiness:

```bash
docker compose --env-file .env -f compose.postgres.yml ps
docker compose --env-file .env -f compose.postgres.yml exec -T postgres psql -U project_creator -d project_creator -c "SELECT extversion FROM pg_extension WHERE extname='vector'"
```

Before the API's first start, the extension may not yet be installed. `connection
refused` usually means the database is not ready or the port is wrong;
`password authentication failed` means the URL password and volume do not match.
Changing `.env` does not change the password in an existing database: change it
through PostgreSQL. A `vector` availability error requires an image with pgvector
and permission to install the extension.

Stopping preserves the volume:

```bash
docker compose --env-file .env -f compose.postgres.yml down
```

To back up, stop writes in the application and run:

```bash
docker compose --env-file .env -f compose.postgres.yml exec -T postgres pg_dump -U project_creator -d project_creator -Fc > project-creator.dump
```

To restore, use an empty database with a compatible pgvector version; stop the
application, then run:

```bash
docker compose --env-file .env -f compose.postgres.yml exec -T postgres pg_restore -U project_creator -d project_creator --exit-on-error < project-creator.dump
```

Before updating, save a dump, test restoring it separately, and review migration
changes. A PostgreSQL major-version update requires `pg_upgrade` or backup/restore,
not simply changing the image tag. The command
`docker compose --env-file .env -f compose.postgres.yml down -v` permanently deletes
the local volume; use it only when intentionally deleting the data.

## Migrating previous SQLite data

Stop both the old and new applications. Save the original SQLite database along
with WAL/SHM files, if present; do not copy only the main file from a running
database. The importer creates a separate consistent backup and does not delete
the source. Specify a nonexistent backup path and an empty target PostgreSQL database:

```bash
export PROJECT_CREATOR_DATABASE_URL='postgresql://project_creator:your-password@127.0.0.1:5432/project_creator?sslmode=disable'
go run ./cmd/import-sqlite --source ./project-creator.db --backup ./project-creator-before-postgres.db
```

SQLite schema version 2 (business requirements, MVP 4) is supported. For an older
database, first use the previous application version to migrate it, keeping a
backup. The importer checks integrity, requirement/project ownership, versions,
and record counts; it preserves IDs, workspace, text, and timestamps. All writes
to PostgreSQL are transactional and roll back on error. A non-empty target
database, unknown SQLite version, or broken relationships are rejected. Importing
again into a populated database is not supported. After a successful import,
compare the reported counts and open projects/requirements in the UI; keep the
source and backup until verified. Do not delete them automatically.

## GraphQL and CI checks

SDL in `graph/schema.graphqls` remains the source for server bindings. A Project
Creator project is a knowledge container, not the product being described.
Request body size is limited to 1 MiB, depth to 8, concurrent resolvers to 10,
and expanded fields/fragments to 100. Pagination requires limit (1–100) and
offset ≥ 0.

```bash
go run ./cmd/codegen
git diff --exit-code -- graph/models_gen.go
```

GitHub Actions starts PostgreSQL + pgvector and runs Go unit/integration tests,
vet, codegen, TypeScript checks, build, and Playwright E2E. E2E uses a separate
database. Ordinary checks use deterministic model stubs. A separate job builds
containers and checks the full HTTP scenario with an Ollama stub; real models
run only in the opt-in smoke test described above. Docker builds for the Intel
runtime, backend, and frontend use separate GitHub Actions layer caches; Compose
starts already downloaded images without rebuilding. The first run is still cold;
caching does not avoid downloading layers and service images. Tool versions,
limits, and validation of the effect are described in the
[CI Docker cache guide](docs/ci-docker-cache.md).

The pair-analysis release scenario matrix, isolated acceptance results, and
remaining limitations are documented in [report #85](docs/pair-analysis-acceptance.md).

### Automatic merge after CI

`main` remains the default branch and contains only accepted changes. Ordinary
changes go through PRs to `develop`; CI runs for PRs and cancels obsolete runs.
`develop` was created from `main` when the release process was prepared. Push CI on
`main` remains enabled to check the actually integrated state; a separate push
workflow for `develop` is disabled to avoid a second run after a PR.

The `automerge` label applies only to PRs from this repository targeting `develop`.
Prepare a release through a separate PR from `develop` to `main`, without this
label; merge it manually after reviewing the diff, CI status, and release readiness.
A release PR does not publish a version.

Changes consisting only of `.md` and `.txt` files do not trigger GitHub Actions for
PRs or pushes to `main`: the `paths-ignore` filter applies to the entire workflow.
If the diff contains at least one other file, CI runs as usual. After reviewing a
documentation-only PR and running `git diff --check`, merge it with a regular merge
commit using `gh pr merge --merge --match-head-commit SHA`, without the
`automerge` label: the merge job does not run in this case either. Do not make a
skipped workflow a required condition for such PRs: GitHub may leave required
checks Pending. Do not bypass branch protection with `--admin`; when adding
required checks, ensure they are compatible with this rule. See GitHub's
[workflow path filters](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushpull_requestpull_request_targetpathspaths-ignore).

For a ready-to-merge PR from a branch in this repository into `develop`, apply the
`automerge` label. The final `automerge` job waits for successful `checks` and
`containers`, then creates a two-parent merge commit: branch commits and their
connection to `develop` remain in history. The script checks the base again and
rejects PRs targeting `main`. This is a separate ordinary CI job, without paid
built-in GitHub auto-merge, a PAT, or additional dependencies.

The repository setting **Automatically delete head branches** is enabled. GitHub
deletes the remote head branch after a successful PR merge (including automerge).
Closing a PR without merging does not trigger this cleanup. Local branches remain
until cleaned up in the clone: `git fetch --prune` removes only stale
remote-tracking refs. After merge, an agent removes a confirmed-integrated local
branch according to `AGENTS.md`, preserving new commits, permanent `main`/`develop`,
and branches used by worktrees. If GitHub deletes `develop` after a release PR,
restore it from the current `main`.

The label is explicit permission to merge: apply it after review. The job reads the
current PR state, so removing the label before its check cancels the merge. Draft,
closed, forked, conflicted PRs, a changed head, or a change to `main` relative to
the checked merge commit cause the job to skip. GitHub receives the expected head
SHA and rejects the merge if the head has changed. The base is checked before the
request; there is no atomic lock on `develop` between that check and the merge.
This is not a merge queue or a replacement for branch protection.

Adding/removing the label does not itself rerun CI. The label can be applied while
checks are running. If CI has already completed, rerun **only the `automerge` job**
in Actions (or run `gh run rerun RUN_ID --job JOB_ID`) and keep the existing check
results. If the head or `develop` changes, update the working branch relative to
`develop` and wait for a new full CI run. An API failure fails the job without
retrying the merge automatically; after investigating the cause, you can rerun
this job.

Only `automerge` gets `contents: write`; tests and builds have `contents: read`.
It uses a short-lived `GITHUB_TOKEN`.
[Events from this token do not trigger push workflows](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs),
so an automatic merge does not cause another CI run on `main` or refresh caches
from that push. PR checks remain the basis for merging; caches are still refreshed
on ordinary pushes/manual merges to `main`.

### Preparing Source + Compose v0.1.0

The manual `Draft prerelease` workflow runs only from `main`. It validates the
semantic version tag and full source SHA, verifies that the tag points exactly to
that SHA and the SHA is contained in `main`, and checks that a successful CI run
on `main` (push or manual run) completed on that same SHA. It also verifies that
a closed hardware issue contains the exact tag/SHA fields and the
`hardware-accepted` label. The workflow then repeats checks and builds on the source
SHA and creates only a prerelease draft. Publishing remains a separate manual
decision by the maintainer after reviewing reports and release notes.

Detailed instructions are available [in Russian](docs/release-acceptance.ru.md)
and [in English](docs/release-acceptance.md); release notes are available
[in Russian](docs/release-notes-v0.1.0.ru.md) and
[in English](docs/release-notes-v0.1.0.md). Actual checks and remaining conditions
are listed in the [report dated November 17, 2026](docs/release-verification-2026-11-17.md).

## Git hooks and formatting

The hook is stored in the tracked `.githooks/pre-commit` directory, and its
implementation is in `scripts/biome-pre-commit.mjs`. Git does not enable hooks from
a repository automatically: on each new computer/clone, run the following with
Node.js 24:

```bash
git clone https://github.com/VolkovDani/project-creator.git
cd project-creator/frontend
npm ci
npm run hooks:install
```

For an existing clone, the last two commands from `frontend/` are sufficient.
The installer sets `git config --local core.hooksPath .githooks`; verify this with
`git config --get core.hooksPath`. Reinstallation is safe. If another
`core.hooksPath` is already configured or `.git/hooks/pre-commit` exists, the
installer preserves it and reports a conflict. In that case, after reviewing it,
connect `.githooks/pre-commit` through the existing hook mechanism.

Before every commit, format changed files with the pinned Biome version:

```bash
cd frontend
./node_modules/.bin/biome check --write 'app/projects/[id]/IndexProgress.tsx'
npm run format:check
```

Pre-commit automatically handles staged `.ts`, `.tsx`, `.json`, and `.jsonc` files
inside `frontend/`, using `frontend/biome.jsonc` configuration and exclusions. It
uses the local executable in `node_modules`; the hook downloads nothing from the
network. It checks a temporary copy of the Git index contents, including
configuration and ignore files. For fully staged files, safe Biome fixes are
applied to the working files and index. Unsafe fixes are not applied. Errors and
remaining warnings block the commit.

For partially staged files, only the staged content is checked; unstaged changes
are not added to the commit or rewritten. If the staged part needs formatting,
the hook stops the commit: run Biome on the file, review the diff, stage the desired
changes again with `git add -p`, and retry. Generated files remain excluded by
Biome configuration. The hook does not follow symlinks to supported files and
refuses to process them.

The hook does not replace `typecheck`, tests, build, or CI. Go and documentation
changes with no supported frontend files do not require Biome. To test hooks, after
`npm ci` in `frontend/`, run `node --test scripts/git-hooks.test.mjs` from the
repository root. In a source checkout, `npm test` also runs this integration check
after dependencies are installed. An early Node test in CI before `npm ci` checks
the installer; scenarios using real Biome run later in frontend tests. The
application Docker build does not include Git tooling, so hook tests are skipped
there; other frontend tests run as usual.

## Next.js interface

Node.js 24 and Go 1.26.10 are required. Start the GraphQL server in one terminal,
setting `PROJECT_CREATOR_TOKEN` and `PROJECT_CREATOR_DATABASE_URL` as described
above. In another terminal:

```bash
cd frontend
npm ci
export PROJECT_CREATOR_TOKEN='the-same-secret-as-the-server'
npm run dev -- -H 127.0.0.1
```

Open `http://127.0.0.1:3000`. The Next.js server forwards GraphQL requests to
`http://127.0.0.1:8080/graphql`; for another address, set the server-side
`PROJECT_CREATOR_GRAPHQL_URL` variable. The token is not sent to the browser.
This is a local single-user mode: do not expose the interface to the internet
without separate authentication and access control.

After changing the GraphQL schema and operations, run:

Codegen versions, the reason for the targeted `minimatch` override, and its removal
process are described in the [codegen dependency assessment](docs/codegen-dependencies.md).

```bash
cd frontend
npm run codegen
npm test
npm run typecheck
npm run build
npx playwright install chromium
npm run e2e
```

For E2E, create a separate empty temporary database with pgvector and set
`E2E_DATABASE_URL` before `npm run e2e`. Playwright starts separate Go and Next.js
servers. Tests create data: for another clean run, recreate only the E2E database;
never point it at the working database. Example setup:

```bash
createdb -h 127.0.0.1 -U project_creator project_creator_e2e
export E2E_DATABASE_URL='postgresql://project_creator:your-password@127.0.0.1:5432/project_creator_e2e?sslmode=disable'
cd frontend
npm run e2e
```

## Business requirements

Requirements can be added, edited, and deleted on a project card. Text is checked
against the shared contract `contracts/requirement.json`, identically in Go and
TypeScript; shared examples are in `contracts/requirement.fixtures.json`.
Requirements belong to a workspace and project, and the list is paginated.

Each change increments the version. Updates and deletion check the expected
version and do not overwrite a newer edit. On conflict, the interface reloads the
list and allows the saved draft to be applied on top of the current version. A
complete history of all revisions is not available yet, but the index and analysis
attempts preserve versions and snapshots of the sources used. Editing a requirement
requires reindexing and makes previous analyses stale. These mechanisms are
implemented and described in the RAG and analysis sections below.

## Keeping two open clients current (SSE, MVP 5)

The Go server provides an authenticated `GET /events`. Next.js forwards the stream
through same-origin `/api/events`, keeping the token on the server only. The
interface remains a local single-user application; this does not replace full
authentication for internet deployment. URLs contain no tokens.

PostgreSQL migration 002 creates a `cache_events` journal and triggers on projects
and requirements. Event logging and data changes are in one transaction; rollbacks
are not published. The journal is a transactional outbox: each server instance
reads the shared PostgreSQL database, so a process failure between commit and send
does not lose an invalidation. Publication does not depend on process memory or
LISTEN/NOTIFY. A transactional advisory lock serializes event-number allocation
before commit so polling cannot skip an event from an earlier, still-uncommitted
transaction. This is an intentional throughput limitation of the local MVP.

GraphQL responses contain `extensions.cacheTags`; `cache.invalidate` events send
`{schemaVersion:1,keys:[...]}`. Tags are `projects`, `project:<id>`, and
`project:<id>:requirements`. For now, `projects` is a conservative shared
workspace-level dependency. Each workspace has its own event stream. The client
cache tracks invalidation generations and repeats a read if an event arrives while
a request is in flight; repeated event IDs are safe.

History is limited to the latest 1000 events. `Last-Event-ID` is an opaque cursor;
unknown, foreign, or deleted cursors cause `cache.reset`. The first connection also
triggers reconciliation. The client reloads data while preserving unsaved input.
The stream checks for new rows every 300 ms and sends a heartbeat every 15 seconds;
after disconnection the client reconnects with exponential backoff up to 30 seconds.
Offline/online recovery uses the same mechanism. `resetSession()` clears the cache
and cursor for future session changes; 401/403 responses clear inaccessible data.
SSE and GraphQL do not use the long-lived Next.js server cache. When configuring a
reverse proxy, disable stream buffering and set the idle timeout longer than the
heartbeat interval.

To check it, open the project list or a project card in two tabs. Creating a project
and creating/editing/deleting requirements in the first tab should appear in the
second without a reload. Playwright checks this in CI; Go tests check workspace
scoping, replay/reset, journal persistence, commit/rollback, and connection
cancellation. Test commands are listed above.

## Local embeddings and requirement search

RAG uses PostgreSQL + pgvector and local Ollama. Projects and requirements continue
to work when no provider is configured; an indexing attempt reports that the model
is not configured. Data is not sent to an external API when the local runtime fails.

Commands in this section assume Ollama and the backend run on the host. For the
full Docker Compose setup, use the first-run section: download models with
`docker compose exec ollama`, configure `.env`, then recreate the backend. Exporting
variables in a terminal does not change the environment of an already running
container; the Ollama port is not published to the host by the full Compose setup.

Install Ollama following the official instructions at https://ollama.com/download
and explicitly download the model (this separately downloads weights; ordinary
tests do not do so):

```bash
ollama pull bge-m3:567m
curl http://127.0.0.1:11434/api/tags
```

Find the selected model entry and save the full `digest`, including `sha256:`.
Set the backend environment and restart it:

```bash
export PROJECT_CREATOR_OLLAMA_URL='http://127.0.0.1:11434'
export PROJECT_CREATOR_EMBEDDING_MODEL='bge-m3:567m'
export PROJECT_CREATOR_EMBEDDING_DIGEST='sha256:full-digest-from-api-tags'
go run ./cmd/server
```

Ollama should listen only on a trusted local interface. Do not expose it to the
internet. Loopback HTTP addresses and the explicit local Docker service name
`ollama` are supported; arbitrary remote addresses are rejected. There is no cloud
fallback. Do not store sensitive addresses or secrets in git.

With a local embedding provider configured, new and edited requirements are
automatically placed in a background queue after commit. Saving does not wait for
the model, and closing the tab does not stop processing. Index states are queued,
indexing, ready, or failed. The indexing page shows overall project progress and
per-requirement status; failures can be retried manually. Without an embedding
provider, requirements are saved without calling a model. Search starts only when
the user clicks “Search requirements” and uses ready fragments from that project.
No search results do not prove that there are no duplicates or contradictions.

Results show the matching fragment, its version, and the original text from that
version; the link opens the requirement. After requirements change, results are
marked stale and search must be repeated. Similarity is not a probability of
correctness and is not described as one.

Ordinary Go/CI tests use a deterministic fake embedding provider; Playwright checks
states and explicit actions with API stubs. A real local-model check runs separately
and needs no cloud key.

Search uses exact cosine comparison with pgvector, without HNSW. The profile ties
together the model name, full digest, 1024 dimensions, and chunking version. BGE-M3
is selected as a multilingual local option; quality on your requirements must be
checked separately. Changing the pinned profile excludes old vectors from search;
on backend startup, current requirements are automatically queued for the new
profile. On backend restart, open clients receive invalidations for status and
results, even if their SSE cursor is still valid.

Chunking is deterministic: up to 1000 Unicode code points, with 100 overlap and a
preference for paragraph/sentence boundaries. A fragment stores exact offsets and
the text from the source version; requirements are not merged. This is a heuristic,
not a guarantee of retrieval quality. Indexing uses a short transaction to claim
an attempt and a two-minute lease; embeddings are computed outside the transaction
with a 90-second timeout. Completion is stored only for the current version and
profile and by the owner of the attempt. The queue recovers after restart or lease
expiry. Temporary model failures receive up to three attempts with 5- and 30-second
pauses; after that, retry explicitly. Invalid embeddings and empty sources are not
retried automatically. There is no cloud fallback or automatic model download.
The server limits search queries to 2000 code points and 20 results.

Opt-in smoke test for the real local provider:

```bash
OLLAMA_SMOKE=1 \
OLLAMA_URL="$PROJECT_CREATOR_OLLAMA_URL" \
OLLAMA_MODEL="$PROJECT_CREATOR_EMBEDDING_MODEL" \
OLLAMA_DIGEST="$PROJECT_CREATOR_EMBEDDING_DIGEST" \
go test ./internal/rag -run TestRealOllama -v -count=1
```

For a full search smoke test, also set `TEST_DATABASE_URL` to a separate test
database with pgvector. Tests create an isolated schema and check the expected
source. This is a short wiring check; it does not replace recall evaluation on a
real set of duplicates and contradictions. Ordinary CI tests do not download the
real model.

## Possible collision analysis (MVP 7)

After completion, the API returns `reasonCode` and saved attempt diagnostics. The
“All pairs” mode does not require an index and does not return `INDEX_INCOMPLETE`.
The primary “Related pairs” mode requires current embeddings: for
`INDEX_INCOMPLETE`, check overall progress, wait for the queue or retry failed
indexing, then explicitly retry the analysis. Analysis does not start automatically.
The former `REQUIREMENT_LIMIT`/`ANALYSIS_LIMIT` limits no longer apply. Older
attempts retain their original reasons. `TOO_FEW_REQUIREMENTS` means there is
nothing to compare; `MODEL_INSUFFICIENT` means the model lacks enough conditions
to draw a conclusion. Technical errors are reflected separately in status and coverage.

Analysis starts only by clicking the button on a project card. Indexing is needed
for semantic search and selecting related pairs; it is not required for the
explicit “All pairs” mode. Below is Ollama generator setup for host processes; in
full Compose, set the same MODEL/DIGEST values in `.env` and recreate the backend.
For OpenVINO NPU, use the [separate guide](docs/intel-npu.md).

```bash
ollama pull qwen2.5:7b
curl http://127.0.0.1:11434/api/tags
export PROJECT_CREATOR_GENERATOR_MODEL='qwen2.5:7b'
export PROJECT_CREATOR_GENERATOR_DIGEST='sha256:full-generator-digest-from-api-tags'
go run ./cmd/server
```

The generator uses the same `PROJECT_CREATOR_OLLAMA_URL` as embeddings, but has a
separate model and digest. Qwen2.5 7B is an example; weights are downloaded only
when explicitly requested by the user. The adapter accepts loopback HTTP or the
local Docker service name `ollama`, checks the installed model and digest before
and after each call, rejects remote models, and does not fall back to cloud. Normal
project and requirement operations work without the generator. Local model
capabilities and quality depend on the selected weights and hardware; passing
fake-based tests does not assess its accuracy on your requirements.

An analysis has status running, failed, or completed. The screen distinguishes a
possible collision, insufficient data, and no collision found among the checked
pairs. The last **does not prove that the project is consistent**: the model can
be wrong and its output needs human review. Exhaustively comparing pairs does not
guarantee finding every semantic contradiction. Confidence percentages are not
calculated. Model output never changes requirements.

Analysis starts explicitly from a button and quickly returns an attempt ID. Work
continues on the server after the tab is closed. The primary “Related pairs” mode
selects the union of exact matches of normalized text and pairs with cosine
similarity at or above the threshold. Shared words do not bypass the threshold.
It uses exact search across all current fragments, without an approximate index or
top-5/top-6 limit. No new structural links are created: the current data model
does not yet have links for requirements.

The interface lets the user set the threshold from 0 to 1; the initial value of
0.65 is a selection heuristic, **not a probability of relatedness, collision, or
confidence**. The threshold and rule version are saved for each run; selection
reasons and, where applicable, cosine scores are saved for each pair. A fixed RU/EN
test set checks more than five relevant neighbors and exclusion of clearly
unrelated requirements. These tests use predefined vectors: they check selection
rules but do not measure the quality of the real embedding model. The threshold
is not calibrated on client data. Embeddings may miss relations or select extra
pairs; unchecked pairs are not considered compatible. `related-v2` rules apply to
new runs; historical queues and results are not recalculated. Use analysis
replacement to change the rules of an active run.

A separate explicit “All pairs” mode creates every unique unordered pair for N
current requirements: N × (N − 1) / 2, or 4950 for 100 requirements and 19900 for
200. The limits of 20 pairs/100 requirements and top-6 do not apply. Embeddings and
embedding-provider calls are not needed in this mode. Invalid sources and technical
errors do not count as successful checks.

Exact duplicates are detected without an LLM after collapsing spaces, tabs, and
line breaks. Case, negation, numbers, and punctuation are preserved. The “Duplicate
requirements” group shows both original texts, an explanation, time, and provenance:
either an exact match by algorithm `exact_text_v1` or a model suggestion. The model
may return `duplicate` for identical obligations with the same conditions, scope,
and constraints. Partial overlap or clarification is not automatically considered
a duplicate. Nothing is merged or deleted.

Preparation saves source snapshots and builds the queue in bounded batches. Until
preparation completes, the total number of pairs is unknown (`totalPairs=null`),
not zero. The interface distinguishes all possible project pairs, selected and
unselected pairs, new and reused results, and pending, running, failed, and
completed pairs. Unselected pairs are not considered checked. Collapsible result
groups remain; rows load in pages, and counters apply to the whole run rather than
the current page. A completed result moves to its group immediately through SSE in
open tabs; reconnect/reset reloads saved data. “No contradictions found” applies
to checked pairs; there is no separate “requirements are unrelated” classification.

Migration 007 moves the queue and snapshots from growing JSONB arrays in
`analysis_runs` into separate `analysis_pairs` and `analysis_sources` rows. It
preserves IDs, versions, and source text, verdict, explanation, check time,
generator profile, and a link to the source check when reusing a result. Analysis
history is available through paginated GraphQL; older runs can be read by ID. The
“Cancel all-pairs analysis” button cancels the entire selected running attempt:
the current check, all pending pairs, and unfinished queue preparation. Completed
results are preserved. GraphQL mutation `cancelProjectAnalysis(projectId,
analysisId)` is idempotent; cancellation is stored as `status=failed`,
`errorCode=CANCELLED`, and the interface shows “Analysis cancelled.” Unfinished
pairs receive `failed/CANCELLED` and are not considered ready. A late model response
is not published. The adapter receives context cancellation; on another backend
instance this is detected at the next heartbeat (within 5 seconds). Releasing model
resources depends on the adapter; the next call waits for the current one to finish.
The NPU runtime stops the cancelled worker and restores it with the same pinned
profile before the next explicit analysis run; a cancelled request is not retried
automatically. You can explicitly retry unfinished pairs in the current run or
start a new analysis that reuses completed results.

Retrying failed pairs is available in the “Latest run” view: select a saved run in
history to open it. Historical results do not replace user text with current
versions. Older results are migrated without assigning unknown provenance.

Successful verdicts `collision`, `compatible`, `insufficient`, and `duplicate` are
reused between runs only within the same workspace/project, for the same
canonically ordered snapshots and the same complete immutable generator profile.
The profile includes adapter, model/digest or NPU manifest, prompt, response schema,
and generation parameters. Matching only a model name is insufficient. Errors and
timeouts are not completed results. Repeating an unchanged project with the same
profile does not call the generator. In full mode, changing one of N requirements
requires N−1 new comparisons; adding one requires N; exact duplicates also skip
inference. Deleted requirements are excluded from the new queue, but history stays.
A stale overall run does not discard reusable results for its unchanged pairs.
Changing embeddings, threshold, or selection rules rebuilds candidates but does
not by itself change whether comparisons of the same snapshots with the same
generator are reusable. All history/cache reads and model context are scoped to
authorized workspace/project; a response received after source changes or lease
loss is not published as a current successful result.

A project has one active attempt; a repeated request returns its status. A worker
processes pairs sequentially, including across projects; a PostgreSQL advisory lock
prevents competing calls from other instances. Queue preparation has a 180-second
limit per batch and each comparison has a 120-second limit; there is no short
overall cutoff for the whole analysis. Adapter responses are limited to 2 MiB.

Pair results, overall progress, and invalidation are saved after successful commit.
At the first model-call error, the batch stops: the current pair gets an error, all
other unchecked pairs get `NOT_PROCESSED`, and completed results remain. The
processed-pair counter includes completed comparisons and started comparisons that
failed; `NOT_PROCESSED` is excluded. After model recovery, “Retry failed pairs”
retries only failed and unprocessed pairs from the latest current attempt, preserving
its ID and completed results. Changed sources or generator/selection profiles
require a new analysis attempt. There are no infinite automatic retries or cloud
fallback.

The queue survives backend restart. Claimed work has a 30-second lease with a
heartbeat every 5 seconds; after a crash, unfinished calls become errors and need
an explicit retry. On graceful shutdown this happens immediately. A queued model
job does not lose its lease while waiting. A late result from a previous worker
does not replace a retry result. Inference interrupted before commit may run again
on explicit retry; already saved successful pairs are not recomputed. Exactly-once
inference is not promised. The decision and limitations are documented in
[ADR-003](docs/adr/003-progressive-analysis.md).

Documents in prompts are passed as untrusted JSON data, separate from system
instructions; the model has no tools. The response is constrained by a JSON
schema; IDs/versions of the exact sources provided, allowed verdicts, and explanation
length are validated. This reduces incorrect responses but does not make LLM output
fact.

Ordinary CI checks retrieval, workflows, access, and known collision/compatible
requirements with deterministic providers. A real local generator check runs separately:

```bash
ANALYSIS_SMOKE=1 \
OLLAMA_URL="$PROJECT_CREATOR_OLLAMA_URL" \
OLLAMA_MODEL="$PROJECT_CREATOR_GENERATOR_MODEL" \
OLLAMA_DIGEST="$PROJECT_CREATOR_GENERATOR_DIGEST" \
go test ./internal/analysis -run TestRealAnalysisSmoke -v -count=1
```

It requires a model already downloaded and checks a small set of explicit examples,
not statistical quality or completeness. Use the commands above for
unit/integration/GraphQL/E2E; integration tests require a separate test PostgreSQL
database.

### Pages and interface updates

| URL | Purpose |
| --- | --- |
| `/` | Project list |
| `/projects/new` | Create a project |
| `/projects/:id` | Project overview |
| `/projects/:id/requirements` | Create, edit, and delete requirements |
| `/projects/:id/indexing` | Overall background queue progress, statuses, and reindexing |
| `/projects/:id/search` | Semantic search |
| `/projects/:id/analysis` | Streaming pair analysis and results |

Project navigation and selected RU/EN language are shared across all pages. The
language is persisted between app visits. Links from search and analysis open the
requirements page, find the relevant list page, and highlight the source card. If
the source was deleted, its saved version text remains in the analysis result. In
analysis groups, pair explanations are visible immediately; source texts and pair
technical details are loaded separately when their sections are expanded.

During SSE invalidation and a repeated request, the last successful content remains
visible. An error displays a local message about stale data and a retry button.
Access denial clears data; an SSE reset after reconnect only refreshes it and keeps
the draft. In paged mode, requirement and indexing counts apply to the current
page of 20 records. Collapsed-group state is preserved in the current tab. Overall
indexing progress applies to the entire project, separately from counts of loaded
cards. `queued` means waiting for background processing, `running` means indexing,
`ready` means a current index is ready, and `failed` means an error. Legacy
`pending` is supported for compatibility. SSE reconnect/reset reloads saved queue
state. The rationale is described in [ADR-005](docs/adr/005-index-queue.md).

Deterministic browser regressions without PostgreSQL or models:

```sh
cd frontend
npm run build
npx playwright install chromium
npm run test:ux
```

Scenarios check desktop 1920×1080 and narrow 390×844 screens, slow requests,
SSE/reconnect/reset, errors and retry, access revocation, language and navigation,
pagination of sources, and collapsible groups with counters. The contract and SSE
use the same GraphQL operations and invalidation mechanism as before.

Long words, URLs, and identifiers wrap within the page, including project
descriptions and model provenance. Line breaks in goals and descriptions are
preserved; provenance JSON is not truncated. Selection lists fit within their
container, and full provenance for the selected run is available when expanded.
Groups and cards use existing shared styles and the `Group` component.

`responsive content` and `responsive analysis` regressions check for horizontal
overflow of the page, text containers, and form elements at widths of 320/390/768/
960/1920 px in RU/EN. They cover long summaries, expanded originals, search,
explanations and analysis profiles, large counters, and keyboard expansion. CSS
scaling at 200% is also checked; 960 px checks desktop reflow at 1920 px with 200%
browser zoom. This is automated emulation, not a manual check of the browser zoom
menu. Existing UX and analysis scenarios also check overflow in their final state,
including errors, loading, and changing analysis parameters. Overview screenshots
are saved in the Playwright results directory; for local diagnostics, set
`--output ../logs/issue-44/ux-results`.

### Viewing long lists

Projects, requirements, and indexing records load in batches of 40 by default while
scrolling. Cards stay on screen during background SSE updates. A “Load more” button
is available for keyboard use; load errors have a local “Retry” button. Long lists
are virtualized based on card height.

The “List mode” switch enables ordinary pages of 20 records. The mode and page are
saved in the URL and support Back/Forward. Returning from a card through client-side
navigation preserves loaded batches and the infinite-list position; reloading the
tab starts a new load. A requirement source link opens the corresponding regular
page.

On the indexing page, statuses load by the requirement IDs in each batch, and group
counters show counts among loaded requirements. Above the list, the number of
loaded requirements and total count are displayed. The mode switch also opens
pages of 20 requirements; the selected page is saved in the URL and supports
browser Back/Forward.

Each collapsible analysis group has its own loading and cursor; closed groups do
not request pairs. Counters describe the full run, not the loaded cards. Completed
pairs move between their previous groups after SSE updates. Cursor contracts and
limitations are described in [ADR-004](docs/adr/004-cursor-lists.md).

### Efficient live analysis updates (#47)

Progress arrives in compact SSE snapshots with a run revision, without repeatedly
requesting the full summary and project card for every pair. Groups are closed by
default; expanding one loads a compact page, and expanding a card loads only its
saved texts and explanation. Immutable model provenance is requested separately
when expanded and reused by run ID. Background synchronization preserves displayed
cards, forms, and expanded groups. Reset/reconnect restores state; cancellation
preserves completed results.

Contract and limitations: [ADR-006](docs/adr/006-analysis-live-snapshots.md).
Before/after measurements, method, and commands: [20 pairs](docs/analysis-live-performance.md).
Scenario: `scripts/analysis-live-benchmark.mjs`.
Ordinary tests do not call real models.

### Replacing parameters of a running analysis

Changing form fields does not stop an analysis. The “Replace analysis parameters”
button explicitly applies the selected mode and threshold using one of two options:

- “Wait for the current pair”: keep the reusable result of the current comparison,
  stop issuing the remaining old pairs, and start a new queue. Completed comparisons
  of unchanged snapshots with the same generator are reused regardless of threshold.
- “Cancel the current model call now”: finish the old run with `REPLACED`, cancel
  the inference context, and start a new queue after the generator is released. A
  late response is not saved. For another backend instance, cancellation is detected
  by heartbeat within 5 seconds; NPU checks client disconnection every 100 ms, then
  waits up to 5 seconds for terminate and up to 5 seconds for kill. Loading the
  pinned model on recovery has its own 600-second limit; each pair is limited to
  120 seconds.

GraphQL: `replaceProjectAnalysis(projectId, analysisId, mode, threshold, strategy)`;
`strategy` is `wait` or `cancel`. The request is tied to the expected run and saved
in PostgreSQL. Repeating an identical operation returns the same run or its successor.
Before the successor is created, a new choice replaces the pending request; after
creation, change parameters through the new `analysisId`. A stale request with
different parameters is rejected. The waiting state is `running/waiting_current_pair`.
SSE/reset and page reload read saved state; closing the tab does not cancel replacement.
The cancel-all button also removes the pending replacement. A replaced run cannot
be resumed through retry.

After intentional cancellation, the NPU worker is restored before the next call,
with its pinned weights and profile checked again. Recovery failure does not trigger
automatic retries. `NPU_RECOVERY_FAILED`, `NPU_WORKER_EXIT`, `NPU_UNAVAILABLE`,
`TIMEOUT`, `PROFILE_CHANGED`, and `INVALID_OUTPUT` are distinct from user cancellation.
Timeout, device failure, or changed files require operator recovery following the
[NPU guide](docs/intel-npu.md); there is no automatic cloud fallback. Technical
events without source requirements or responses are saved in `logs/npu/`.

Deterministic replacement acceptance (a separate test database is required):

```bash
go test ./internal/analysis -run 'TestReplacement|TestCancelWaiting' -count=1
go test ./internal/api -run TestGraphQLReplacement -count=1
python3 -m unittest discover -s runtime/npu -v
cd frontend
E2E_CHROMIUM_PATH=/path/to/chrome npm run test:ux -- --grep 'replacing an analysis'
```

These checks use stubs and do not prove NPU hardware readiness. Hardware testing
of cancellation and regeneration runs separately and explicitly.

Hardware checking is performed on a **free** runtime started from this code
version. It intentionally interrupts one inference and expects a new comparison
with the same pinned profile:

```bash
NPU_REAL_CANCEL_SMOKE=1 \
PROJECT_CREATOR_NPU_SOCKET=/absolute/path/runtime.sock \
PROJECT_CREATOR_NPU_PROFILE=<manifest-sha256> \
go test ./internal/analysis -run '^TestOpenVINORealNPUCancelThenGenerate$' -v -count=1
```

Before cancellation, the test waits for the worker request (`generation_started`);
afterward, it waits for `worker_stopped` for its `call_id` in
`logs/npu/runtime-lifecycle.jsonl`. This distinguishes stopping the native worker
from merely closing the HTTP request. The generation-start event itself does not
prove that computation ran on the device. The new result is checked against the
schema and sources; verdict quality is evaluated with separate hardware examples.
`backend-lifecycle.jsonl` links `callId` to run and pair IDs; the `X-NPU-Call-ID`
header links it to the runtime log. Neither log contains requirement text, prompts,
or responses.

### Automatic requirement summaries (#72 included in #48)

Summary languages are configured in `.env` with
`PROJECT_CREATOR_SUMMARY_LANGUAGES=ru,en` (also the default when unset). Supported
codes are `ru`, `en`, `de`, `fr`, `es`, `uk`, `zh`, and `ja`. For example, `ru,en,de`
adds German. Empty elements and unknown codes are configuration errors. Apply the
updated configuration with `./project-creator up`, rebuilding the backend if code
changed. The language list is part of the profile: old summaries remain in history,
and new ones are generated automatically for a new profile.

The model keeps the original language if it is allowed; otherwise, it translates
the summary into the first language of the canonical list (codes are sorted and
duplicates removed). The server checks the alphabet and permits digits, spaces,
punctuation, symbols, and emoji. Other letters are not stripped: the model gets
one correction attempt; if that also fails, the summary is not published and the
final LLM may check the originals. Combined letters represented by separate
combining diacritics are rejected; the model must return the ordinary single
letter from an allowed alphabet. Alphabet checks cannot distinguish languages
that share letters, so the model instructions also specify the language constraint.
The new rule does not restrict original requirements.

With `PROJECT_CREATOR_SUMMARY_MODEL` and a full `PROJECT_CREATOR_SUMMARY_DIGEST`
explicitly set, requirements automatically enter a local background queue after
saving. `PROJECT_CREATOR_SUMMARY_URL` selects a separate Ollama runtime. CRUD and
search do not wait for generation; summaries are tied to the source version and
profile. 350 UTF-8 bytes is a soft generation target and a Laya eligibility limit,
but longer valid summaries are retained. In a card, the summary is a preview, and
the full original is available by expanding it. Laya compares summaries; the final
LLM receives full originals. If a summary is missing, invalid, longer than 350
bytes, or Laya returns an error, the pair skips preliminary classification and
still goes to the final LLM with full originals. The skip reason is saved; a summary
remains a model suggestion.

Configuration, cancellation/retry, and limitations: [Laya pipeline](docs/laya-pipeline.md),
[ADR-009](docs/adr/009-requirement-summaries.md), and
[ADR-010](docs/adr/010-optional-laya.md).
Hardware results and limitations of the real model are in the
[verification report](docs/requirement-summary-verification.md).
