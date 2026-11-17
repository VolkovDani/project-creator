# Automated dependency updates

**Русская версия:** [docs/dependency-updates.ru.md](dependency-updates.ru.md).

The repository enables Dependabot version updates with
[`.github/dependabot.yml`](../.github/dependabot.yml). The configuration belongs
on the default branch (`main`); each version-update entry targets `develop`, so
Dependabot reads manifests from `develop` and opens version-update pull requests
against `develop`. The `develop` branch existed when this configuration was
prepared.

## Coverage

| Ecosystem | Configured locations | Tracked files |
| --- | --- | --- |
| Go modules (`gomod`) | `/` | `go.mod`, `go.sum` |
| Bun | `/frontend` | `package.json`, `bun.lock` |
| pip | `/runtime/npu` | `requirements.txt`, `requirements-export.txt` |
| Docker | `/`, `/frontend` | Root `Dockerfile`, root `Dockerfile.*`, and `frontend/Dockerfile` |
| Docker Compose | `/` | Root `compose*.yml` files |
| GitHub Actions | `/` | Workflows under `.github/workflows` |

The Python updater supports `.txt` dependency files. The Docker updater recognizes
Dockerfile names containing `dockerfile` (including the repository's
`Dockerfile.ollama-*` variants); the Dependabot source currently uses a
case-insensitive `dockerfile|containerfile` filename matcher. This is intended
coverage, not a guarantee that every pin will be updated. In particular, the
export requirements include a direct wheel URL, local image tags in Compose are
excluded, and Docker dependency parsing has limits described below.

## Bun package manager assessment

