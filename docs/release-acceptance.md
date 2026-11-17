# v0.1.0 hardware release acceptance

Русская версия: [docs/release-acceptance.ru.md](release-acceptance.ru.md).

A successful CI run proves deterministic tests and container builds. It does not
prove that a real model profile works on the target device. Keep the v0.1.0
prerelease in draft until maintainers have reviewed real hardware reports.

## Required report for each hardware/profile combination

Use the **Hardware release acceptance** issue template. Attach complete smoke
logs and record:

- Candidate tag and full source commit SHA.
- Exact standalone lines in the report body: `release-tag: v0.1.0` and
  `source-sha: <full 40-character SHA>`; these bind maintainer acceptance to the
  release candidate.
- CPU, GPU/NPU model and device identifiers, memory, OS, kernel and driver/runtime versions.
- Compose and application versions, Ollama/runtime version, model name and digest, profile identifier and configuration hash.
- Exact smoke commands and pass/fail outcomes, including startup, representative generation, restart/recovery, and clean shutdown. Include timing and resource observations where available.
- Full unedited logs, with credentials and personal data redacted.
- Optional links to independent community reports, grouped by release patch and
  listing the device/profile combinations and outcomes they confirm.
- Patch notes, compatibility notes, and known limitations.

## Acceptance and draft release gate

A maintainer reviews the full evidence and records the decision in the issue. The
initial hardware/profile report does not require independent community
reproduction. Apply the `hardware-accepted` label and close the issue only after
the maintainer accepts the complete report.
The manual release workflow requires that closed, labeled issue before preparing
a draft GitHub prerelease. This label is an explicit human decision; CI output or
model self-assessment cannot substitute for it.

Run the following operational checks for the candidate and record commands,
outcomes, and complete logs in the report:

1. Start from a clean checkout and follow the README setup instructions using a
   fresh `.env`; record image, app, database, runtime, and model versions.
2. Confirm the configured profile and model digests, then run the documented
   smoke with a representative project and requirements. Include both a known
   collision and a compatible pair and inspect sources/provenance.
3. Restart the Compose stack and confirm persisted data, model connectivity, and
   analysis recovery. Exercise backup and restore using the README commands.
4. Upgrade an existing installation from the previous supported revision after
   taking a backup; record migration output and verify existing project and
   requirement data remains readable.
5. Record shutdown and resource use, and capture failures with full diagnostics.

The workflow accepts only a semantic version tag, a full source SHA that is the
commit of the tag and is reachable from `main`, and a dispatch from `main`. For
the initial Source + Compose prerelease, use `v0.1.0`. It runs reproducible
release checks against that commit and creates a **draft** prerelease. Keep it
draft until release notes and the final candidate have been reviewed. Publishing
is a separate maintainer action after acceptance; do not publish while any
required hardware/profile acceptance remains open.

## Community confirmations by release patch

Group independent reports under the release patch they confirm. Include logs,
versions, identifiers, and outcomes where available. Do not extrapolate a result
from one device or profile to an entire vendor family. Release notes should list
tested combinations and state which combinations remain untested.
