<img src="https://raw.githubusercontent.com/VolkovDani/project-creator/9d04799dc35617490cdd4e4ddc8642d9fa8e8433/frontend/public/icon-192.png" width="80" height="80" alt="">

# Project Creator

[Русская версия](README.md)

Project Creator helps you find conflicts in a project's business requirements.
You can create and edit projects and requirements without a model. Connect an
embedding model to search by meaning, or a generation model to compare requirements.
Project Creator shows each potential conflict alongside the source versions so you
can decide whether to make a change.

The application runs locally for one user and needs no cloud API keys.

## Quick start

Use native x86_64 Linux, Python 3 and Docker Engine with Compose 2.20 or later.

```bash
git clone --branch develop https://github.com/VolkovDani/project-creator.git
cd project-creator
./project-creator setup
```

Setup starts PostgreSQL, the backend and the web interface. Open the URL it prints (by default [127.0.0.1:3000](http://127.0.0.1:3000)) to create projects and requirements.

Use **Dark theme** beside the language selector in the header to switch themes. The browser remembers your choice for future visits.

## Connect models

[Models Configurator](https://github.com/VolkovDani/models-configurator/tree/develop) is a separate application for connecting local and external models. Its read-only panel shows the configured profiles, provider availability and which local models are loaded. Multiple applications on the same host can share one manager.

Start the manager, copy the profile ID for each role you need and set it in Project Creator’s private `.env`. Generation and embeddings can use different providers. Once configured, run `./project-creator setup --device remote --switch-device`. The same mode supports a direct OpenAI-compatible endpoint. Existing CPU, GPU and NPU installations can continue using their saved setup during migration. [Connection example](docs/adr/012-standalone-model-manager.md) · [Installation and provider options](docs/setup-cli.en.md)

## Try a first comparison

1. Create a project and add two requirements about the same rule. For example, “Keep log entries for 30 days” and “Delete log entries after 7 days.”
2. Open the analysis page, select “All pairs”, and run the analysis.
3. Open the result for the pair and compare the explanation with the source texts.

To stop the application and keep your data and models, run `./project-creator down`.
Start it again with `./project-creator up`.

## Documentation

- [Installation and troubleshooting](docs/setup-cli.en.md) · [Install with an AI assistant](docs/ai-installation.md)
- [Manual Docker Compose setup, updates, and backups](docs/compose.md)
- [Requirement search, indexing, and analysis](docs/requirement-analysis.md)
- [External assistant through MCP](docs/mcp-integration.md)
- [Development, tests, and CI](docs/development.md) · [Architecture (Russian)](ARCHITECTURE.md)
- [Features and limitations in v0.1.0](docs/release-notes-v0.1.0.md) · [Project goals](GOAL.en.md)

[Semantic types, representations, manual confirmation and optional background review with local Laya](docs/requirement-classification.md).