The frontend pins Bun 1.4.4 in `frontend/package.json` and commits
`frontend/bun.lock`. GitHub documents [Dependabot's `bun` ecosystem](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)
for Bun 1.1.39 and newer. The [dependency graph ecosystem list](https://docs.github.com/en/code-security/reference/supply-chain-security/dependency-graph-supported-package-ecosystems)
does not currently list Bun. The name/version set migrated from npm is unchanged: all 317 resolved
package-name/version entries match, including the
`graphql-config` → `minimatch@11.0.1` override. This comparison does not prove
identical peer or hoisting topology; frontend tests, type checking, builds, and
container smoke checks validate the resulting installation. On 2026-11-17,
`bun audit` reported no advisories among the 295 packages in the Bun lockfile.

Dependency-graph alerts therefore cannot be relied on to scan `bun.lock`. CI and
release validation run [`bun audit`](https://bun.sh/docs/pm/cli/audit) against the
locked graph, and a weekly/manual audit workflow checks the default branch. Bun
sends the package list to the npm advisory endpoint; a failed or unreadable
response fails the audit. This check does not replace review of upstream notices.

Bun is [MIT-licensed](https://github.com/oven-sh/bun/blob/main/docs/project/license.mdx);
its official licensing page also lists LGPL-2 JavaScriptCore/WebKit components
and the licenses of bundled libraries. Bun is used for frontend
dependency management and the Next.js build stage only. The production image and
server remain on Node.js, so the Bun executable and its libraries are not included
in the shipped frontend runtime image. The production-runtime choice follows
[Next.js deployment guidance](https://nextjs.org/docs/app/guides/deploying-to-platforms),
which specifies a Node.js server.

Bun 1.4.4 was the [latest release](https://github.com/oven-sh/bun/releases/tag/bun-v1.4.4)
on the migration date and its GitHub tag points to a verified, signed release
commit. It fixes the [high-severity HTTP/3 QPACK header amplification issue](https://github.com/oven-sh/bun/security/advisories/GHSA-cf8x-9m69-3gg3)
affecting Bun 1.4.3 and earlier. The platform-specific
[npm package](https://www.npmjs.com/package/@oven/bun-linux-x64/v/1.4.4) is integrity-checked by npm and its `os`/`cpu` fields identify Linux
x64; its human-readable description incorrectly says macOS arm64. The installed
binary itself reported Linux x64 and revision `1.4.4+38afee6c8`, confirming that
the mismatch is limited to package description metadata.

`trustedDependencies` is explicitly empty, so Bun does not run third-party
dependency installation scripts. The current locked frontend dependencies do
not need those scripts; frontend tests, type checking, builds, and container
smoke checks are used to detect a future dependency that changes this requirement.
GitHub Actions caches only Bun's registry package cache under a key derived from
`frontend/bun.lock`; the repository cache quota is unchanged.

On local Linux x64, with Node 24.21.0, Bun 1.4.4, and Next.js 16.4.6, three clean
builds had medians of 4.31 seconds with Node and 3.88 seconds with Bun. Three
cached builds had medians of 1.98 and 1.51 seconds, respectively. Each build
succeeded. These are local measurements rather than a guarantee across hardware.
The change runs only `next build` under Bun; unit tests and the production server
continue to use Node.

## Consolidated dependency refresh (2026-11-17)

This refresh consolidates the dependency proposals from Dependabot pull requests [#112–120, #122, #133, and #134](https://github.com/VolkovDani/project-creator/issues/137). The older npm lockfile proposal in #121 was superseded by the Bun lockfile updates in #133 and #134; `package-lock.json` was not restored.

The frontend now pins React 19.3.1, Biome 2.6.7, Playwright 1.65.1, Node types 26.6.6, and TypeScript 7.0.2. Frozen Bun installation, formatting, type checking, unit tests, and the production build passed. The Node 26.13.1 production image also built successfully, including the Next.js build and deterministic tests.

The backend module versions follow #117, and the build image follows the Go 1.27.4 proposal in #114. Compose now uses pgvector 0.8.7. GitHub Actions checkout and artifact actions follow #116 and #118. The NPU export pins include NNCF 3.4.0, ONNX 1.23.2, and Optimum Intel 2.2.0.

The proposed `transformers==5.23.0` from #119 was not retained: the published [Optimum Intel 2.2.0 metadata](https://pypi.org/project/optimum-intel/2.2.0/) requires `transformers>=4.51,<5.6`. The current pin is **5.5.4**, the latest non-yanked stable version in that range checked on 2026-11-17. The earlier resolver-only check accepted 5.5.2; it did not establish export or inference compatibility.

A fresh install initially selected PyPI `torchvision==0.29.2`: dependency consistency passed, but importing it failed with `operator torchvision::nms does not exist`. The export requirements now also pin the official Linux x86_64 / CPython 3.12 CPU wheel `torchvision==0.29.1+cpu` with its SHA256. This is the newest matching CPU wheel found in the official index on the verification date. A regression test failed before that change and then passed, exercising the native CPU NMS operator and the Optimum exporter import. A fresh install of the existing `main` requirements at `8ff1e0e` did not install torchvision and imported the exporter successfully; this specific failure was introduced by the dependency refresh, not reproduced on that main baseline.

Run the opt-in environment check after installing `runtime/npu/requirements-export.txt` into the separate export environment:

```bash
uv pip check --python .npu/export-env/bin/python
NPU_EXPORT_ENV_SMOKE=1 .npu/export-env/bin/python -m unittest discover -s runtime/npu -p test_export_environment.py -v
```

Ordinary unit tests skip this check and do not install model libraries. Passing it proves native-op/import compatibility, not model quality; export and NPU inference still require explicit hardware checks.

**Temporary security limitation:** Transformers 5.5.4 remains affected by [GHSA-xrqw-3rrv-vx5w](https://github.com/advisories/GHSA-xrqw-3rrv-vx5w) and [GHSA-x9r9-c232-4q39](https://github.com/advisories/GHSA-x9r9-c232-4q39). On 2026-11-17 the owner chose compatible stable releases while awaiting a released Optimum Intel supporting patched Transformers. Follow-up [#144](https://github.com/VolkovDani/project-creator/issues/144) tracks replacement and validation; alerts are not dismissed and security PR #142 is not superseded. Until then, export only reviewed, pinned local model snapshots in an isolated environment without credentials or networking and with only the dedicated output directory writable. These precautions reduce exposure but do not fix the library vulnerabilities. The export environment is separate from the application's OpenVINO GenAI runtime.

### Hardware verification of the export refresh (2026-11-17)

Application/runtime code: `bd8bdd0288a337257a7b2ef0558f434e33b8f8f4`, with the export pins above. Host: Core Ultra 7 268V, Intel AI Boost, 32 GB RAM, Linux `7.0.0-43-generic`; Intel NPU driver/compiler/firmware `1.38.0.20260910-34487311128~ubuntu26.04`. Python 3.12.14; OpenVINO/GenAI/tokenizers 2026.4.1/2026.4.1.0/2026.4.1.0; smoke test binary built with Go 1.27.3.

A new offline export of `Qwen/Qwen2.5-7B-Instruct` revision `a09a35458c702b33eeacc393d103063234e8bc28` used `text-generation-with-past`, INT4 symmetric weights, ratio 1.0, group size -1. It completed in **44.44 seconds**, peak RSS **18.44 GiB**, producing about **4.1 GiB** of model files. The exporter ran with network access disabled, home directories hidden, and persistent writes confined to the test output, under a 20 GiB memory / 2 GiB swap limit. Source weights and the existing application model were not overwritten.

The new runtime profile is `a31f6d86ee327e8fd8a0283868fd4a000ec8efda30e637577d37d192eff418fc` (2048 prompt / 512 output tokens). Cold NPU load took **33.84 seconds**; a full restart loaded the cached model in **2.40 seconds** and passed a new collision comparison; all **14 fixed pair comparisons** passed expected verdict/source checks. The NPU busy counter increased by **111.46 seconds** during that suite. Cancellation followed by worker recovery and new generation passed in **13.21 seconds**. The first cancellation test was interrupted because an old root-owned diagnostic file prevented lifecycle logging; its contents were preserved and write access restored before the successful retry.

Local evidence is in root `logs/npu/issue137/` (`export.log`, `smoke.log`, `cancel.log`, environment/import checks and device counters); model files, profile and dependency freeze are in `.npu/issue137/`. Logs and weights are not committed. These fixed examples demonstrate this export/runtime configuration, not general model accuracy, a CPU/GPU comparison, or resolution of the remaining security advisories.

## Schedule and pull request volume

Each ecosystem is checked weekly, staggered from Monday through Saturday. Within
each ecosystem, SemVer minor and patch updates are grouped into one pull request;
major updates remain separate. Dependabot may keep at most two open version
update pull requests per ecosystem entry. With six configured entries, this
bounds open version-update pull requests to twelve across the repository.
Security update pull requests have a separate GitHub limit and are not included
in this configured version-update limit.

Dependabot's default three-day cooldown applies to version updates; it does not
delay security updates. The configuration does not add an `automerge` label or
automation. Dependabot's default dependency labels do not request automatic
merging; updates still require CI and review under the repository's normal
policy.

Dependabot-triggered pull requests run the repository's existing pull-request
CI. GitHub's current billing documentation lists standard GitHub-hosted runner
usage for Dependabot as free, including for private repositories. The repository
is private, but its broader account billing settings and remaining quota were
not checked; this does not guarantee that unrelated Actions use or storage will
never incur charges.

## Branches and security updates

The repository's default branch remains `main`. `target-branch: develop` applies
to version updates: Dependabot checks manifests from `develop` and targets those
pull requests at `develop`. GitHub documents that `target-branch` options do not
apply to security updates. Security update pull requests continue to target the
default branch (`main`) and use their separate configuration and limits.

On 2026-11-17, read-only GitHub API checks returned HTTP 204 for the repository's
vulnerability-alerts endpoint and `{ "enabled": true, "paused": false }` for
automated security fixes. These results confirm those two endpoint states at
that time; they do not establish the state of every code-security feature.

## Review limits

Dependabot can update manifests but does not replace compatibility review or CI.
GitHub documents that Dockerfile updates may affect only the first `FROM`
directive and do not update image references in `ARG` directives. The frontend
Dockerfile uses the same pinned Node image in both build and runtime stages, so
review its generated updates for consistency. Review updates to all Compose and
Docker image references as well; locally built `project-creator-ollama-intel`
and `project-creator-ollama-amd-vulkan` images have no external version source
and are ignored by the Compose updater.

Check each proposed change's release notes and run the applicable repository
checks before merging. Do not merge a major update solely because it was
proposed by Dependabot.

## References

- [Dependabot version updates](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependabot-version-updates)
- [Supported Dependabot ecosystems](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)
- [Dependabot configuration options](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference)
- [Dependabot and GitHub Actions permissions](https://docs.github.com/en/code-security/reference/supply-chain-security/troubleshoot-dependabot/dependabot-on-actions)
- [GitHub Actions billing and usage](https://docs.github.com/en/actions/concepts/billing-and-usage)
