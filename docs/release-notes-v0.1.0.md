# Source + Compose v0.1.0 (initial MVP prerelease)

Русская версия: [release-notes-v0.1.0.ru.md](release-notes-v0.1.0.ru.md).

This release packages the local, single-user Project Creator MVP. It is intended
for local use by one user; it does not provide multi-user access controls. The
release is an initial MVP prerelease. The hardware reports below are limited to
the recorded devices and profiles; no broad hardware compatibility or quality
claim is made.

## Included

- Project cards and business requirements backed by PostgreSQL and pgvector.
- Local embeddings and explicit semantic search scoped to a project.
- Explicit local-model pair analysis with versioned sources, cancellation,
  retries, recovery, progress updates, and cached results.
- Russian-only summaries preserve isolated source technical names such as Excel, API,
  and PDF without enabling arbitrary English prose (issue #136).
- Docker Compose deployment with CPU as the default and optional Intel, NVIDIA,
  AMD ROCm, and AMD Vulkan configurations where host drivers support them.

## Evidence recorded on the historical baseline

The dated verification report and detailed checks refer to source baseline
`5fb98a8d29c4161c0904eb43d103442853ddd053`, not the final publication SHA. They
are historical evidence and do not certify the final release candidate.

- Deterministic Go/frontend/Playwright and container-smoke results are recorded
  in CI and acceptance reports; they do not establish model quality on real
  devices.
- One real `TestRealAnalysisSmoke` run passed 2/2 cases (known collision and
  compatible pair) using Qwen2.5:7b, digest
  `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`, with the
  then-current Ollama container and a temporary loopback relay. Full log:
  `logs/release-v0.1.0/real-analysis-loopback.log` (local diagnostic; not
  committed). This checks one analysis profile on one host only.
- **Open known limitation:** the separate real Qwen2.5:7b summary quality smoke
  scored 3/5 in 135.14 s; two Russian lexical fixtures received English output.
  Track this in [issue #108](https://github.com/VolkovDani/project-creator/issues/108).
  A run over 90 identifiers completed in 98 seconds without timeout. This is a
  recorded limitation, not acceptance of summary quality.
- An isolated PostgreSQL `pg_dump`/restore check passed and preserved all 34
  data insert statements. This covers the database backup round-trip only, not
  an application upgrade.
- See [the dated verification report](release-verification-2026-11-17.md) for
  its exact baseline, commands, outputs, and outstanding checks.

## Known limitations

- Hardware/profile coverage is incomplete. One smoke run does not establish
  broad hardware compatibility or model quality. Additional reports can be
  collected after publication; list tested and untested combinations precisely.
- A fresh full installation with a real model and an application upgrade/migration
  have not been accepted. Container smoke used stubs; the separate PostgreSQL
  dump/restore check covers only the database round-trip. The release workflow
  must pass on the exact source SHA before publication.
- GPU/NPU configurations require compatible host hardware, drivers, and runtime;
  capability varies by device. See the README hardware matrix.
- Search and pair preparation can become expensive as project size grows; no
  general supported maximum size has been established.
- Documentation and installation corrections can be delivered in patch releases.
