# Source + Compose v0.1.0 (draft prerelease)

Русская версия: [release-notes-v0.1.0.ru.md](release-notes-v0.1.0.ru.md).

This draft packages the local, single-user Project Creator MVP. Do not publish
until maintainers finish and accept the required hardware/profile and clean
installation/update checks.

## Included

- Project cards and business requirements backed by PostgreSQL and pgvector.
- Local embeddings and explicit semantic search scoped to a project.
- Explicit local-model pair analysis with versioned sources, cancellation,
  retries, recovery, progress updates, and cached results.
- Docker Compose deployment with CPU as the default and optional Intel, NVIDIA,
  AMD ROCm, and AMD Vulkan configurations where host drivers support them.

## Evidence recorded for this candidate

- Deterministic Go/frontend/Playwright and container-smoke results are recorded
  in the repository's CI and acceptance reports; these do not establish model
  quality on real devices.
- One real `TestRealAnalysisSmoke` run passed 2/2 cases (known collision and
  compatible pair) using Qwen2.5:7b, digest
  `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`, with
  the current Ollama container and a temporary loopback relay. Full log:
  `logs/release-v0.1.0/real-analysis-loopback.log` (local diagnostic; not
  committed). This checks a single analysis profile on one host only.
- The separate real Qwen2.5:7b summary quality smoke scored 3/5 in 135.14 s; two Russian
  lexical fixtures received English output. A run over 90 identifiers completed
  in 98 seconds without timeout. This is a recorded limitation, not acceptance
  of summary quality.
- An isolated PostgreSQL `pg_dump`/restore check passed and preserved all 34 data
  insert statements. This covers the database backup round-trip only, not an
  application upgrade.
- See [the dated verification report](release-verification-2026-11-17.md) for
  detailed commands, outputs, and outstanding acceptance checks.

## Known limitations and pending checks

- Real model/profile acceptance is incomplete; one smoke run does not establish
  broad hardware compatibility or quality.
- Clean installation, application upgrade/migration, additional hardware/profile
  reports where available, and final candidate SHA checks remain to be recorded.
- GPU/NPU configurations require compatible host hardware, drivers, and runtime;
  capability varies by device. See the README hardware matrix.
- Search and pair preparation can become expensive as project size grows; no
  general supported maximum size has been established.
- This prerelease is intentionally not publishable until the hardware
  acceptance issue is reviewed and closed with `hardware-accepted`.
