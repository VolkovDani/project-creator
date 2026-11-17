# Project Creator — Project Goal

[Русская версия](GOAL.md)

## Why We Are Building It

A full-stack application for managing product knowledge, requirements, and changes.
It should help people understand why a product exists, how it is built, what new
functionality will affect, and who is responsible for its implementation and
operation.

We support multiple products and clients, including different product versions,
settings, and client-specific customizations. Sections evolve independently but
share entities, relationships, versions, and knowledge sources.

## Participants

- **PM (Product/Project Manager)** — leads the product and process, clarifies and agrees on requirements.
- **Developer / technical specialist** — implements, verifies, and maintains the product. This group has subroles: frontend developer, backend developer, QA tester, and data analyst.
- **Customer** — purchases the product for use and initiates its expansion.
- **User** — uses the product; whether users need separate system access remains open.

Project Creator participant roles and roles within the product being described
are separate entities. Participant permissions must account for the organization,
product, and client.

### Technical subroles and work scenarios

One participant may hold several subroles within a product. A subrole describes
a specialization and helps select a work scenario; specific responsibility for
a block, requirement, or change is assigned separately. A subrole does not
identify who caused an error and does not grant access by itself. User roles
within the described product continue to be modeled separately.

The following are scenario directions for a future interface. They clarify the
project goal; subrole selection, personalized screens, and the listed processes
are not implemented in the current local, single-user MVP.

| Subrole | Primary focus | Proposed work scenario |
| --- | --- | --- |
| Frontend developer | Interfaces, user scenarios, and client integration | Open a change → review requirements, scenarios, mockups, and the API contract → identify affected screens and states → implement the interface and verify acceptance criteria. |
| Backend developer | Business rules, APIs, data, and integrations | Open a change → review rules, contracts, and dependencies → assess API, storage, and migration changes → implement and verify server behavior. |
| QA tester | Requirement testability, quality, and regressions | Open a requirement or change → clarify acceptance criteria and risks → prepare test scenarios → record the result and link discovered defects to requirements and the product version. |
| Data analyst | Metrics, data sources, and analytical checks | Open a goal or change → clarify metric definitions and sources → check data availability and quality → prepare a calculation or analysis with its basis and limitations. |

Shared requirements, relationships, versions, and sources remain available to
all subroles. The entry point, information presentation, and order of actions may
differ. Switching work scenarios must preserve the organization, product, and
client access scope; the detailed permission matrix remains an open decision.
A data analyst does not replace the PM in agreeing on business requirements.

## Proposed Sections

1. **Products and goals.** Purpose, problems addressed, audience, boundaries, and success criteria.
2. **Product structure.** Processes, scenarios, functional blocks, business rules, dependencies, integrations, user roles, and access to functionality. Business processes, functional blocks, and technical components are distinct.
3. **Requirements and changes.** Collection and agreement of requirements, acceptance criteria, and discovery of duplicates, conflicts, ambiguities, and missing conditions.
4. **Change impact and risks.** Critical blocks, affected dependencies and clients, compatibility, migrations, testing, release and rollback; assessment of transition complexity.
5. **Planning and development.** Breakdown, task dependencies, team workload, effort and calendar estimates for tasks and blocks, and plan-versus-actual comparison. Whether to build a tracker or adapters for existing trackers remains open.
6. **Responsibility and quality.** Block owners, frontend, backend, and infrastructure responsibility, bug routing, tests, and incidents. The person responsible for initial triage is not necessarily the source of an error.
7. **Clients and deployments.** Client-product relationship, versions, enabled modules, settings, customizations, integrations, and the current state of each deployment. Current operational status requires monitoring and deployment data.
8. **Audience and markets.** Geography, cities and settlements, internet quality, devices, user-group problems, sales regions, and advertising segments. User facts are kept separate from market hypotheses.
9. **Economics and pricing.** Cost of developing blocks, operation, support, and deployment; plans, regional and special terms, customer pricing, and profitability calculations. Cost and price are distinct.
10. **Documentation and training.** Instructions for users, customers, developers, and PMs that account for version and configuration; detection of outdated materials.
11. **Brand and promotion.** Product and block names, slogans, icons, positioning, value proposition, and reasons to buy.
12. **Search, relationship graph, and AI assistant.** Search by words, phrases, and meaning; browse related entities and the graph, including Mermaid representation; answers with sources and information verification without requiring a dialogue.

