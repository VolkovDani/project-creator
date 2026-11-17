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
| npm | `/frontend` | `package.json`, `package-lock.json` |
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
