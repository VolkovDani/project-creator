# Repository Instructions

**Русская версия:** [AGENTS.ru.md](AGENTS.ru.md).

- Read `GOAL.md` before making changes. Preserve the distinction between agreed
  goals, proposals, and open decisions; do not expand the task to cover the entire
  roadmap.
- Work using **TDD: Red → Green → Refactor**. For behavior changes, add a test
  first and confirm that it fails for the expected reason, then implement the
  minimum needed to pass and refactor. For bugs, add a regression test first.
  Documentation-only changes do not require artificial tests.
- Go is the preliminary choice for the target architecture; the backend MVP is
  already implemented in Go. Follow the repository's actual stack; do not choose
  additional frameworks or services without necessity.
- Respect module boundaries. Separate business logic from HTTP, storage, RAG,
  and model adapters. Prefer a simple architecture without premature
  microservices.
- Prefer local models for generation and embeddings. External providers (OpenAI,
  Anthropic, and others) must connect through replaceable adapters and explicit
  configuration. Do not add automatic cloud data transmission when a local
  model fails.
- Do not treat LLM output as a confirmed fact. Preserve sources and the
  provenance of proposals; do not present a model's self-assessment as a
  statistical probability.
- Check access rights before searching or passing context to a model. Prevent
  cross-client data leaks. Do not commit keys, secrets, or real client data.
- Use deterministic provider stubs in ordinary tests. Tests with real models and
  external APIs must run separately and explicitly; check RAG quality against
  fixed examples with expected sources.
- After changing Go code, run `gofmt`, `go test ./...`, and `go vet ./...` from
  the relevant Go module; run the necessary checks for affected components. Do
  not report a check as successful if it was not run.
- Keep changes small and task-focused. Update documentation when behavior
  changes and record significant architectural decisions with their rationale.
- For large changes, where practical, preserve completed working pieces in
  separate small commits as they are ready. Before each commit, run the checks
  relevant to its changes; intermediate commits must build and remain functional
  so they can be safely reverted and regressions located with `git bisect`. Do
  not include unfinished related changes or other people's changes in such a
  commit.
- Store logs and diagnostic reports in the root `logs/` directory (for NPU, in
  `logs/npu/`), which is excluded from Git and the Docker build context. Keep
  model weights, profiles, and provenance files in their designated directories.
- In your final report, briefly state what changed, which checks were run, and
  what limitations remain.

## Frontend and Backend Contracts

- For TypeScript/TSX changes, follow `frontend/biome.jsonc`. Do not format files
  outside the current task, edit generated GraphQL types manually, or use `any`
  to bypass TypeScript errors.
- Before each commit, format changed files covered by `frontend/biome.jsonc`
  with Biome: from `frontend/`, run
  `./node_modules/.bin/biome check --write <paths>` without `--unsafe`. Do not
  replace formatting with manual whitespace edits or format unrelated files.
- The frontend package manager is pinned to Bun 1.4.4 in `frontend/package.json`;
  use `bun install --frozen-lockfile` and `bun run <script>`. Node.js 26 remains
  required for unit tests, repository scripts, and the production Next.js server.