This is a development map, not a commitment to implement every section in the
first version.

## Knowledge and AI

- Structured entities and explicit relationships are the foundation. RAG supplements them with document search; it does not replace the data model.
- Claims have a source, version, approval status, and change history. Confirmed facts, model proposals, and assumptions are distinguished.
- Three interfaces are envisioned: standard forms and search, automatic checks of entered information, and a small chat assistant with RAG access.
- Local models, including generation and embeddings, are preferred: the primary AI scenario must work without a required cloud API.
- External providers connect optionally through adapters: OpenAI (ChatGPT models through the API), Anthropic, and similar services. Cloud processing is enabled explicitly; there must be no silent switch from a local model to an external API.
- Access to sources is checked before context is passed to a model. Answers, search, and the graph must not expose another client’s data.
- Answers show sources and limitations. When data is insufficient, the system says so instead of presenting a guess as fact.
- Probabilistic estimates of timelines and risks are a separate capability that requires data and forecast quality checks. An LLM’s self-assessment is not a calibrated probability of correctness. Until data is available, use explainable estimates, ranges, and explicit assumptions.

## Proposed First Product Version (Long-Term Direction)

Primary scenario: a PM describes a product, its blocks, and current requirements,
then enters a new requirement. The system helps find possible conflicts, related
blocks, responsible people, and questions that need clarification.

This section describes a broader product direction, not the required scope of the
published local pair-analysis MVP described below.

Minimum proposed scope:

1. Product overview and participants with basic access control.
2. Functional blocks, rules, requirements, and explicit relationships between them.
3. Sources, requirement approval statuses, and change history.
4. Search and relationship browsing.
5. Check a new requirement with a local model and RAG: possible conflicts, affected blocks, clarification questions, and links to supporting evidence.
6. A thin model-provider interface that allows external APIs to be connected.

First-version verification criteria:

- On prepared examples, the check finds a known conflict and shows its sources.
- Not finding a conflict is not presented as proof of correctness.
- Model proposals do not automatically change approved requirements.
- The primary scenario works with a local model and no cloud keys.
- Users cannot receive another party’s restricted data through search, the graph, or AI.
- Core product and requirement operations are available without chat.

Branding, market analysis, complex pricing, a full-featured built-in task tracker,
and statistical forecasts are outside the first version. The first-version
boundaries are a proposal and will be refined during development.

## Technical Directions and Process

- **Go** is the language of the implemented backend MVP; its use across the target architecture remains preliminary, with no commitment to use it in every future component.
- **Next.js + TypeScript** is the selected frontend direction.
- **GraphQL** is the primary query and mutation contract. A versioned SDL schema is stored in the repository and generates frontend types and typed operations.
- **SSE** delivers cache invalidation keys (tags) from the backend, with freshness restored after a connection is interrupted.
- The frontend and backend use a shared declarative validation contract; the server remains the final source of validation.
- Input errors are returned with a field path, stable code, translation key, and parameters. The current API supports field errors; nested entities and separate general form errors require an SDL extension.
- All user-facing text goes through i18n. Shared Go and TypeScript dictionaries are stored in the repository; technical logs and user content are not translatable interface strings.
- The primary storage is PostgreSQL + pgvector ([ADR-001](docs/adr/001-postgresql-pgvector.md)). The MVP uses Ollama, BGE-M3 embeddings, and exact cosine search; the Qwen2.5 7B generator is configured separately. Running the entire MVP locally is described in the root README; Docker Compose stores the database and Ollama weights in volumes. The experimental OpenVINO generator on Intel NPU is selected explicitly and uses host weights and runtime ([ADR-002](docs/adr/002-openvino-npu.md)); fixed hardware examples, including insufficient-data cases, pass after link normalization. Full acceptance with CPU/GPU comparison remains a separate task.
- Contract details and verification criteria are described in [ARCHITECTURE.md](ARCHITECTURE.md).
- Start with a simple application with explicit module boundaries; independently evolving sections do not by themselves require microservices.
- Development follows **TDD: Red → Green → Refactor**.
- Business logic is separated from transport, storage, and model providers.
- Ordinary tests are reproducible and require neither external APIs nor a running LLM; real models are checked in separate integration tests and evaluation sets.

