type RevisionSegment = { kind: 'equal' | 'removed' | 'added'; text: string };
type Match = { oldStart: number; newStart: number; length: number };
type MatchPath = Match & { parent: MatchPath | undefined };
type Path = { oldEnd: number; matches?: MatchPath };

const words = new Intl.Segmenter('en', { granularity: 'word' });
// Bound the Myers search and the number of spans rendered for a revision.
const maxEdits = 64;

function sharedRuns(before: string[], after: string[]): Match[] {
  if (!before.length || !after.length || Math.abs(before.length - after.length) > maxEdits) {
    return [];
  }

  // Each diagonal keeps the path that reached furthest using the given edit count.
  // The caller has already consumed the common prefix.
  let paths = new Map<number, Path>([[0, { oldEnd: 0 }]]);
  for (let edits = 1; edits <= maxEdits; edits++) {
    const next = new Map<number, Path>();
    for (let diagonal = -edits; diagonal <= edits; diagonal += 2) {
      const removal = paths.get(diagonal - 1);
      const addition = paths.get(diagonal + 1);
      const remove = removal && (!addition || removal.oldEnd >= addition.oldEnd);
      const path = remove ? removal : addition;
      if (!path) continue;
      const oldStart = path.oldEnd + (remove ? 1 : 0);
      const newStart = oldStart - diagonal;
      if (oldStart > before.length || newStart > after.length) continue;
      let length = 0;
      while (
        oldStart + length < before.length &&
        before[oldStart + length] === after[newStart + length]
      ) {
        length++;
      }
      const matches = length ? { oldStart, newStart, length, parent: path.matches } : path.matches;
      const oldEnd = oldStart + length;
      if (oldEnd === before.length && newStart + length === after.length) {
        const runs: Match[] = [];
        for (let match = matches; match; match = match.parent) runs.push(match);
        return runs.reverse();
      }
      next.set(diagonal, { oldEnd, matches });
    }
    paths = next;
  }
  return [];
}

export function revisionDiff(previous: string, current: string): RevisionSegment[] {
  if (previous === current) return previous ? [{ kind: 'equal', text: previous }] : [];
  const before = Array.from(words.segment(previous), ({ segment }) => segment);
  const after = Array.from(words.segment(current), ({ segment }) => segment);
  const shorter = Math.min(before.length, after.length);
  let prefix = 0;
  while (prefix < shorter && before[prefix] === after[prefix]) prefix++;
  let suffix = 0;
  while (suffix < shorter - prefix && before.at(-1 - suffix) === after.at(-1 - suffix)) suffix++;
  const oldMiddle = before.slice(prefix, before.length - suffix);
  const newMiddle = after.slice(prefix, after.length - suffix);

  const segments: RevisionSegment[] = [];
  function append(kind: RevisionSegment['kind'], text: string) {
    if (text) segments.push({ kind, text });
  }
  append('equal', before.slice(0, prefix).join(''));
  let oldIndex = 0;
  let newIndex = 0;
  for (const match of sharedRuns(oldMiddle, newMiddle)) {
    append('removed', oldMiddle.slice(oldIndex, match.oldStart).join(''));
    append('added', newMiddle.slice(newIndex, match.newStart).join(''));
    oldIndex = match.oldStart + match.length;
    newIndex = match.newStart + match.length;
    append('equal', oldMiddle.slice(match.oldStart, oldIndex).join(''));
  }
  // An exhausted search returns no interior matches and preserves both complete middles.
  append('removed', oldMiddle.slice(oldIndex).join(''));
  append('added', newMiddle.slice(newIndex).join(''));
  append('equal', before.slice(before.length - suffix).join(''));
  return segments;
}
