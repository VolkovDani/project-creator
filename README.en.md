<img src="https://raw.githubusercontent.com/VolkovDani/project-creator/9d04799dc35617490cdd4e4ddc8642d9fa8e8433/frontend/public/icon-192.png" width="80" height="80" alt="">

# Project Creator

[Русская версия](README.md)

Project Creator helps you find conflicts in a project's business requirements.
You record requirements, search them by meaning, and run pairwise comparisons.
A local model explains potential conflicts and shows the versions of both
requirements it used. You decide whether to make a change.

The application runs locally for one user and needs no cloud API keys.

## Quick start

Use native x86_64 Linux, Python 3 and Docker Engine with Compose 2.20 or later.

```bash
git clone https://github.com/VolkovDani/project-creator.git
cd project-creator
./project-creator setup
```

Setup starts PostgreSQL, the backend and the web interface. Open the URL it prints (by default [127.0.0.1:3000](http://127.0.0.1:3000)) to create projects and requirements.

For model-assisted comparisons, rerun `./project-creator setup --device cpu --switch-device` (allow 24 GiB free disk for the first CPU installation), select a GPU/NPU, or configure a compatible API for generation, embeddings, or both. [Installation and provider options](docs/setup-cli.en.md)

Use **Dark theme** beside the language selector in the header to switch themes. The browser remembers your choice for future visits.

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
- [Development, tests, and CI](docs/development.md) · [Architecture (Russian)](ARCHITECTURE.md)
- [Features and limitations in v0.1.0](docs/release-notes-v0.1.0.md) · [Project goals](GOAL.en.md)

[Semantic types, representations, manual confirmation and optional background review with local Laya](docs/requirement-classification.md).
