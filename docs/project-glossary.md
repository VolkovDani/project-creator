# Project glossary

[Русская версия](project-glossary.ru.md)

Open **Project glossary** to create, search, change or delete confirmed concepts for the current project. A concept has a canonical name, definition, usage context and variants (abbreviations, synonyms and alternative spellings). Select text in an expanded original requirement to start a new entry or attach the selection to an existing concept. The editor records the source requirement and its version. Edits use the concept version to detect conflicts.

The glossary scans requirements for a full name followed by its capital-letter abbreviation in parentheses. The final words must spell the initials in order. Select a candidate to review its text and explicitly save it. Browse terms and suggestions independently in batches of 20 matches, with infinite scrolling or numbered pages. A suggestion alone never changes the dictionary.

In an original requirement, matching uses Unicode letters, combining marks, digits and underscores as word characters and prefers the longest whole spelling. Hover over a highlighted spelling or focus it to see its definitions beside the text. Click or tap to pin the tooltip and access editing for a particular meaning. Press Escape to dismiss it. Copying the original preserves its text.

The local summary worker receives matching confirmed definitions with term IDs and revisions, bounded to approximately 3 KiB of JSON. Ambiguous names carry an ambiguity marker. Glossary changes invalidate affected current-version summaries and fence in-flight results. A configured worker regenerates them asynchronously. Cancellation stays effective. The canonical requirement, its version and the embedding index are preserved.

Choose **View mentions** on a term to see current project requirements that
match its name or variants using the same rules as source highlighting. Select
a requirement to open its card. Each matching requirement appears once.

Requirements, search, indexing and pair analysis open the glossary dialog from
a text selection. Use **New concept** on the project glossary page for manual entry. Source selections retain the displayed requirement
version. Selections from generated previews and explanations prefill only the name.

Inside an analysis pair, highlighted names and variants refer to the current project glossary. Matches use the original text and version saved with the run, even after a requirement is edited.

Ready previews highlight matches in the summary text using the current project glossary. Highlighting works with previously saved previews and requires no generation.

Highlighted texts receive match ranges and term IDs. When a text is displayed, the browser fetches missing definitions in batches and reuses them across texts in the same project. Opening a collapsed original starts its lookup. Glossary changes refresh displayed matches and definitions.
