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

### Proposal: external AI assistant through MCP

An optional MCP adapter to existing business logic is under consideration: an
assistant reads authorized requirements and sources with their versions,
performs its own analysis, and saves a proposal with provenance, evidence, and
limitations. Proposals do not automatically change approved requirements;
stale sources remain visible. Access is checked before context is passed and
before a write, and cloud processing requires explicit consent. Local model
failure does not enable cloud use automatically. This proposal is not implemented
and does not choose an SDK or transport; open questions are tracked in
[Issue #146](https://github.com/VolkovDani/project-creator/issues/146). See the
[MCP proposal](docs/mcp-integration.md) for details.

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
- The primary store is PostgreSQL with pgvector and exact cosine search. By default, models
  run locally through Ollama with BGE-M3 for embeddings. Generation can explicitly use
  experimental OpenVINO on Intel NPU. The CPU and Arc GPU comparison applies to the
  devices, weights and inputs in the report. [ADR-001](docs/adr/001-postgresql-pgvector.md),
  [ADR-002](docs/adr/002-openvino-npu.md), [hardware verification](docs/intel-device-verification.md).

- Contract details and verification criteria are described in [ARCHITECTURE.md](ARCHITECTURE.md).
- Start with a simple application with explicit module boundaries; independently evolving sections do not by themselves require microservices.
- Development follows **TDD: Red → Green → Refactor**.
- Business logic is separated from transport, storage, and model providers.
- Ordinary tests are reproducible and require neither external APIs nor a running LLM; real models are checked in separate integration tests and evaluation sets.

## Published MVP and Near-Term Tasks

### Published as of November 19, 2026

The latest published prerelease is **v0.1.1**, dated November 18, at commit
`142392b17a290527a26a2fea123de8d5ca5bf560`. It is a local, single-user MVP
for managing requirements and their versions, indexing, search and pair analysis.
The patch added a requirement editing dialog, alphabet validation for pair
explanations, and installation fixes. Multi-user access control, the knowledge base
and the relationship map remain outside the published MVP. The first Source + Compose release was
v0.1.0 on November 17. [v0.1.1 release](https://github.com/VolkovDani/project-creator/releases/tag/v0.1.1),
[changes and limitations](docs/release-notes-v0.1.1.md),
[v0.1.0 release](https://github.com/VolkovDani/project-creator/releases/tag/v0.1.0).

The November 17 report records results for commit `5fb98a8d29c4161c0904eb43d103442853ddd053`.
Pair analysis passed 2 of 2 examples. Qwen2.5:7b summaries passed 3 of 5, with
two English responses to Russian sources. A full clean installation with a real
model, an application upgrade with migrations and the maximum supported project
size remain open acceptance questions. The PostgreSQL dump/restore check covers
database recovery. [Verification report](docs/release-verification-2026-11-17.en.md).

### Changes in `develop` since v0.1.1

The branch was checked at commit `8bb95ea` on November 19.

- **Pair analysis.** New runs identify exact duplicates, reuse eligible cached
  results and pass the full originals of selected pairs to the generator. The
  preliminary Laya stage was removed. Historical results retain their provenance.
  [PR #205](https://github.com/VolkovDani/project-creator/pull/205).
- **Summaries.** A requirement card shows the original until a summary is ready
  for that version. Generation preserves the source language when allowed and
  retains technical identifiers. There is no byte target. Users can see failures
  and retry a single requirement or all failed summaries in the project.
  [Analysis guide](docs/requirement-analysis.md).
- **Glossary.** Readers can look up confirmed terms in requirements, summaries and
  saved pairs. For a suggested abbreviation they can open the source mention and
  request a draft definition. If the quotation provides insufficient evidence,
  the model can offer a labeled hypothesis. Users can copy or edit a draft and
  decide whether to save the glossary entry. [Glossary guide](docs/project-glossary.md).
- **Classification.** Users can confirm a semantic type and a representation for
  a specific requirement version. Editing the text marks the decision as stale
  and preserves its history. The contract specifies `classifying` for a classification
  in progress. [Classification rules](docs/requirement-classification.md).
- **Model providers.** Generation and embeddings can independently use an explicitly
  configured OpenAI-compatible API, including a self-hosted vLLM server. The generator
  receives texts for comparison or summarization. The embedding service receives
  document chunks and search queries. Integration checks used deterministic HTTP
  services. Quality on live remote models still requires evaluation.
  [Setup guide](docs/setup-cli.en.md), [PR #235](https://github.com/VolkovDani/project-creator/pull/235).
- **Interface.** A dark theme followed the Untitled UI pilot. The control next
  to the language selector saves the user's preference.
  [PR #245](https://github.com/VolkovDani/project-creator/pull/245).

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

**#48 and #72 in v0.1.0.** The original pipeline used 350 UTF-8 bytes as the
threshold for admitting a summary to Laya. Hardware checks on that implementation
covered 21 pairs on CPU + NPU and Arc + NPU and search during analysis. The v3
summary profile passed 4 of 5 examples. Generation timed out on the example with
90 identifiers. [PR #71](https://github.com/VolkovDani/project-creator/pull/71),
[historical report](docs/requirement-summary-verification.md).

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
quadratic iteration.

### Open work and dependencies as of November 19

| Area | Status and next step |
| --- | --- |
| Summaries and pair analysis | The summary-language fix passed five fixed examples on the pinned Qwen profile. Showing a draft explanation while a pair is being compared is a separate issue marked `Ready`. Streaming still needs verification for Ollama, OpenVINO and the OpenAI-compatible API. [PR #158](https://github.com/VolkovDani/project-creator/pull/158), [#218](https://github.com/VolkovDani/project-creator/issues/218). |
| Classification quality | Manual two-axis classification is available. For #50, the evaluator calculates metrics on synthetic examples. The issue is marked `Blocked`. Evaluating a model requires independently labeled Russian requirements and actual runs with a recorded checkpoint and prompting schemes, including coverage of long inputs. [#50](https://github.com/VolkovDani/project-creator/issues/50), [evaluation method](docs/classification-evaluator.md). |
| Automatic classification | The model and thresholds for #51 depend on measurements from #50. Laya is a candidate for classifying individual records with explicit configuration. Showing the provenance of a suggestion is planned in #52. For #53, when to explain a saved classification remains open, including manual decisions and cases without an automatic suggestion. [#51](https://github.com/VolkovDani/project-creator/issues/51), [#52](https://github.com/VolkovDani/project-creator/issues/52), [#53](https://github.com/VolkovDani/project-creator/issues/53). |
| Relationship map | #57 proposes a Mermaid viewer. #58 would display saved relationships and requires agreement on their model, provenance and freshness. Users should be able to view a saved relationship with manual classification. A matching type alone does not create a relationship. [#57](https://github.com/VolkovDani/project-creator/issues/57), [#58](https://github.com/VolkovDani/project-creator/issues/58). |
| BGE-M3 search | #64 remains in the backlog. Retrieval alternatives should be evaluated on reviewed examples against current candidate selection and full-source comparison. This evaluation can run independently of the historical Laya stage. [#64](https://github.com/VolkovDani/project-creator/issues/64). |
| Operations | The NPU export dependency update remains open in #144. Docker Hardened Images require registry access locally and in CI while retaining the established Go and Node versions. The SSR decision depends on measurements. [#144](https://github.com/VolkovDani/project-creator/issues/144), [#98](https://github.com/VolkovDani/project-creator/issues/98), [#40](https://github.com/VolkovDani/project-creator/issues/40). |

The knowledge base (#78) and observability (#39) are deferred. Starting work or
creating a branch or PR requires a separate explicit instruction from the owner.
If the #78 experiment is approved, its baseline is the current pair selection,
exact duplicate handling, cache reuse and full-source comparison. Classification
scope, a shared relationship model with #58 and a reference set for retrieval
with #64 remain open. Integration requires measurements and a separate decision.
[#78](https://github.com/VolkovDani/project-creator/issues/78),
[#39](https://github.com/VolkovDani/project-creator/issues/39).

Previous decisions remain in force: #31 was closed without implementing the
“one line — one requirement” scenario; a new bulk-entry format has not been
agreed. #33 was completed independently of #31; references to #31 do not put it
back in the plan. The historical limit of 20 pairs from #48 does not replace
the current modes, duplicate handling, and cache in #35.

### Record classification on 20 November

Taxonomy version 2 includes a test case representation. None of the six Laya schemes qualified for automatic labels in the experiment with 28 reviewed Russian records. On the 13 holdout records, the hierarchical scheme found the correct meaning four times and the correct representation three times. Users classify records manually by default. An explicitly enabled local background worker provides option scores for review. The interface supports filters, decision provenance and history, and manual confirmation or correction of both axes. [Measurements](docs/classification-evaluator.md), [classification review](docs/classification-review.md).

## Open Decisions

- Detailed role permissions and whether end users need access.
- Quality of the selected local models on real requirements and parameters for future hybrid search.
- Expansion and possible review of the current GraphQL, cache, codegen, validation, and i18n tools. The MVP already uses graph-gophers/graphql-go, GraphQL Code Generator for TypeScript, custom Go codegen, a tagged client cache, shared JSON validation contracts, and RU/EN dictionaries.
- Quality of local models on target languages, data, and hardware.
- Conditions for adding further external providers and policies for transferring client data.
- Initial integrations with a tracker, repository, and documentation.
- Concrete data and methodology for estimating timelines and risks.
