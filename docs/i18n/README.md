# Documentation languages

[Русская версия](README.ru.md)

All new documents and documentation updates require English and Russian versions
with matching requirements, commands, limitations, and verification status.
Review both versions in the same PR. Link each version to its counterpart.

New documents use `<name>.md` for English and `<name>.ru.md` for Russian.
Existing Russian canonical files keep their paths and use `<name>.en.md` for
English. Keep tool-discovered names such as `AGENTS.md` unchanged. Existing
untranslated documents are legacy gaps, not evidence of bilingual coverage.
Translate them when updating them; preserve dates and SHA in historical reports.

## Adding another language

1. Choose a language tag such as `de` or `pt-BR` and create
   `docs/i18n/<language-tag>/`.
2. Mirror each original repository-relative path under that directory. For
   example, a German translation of `docs/release-acceptance.md` belongs at
   `docs/i18n/de/docs/release-acceptance.md`.
3. Add a language index with links to translations, originals, and the English
   and Russian language instructions. Document any missing translations.
4. Translate prose, not commands, environment variables, identifiers, API keys,
   paths, or model names. Never include real secrets or client data.
5. Check relative links from the translated file's actual location. Review
   command equivalence, safety warnings, acceptance criteria, and release status
   against both baseline languages. Update all existing language variants when
   behavior changes; do not silently leave stale instructions.

Translation alone does not require artificial tests; run `git diff --check` and
validate referenced paths. Changes to behavior still require the normal checks.
