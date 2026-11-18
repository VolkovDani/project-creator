# v0.1.0 release acceptance and hardware reports

Русская версия: [docs/release-acceptance.ru.md](release-acceptance.ru.md).

The initial v0.1.0 MVP prerelease may be published once the release candidate
passes the release workflow and has no known critical issue. Hardware/profile
reports remain useful evidence to collect and review after publication; they are
not a prerequisite for publishing v0.1.0. Do not describe an untested device or
profile as verified.

The owner clarified this publication policy on 17 November 2026: ship the initial
MVP, then collect hardware confirmations and installation refinements in patches.
For v0.1.0 this supersedes the earlier pre-publication hardware and full
installation/upgrade acceptance sequence in GOAL.md. A known critical defect,
including a critical installation failure, still blocks publication.

## Hardware/profile reports

Use the **Hardware release acceptance** issue template for each hardware/profile
combination. Attach complete smoke logs and record:

- Candidate tag and full source commit SHA.
- Exact standalone lines in the report body: `release-tag: v0.1.0` and
  `source-sha: <full 40-character SHA>`; these bind the report to a release.
- CPU, GPU/NPU model and device identifiers, memory, OS, kernel and driver/runtime versions.
- Compose and application versions, Ollama/runtime version, model name and digest, profile identifier and configuration hash.
- Exact smoke commands and pass/fail outcomes, including startup, representative generation, restart/recovery, and clean shutdown. Include timing and resource observations where available.
- Full unedited logs, with credentials and personal data redacted.
- Optional links to independent community reports, grouped by release patch and
  listing the device/profile combinations and outcomes they confirm.
- Patch notes, compatibility notes, and known limitations.

A maintainer may review a report and record an explicit decision with the
`hardware-accepted` label. This label records human-reviewed evidence; it does
not gate v0.1.0 publication. CI output and model self-assessment cannot substitute
for hardware evidence. A report applies only to its recorded device and profile.

Suggested operational checks for reports include:

1. Start from a clean checkout and follow the README setup instructions using a
   fresh `.env`; record image, app, database, runtime, and model versions.
2. Confirm the configured profile and model digests, then run the documented
   smoke with a representative project and requirements. Include both a known
   collision and a compatible pair and inspect sources/provenance.
3. Restart the Compose stack and confirm persisted data, model connectivity, and
   analysis recovery. Exercise backup and restore using the [backup and restore commands](compose.md#stop-update-and-back-up).
4. Upgrade an existing installation from the previous supported revision after
   taking a backup; record migration output and verify existing project and
   requirement data remains readable.
5. Record shutdown and resource use, and capture failures with full diagnostics.

## Preparing and publishing v0.1.0

The manual release workflow runs only when dispatched from `main`. It accepts a
semantic version tag and full source SHA, verifies that the tag points exactly
to that SHA, that the SHA is reachable from `main`, and that a successful CI run
on `main` completed on the same SHA. For the initial Source + Compose MVP
prerelease, use `v0.1.0`. The workflow repeats release checks and builds against
the source SHA and creates a **draft prerelease**. A maintainer reviews the
candidate and release notes, confirms there is no known critical issue, and
separately publishes the prerelease. Hardware acceptance is not a publication
gate; unresolved installation or documentation issues can be addressed in patch
releases.

## Community confirmations by release patch

Group independent reports under the release patch they confirm. Include logs,
versions, identifiers, and outcomes where available. Do not extrapolate a result
from one device or profile to an entire vendor family. Release notes should list
tested combinations and state which combinations remain untested.