- After cloning and installing dependencies, install the tracked Git hook:
  `cd frontend && bun install --frozen-lockfile && bun run hooks:install`. It is stored in
  `.githooks/pre-commit`; the `core.hooksPath` setting is local to each clone
  and is not automatically transferred through Git. See [Git hooks and formatting](docs/development.md#git-hooks-and-formatting) for full instructions.
- The pre-commit hook checks the Git index, applies safe Biome fixes to fully
  staged files, and adds the fixed files back to the index. Unstaged changes in
  partially staged files are preserved; if the staged portion needs fixes, the
  commit is blocked until it is explicitly formatted and staged again. The hook
  does not replace required tests, type checks, or builds; do not bypass it to
  hide check failures.
- Do not suppress Biome rules without justification or apply unsafe automatic
  fixes without checking their consequences. After frontend changes, run
  `bun run format:check`, `bun run typecheck`, and the relevant tests from
  `frontend/`.
- Follow `ARCHITECTURE.md`: use GraphQL for queries/mutations, SSE for
  invalidation, and Next.js + TypeScript for the frontend.
- SDL is the source for type generation, and the shared constraints contract is
  the source for validation. Do not edit generated files manually. Use existing
  CI checks to validate operations, generation, and the absence of contract
  drift.
- Do not independently duplicate shared field constraints in Go and TypeScript.
  Check the same valid and invalid examples on both sides.
- Server-side validation is required. Pass field errors as
  `path/code/messageKey/params`; do not use translated text to determine an
  error programmatically.
- Publish invalidation events after a successful data commit and check the
  access scope. Check duplicate events, reconnect/reset, lists, and races between
  a query and an event. Permission changes must clear inaccessible data.
- Always use infinite scrolling for user-facing lists that load in batches. Provide a keyboard-accessible **Load more** button.
- Follow the [UI guidelines for controls, links and text](docs/development.md#controls-links-and-text).
- Put all user-facing strings through i18n. Validate the shared Go/TypeScript
  dictionaries and translation parameters automatically. Do not translate user
  content.
- For frontend work, run the project's prescribed tests, type checks, and build;
  use the commands in the [development guide](docs/development.md#nextjs-interface) and update them when the process changes.

## Dependency Selection

- Choose dependencies that are community-vetted and suitable for the task.
  Assess recognition and actual usage, maintainer support and activity,
  documentation, license, package provenance, and known vulnerabilities. Star
  counts are one possible signal, with no fixed threshold.
- Check direct and significant transitive dependencies and build tools; consider
  which dependencies ship in the application and which are only used for
  testing. Use official sources and pin versions.
- If significant issues are found, choose a safe alternative or document the
  assessment and mitigation in the PR. A language standard library is not a
  third-party dependency.

## Delegating to Subagents

### When to Delegate

- Before complex work, identify independent, bounded subtasks and by default
  delegate a suitable one when doing so would improve quality or reduce total
  time. If you decide to do all the work yourself, briefly tell the user the
  specific reason, such as dependence on a decision that has not yet been made
  or the expected cost of context transfer and verification exceeding its
  benefit. Do not delegate just to increase the agent count; the number of steps
  alone does not make delegation necessary.
- Delegate suitable independent tasks: code search, bounded review, failure
  analysis, tests, documentation, or implementation of a separate component.
  Do simple edits yourself when the handoff and verification would take more
  effort than the work. Separate implementation, testing, and review among
  agents when useful. While a subagent works, do other useful independent work;
  do not repeat its assigned work without a specific reason.
- Current repository data may be delegated to a subagent: issue and PR states,
  branches and SHAs, GitHub Actions results, review comments, and other
  information needed for the task. Specify read-only mode and limit requests to
  the needed data; use batched requests and reasonable CI polling intervals to
  reduce cost and duplicate requests. The subagent provides the primary agent with a
  concise summary including sources, SHA, and state at the time of checking,
  and marks unavailable or unconfirmed data. The primary agent makes decisions
  and verifies key facts before changing the repository or merging a PR.
- Do not repeat a completed and verified review of unchanged code. Repeat a
  check only for a specific reason: new changes, a discovered gap, or
  contradictory results. Limit the repeat check to that reason.

### Subtask Setup and Assignment

- Use `fork_turns: "none"` and omit the model override to inherit the parent
  model by default. If the task explicitly requires a particular model, check
  the models offered by the current tool before requesting it.
- Provide context explicitly: goal, completion criteria, constraints, relevant
  files, starting commit, and checks already performed. Specify the expected
  result, such as a list of confirmed defects with code locations or specific
  changes and their test results.
- State the allowed actions explicitly. For search and review, allow read-only
  actions; for implementation, list the files or modules the agent may change.
  TDD and all other repository instructions also apply to subagents.
- Assign non-overlapping write areas because agents share a filesystem. Before
  assigning the same files to another worker, stop the previous worker and
  inspect its remaining changes.
- Set a checkpoint and require a progress report when assigning work. By
  default, check progress after 2 minutes for search and bounded review, and
  after 5 minutes for implementation and tests. Set a different checkpoint in
  advance for work expected to take longer. A checkpoint is a time to assess
  progress, not an automatic declaration of completion.

### Waiting and Handoff

- At a checkpoint, assess concrete progress: locations found in code, changes
  made, check results, or an identified obstacle. A status of “working” and
  repeated promises to finish do not demonstrate progress.
- If the result is still needed and progress is confirmed, set a justified new
  checkpoint. If there is no progress or waiting now costs more than finishing
  independently, stop the subagent, take over its work, or reassign the task.
  Do not extend a checkpoint repeatedly without new results.
- Do not enter a cycle of short polls and repeated messages asking for the same
  thing. Wait for a notification; tell the user about significant results,
  delays, and changes in approach.
- After a subagent fails or stops, inspect the actual files because partial
  changes may remain. Check any late response against the current code state.

### Accountability and Verification

- The primary agent is responsible for architecture, security, integration,
  final checks, and compliance with the task. These decisions are not
  automatically delegated to a subagent.
- Verify review findings against the code, and verify changes against the diff
  and relevant tests. A subagent's report alone does not prove correctness or
  successful test execution.
- After integration, run the necessary checks for affected components. Keep the
  results of checks already performed and repeat them if there are new changes,
  failures, or a specific untested hypothesis.

## GitHub

- Write GitHub Issue and Pull Request titles, descriptions, review text, and
  comments in English only. Keep Russian text in documentation, including its
  required Russian translations; do not duplicate GitHub discussions in Russian.

- Format milestone descriptions in readable Markdown: separate the goal,
  release contents, required release conditions, and scope boundaries using
  headings and paragraphs. Use lists for tasks and conditions; leave blank
  lines between headings, paragraphs, and lists. Bold the target date and key
  conditions, and label tentative dates explicitly as tentative. When writing
  through an API, preserve real line breaks rather than literal `\n`; verify
  the saved description afterward.
- CI must account for the owner's goal of staying on GitHub Free: use standard
  Linux runners, avoid duplicate push/PR runs, and cancel obsolete runs. Start
  large reruns to test a specific hypothesis, not merely to repeat confirmed
  results.
- Do not use paid runners, Packages/registries, or cache quota increases beyond
  the free 10 GiB to speed things up. Cache and artifact limits are separate;
  limit artifact retention and do not upload unnecessary build records.
- Workflow settings reduce quota usage but do not replace a billing overage
  block. Do not guarantee that no charges will occur unless the account budget
  and limits have been checked.
- If a PR diff contains only `.md` and `.txt` files, GitHub Actions skip it,
  except for `requirements*.txt` files in any directory, which trigger CI even
  on their own. If CI is skipped for a documentation-only PR, review the current
  diff and run `git diff --check`, then merge with a regular merge commit using
  `gh pr merge --merge --match-head-commit SHA` without `automerge`, since the
  merge job will not run either. Do not use `--admin` or wait for absent CI
  checks. Use the normal required checks and `automerge` process below for
  `requirements*.txt` and mixed diffs.
- By default, target ordinary changes to `develop` and create each such PR with
  the `automerge` label after checking the current diff and required local
  checks. The user has authorized this workflow in advance; separate merge
  approval for each PR is not required. After `checks` and `containers` succeed,
  the final CI job creates a merge commit with two parents, preserving the
  branch's connection to `develop` in history.
- Target release-preparation PRs from `develop` to `main` without `automerge`;
  merge them manually after checking CI, the diff, and release acceptance.
  `main` remains the default branch. Publish the initial v0.1.0 local MVP
  prerelease when its release workflow passes on the exact source SHA and no
  known critical issue remains. Hardware verification is not a publication gate;
  record hardware evidence and limitations accurately and collect further reports
  after publication. Documentation and installation fixes may ship in patch releases.
- If you determine that a PR should not be automatically merged into `develop`
  (for example, work is unfinished or there is an unresolved defect, risk, or
  open decision), create it without `automerge` and briefly state the specific
  reason in the PR description and final report. If an obstacle is found after
  adding the label, remove it before the job is checked; restore it after the
  reason is resolved.
- Before adding `automerge`, confirm that the PR is from a branch in this
  repository and targets `develop`, and that its workflow already contains the
  `automerge` job. Update an old branch without this job from current `develop`
  and push the changes to start new CI; adding the label does not update a
  completed workflow run. Use this process, not GitHub's built-in auto-merge
  (`gh pr merge --auto`).
- The label may be applied while CI is running. If CI has already completed,
  after adding the label rerun only the `automerge` job with
  `gh run rerun RUN_ID --job JOB_ID`; obtain `JOB_ID` from the `databaseId`
  field in `gh run view RUN_ID --json jobs`. Do not rerun all CI just for the
  label. If the head or base changes, update the branch and wait for checks on
  the new version; do not use an old successful run.
- After the job runs, verify the actual PR state (`MERGED`, `mergedAt`, and
  merge commit); a successful job may mean it skipped the merge. To cancel a
  pending auto-merge, remove the label before its job checks it. See [Automatic merge after CI](docs/development.md#automatic-merge-after-ci) for limitations of this mechanism.
- For concrete implementation tasks that change code, create a GitHub Issue or
  use a suitable existing task. Implement each GitHub Issue in a separate Pull
  Request.
- Changes to documentation only, including `GOAL.md`, `README.md`, and
  `AGENTS.md`, do not require a GitHub Issue. If documentation is updated as
  part of a code-change task, include it in the same task and PR.

## Workspace and Task Completion

- Before starting code work, create a task (GitHub Issue) or select a suitable
  existing one, and create a separate working branch for it. Documentation-only
  changes do not require an Issue.
- When you start work on a GitHub Issue, set its status in GitHub Projects to
  `In Progress`.
- Create containers and other temporary resources only when necessary; track
  which resources you created for the task.
- After completing the task successfully and closing its PR, clean up after
  yourself: stop and remove containers created for the task, and remove your
  temporary files and other temporary resources that are no longer needed.
  Closing a PR without merging it does not count as successful completion.
- Leave the workspace free of your own unfinished changes and temporary
  artifacts. Do not delete or reset other people's changes, shared containers,
  persistent volumes, data, or model weights as cleanup.
- After a PR merge is confirmed, delete its working branch without asking
  separately; the user has authorized this cleanup in advance. Closing a PR
  without merging is not grounds for deletion. GitHub is configured to delete
  head branches automatically after merge; verify the result and update remote
  refs with `git fetch --prune`. This command does not delete local branches.
- Before deleting a temporary local branch, verify that its commits are included
  in current `origin/develop` (ordinary PR) or `origin/main` (release PR), or
  that its HEAD matches the merged PR head SHA whose merge commit is included in
  the target branch (including after squash or rebase). A branch name or closed
  PR alone is insufficient; preserve new commits made after the merge.
- `main` and `develop` are permanent integration branches; never delete them
  locally or remotely. If GitHub auto-deletes `develop` after a manually accepted
  release PR, immediately recreate it from the updated `main` and verify it.
  First switch away from a temporary branch before deleting it. Do not delete
  branches without confirmed integration, or branches used by another
  worktree. Preserve worktrees with uncommitted changes and other people's
  resources; only your own completed, clean worktrees may be removed. Update
  local `main` only by fast-forward; if it has diverged, preserve its commits
  and report the divergence instead of resetting it for cleanup.
- Delete local branches when finishing your work after merge or during the
  next synchronization: GitHub does not delete branches on the local machine
  automatically. In the final report, list any branches preserved and why.
- In the final report, state the cleanup result and any remaining resources,
  with the reason they had to be kept.

## Documentation Languages

- Accompany every new document and documentation change with English and Russian
  versions. Keep requirements, examples, limitations, and verification statuses
  aligned; link the versions to each other. Do not claim a translation is current
  until it has been checked against its source.
- For new documents, use English `<name>.md` and Russian `<name>.ru.md` paths.
  Existing Russian canonical documents keep their paths; the English version
  gets the `.en.md` suffix. Keep `AGENTS.md` as the canonical English
  tool-required instructions; its Russian version is `AGENTS.ru.md`.
- Place additional languages in `docs/i18n/<language-tag>/`, mirroring the
  original document's path from the repository root. First add a description of
  the language and instructions for checking its translation according to
  `docs/i18n/README.md` and `README.ru.md`.
- Do not rewrite historical reports to reflect new results. When translating,
  preserve the original date, SHA, and status; explicitly mark existing
  untranslated materials.
