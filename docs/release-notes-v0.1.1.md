# Source + Compose v0.1.1

[Русская версия](https://github.com/VolkovDani/project-creator/blob/v0.1.1/docs/release-notes-v0.1.1.ru.md)

This patch updates the local, single-user MVP installed from source with Docker
Compose. The installation and hardware configuration guides are linked from the
[README](https://github.com/VolkovDani/project-creator/blob/v0.1.1/README.en.md).

## Changes

- GraphQL TypeScript generation reads local schema and operation files. The
  frontend retains its generated types and checks, and removes the vulnerable
  CLI dependency chain from the lockfile. [#151](https://github.com/VolkovDani/project-creator/issues/151)
- The Compose installation smoke test waits for indexing of the required
  requirement version when indexing is already in progress. Waiting has a
  deadline. [#152](https://github.com/VolkovDani/project-creator/issues/152)
- Requirements open for editing in an accessible dialog that preserves the
  separate creation draft and shows save errors and version conflicts in place.
  A keyboard-accessible Back to top button respects reduced-motion preferences.
  [#145](https://github.com/VolkovDani/project-creator/issues/145)
- Ollama and OpenVINO pair analysis validate the explanation alphabet. Russian
  and ASCII Latin letters are allowed. A violation triggers one extra generation
  attempt within the remaining pair deadline. A second violation fails the pair.
  [#99](https://github.com/VolkovDani/project-creator/issues/99)
- The patch also includes Node.js 26 and dependency updates, an AI-assisted
  installation guide, and a shorter README that links to setup guides.
  [#140](https://github.com/VolkovDani/project-creator/pull/140),
  [#147](https://github.com/VolkovDani/project-creator/pull/147),
  [#150](https://github.com/VolkovDani/project-creator/pull/150)

## Known limitations

- Alphabet validation does not establish that an explanation is in Russian
  or that its verdict is correct. Review model findings against the original
  requirements. [Pair analysis guide](https://github.com/VolkovDani/project-creator/blob/v0.1.1/docs/requirement-analysis.md#possible-collision-analysis-mvp-7)
- Preserving the source language in generated requirement summaries remains open.
  [#108](https://github.com/VolkovDani/project-creator/issues/108)
- Hardware coverage remains limited to individually tested configurations.
  GPU and NPU use requires compatible hardware, drivers, and a local runtime.
  [Hardware verification tracking](https://github.com/VolkovDani/project-creator/issues/22)
