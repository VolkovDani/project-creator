# Connect an assistant through MCP

[Русская версия](mcp-integration.ru.md)

An MCP client can read active requirements from one Project Creator project,
analyze them with its selected model, and save a proposal for human review.
Open **Analysis → External proposals** to see the analysis, limitations, and
source versions. Apply any resulting edits yourself through **Requirements**.
[Issue #146](https://github.com/VolkovDani/project-creator/issues/146)

## Connect a client

Use an updated [local installation](ai-installation.md) with the backend and
web interface running. Open the project in your browser and copy the ID from
`/projects/<id>`. Use one client configuration entry per project you want to share.

Choose `--sharing=local` for a client configured to use only a local model.
Use `--sharing=cloud` only if you agree to send that project's contents to the
client's external provider. Each mode grants the selected client access to the
active requirements and saved proposals in that project. The setting records
your choice. Check the client's provider, data retention and fallback settings
before connecting. Project Creator makes no inference calls through this adapter.

These examples use Docker Compose. Use the same project directory, Compose
project name and file overrides as your running installation. Supply any
required global Compose options before `exec`. The subprocess inherits the
backend's workspace token inside the container.

### Local model with OpenCode

On OpenCode 1.19.6, add this entry to the CLI's `opencode.json`. Set up a
local provider such as Ollama in OpenCode and select its model.

```json
{
  "mcp": {
    "project_creator": {
      "type": "local",
      "command": ["docker", "compose", "--project-directory", "/absolute/path/to/project-creator",
        "exec", "-T", "backend", "/app/server", "mcp", "--project=PROJECT_ID", "--sharing=local"],
      "enabled": true
    }
  }
}
```

Run `opencode mcp list` to check the connection. Use the server's tools
with the selected local model.
[OpenCode 1.19.6](https://github.com/anomalyco/opencode/releases/tag/v1.19.6) · [Configure Ollama](https://opencode.ai/docs/providers/#ollama)

### Codex CLI with a ChatGPT plan

Use a ChatGPT account with Codex CLI access. Add the server to Codex's
`config.toml` and restart the CLI.

```toml
[mcp_servers.project_creator]
command = "docker"
args = [
  "compose", "--project-directory", "/absolute/path/to/project-creator",
  "exec", "-T", "backend", "/app/server", "mcp",
  "--project=PROJECT_ID", "--sharing=cloud"
]
```

To test, ask the client to list requirements in a project containing sample
data. After reviewing the result, ask it to save a proposal citing the IDs and
versions it read. Check the proposal in Project Creator.
[Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli) · [Codex plan access](https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan)

### Backend running on the host

When the backend runs directly on the host, replace `docker compose … exec -T
backend /app/server` with the absolute path to a binary built from `./cmd/server`.
Pass the same workspace token in the subprocess's private
`PROJECT_CREATOR_TOKEN` environment and use the backend port from
`PROJECT_CREATOR_ADDR` if it differs from 8080. Keep the token out of CLI
arguments, shared configurations, and logs.

## Tools and review

- `list_requirements` returns active requirement IDs, current versions and
  text. Use the returned end cursor to continue a page.
- `list_similar_requirement_pairs` reads distinct pairs of active requirements
  whose measured cosine similarity meets the threshold. Each match returns both
  original texts, IDs, versions and the score. The page identifies the
  embedding profile used.
- `save_external_proposal` stores analysis, limitations, client and model
  provenance, sharing mode, and between 1 and 20 distinct requirement versions.
  The model identifier is supplied by the client. Client identity is
  self-reported through MCP. If a cited requirement changes or becomes
  unavailable before saving, refresh it and analyze the current text again.
  Retry the same submission with the same request ID and content after a
  connection failure.
- `list_external_proposals` returns saved drafts and source freshness, with
  cursor pagination. A source is **current** while its active version matches,
  **changed** when a newer active version exists, and **unavailable** after
  archiving or deletion. The cited ID and version remain visible.

The proposal starts in **Pending review**. Verify the reasoning and source
versions before making an edit. The adapter exposes current requirement text,
pairs selected by cosine similarity, and proposals. The current scope excludes
historical text retrieval, other source types, requirement mutations, and
remote MCP hosting.

## Compare requirement pairs in chat

For example, ask the connected assistant to use
`list_similar_requirement_pairs` with `threshold: 0.8`, compare each returned
pair for overlap or contradictions, and cite both source IDs and versions.
The score is the maximum cosine similarity across the two requirements'
indexed chunks for their current versions. Even identical texts must meet
the requested score. The assistant decides what the similarities and
differences mean. It can save its conclusions with `save_external_proposal`
when you ask.

Use the same threshold and the returned `pageInfo.endCursor` while
`hasNextPage` is true. The tool scans bounded groups of pairs, so an empty
page can still have a continuation. If the requirements, profile or indexes
change, restart from the first page after `cursor_stale`. If the index is
incomplete, wait for the active requirements to finish indexing and try
again. The tool reads existing embeddings and returns
`embeddings_not_configured` when no profile is active.

## Design and verification

Local stdio uses the official Go SDK and works without a public endpoint or
separate backend service. The subprocess calls fixed GraphQL operations on
IPv4 loopback with the existing workspace token. Its launch arguments bind it
to one project. The backend checks each read and write against that workspace
and project. Only the machine owner should configure the subprocess or grant
Docker access.

PostgreSQL stores proposals separately from requirements. It verifies cited
active versions in the save transaction, commits the invalidation with the
draft, and scopes request IDs to the project so retries keep one result.
The review page recalculates freshness using current sources. A draft can
contain quotes from its sources and stays available if a source is removed.

Deterministic integration tests cover the MCP tools, GraphQL, PostgreSQL and
browser review. Pair selection coverage includes inclusive thresholds, scoped
access, and continuation across empty pages. A disposable Compose stack
exercised the original three tools through `docker compose exec -T`. Codex CLI
0.172.0 and OpenCode 1.19.6 discovered and called those tools over stdio using
the Project Creator binary and synthetic GraphQL data. Codex used direct tool calls. OpenCode used a loopback model
stub. The runs left actual subscription inference and Ollama model behavior
unverified.
