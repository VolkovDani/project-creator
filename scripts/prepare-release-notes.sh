#!/usr/bin/env bash
set -euo pipefail

wiki_url="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY"
temp_dir=$(mktemp -d "$RUNNER_TEMP/release-wiki.XXXXXX")
trap 'rm -rf -- "$temp_dir"' EXIT
wiki_git="$temp_dir/wiki.git"

git init --bare --quiet "$wiki_git"
git --git-dir="$wiki_git" fetch --quiet --no-tags --depth=1 -- "$wiki_url.wiki.git" "$NOTES_SHA"
test "$(git --git-dir="$wiki_git" rev-parse 'FETCH_HEAD^{commit}')" = "$NOTES_SHA"

version_core="${RELEASE_TAG%%-*}"
page_version="${version_core//./-}"
if [[ "$RELEASE_TAG" == "$version_core" ]]; then
  english_page="Release-Notes-$page_version"
  russian_page="$english_page-RU"
else
  page_version+="${RELEASE_TAG#"$version_core"}"
  english_page="Release-Notes-EN-$page_version"
  russian_page="Release-Notes-RU-$page_version"
fi

for title in "$english_page" "$russian_page"; do
  path="releases/notes/$title.md"
  if ! git --git-dir="$wiki_git" cat-file blob "$NOTES_SHA:$path" > "$temp_dir/$title.md" ||
     ! grep -q '[^[:space:]]' "$temp_dir/$title.md"; then
    printf 'Missing or empty release notes at Wiki commit %s: %s\n' "$NOTES_SHA" "$path" >&2
    exit 1
  fi
done

{
  cat "$temp_dir/$english_page.md"
  printf '\n\n'
  cat "$temp_dir/$russian_page.md"
  printf '\n\n'
  cat <<END_NOTES
## Candidate under review

- Tag: $RELEASE_TAG
- Source commit: $SOURCE_SHA
- Wiki notes commit: $NOTES_SHA
- Hardware confirmations are tracked per configuration after publication.
- [English release notes at the pinned revision]($wiki_url/wiki/$english_page/$NOTES_SHA)
- [Русские заметки закреплённой версии]($wiki_url/wiki/$russian_page/$NOTES_SHA)
END_NOTES
} > "$temp_dir/release-notes-draft.md"
mv -- "$temp_dir/release-notes-draft.md" "$RUNNER_TEMP/release-notes-draft.md"
