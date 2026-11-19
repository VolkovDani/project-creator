# Project Creator

[Русская версия](README.md)

Project Creator helps you find conflicts in a project's business requirements.
You record requirements, search them by meaning, and run pairwise comparisons.
A local model explains potential conflicts and shows the versions of both
requirements it used. You decide whether to make a change.

The application runs locally for one user and needs no cloud API keys.

## Quick start

The CPU setup requires native x86_64 Linux, Python 3, and Docker Engine with
Compose 2.20 or later. Allow 24 GiB of free disk space for the first install.
For GPUs and Intel NPUs, see [device selection and system requirements](docs/setup-cli.en.md).

```bash
git clone https://github.com/VolkovDani/project-creator.git
cd project-creator
./project-creator setup --device cpu
```

Setup downloads the models, starts the application, and checks indexing and analysis
with sample data. Open the URL it prints (by default [127.0.0.1:3000](http://127.0.0.1:3000)).

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

[Semantic types, representations and manual classification history](docs/requirement-classification.md).