## Published MVP and Near-Term Tasks

The v0.1.0 scope is limited to a local, single-user MVP: requirements, their
versions and indexing, search, pair analysis, and execution management.
Multi-user access control is not implemented. Typology, automatic record
classification, the glossary, knowledge base, and relationship map remain a
later stage, not capabilities of the published MVP.

### State snapshot as of November 17, 2026

[v0.1.0](https://github.com/VolkovDani/project-creator/releases/tag/v0.1.0)
was published as the initial Source + Compose prerelease. The tag points to
`8ff1e0e56f754b932bf611f44244e42e0c04eeef`, the `main` branch head at the time
of verification. The [release workflow](https://github.com/VolkovDani/project-creator/actions/runs/44100924433)
succeeded on the same SHA. [PR #139](https://github.com/VolkovDani/project-creator/pull/139)
includes the #136 fix from [PR #138](https://github.com/VolkovDani/project-creator/pull/138).
Russian summaries retain separate technical names from the source, such as
Excel, API, and PDF, without allowing arbitrary English text. This does not
resolve the separate summary-language switching issue #108.

Release UI fixes #44, the infinite-scroll fix from PR #80, and the auto-merge
instructions from PR #84 have already been integrated; the previous list of
release PRs awaiting integration is no longer the current plan.

[Pair analysis acceptance](docs/pair-analysis-acceptance.md) and the
[November 17 report](docs/release-verification-2026-11-17.md) retain their
original SHAs and results. They do not confirm model quality or full hardware
compatibility of the final release. On the historical baseline
`5fb98a8d29c4161c0904eb43d103442853ddd053`, real analysis passed 2/2 smoke
examples, while Qwen2.5:7b summaries passed 3/5: two Russian examples received
English answers ([#108](https://github.com/VolkovDani/project-creator/issues/108)).
The example with 90 identifiers completed without a timeout. This result does
not rewrite the previous v3 report of 4/5 with one timeout.

A complete clean installation with a real model and application/migration
upgrade have not yet been accepted; PostgreSQL dump/restore verification
confirms database recovery only. A generally supported maximum project size has
not been established. Limitations are published in the [release notes](docs/release-notes-v0.1.0.md).

### Foundation already implemented

**#33 completed:** [PR #56](https://github.com/VolkovDani/project-creator/pull/56)
merged on November 5, 2026: a durable indexing queue, bounded retries, recovery,
version/profile checks, and shared project progress through GraphQL/SSE. The
implementation is in the current repository branch; requirement analysis still
starts explicitly. Implementation check results are listed in the PR; those
checks did not use real models or hardware.

**#59 completed:** [PR #60](https://github.com/VolkovDani/project-creator/pull/60)
merged on November 5, 2026. The button cancels the entire running analysis: the
current pair, all queued pairs, and unfinished preparation. Completed results
are preserved, and a late response from a cancelled call is not published. The
next explicit run or retry after user cancellation restores the NPU worker with
the same pinned model and profile. A cancelled check is not retried
automatically. Timeout, unexpected worker termination, and model file changes
still require operator recovery. Regressions were checked with deterministic
tests; local NPU readiness was confirmed through health checks, but the hardware
scenario “cancel → new generation” was not run in these checks.

**#45 implemented:** [PR #65](https://github.com/VolkovDani/project-creator/pull/65)
adds explicit parameter replacement, with a choice to wait for the current pair
or cancel inference. The replacement request is stored in the database; a usable
result from the current pair is reused, and the remaining old queue is stopped.
New inference starts after the old one completes; the NPU is restored with the
pinned profile and a bounded recovery attempt. Cause diagnostics and ADR-007
were added. Deterministic checks and a separate hardware scenario
“cancel → stop worker → recover → new generation” passed; methods and
limitations are described in the [NPU instructions](docs/intel-npu.md). This
checks lifecycle behavior, not overall model accuracy or the CPU/GPU comparison
from #22.

**#47 completed:** [PR #63](https://github.com/VolkovDani/project-creator/pull/63)
merged on November 5, 2026. Compact SSE snapshots with a run revision and
targeted updates replaced broad invalidation during analysis progress. Pair
texts and model provenance are loaded on expansion and cached separately.
Cancellation, retry, isolation, and protection from stale responses were
preserved. Measurement methods and limitations are described in the
[report](docs/analysis-live-performance.md); these checks did not use real
models or hardware. #39 remains in BACKLOG.

**#48 + #72 completed:** [PR #71](https://github.com/VolkovDani/project-creator/pull/71)
merged on November 9, 2026 after successful CI. At the owner’s direction, #72
was completed in the same PR as #48. Summaries are created automatically after
a requirement is saved and used as card previews; the full original is
available by expanding it. 350 UTF-8 bytes is a soft size target and a
condition for admitting a summary to Laya, but not a storage limit: a longer
valid summary is saved. If a summary is missing, failed, exceeds 350 bytes, or
Laya fails, the pair skips Laya, while the final LLM still compares the full
originals. A preliminary error is retained as the skip reason. 21 pairs were
hardware-tested on CPU + NPU and Arc + NPU, along with search during analysis
and the end-to-end path from real summaries to full comparison on NPU
([report](docs/requirement-summary-verification.md)). The historical strict
Qwen v2 evaluation set had dense examples that did not pass. The new v3 profile
quality smoke passed 4/5: valid dense summaries of 447 and 767 bytes were saved
but not admitted to Laya; generation of the example with 90 identifiers timed
out without publishing a result.

**#73 completed:** [PR #74](https://github.com/VolkovDani/project-creator/pull/74)
merged on November 9, 2026 and is included in `main`. Summaries use only allowed
languages (Russian and English by default), digits, and symbols. The list is
changed through `PROJECT_CREATOR_SUMMARY_LANGUAGES` in `.env`; it is part of the
generation profile. The server checks the allowed character set, and the model
receives a language instruction. This restriction does not apply to source
requirements. Character-set validation is not full linguistic language
detection.

### Release policy and further verification

By the owner’s decision on November 17, the initial MVP is published after the
release workflow succeeds on the exact source SHA and if no known critical
defects remain. Hardware acceptance is not a condition for publishing v0.1.0;
device and profile confirmations are collected after release. This replaces the
previous mandatory sequence “full hardware acceptance and installation →
release.” The policy and report requirements are in [release acceptance](docs/release-acceptance.md).

Installation and documentation fixes may ship in patch releases. Each new check
must state its SHA, profile, hardware, and limitations; stub-based checks do not
replace real model runs.

Further acceptance and regression scenarios:

- In `all`, every expected pair is accounted for; in `related`, unselected pairs are not called checked. Exact duplicates and cached results are distinguishable from a new model call.
- Fixed conflicts, duplicates, compatible cases, and insufficient-data cases; sources, versions, and output limitations are available to the user.
- A requirement changing during analysis, result becoming stale, and reuse of a suitable cache without unnecessary model calls.
- Cancellation, run replacement, restart, SSE loss, timeout, and model unavailability.
- A missing or overly long summary does not block comparison of originals within the selected final model’s context; silent truncation is not allowed.
- Search during analysis; time and memory at the selected project size. An acceptable size and acceptance thresholds still need to be defined and recorded.

Current pipeline limitations are confirmed by the code:
[prepare.go](internal/analysis/prepare.go) iterates over potential pairs in
batches and in `related` mode; reducing model calls does not eliminate the
quadratic iteration. In [pipeline.go](internal/analysis/pipeline.go), the Laya
result does not filter a pair out of final comparison; Laya failure or skipping
allows the full originals to be compared. Exact duplicates are handled
deterministically, and ready results may be reused from cache. Savings from
Laya have not yet been demonstrated. The historical v3 summary quality smoke
result — **4/5, one timeout** — is retained in the
[report](docs/requirement-summary-verification.md).

### Next stage and open agreements

These are directions and proposals for reconsidering tasks, not an approved new
architecture or a commitment to implement all listed capabilities.

| Tasks | Place in the plan |
| --- | --- |
| [#137 — dependencies and Node 26](https://github.com/VolkovDani/project-creator/issues/137) | Open: consolidate accumulated Dependabot updates and check runtime, CI, and type compatibility. The migration has not been done. |
| [#99 — explanation character set](https://github.com/VolkovDani/project-creator/issues/99) | Checking allowed characters and one retry within the remaining time budget are agreed; implementation remains to be done. |
| [#108 — summary language](https://github.com/VolkovDani/project-creator/issues/108) | Reproduce on the pinned profile, agree on language preservation or an explicit translation policy, then repeat the quality smoke without silently weakening the evaluator. |
| [#105 — Untitled UI](https://github.com/VolkovDani/project-creator/issues/105) | Selected direction instead of MD2/MUI from #46. First resolve licensing, integration method, shared tokens, and a pilot of requirements/analysis screens. License purchase is not agreed; #46 remains open and serves as a source of requirements, not a parallel design system. |
| #98 — Docker Hardened Images | Separate infrastructure task; coordinate with the runtime change in #137. |
| #22 — Intel NPU | Additional hardware reports, embeddings, and CPU/GPU/NPU comparison; these do not block the already published prerelease. |
| #40 — SSR | Based on measured need. |
| #49–53 — typology and classification | After reviewing the data model and quality measurements; benchmark #50 must precede the decision on automatic classification. |
| [#77 — glossary](https://github.com/VolkovDani/project-creator/issues/77) | Next stage: agree on shared domain context. |
| #57–58 — Mermaid and map | After agreeing on the relationship model; #57 provides a viewer, #58 displays actual saved relationships. Neither is a condition of the published MVP. |
| #64 — BGE-M3 search | Deferred research; first revisit hypotheses using Laya results and coordinate with #78. Implementation requires a separate decision and PR. |
| [#78 — knowledge base](https://github.com/VolkovDani/project-creator/issues/78) | Post-release research direction, retaining current pair analysis as the baseline. BACKLOG: start only on a separate explicit direction from the owner. |
| #39 — observability | BACKLOG: do not start or create a branch or PR without a separate explicit direction from the owner. |

Before implementing the next stage, the following must be agreed:

- A unified status contract for #49/#52 or explicit mapping of server statuses in the UI: #49 lists `queued/classifying/suggested`, while #52 lists `running`.
- Model and automation choice for #51 based on #50; if quality is insufficient, consider manual classification and `needs_review`.
- Availability of #53 explanations for manual classification and operation without Laya. Currently #53 describes a separate generative-LLM explanation on request after saving the Laya result, while #51 allows Laya to be unconfigured.
- A shared relationship, provenance, and freshness model for #58/#78; this is an overlap of directions, not a proven contradiction in the current implementation. Automatic classification must not become an implicit condition for viewing already saved relationships. Matching types alone do not create a relationship.
- A shared reference set and baseline for candidate search in #64/#78.
- Integration method and licensing for Untitled UI in #105, and the future status of the replaced #46.
- Classification boundaries in #78 — only with separate authorization for that direction.

Proposed near-term order: #137 → #99 → decision on #108 → #105 pilot →
glossary #77. This is a recommendation for choosing the next work, not an
approved schedule or authorization to start all tasks. For classification:
agree on #49 → reference set and benchmark #50 → decide on #51–53. Measurements
of the current analysis quality and cost remain the basis for future decisions.

#78 and #39 remain in BACKLOG until the owner gives a separate explicit
direction. Experimental work on #78 was previously proposed for
`experiment/knowledge-base`; branch creation is unconfirmed. Moving results to
integration branches requires measurements and a separate decision.

Previous decisions remain in force: #31 was closed without implementing the
“one line — one requirement” scenario; a new bulk-entry format has not been
agreed. #33 was completed independently of #31; references to #31 do not put it
back in the plan. The historical limit of 20 pairs from #48 does not replace
the current modes, duplicate handling, and cache in #35.

## Open Decisions

- Detailed role permissions and whether end users need access.
- Quality of the selected local models on real requirements and parameters for future hybrid search.
- Expansion and possible review of the current GraphQL, cache, codegen, validation, and i18n tools. The MVP already uses graph-gophers/graphql-go, GraphQL Code Generator for TypeScript, custom Go codegen, a tagged client cache, shared JSON validation contracts, and RU/EN dictionaries.
- Quality of local models on target languages, data, and hardware.
- The first external provider and data-transfer policy.
- Initial integrations with a tracker, repository, and documentation.
- Concrete data and methodology for estimating timelines and risks.
