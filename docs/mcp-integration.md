# MCP integration proposal

[Русская версия](mcp-integration.ru.md)

This document records a **proposal**, not implemented behavior or an approved
commitment. The existing application remains the source of truth for projects,
requirements, source documents, and their versions. An external assistant could
connect through the Model Context Protocol (MCP) as an optional adapter to the
existing business logic. Tracking: [Issue #146](https://github.com/VolkovDani/project-creator/issues/146).

## Proposed scope

An MCP adapter would call application domain services rather than access storage
directly, duplicate business rules, or create a second domain model. Some
capabilities in this proposal, especially saving provenance-rich assistant
proposals, do not exist today and would need a separately designed domain
service and application API. The first useful direction is deliberately narrow:

- Read requirements and source material only within the caller's authorized
  project/client scope.
- Return source identifiers, versions, and freshness information with the
  context, so the external assistant can analyze it itself.
- Save an assistant's **proposal** with its cited source versions, origin,
  limitations, and status. Saving a proposal does not make it a confirmed fact.
- Make changes to source requirements or their approval status unavailable
  through this proposed integration. A person continues to review and apply
  changes through the application.
- Keep changed or deleted sources visible as stale or unavailable in the saved
  proposal; do not silently present an old analysis as current.

Authorization must be checked before retrieving or passing any context to an
assistant, and checked again before writing a proposal. The adapter must not
allow cross-client access. Sending context to a cloud model requires explicit
configuration and user consent. A local model failure must not trigger an
automatic cloud fallback.

## Not decided

The MCP SDK, transport (for example, stdio or HTTP), authentication mechanism,
hosting arrangement, permission mapping, and exact read/write tool surface are
open decisions. They require a separate implementation issue and design review.
This proposal does not select an SDK, add a dependency, promise a release date,
or authorize implementation. See [Issue #146](https://github.com/VolkovDani/project-creator/issues/146)
for discussion.

## Current AI setup

The repository already provides a local-model setup flow through the
`project-creator` CLI. Follow the [AI installation guide](ai-installation.md) for
hardware discovery, setup, verification, diagnostics, and data-preservation
guidance. The current CLI manages the application's local runtime; it does not
implement MCP.
