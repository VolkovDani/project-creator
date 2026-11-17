# Prerelease verification — 17 November 2026

[Русская версия](release-verification-2026-11-17.md)

Application baseline: `5fb98a8d29c4161c0904eb43d103442853ddd053`.
Release process changes are checked separately in the PR for issue #106.
This report is not acceptance of the final release SHA.

## Confirmed results

- Frontend: format:check, 68 tests, typecheck, and production build passed.
- Playwright UX: 96 passed, 6 skipped; E2E: 54 passed.
- Isolated Compose smoke passed, including PostgreSQL/backend restart.
- `pg_dump -Fc` and `pg_restore --exit-on-error` passed on an isolated database
  after E2E; data dumps matched (34 INSERT statements). This verifies database
  restore, not the full application upgrade.

- `python3 -m unittest discover -s runtime/setup -v`: 87 tests passed.
- `python3 -m unittest discover -s runtime/npu -v`: 36 tests passed.
- `go test -p 1 ./...` passed using an isolated PostgreSQL test database;
  `go vet ./...` passed.
- `TestRealAnalysisSmoke`: both scenarios passed — collision (28.02 s) and
  compatibility (26.74 s), 54.76 s total.
- `TestSummaryQualitySmoke`: 3 of 5 scenarios passed in 135.14 s. Both English
  scenarios passed. For both Russian scenarios, the model returned English
  summaries, so lexical checks for Russian phrases failed. This does not prove
  that all reported facts were lost, but requires review of language behavior
  and does not permit reporting a successful quality smoke.
- The 90-identifier case passed in 98.03 s without a timeout; the summary
  preserved the identifiers and remained too large for Laya.

Real tests used the installed local `qwen2.5:7b`, digest
`sha256:845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`.
Ollama was accessed through temporary loopback proxies, which were stopped
following the tests. The running application Compose stack was not restarted.

Complete local logs are in the Git-ignored `logs/release-v0.1.0/` directory.
They are not included in the release sources.

## Outstanding acceptance

- Full CI and container smoke on the final SHA.
- Clean installation following the published instructions and database upgrade.
- Real browser reconnect/reset and user scenario verification.
- Recorded hardware configuration, memory use, and tested scale limits.
- Review of the quality smoke language results and an explicit release profile decision.

The v0.1.0 tag and publication are not confirmed until these items are complete.

## Cleanup

Tracked files contain no logs, temporary files, screenshots, Playwright reports,
or local worktree contents. Git and Docker exclusions already cover logs,
`.env`, `.npu/` weights, and backups. Unfinished branches and other worktrees
are preserved; their presence does not make them disposable artifacts.

Task-created Compose resources and the separate test database container were removed.
Temporary dumps were deleted; diagnostic logs were retained for investigation.
