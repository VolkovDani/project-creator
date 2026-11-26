# Requirement search and analysis

[Project Creator](../README.en.md) · [Русская версия](requirement-analysis.ru.md)

## Editing a requirement and reading its history

Choose **Edit** on a requirement card, update the source text and enter the **Reason for change**. The reason belongs to the saved text version. Creating a requirement and confirming its classification do not require a reason. If someone saves another edit first, the dialog keeps your draft and reason so you can decide whether to apply them to the latest version.

Open **Requirement text history** on the card to read the revisions, newest first. Each edit shows the previous and new wording, the submitted reason and the time of the change. When both versions are available, removed words are crossed out in the previous text and added words are underlined in the new text. For extensive rewrites, the changed passage can be highlighted as a single block. The local workspace token identifies a workspace only, so edits made with it have no recorded individual author. For requirements that predate revision history, the first available entry contains the text and version that existed at upgrade. Earlier wording and reasons cannot be reconstructed from that entry.

## Local embeddings and requirement search

RAG stores vectors in PostgreSQL with pgvector. This section uses the local Ollama adapter. For remote indexing, search, comparisons and previews, see [OpenAI-compatible providers](setup-cli.en.md#remote-generation-and-embeddings).
Projects and requirements continue
to work when no provider is configured; an indexing attempt reports that the model
is not configured. Data is not sent to an external API when the local runtime fails.

Commands in this section assume Ollama and the backend run on the host. For the
full Docker Compose setup, see [first run and model download](compose.md#first-run-and-model-download): download models with
`docker compose exec ollama`, configure `.env`, then recreate the backend. Exporting
variables in a terminal does not change the environment of an already running
container; the Ollama port is not published to the host by the full Compose setup.

Install Ollama following the official instructions at https://ollama.com/download
and explicitly download the model (this separately downloads weights; ordinary
tests do not do so):

```bash
ollama pull bge-m3:567m
curl http://127.0.0.1:11434/api/tags
```

Find the selected model entry and save the full `digest`, including `sha256:`.
Set the backend environment and restart it:

```bash
export PROJECT_CREATOR_OLLAMA_URL='http://127.0.0.1:11434'
export PROJECT_CREATOR_EMBEDDING_MODEL='bge-m3:567m'
export PROJECT_CREATOR_EMBEDDING_DIGEST='sha256:full-digest-from-api-tags'
go run ./cmd/server
```

Ollama should listen only on a trusted local interface. Do not expose it to the
internet. Loopback HTTP addresses and the explicit local Docker service name
`ollama` are supported; arbitrary remote addresses are rejected. There is no cloud
fallback. Do not store sensitive addresses or secrets in git.

With a local embedding provider configured, new and edited requirements are
automatically placed in a background queue after commit. Saving does not wait for
the model, and closing the tab does not stop processing. Index states are queued,
indexing, ready, or failed. The indexing page shows overall project progress and
per-requirement status; failures can be retried manually. Without an embedding
provider, requirements are saved without calling a model. Search starts only when
the user clicks “Search requirements” and uses ready fragments from that project.
No search results do not prove that there are no duplicates or contradictions.

Results show the matching fragment, its version, and the original text from that
version; the link opens the requirement. After requirements change, results are
marked stale and search must be repeated. Similarity is not a probability of
correctness and is not described as one.

Ordinary Go/CI tests use a deterministic fake embedding provider; Playwright checks
states and explicit actions with API stubs. A real local-model check runs separately
and needs no cloud key.

Search uses exact cosine comparison with pgvector, without HNSW. The profile ties
together the model name, full digest, 1024 dimensions, and chunking version. BGE-M3
is selected as a multilingual local option; quality on your requirements must be
checked separately. Changing the pinned profile excludes old vectors from search;
on backend startup, current requirements are automatically queued for the new
profile. On backend restart, open clients receive invalidations for status and
results, even if their SSE cursor is still valid.

Chunking is deterministic: up to 1000 Unicode code points, with 100 overlap and a
preference for paragraph/sentence boundaries. A fragment stores exact offsets and
the text from the source version; requirements are not merged. This is a heuristic,
not a guarantee of retrieval quality. Indexing uses a short transaction to claim
an attempt and a two-minute lease; embeddings are computed outside the transaction
with a 90-second timeout. Completion is stored only for the current version and
profile and by the owner of the attempt. The queue recovers after restart or lease
expiry. Temporary model failures receive up to three attempts with 5- and 30-second
pauses; after that, retry explicitly. Invalid embeddings and empty sources are not
retried automatically. There is no cloud fallback or automatic model download.
The server limits search queries to 2000 code points and 20 results.

Opt-in smoke test for the real local provider:

```bash
OLLAMA_SMOKE=1 \
OLLAMA_URL="$PROJECT_CREATOR_OLLAMA_URL" \
OLLAMA_MODEL="$PROJECT_CREATOR_EMBEDDING_MODEL" \
OLLAMA_DIGEST="$PROJECT_CREATOR_EMBEDDING_DIGEST" \
go test ./internal/rag -run TestRealOllama -v -count=1
```

For a full search smoke test, also set `TEST_DATABASE_URL` to a separate test
database with pgvector. Tests create an isolated schema and check the expected
source. This is a short wiring check; it does not replace recall evaluation on a
real set of duplicates and contradictions. Ordinary CI tests do not download the
real model.

## Possible collision analysis (MVP 7)

After completion, the API returns `reasonCode` and saved attempt diagnostics. The
“All pairs” mode does not require an index and does not return `INDEX_INCOMPLETE`.
The primary “Related pairs” mode requires current embeddings: for
`INDEX_INCOMPLETE`, check overall progress, wait for the queue or retry failed
indexing, then explicitly retry the analysis. Analysis does not start automatically.
The former `REQUIREMENT_LIMIT`/`ANALYSIS_LIMIT` limits no longer apply. Older
attempts retain their original reasons. `TOO_FEW_REQUIREMENTS` means there is
nothing to compare; `MODEL_INSUFFICIENT` means the model lacks enough conditions
to draw a conclusion. Technical errors are reflected separately in status and coverage.

Analysis starts only by clicking the button on a project card. Indexing is needed
for semantic search and selecting related pairs; it is not required for the
explicit “All pairs” mode. Below is Ollama generator setup for host processes; in
full Compose, set the same MODEL/DIGEST values in `.env` and recreate the backend.
For OpenVINO NPU, use the [separate guide](intel-npu.en.md).

```bash
ollama pull qwen2.5:7b
curl http://127.0.0.1:11434/api/tags
export PROJECT_CREATOR_GENERATOR_MODEL='qwen2.5:7b'
export PROJECT_CREATOR_GENERATOR_DIGEST='sha256:full-generator-digest-from-api-tags'
go run ./cmd/server
```

The generator uses the same `PROJECT_CREATOR_OLLAMA_URL` as embeddings, but has a
separate model and digest. Qwen2.5 7B is an example; weights are downloaded only
when explicitly requested by the user. The adapter accepts loopback HTTP or the
local Docker service name `ollama`, checks the installed model and digest before
and after each call, rejects remote models, and does not fall back to cloud. Normal
project and requirement operations work without the generator. Local model
capabilities and quality depend on the selected weights and hardware; passing
fake-based tests does not assess its accuracy on your requirements.

An analysis has status running, failed, or completed. The screen distinguishes a
possible collision, insufficient data, and no collision found among the checked
pairs. The last **does not prove that the project is consistent**: the model can
be wrong and its output needs human review. Exhaustively comparing pairs does not
guarantee finding every semantic contradiction. Confidence percentages are not
calculated. Model output never changes requirements.

Analysis starts explicitly from a button and quickly returns an attempt ID. Work
continues on the server after the tab is closed. In **“Related pairs”** mode,
a threshold closer to **0** admits more pairs, while a threshold closer to **1**
is stricter. Each indexed fragment of one requirement is compared with every
fragment of the other. The highest cosine similarity determines whether the
pair qualifies. For example, if the best match is between the final paragraphs
and scores `0.72`, the pair qualifies at `0.65` or `0.72` and falls below
a threshold of `0.80`. When the model checks a pair, it receives the full text
of both requirements.

Exact matches of normalized text qualify at any threshold. Choose **“All pairs”**
to check every combination of requirements regardless of similarity. Shared words
do not bypass the threshold. The search across current fragments is exact, without
an approximate index or a top-5/top-6 limit. No new structural links are created: the current data model
does not yet have links for requirements.

To change the initial threshold for “Related pairs”, set
`PROJECT_CREATOR_ANALYSIS_DEFAULT_THRESHOLD=0.8` in the repository-root `.env`.
The backend reads this setting at startup. An empty or missing value selects
`0.65`. An explicit value must be a finite number from 0 to 1 inclusive, or
the backend fails to start.

For an installation managed by the launcher, run `./project-creator up` to
apply the setting using the saved device configuration. To reuse installed
images with Compose, run `docker compose up -d --no-deps --no-build --wait backend`
with the same Compose project name and override files used at startup. For a
host backend, export the variable and restart the server. Reload the analysis
page to see the configured value for a new run. You can override it for an
individual run. Saved runs keep their original thresholds.

The threshold is a selection heuristic. A cosine score does not measure the
probability of relatedness, collision, or a correct verdict. Each run records
the threshold and rule version. Each pair records the reason for its selection
and, where applicable, its cosine score. A fixed RU/EN
test set checks more than five relevant neighbors and exclusion of clearly
unrelated requirements. These tests use predefined vectors: they check selection
rules but do not measure the quality of the real embedding model. The threshold
is not calibrated on client data. Embeddings may miss relations or select extra
pairs; unchecked pairs are not considered compatible. `related-v2` rules apply to
new runs; historical queues and results are not recalculated. Use analysis
replacement to change the rules of an active run.

A separate explicit “All pairs” mode creates every unique unordered pair for N
current requirements: N × (N − 1) / 2, or 4950 for 100 requirements and 19900 for
200. The limits of 20 pairs/100 requirements and top-6 do not apply. Embeddings and
embedding-provider calls are not needed in this mode. Invalid sources and technical
errors do not count as successful checks.

Exact duplicates are detected without an LLM after collapsing spaces, tabs, and
line breaks. Case, negation, numbers, and punctuation are preserved. The “Duplicate
requirements” group shows both original texts, an explanation, time, and provenance:
either an exact match by algorithm `exact_text_v1` or a model suggestion. The model
may return `duplicate` for identical obligations with the same conditions, scope,
and constraints. Partial overlap or clarification is not automatically considered
a duplicate. Nothing is merged or deleted.

For each model-assisted comparison, the generator receives both originals and
confirmed project definitions whose names or aliases match either text.
An ambiguous match is marked for clarification. Definitions are reference
data. The pair's **Technical information** shows the saved definitions and
their versions.

Preparation saves source snapshots and builds the queue in bounded batches. Until
preparation completes, the total number of pairs is unknown (`totalPairs=null`),
not zero. The interface distinguishes all possible project pairs, selected and
unselected pairs, new and reused results, and pending, running, failed, and
completed pairs. Unselected pairs are not considered checked. Rows in each expanded result group load in batches as you scroll.
The counters apply to the whole run. A completed result moves to its group immediately through SSE in
open tabs; reconnect/reset reloads saved data. “No contradictions found” applies
to checked pairs; there is no separate “requirements are unrelated” classification.

Migration 007 moves the queue and snapshots from growing JSONB arrays in
`analysis_runs` into separate `analysis_pairs` and `analysis_sources` rows. It
preserves IDs, versions, and source text, verdict, explanation, check time,
generator profile, and a link to the source check when reusing a result. Analysis
history is available through paginated GraphQL; older runs can be read by ID. The
“Cancel all-pairs analysis” button cancels the entire selected running attempt:
the current check, all pending pairs, and unfinished queue preparation. Completed
results are preserved. GraphQL mutation `cancelProjectAnalysis(projectId,
analysisId)` is idempotent; cancellation is stored as `status=failed`,
`errorCode=CANCELLED`, and the interface shows “Analysis cancelled.” Unfinished
pairs receive `failed/CANCELLED` and are not considered ready. A late model response
is not published. The adapter receives context cancellation; on another backend
instance this is detected at the next heartbeat (within 5 seconds). Releasing model
resources depends on the adapter; the next call waits for the current one to finish.
The NPU runtime stops the cancelled worker and restores it with the same pinned
profile before the next explicit analysis run; a cancelled request is not retried
automatically. You can explicitly retry unfinished pairs in the current run or
start a new analysis that reuses completed results.

On the **Analysis** page, you can retry failed pairs in the latest run until you schedule it for deletion. Historical results preserve the source text from that run. Older results are migrated without assigning unknown provenance.

Open the project’s **Technical section** to find **Analysis history**. You can view a saved run there and schedule a finished run for deletion. The displayed deadline is counted from when you schedule deletion. Scheduling the same run again keeps its original deadline. Once scheduled, you can permanently delete it immediately. Deletion removes that run and its saved pairs and source snapshots. The project, its requirements and other runs remain available. Unscheduled runs are kept regardless of age.

To preview and delete finished runs for one project in bulk, see [Clear analysis history for one project](setup-cli.en.md#clear-analysis-history-for-one-project).

Set `ANALYSIS_HISTORY_RETENTION_DAYS` in the repository-root `.env` to change the period for newly scheduled deletions. The default is 30 days. Use a whole number from 1 through 106751 inclusive, then recreate the backend with the same launcher or Compose configuration. For a host backend, export the variable and restart the server. Existing deadlines keep their saved dates. The backend checks for expired runs at startup and once an hour while running. If a check fails, cleanup retries on the next sweep.

Successful verdicts `collision`, `compatible`, `insufficient`, and `duplicate` are
reused between runs only within the same workspace/project, for the same
canonically ordered snapshots, applicable glossary context, and complete
immutable generator profile.
The profile includes adapter, model/digest or NPU manifest, prompt, response schema,
and generation parameters. Matching only a model name is insufficient. Errors and
timeouts are not completed results. Repeating an unchanged project with the same
profile does not call the generator. In full mode, changing one of N requirements
requires N−1 new comparisons; adding one requires N; exact duplicates also skip
inference. Deleted requirements are excluded from the new queue, but history stays.
A stale overall run does not discard reusable results for its unchanged pairs.
If a glossary edit changes the applicable context, the affected pairs are
marked stale. A result with outdated context from a model call already in
progress is rejected before publication. Start a new analysis to recheck the
affected pairs and reuse results whose inputs still match.
Changing embeddings, threshold, or selection rules rebuilds candidates but does
not by itself change whether comparisons of the same snapshots with the same
generator are reusable. All history/cache reads and model context are scoped to
authorized workspace/project; a response received after source changes or lease
loss is not published as a current successful result.

A project has one active attempt; a repeated request returns its status. A worker
processes pairs sequentially, including across projects; a PostgreSQL advisory lock
prevents competing calls from other instances. Queue preparation has a 180-second
limit per batch and each comparison has a 120-second limit; there is no short
overall cutoff for the whole analysis. Adapter responses are limited to 2 MiB.

Pair results, overall progress, and invalidation are saved after successful commit.
If the model returns an invalid comparison, the pair gets `INVALID_OUTPUT`
and the worker continues with the next pair. The run shows a failure if any
pair still needs a result. In the sequential queue without Laya, a timeout or
unavailable model stops the run and marks the remaining pairs `NOT_PROCESSED`.
The processed-pair counter includes successful and failed comparisons that
actually started. “Retry failed pairs”
retries only failed and unprocessed pairs from the latest current attempt, preserving
its ID and completed results. Changed sources or generator/selection profiles
require a new analysis attempt. There are no infinite automatic retries or cloud
fallback.

The queue survives backend restart. Claimed work has a 30-second lease with a
heartbeat every 5 seconds; after a crash, unfinished calls become errors and need
an explicit retry. On graceful shutdown this happens immediately. A queued model
job does not lose its lease while waiting. A late result from a previous worker
does not replace a retry result. Inference interrupted before commit may run again
on explicit retry; already saved successful pairs are not recomputed. Exactly-once
inference is not promised. The decision and limitations are documented in
[ADR-003](adr/003-progressive-analysis.md).

Documents in prompts are passed as untrusted JSON data, separate from system
instructions; the model has no tools. The response is constrained by a JSON
schema; IDs/versions of the exact sources provided, allowed verdicts, and explanation
length are validated. This reduces incorrect responses but does not make LLM output
fact.

Both Ollama and OpenVINO ask for pair explanations in Russian. The server accepts
Russian letters and ASCII Latin letters for technical terms, along with digits,
whitespace, punctuation, and symbols such as emoji. The check applies to the
entire explanation, including single-letter insertions and quotations. It places
no alphabet restriction on the source requirements.

If the explanation contains a disallowed character, the adapter makes one more
generation attempt with a stricter language instruction and the original pair.
On OpenVINO NPU, the retry asks for one short Russian sentence explaining
the decisive condition in the comparison. During generation its JSON schema
fixes the source aliases and versions, leaves all four verdicts available,
and bounds the explanation to 320 characters using Russian or ASCII Latin
letters, digits, spaces and common punctuation. The adapter checks that the
explanation contains a letter, ends in a period, question mark or exclamation
mark, and cites the expected source versions in the response fields. The same
NPU token budget applies to both attempts.
The rejected response is omitted from the prompt. The retry uses the time left
within the pair's 120-second limit and stops on cancellation. A second violation
fails the pair and follows the existing manual retry flow. Other errors do not
trigger an automatic retry. Rejected output is neither edited nor saved as a
successful finding. This check enforces an alphabet and cannot determine the
language or quality of the explanation.

The retry instruction and response schema are part of the generator profile.
After updating the NPU adapter to use the bounded retry, start a new analysis.
The new run recomputes model findings from the previous NPU profile. Earlier
runs remain available in history. This update changes only the OpenVINO NPU profile.

Ordinary CI checks retrieval, workflows, access, and known collision/compatible
requirements with deterministic providers. A real local generator check runs separately:

```bash
ANALYSIS_SMOKE=1 \
OLLAMA_URL="$PROJECT_CREATOR_OLLAMA_URL" \
OLLAMA_MODEL="$PROJECT_CREATOR_GENERATOR_MODEL" \
OLLAMA_DIGEST="$PROJECT_CREATOR_GENERATOR_DIGEST" \
go test ./internal/analysis -run TestRealAnalysisSmoke -v -count=1
```

It requires a model already downloaded and checks a small set of explicit examples,
not statistical quality or completeness. See the [development guide](development.md) for
unit/integration/GraphQL/E2E checks; integration tests require a separate test PostgreSQL
database.

### Pages and interface updates

| URL | Purpose |
| --- | --- |
| `/` | Project list |
| `/projects/new` | Create a project |
| `/projects/:id` | Project overview |
| `/projects/:id/requirements` | Search by ID, text and meaning. Create, edit and archive requirements |
| `/projects/:id/indexing` | Overall background queue progress, statuses, and reindexing |
| `/projects/:id/search` | Redirect to search on the requirements page |
| `/projects/:id/analysis` | Streaming pair analysis and results |

Project navigation and selected RU/EN language are shared across all pages. The
language is persisted between app visits. Links from search and analysis open the
requirements list, load any batches needed to reach the source, and focus its card. If
the source was deleted, its saved version text remains in the analysis result. In
analysis groups, pair explanations are visible immediately; source texts and pair
technical details are loaded separately when their sections are expanded.

During SSE invalidation and a repeated request, the last successful content remains
visible. An error displays a local message about stale data and a retry button.
Access denial clears data; an SSE reset after reconnect only refreshes it and keeps
the draft. Collapsed-group state is preserved in the current tab. Overall
indexing progress applies to the entire project, separately from counts of loaded
cards. `queued` means waiting for background processing, `running` means indexing,
`ready` means a current index is ready, and `failed` means an error. Legacy
`pending` is supported for compatibility. SSE reconnect/reset reloads saved queue
state. The rationale is described in [ADR-005](adr/005-index-queue.md).

Deterministic browser regressions without PostgreSQL or models:

```sh
cd frontend
bun run build
bunx playwright install chromium
bun run test:ux
```

Scenarios check desktop 1920×1080 and narrow 390×844 screens, slow requests,
SSE/reconnect/reset, errors and retry, access revocation, language and navigation,
links to sources in later batches, and collapsible groups with counters.

Long words, URLs, and identifiers wrap within the page, including project
descriptions and model provenance. Line breaks in goals and descriptions are
preserved; provenance JSON is not truncated. Selection lists fit within their
container, and full provenance for the selected run is available when expanded.
Groups and cards use existing shared styles and the `Group` component.

`responsive content` and `responsive analysis` regressions check for horizontal
overflow of the page, text containers, and form elements at widths of 320/390/768/
960/1920 px in RU/EN. They cover long summaries, expanded originals, search,
explanations and analysis profiles, large counters, and keyboard expansion. CSS
scaling at 200% is also checked; 960 px checks desktop reflow at 1920 px with 200%
browser zoom. This is automated emulation, not a manual check of the browser zoom
menu. Existing UX and analysis scenarios also check overflow in their final state,
including errors, loading, and changing analysis parameters. Overview screenshots
are saved in the Playwright results directory; for local diagnostics, set
`--output ../logs/issue-44/ux-results`.

### Viewing long lists

Projects, requirements, and indexing records load in batches of 40. Scroll to
the end of the loaded records or press **Load more** to append the next batch.
A failed request offers a **Retry** button and keeps the loaded cards visible.
Older links containing `listMode` or `page` open the scrolling list.

Project and requirement cards use height-based virtualization. Returning to
either list through client-side navigation restores the loaded batches and
scroll position. Reloading the tab starts a new load.

On the indexing page, statuses load by the requirement IDs in each batch. Group
counts cover the loaded requirements. The list heading shows both the loaded
count and the project total.

Scroll within the analysis run selector to load older runs in batches of 20.
Each expanded result group loads pairs with its own cursor. Opening a group
starts its first request. Counts describe the whole run. SSE updates move
completed pairs into the corresponding result groups. The cursor contracts
are described in [ADR-004](adr/004-cursor-lists.md).

### Efficient live analysis updates (#47)

Progress arrives in compact SSE snapshots with a run revision, without repeatedly
requesting the full summary and project card for every pair. Groups are closed by
default; expanding one loads a compact page, and expanding a card loads only its
saved texts and explanation. Immutable model provenance is requested separately
when expanded and reused by run ID. Background synchronization preserves displayed
cards, forms, and expanded groups. Reset/reconnect restores state; cancellation
preserves completed results.

Contract and limitations: [ADR-006](adr/006-analysis-live-snapshots.md).
Before/after measurements, method, and commands: [20 pairs](analysis-live-performance.md).
Scenario: `scripts/analysis-live-benchmark.mjs`.
Ordinary tests do not call real models.

### Replacing parameters of a running analysis

Changing form fields does not stop an analysis. The “Replace analysis parameters”
button explicitly applies the selected mode and threshold using one of two options:

- “Wait for the current pair”: keep the reusable result of the current comparison,
  stop issuing the remaining old pairs, and start a new queue. Completed comparisons
  of unchanged snapshots with the same generator are reused regardless of threshold.
- “Cancel the current model call now”: finish the old run with `REPLACED`, cancel
  the inference context, and start a new queue after the generator is released. A
  late response is not saved. For another backend instance, cancellation is detected
  by heartbeat within 5 seconds; NPU checks client disconnection every 100 ms, then
  waits up to 5 seconds for terminate and up to 5 seconds for kill. Loading the
  pinned model on recovery has its own 600-second limit; each pair is limited to
  120 seconds.

GraphQL: `replaceProjectAnalysis(projectId, analysisId, mode, threshold, strategy)`;
`strategy` is `wait` or `cancel`. The request is tied to the expected run and saved
in PostgreSQL. Repeating an identical operation returns the same run or its successor.
Before the successor is created, a new choice replaces the pending request; after
creation, change parameters through the new `analysisId`. A stale request with
different parameters is rejected. The waiting state is `running/waiting_current_pair`.
SSE/reset and page reload read saved state; closing the tab does not cancel replacement.
The cancel-all button also removes the pending replacement. A replaced run cannot
be resumed through retry.

After intentional cancellation, the NPU worker is restored before the next call,
with its pinned weights and profile checked again. Recovery failure does not trigger
automatic retries. `NPU_RECOVERY_FAILED`, `NPU_WORKER_EXIT`, `NPU_UNAVAILABLE`,
`TIMEOUT`, `PROFILE_CHANGED`, and `INVALID_OUTPUT` are distinct from user cancellation.
Timeout, device failure, or changed files require operator recovery following the
[NPU guide](intel-npu.en.md); there is no automatic cloud fallback. Technical
events without source requirements or responses are saved in `logs/npu/`.

Deterministic replacement acceptance (a separate test database is required):

```bash
go test ./internal/analysis -run 'TestReplacement|TestCancelWaiting' -count=1
go test ./internal/api -run TestGraphQLReplacement -count=1
python3 -m unittest discover -s runtime/npu -v
cd frontend
E2E_CHROMIUM_PATH=/path/to/chrome bun run test:ux -- --grep 'replacing an analysis'
```

These checks use stubs and do not prove NPU hardware readiness. Hardware testing
of cancellation and regeneration runs separately and explicitly.

Hardware checking is performed on a **free** runtime started from this code
version. It intentionally interrupts one inference and expects a new comparison
with the same pinned profile:

```bash
NPU_REAL_CANCEL_SMOKE=1 \
PROJECT_CREATOR_NPU_SOCKET=/absolute/path/runtime.sock \
PROJECT_CREATOR_NPU_PROFILE=<manifest-sha256> \
go test ./internal/analysis -run '^TestOpenVINORealNPUCancelThenGenerate$' -v -count=1
```

Before cancellation, the test waits for the worker request (`generation_started`);
afterward, it waits for `worker_stopped` for its `call_id` in
`logs/npu/runtime-lifecycle.jsonl`. This distinguishes stopping the native worker
from merely closing the HTTP request. The generation-start event itself does not
prove that computation ran on the device. The new result is checked against the
schema and sources; verdict quality is evaluated with separate hardware examples.
`backend-lifecycle.jsonl` links `callId` to run and pair IDs; the `X-NPU-Call-ID`
header links it to the runtime log. Neither log contains requirement text, prompts,
or responses.

### Automatic requirement summaries (#72 included in #48)

Summary languages are configured in `.env` with
`PROJECT_CREATOR_SUMMARY_LANGUAGES=ru,en` (also the default when unset). Supported
codes are `ru`, `en`, `de`, `fr`, `es`, `uk`, `zh`, and `ja`. For example, `ru,en,de`
adds German. Empty elements and unknown codes are configuration errors. Apply the
updated configuration with `./project-creator up`, rebuilding the backend if code
changed. The language list is part of the profile: old summaries remain in history,
and new ones are generated automatically for a new profile.

The model keeps the original language if it is allowed; otherwise, it translates
the summary into the first language of the canonical list (codes are sorted and
duplicates removed). The server checks the alphabet and permits digits, spaces,
punctuation, symbols, and emoji. Other letters are not stripped: the model gets
one correction attempt; if that also fails, the summary is not published and the
final LLM may check the originals. Combined letters represented by separate
combining diacritics are rejected; the model must return the ordinary single
letter from an allowed alphabet. Alphabet checks cannot distinguish languages
that share letters, so the model instructions also specify the language constraint.
The new rule does not restrict original requirements.

For each requirement, the server narrows the configured alphabets to those that
cover its source text, allowing isolated ASCII identifiers. With `ru,en`, an
English translation of Russian prose therefore triggers one correction attempt.
If several alphabets match, they remain eligible. When none covers the source
(e.g. mixed-language prose), the configured list applies.

The output may use isolated ASCII technical names from the source, identifiers used in calls, typed parameters, annotations, assignments or array indexing, and verbatim labels such as `SSO (Single Sign-On)`. The surrounding prose must use the configured alphabet. A foreign passage elsewhere in the source does not invalidate an isolated name. Invented names and foreign prose still fail validation.

After a language rejection the worker makes one fresh attempt using the original text and its confirmed glossary hints. When the source is already written in Russian and the selected language is Russian, the worker also uses this repair after a missing number. The Russian correction prompt lists the source numbers and numbered identifiers explicitly. The repair request omits glossary version numbers so the model can focus on numbers in the requirement. When the request budget permits, the corrective JSON schema constrains the spelling to the configured alphabet, source symbols and recognized identifiers. The correction requires wording in the configured alphabet even when the source only lists identifiers or numbers. The worker still validates the result before publication. New results record the validation and correction policy in their provenance. Existing ready previews keep their base model profile and generation history.

For an OpenVINO NPU model managed by Models Configurator, the corrective language schema explicitly limits the JSON object to one field. The server requires that field to be `summary` and checks it before saving the preview. If the Russian language correction drops a source number, the worker makes one additional attempt when the request fits the budget. The additional attempt requires the source numbers to occur in source order with their original signs, decimal punctuation, dates, units and numbered identifiers. The worker checks this constraint and the other output rules before publication.

A failed preview shows the reason in the interface. `INVALID_FORMAT` identifies a malformed response, `INVALID_TEXT` an empty or unpublishable string, `INVALID_LANGUAGE` an alphabet mismatch, `MISSING_SOURCE_NUMBERS` an omitted numeric literal or numbered identifier, and `CORRECTION_TOO_LONG` a correction that exceeds the request budget. Historical `INVALID_OUTPUT` records remain retryable.

If any summaries failed, the Requirements page shows **Retry all failed summaries** with a count for the whole project, including interrupted generations (`INTERRUPTED`). Click it to queue those summaries again for the current requirement versions and active profile. To retry just one summary, use the action on its requirement card.

The details of the selected requirement always show its full original text. Open **Short summary** below the original to see the preview.

With `PROJECT_CREATOR_SUMMARY_MODEL` and a full `PROJECT_CREATOR_SUMMARY_DIGEST`
explicitly set, requirements automatically enter a local background queue after
saving. `PROJECT_CREATOR_SUMMARY_URL` selects a separate Ollama runtime. CRUD and
search do not wait for generation; summaries are tied to the source version and
profile. The generator removes repetition and preserves obligations and identifiers without a byte target. Longer valid previews
remain available alongside the full original. Pair analysis compares full sources
independently of summary availability or length. New runs use the direct generator
profile for cache reuse. Previous Laya runs retain their stage details and model
provenance. Start a new run to repeat an unfinished comparison from that profile.

The former pair classifier could skip long previews and still required final
comparison for every candidate. Issue #204 retires this stage. Legacy
`PROJECT_CREATOR_LAYA_*` settings are ignored. The single-record classifier
in #51 is a separate decision informed by benchmark #50.

Hardware results and limitations of the real model are in the
[verification report](requirement-summary-verification.md).
