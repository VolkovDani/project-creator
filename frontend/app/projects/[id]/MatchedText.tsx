'use client';
import { type ReactNode, useCallback } from 'react';
import { clientCache } from '../../../lib/client';
import type { RequirementGlossaryQuery } from '../../../lib/generated';
import { loadGlossaryTerms } from '../../../lib/glossaryTerms';
import type { Locale } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import GlossaryMatch from './GlossaryMatch';

export type GlossaryMatches = NonNullable<
  RequirementGlossaryQuery['requirementGlossary']
>['matches'];
export type RenderMatch = (
  match: GlossaryMatches[number],
  label: ReactNode,
  key?: number,
) => ReactNode;

type MatchedTextProps = {
  highlight?: { start: number; end: number };
  children?: (renderMatch: RenderMatch) => ReactNode;
  projectId: string;
  text: string;
  matches: GlossaryMatches;
  locale: Locale;
};

function candidateSegment(
  chars: string[],
  start: number,
  end: number,
  highlight: MatchedTextProps['highlight'],
) {
  const left = Math.max(start, highlight?.start ?? end);
  const right = Math.min(end, highlight?.end ?? start);
  if (left >= right || !highlight || highlight.end > chars.length)
    return chars.slice(start, end).join('');
  return (
    <span key={start}>
      {chars.slice(start, left).join('')}
      <mark data-candidate="">{chars.slice(left, right).join('')}</mark>
      {chars.slice(right, end).join('')}
    </span>
  );
}

export function MatchedText(props: MatchedTextProps) {
  const ids = [...new Set(props.matches.flatMap((match) => match.terms.map((term) => term.id)))];
  if (ids.length === 0) {
    if (props.children) return props.children((_match, label) => label);
    if (!props.highlight) return props.text;
    const chars = Array.from(props.text);
    return candidateSegment(chars, 0, chars.length, props.highlight);
  }
  const idsKey = JSON.stringify(ids.sort());
  return (
    <HighlightedText key={JSON.stringify([props.projectId, idsKey])} {...props} idsKey={idsKey} />
  );
}

function HighlightedText({
  projectId,
  text,
  matches,
  locale,
  idsKey,
  highlight,
  children,
}: MatchedTextProps & {
  idsKey: string;
}) {
  const load = useCallback(
    () => loadGlossaryTerms(projectId, JSON.parse(idsKey)),
    [projectId, idsKey],
  );
  const retryTag = `project:${projectId}:glossary-term-retry`;
  const { data, loading, error } = useResource(load, [
    `project:${projectId}`,
    `project:${projectId}:glossary`,
    retryTag,
  ]);
  const reload = async () => {
    // Mounted previews share the term cache but have independent error states.
    clientCache.invalidate([retryTag]);
  };
  const details = new Map(data?.map((term) => [term.id, term]));
  const renderMatch: RenderMatch = (match, label, key = match.start) => {
    const terms = match.terms.flatMap(({ id }) => {
      const term = details.get(id);
      return term ? [term] : [];
    });
    return (
      <GlossaryMatch
        key={key}
        text={label}
        terms={terms}
        locale={locale}
        loading={loading}
        error={error}
        reload={reload}
      />
    );
  };
  if (children) return children(renderMatch);
  const chars = Array.from(text);
  const nodes: ReactNode[] = [];
  let pos = 0;
  for (const match of matches) {
    nodes.push(candidateSegment(chars, pos, match.start, highlight));
    nodes.push(renderMatch(match, candidateSegment(chars, match.start, match.end, highlight)));
    pos = match.end;
  }
  nodes.push(candidateSegment(chars, pos, chars.length, highlight));
  return <>{nodes}</>;
}
