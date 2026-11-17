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
