# Development and testing

[Project Creator](../README.en.md) · [Русская версия](development.ru.md)

[Local checks](#local-checks) · [Run the API](#postgresql-and-running-the-api) · [Run the frontend](#nextjs-interface) · [Git hooks](#git-hooks-and-formatting)

## Container smoke and CI

When a worker already owns an indexing job, the smoke test polls the status of that requirement version for up to two minutes.

The regular smoke test starts a temporary Compose project with a test Ollama stub.
It checks building both images, migrations, and the complete HTTP path through
Next.js: project → requirements → indexing → collision with source versions,
then data persistence after restarting PostgreSQL/backend. No weights or GPU are
needed. The test stub is included only through explicit `compose.smoke.yml`; it
is not in the production Compose setup.

Node.js 26 is additionally required to run the smoke script on the host. The
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
  export COMPOSE_FILE=compose.yml:compose.cpu.yml
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
  export COMPOSE_FILE=compose.yml:compose.cpu.yml
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
docker compose --env-file .env.real-smoke -p project-creator-real-smoke -f compose.yml -f compose.cpu.yml down --volumes
```

This run does not assess statistical LLM quality and may fail on a slow CPU; it
is not part of ordinary CI. See the [search and analysis guide](requirement-analysis.md) for standalone checks of real adapters
and [local checks](#local-checks) for test database requirements. GitHub Actions separately runs
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

Go 1.26.10, a C compiler for SQLite import/testing, Node.js 26, and PostgreSQL 16
with pgvector 0.8.7 are required. Ordinary tests do not require an LLM or cloud
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

The storage decision is documented in [ADR-001](adr/001-postgresql-pgvector.md).
`compose.postgres.yml` pins PostgreSQL 16 and pgvector 0.8.7. It provides only a
database for development with processes on the host. To run the whole application,
use the main `compose.yml` as described in the [Compose guide](compose.md). SQLite is no longer the server's
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
[CI Docker cache guide](ci-docker-cache.md).

When CI runs on a PR to `develop` confined to application sources, Go module files,
test data, or the docs directory, it skips the optional Intel/AMD Vulkan image builds and their disk
cleanup. It still validates every Compose profile, builds both application images,
and runs the HTTP smoke and restart checks. A changed workflow, Vulkan Dockerfile, Compose file, runtime
configuration, an unrecognized path, or an unavailable base commit triggers both
Vulkan builds. Release PRs and runs on `main` also build both images.

CI downloads the Chromium headless shell pinned by Playwright and takes a blank
screenshot to check that it starts with the Ubuntu runner libraries. If that
probe fails, Playwright installs the system dependencies before the UX, screenshot,
and E2E suites run.

The pair-analysis release scenario matrix, isolated acceptance results, and
remaining limitations are documented in [report #85](pair-analysis-acceptance.md).

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

Changes consisting only of `.md` and `.txt` files are skipped by the GitHub Actions
path filter, except for `requirements*.txt` files in any directory, which trigger
CI even on their own. Mixed changes containing code also trigger CI. After
reviewing a documentation-only PR whose workflow was skipped and running
`git diff --check`, merge it with a regular merge commit using
`gh pr merge --merge --match-head-commit SHA`, without the `automerge` label: the
merge job does not run in this case either. Do not make a
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

After GitHub confirms the merge, the `close-issues` job closes the designated
issues as completed. Put `Closes #123` on its own line in the PR description
for each issue in this repository. The job also accepts the GitHub closing
keywords `Fixes` and `Resolves` and their verb forms. For an issue already on
the Development Roadmap, its **Item closed** workflow sets the card to **Done**.
If the close step fails, rerun the failed job to finish processing open issues.

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
on `main` (push or manual run) completed on that same SHA. Use `v0.1.0` for the
initial prerelease of the local single-user MVP. The workflow repeats checks and
builds on the source SHA and creates a prerelease draft. A maintainer separately
publishes it after reviewing the candidate and release notes and confirming there
is no known critical issue. Hardware reports are collected and reviewed separately
and do not block publication. Documentation and installation fixes can be included
in patch releases.

Pass a full Wiki commit as `notes_sha`. Before building, the workflow
reads the English and Russian release notes for the tag at that commit.
The draft records both the source and Wiki commits.

Detailed instructions are available [in Russian](release-acceptance.ru.md)
and [in English](release-acceptance.md); release notes are available
[in Russian](release-notes-v0.1.0.ru.md) and
[in English](release-notes-v0.1.0.md). Actual checks and remaining conditions
are listed in the [report dated November 17, 2026](release-verification-2026-11-17.en.md).

## Frontend package manager and build/runtime

The frontend package manager is Bun 1.4.4, pinned in `frontend/package.json`;
the resolved graph is recorded in `frontend/bun.lock`. From `frontend/`, install
with `bun install --frozen-lockfile` and run scripts with `bun run <script>`.
`trustedDependencies` is deliberately empty, so dependency install scripts are
disabled; the pinned frontend graph does not require them. `bun audit` checks the
dependencies in `bun.lock` for npm advisories.

The Next.js production build runs with Bun because repeated local builds were
faster than Node under matched conditions. On Linux x64 with Node 24.21.0, Bun
1.4.4, and Next.js 16.4.6, three clean builds had median times of 4.31 seconds
with Node and 3.88 seconds with Bun; three cached builds had medians of 1.98 and
1.51 seconds. These are local measurements, not a performance guarantee. Node.js
26 remains in use for unit tests and the production server. The production image
uses Node, following [Next.js deployment guidance](https://nextjs.org/docs/app/guides/deploying-to-platforms).
For Docker Compose, Bun is installed in the build image; the host does not need
Bun to start the full stack.

The [dependency update guide](dependency-updates.md) records the Bun package
manager, provenance, security, and benchmark assessment.

## Git hooks and formatting

The hook is stored in the tracked `.githooks/pre-commit` directory, and its
implementation is in `scripts/biome-pre-commit.mjs`. Git does not enable hooks from
a repository automatically: on each new computer/clone, run the following with
Node.js 26:

```bash
git clone https://github.com/VolkovDani/project-creator.git
cd project-creator/frontend
bun install --frozen-lockfile
bun run hooks:install
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
bun run format:check
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
`bun install --frozen-lockfile` in `frontend/`, run `node --test scripts/git-hooks.test.mjs` from the
repository root. In a source checkout, `bun run test` also runs this integration check
after dependencies are installed. An early Node test in CI before `bun install --frozen-lockfile` checks
the installer; scenarios using real Biome run later in frontend tests. The
application Docker build does not include Git tooling, so hook tests are skipped
there; other frontend tests run as usual.

## Next.js interface

Node.js 26 and Go 1.26.10 are required. Start the GraphQL server in one terminal,
setting `PROJECT_CREATOR_TOKEN` and `PROJECT_CREATOR_DATABASE_URL` as described
above. In another terminal:

```bash
cd frontend
bun install --frozen-lockfile
export PROJECT_CREATOR_TOKEN='the-same-secret-as-the-server'
bun run dev -- -H 127.0.0.1
```

Open `http://127.0.0.1:3000`. The Next.js server forwards GraphQL requests to
`http://127.0.0.1:8080/graphql`; for another address, set the server-side
`PROJECT_CREATOR_GRAPHQL_URL` variable. The token is not sent to the browser.
This is a local single-user mode: do not expose the interface to the internet
without separate authentication and access control.

The workspace opens in dark mode on the first visit and restores an explicitly selected theme on later visits. Both themes use the same semantic tokens from `frontend/app/tokens.css`. Text passages have a reading measure, while tables and analysis panels use the available workspace width.

The project menu follows the working order Overview, Requirements, Classification, Glossary and Analysis. Hover over a help icon or focus it with Tab to see what the section does. Press Escape to dismiss the tooltip. Open the **Technical section** to review analysis history, manage scheduled deletions, check automatic indexing or retry a failed record. The existing indexing URL still opens directly. The menu can be collapsed on desktop and opened when needed on mobile.

To switch projects, choose another project you have access to directly from the desktop sidebar. On mobile, open the project menu to make the same choice. The selected project opens in the current section. With a keyboard, press Tab to reach the project switcher.

The language selector and dark-theme switch are at the bottom of the project menu. On the project list and creation pages they are in the header. Section links use the same Feather icons as the FigCraft design. Their text labels provide the accessible names.

Browser tabs show the current section and the Project Creator name. Titles and page descriptions follow the selected interface language and use only interface strings.

Next.js serves the multi-size browser icon from `frontend/app/favicon.ico`. The standalone image also ships the supplied PNG variants from `frontend/public/`, including the 180 px Apple touch icon.

Use static imports for interface images. Pass the import to `next/image`, or use its `.src` when a URL is required, such as an icon link. If you add a local font, load it with `next/font/local`. The production build includes a content hash in each generated asset URL, so browsers can cache it as `immutable` and fetch a new URL when the file changes. Keep existing root URLs available for compatibility and allow browsers to revalidate them.

After changing the GraphQL schema and operations, run:

The generator reads the local SDL and `operations/*.graphql` with GraphQL Code Generator Core and the two pinned TypeScript plugins. CI validates the operations and compares the generated file with Git.

```bash
cd frontend
bun run codegen
bun run test
bun run typecheck
bun run build
bunx playwright install chromium
bun run e2e
```

For E2E, create a separate empty temporary database with pgvector and set
`E2E_DATABASE_URL` before `bun run e2e`. Playwright starts separate Go and Next.js
servers. Tests create data: for another clean run, recreate only the E2E database;
never point it at the working database. Example setup:

```bash
createdb -h 127.0.0.1 -U project_creator project_creator_e2e
export E2E_DATABASE_URL='postgresql://project_creator:your-password@127.0.0.1:5432/project_creator_e2e?sslmode=disable'
cd frontend
bun run e2e
```

### Controls, links and text

- Use a native button for actions such as **Copy code** or **Save**. Give it a visible boundary or fill so the action is recognizable before hover.
- Give enabled destructive buttons such as **Delete** a solid 1 px border in the theme’s danger color, visible at rest and on hover. Keep the keyboard focus outline separate from the border.
- Before permanently deleting saved user data, identify the item and ask for explicit confirmation. Put initial keyboard focus on **Cancel**. Cancel and Escape must leave the item intact.
- Use a link with an `href` for navigation. Underline links within text by default, including links in rendered Markdown. Keep the underline visible on touch screens.
- A navigation group such as the project menu can use a consistent row or tab treatment in place of underlines. Make each destination recognizable before hover and mark the current page with a visible indicator and `aria-current="page"`.
- Keep headings, labels and metadata such as a requirement version or indexing status visually distinct from controls. Reserve the button treatment and pointer cursor for interactive elements.
- Use native `details` and `summary` for expandable sections. The shared stylesheet supplies their frame, spacing and summary styling. Keep the native disclosure marker and keyboard interaction.
- Give keyboard focus a visible outline in both themes. A focused navigation link must remain distinguishable from the current page.

## Business requirements

Use **Add requirement** to open the create dialog. In **Search requirements**, submit an ID or text with Enter or **Search**. One submission shows case-insensitive ID and text matches from active and archived requirements, plus separate semantic results from the project's index. ID and text matches are available independently of the embedding provider. Text results mark the matching passage in its original casing and show the surrounding context. A matching ID is highlighted beside the requirement text. Open a result to see its details. The previous search URL redirects to this form.

Select a row to open its details beside the list. The panel shows the full requirement ID used by the API and MCP, the project, version, dates, status and original text. Below the original, **Short summary** is collapsed by default, followed by classification, revision history and **Summary version and origin** for a ready summary. Opening the last section loads the provenance JSON through a separate GraphQL query. On narrow screens, **Back to requirements** returns to the selected row. Archived rows open in the same panel.

Requirements render Markdown headings, paragraphs, lists, tables, quotations, links and fenced code. Known fence languages receive syntax highlighting. Use **Copy code** for the contents of a block. Select visible document text to add a glossary entry. In formatted requirements and ready summaries, project glossary terms have a dotted underline. Hover over a term or focus it with Tab to read its definition. Markdown links and code retain their usual behavior. **Markdown source** shows every matched range in the stored text, including terms that overlap Markdown syntax. Links to a specific glossary occurrence open the exact text at its saved offsets. Editing always uses the stored source.

The document renderer uses `markdown-to-jsx` for parsing and `sugar-high` for syntax tokens. Both are pinned MIT packages with no additional runtime dependencies. React renders the tokens as text. Raw HTML stays literal, and embedded images display their alternative text to avoid fetching remote assets from a requirement.

Active requirements offer **Archive**. A dialog shows an excerpt and initially focuses **Cancel**. Archiving preserves the full text revision history, including change reasons, and the classification history. Archived requirements are excluded from semantic search and new project analyses. They can still be found by ID or literal text. Open **Requirements archive** to restore an entry or choose **Delete permanently** and confirm in a second dialog. Escape and Cancel close either dialog without a mutation. The API permits permanent deletion only with explicit confirmation, the matching archived entry and its version, within the same project and workspace.

Requirement text is checked
against the shared contract `contracts/requirement.json`, identically in Go and
TypeScript; shared examples are in `contracts/requirement.fixtures.json`.
Requirements belong to a workspace and project. The interface appends a batch of up to 40 requirements when the reader reaches the end of the loaded list or presses **Load more**.

The update mutation checks the expected version and requires `input.changeReason`. A missing, empty or whitespace-only reason returns a field error at that path. Both validators use the examples in `contracts/revision-reason.fixtures.json`. PostgreSQL saves the new text, version, reason and timestamp atomically. On a version conflict the interface keeps the draft text and reason. Deletion also checks the expected version.

The requirement card shows text revisions recorded since upgrade. Indexing and analysis retain their own source snapshots. Editing triggers reindexing and marks previous analyses stale. See the [search and analysis guide](requirement-analysis.md).

Editing opens a modal dialog; saving closes it, while Cancel and Escape discard
the edit draft. Clicking the backdrop does not close the dialog, and an in-flight
save cannot be dismissed. Adding a requirement keeps its own draft. A global
Back to top button appears after scrolling more than 400 pixels and respects the
operating system's reduced-motion setting.

## Keeping two open clients current (SSE, MVP 5)

The Go server provides an authenticated `GET /events`. Next.js forwards the stream
through same-origin `/api/events`, keeping the token on the server only. The
interface remains a local single-user application; this does not replace full
authentication for internet deployment. URLs contain no tokens.
The browser requests the long-lived event stream at low priority so it can load the page first.

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
