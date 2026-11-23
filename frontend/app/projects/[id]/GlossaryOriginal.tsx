'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import { RequirementGlossaryDocument, type RequirementGlossaryQuery } from '../../../lib/generated';
import { loadGlossaryTerms } from '../../../lib/glossaryTerms';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import GlossaryMatch from './GlossaryMatch';
import { RequirementMarkdown } from './RequirementMarkdown';

type Props = {
  projectId: string;
  requirementId: string;
  analysisId?: string;
  version: number;
  original: string;
  highlight?: { start: number; end: number };
  locale: Locale;
};
export default function GlossaryOriginal({
  collapsible,
  ...props
}: Props & { collapsible: boolean }) {
  const [open, setOpen] = useState(false);
  if (!collapsible) return <SourceText key={props.version} {...props} />;
  return (
    <details onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary>{t(props.locale, 'summary.original')}</summary>
      {open ? <SourceText key={props.version} {...props} /> : <p>{props.original}</p>}
    </details>
  );
}
export function SourceText({
  projectId,
  requirementId,
  version,
  original,
  locale,
  analysisId,
  highlight,
}: Props) {
  const tag = `project:${projectId}:glossary`;
  const load = useCallback(
    async () =>
      (
        await query(
          RequirementGlossaryDocument,
          { projectId, requirementId, version, analysisId },
          { tags: [tag, `project:${projectId}:requirements`] },
        )
      ).requirementGlossary,
    [projectId, requirementId, version, tag, analysisId],
  );
  const result = useResource(load, [tag, `project:${projectId}:requirements`]);
  return (
    <RequirementMarkdown
      text={original}
      locale={locale}
      sourceId={requirementId}
      version={version}
      showExact={!!highlight}
    >
      <MatchedText
        highlight={highlight}
        projectId={projectId}
        text={original}
        matches={result.data?.matches ?? []}
        locale={locale}
      />
    </RequirementMarkdown>
  );
}

type MatchedTextProps = {
  highlight?: Props['highlight'];
  projectId: string;
  text: string;
  matches: NonNullable<RequirementGlossaryQuery['requirementGlossary']>['matches'];
  locale: Locale;
};

function candidateSegment(
  chars: string[],
  start: number,
  end: number,
  highlight: Props['highlight'],
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
}: MatchedTextProps & {
  idsKey: string;
}) {
  const load = useCallback(
    () => loadGlossaryTerms(projectId, JSON.parse(idsKey)),
    [projectId, idsKey],
  );
  const { data, loading, error, reload } = useResource(load, [
    `project:${projectId}`,
    `project:${projectId}:glossary`,
  ]);
  const details = new Map(data?.map((term) => [term.id, term]));
  const chars = Array.from(text);
  const nodes: React.ReactNode[] = [];
  let pos = 0;
  for (const match of matches) {
    nodes.push(candidateSegment(chars, pos, match.start, highlight));
    const terms = match.terms.flatMap(({ id }) => {
      const term = details.get(id);
      return term ? [term] : [];
    });
    nodes.push(
      <GlossaryMatch
        key={match.start}
        text={candidateSegment(chars, match.start, match.end, highlight)}
        terms={terms}
        locale={locale}
        loading={loading}
        error={error}
        reload={reload}
      />,
    );
    pos = match.end;
  }
  nodes.push(candidateSegment(chars, pos, chars.length, highlight));
  return <>{nodes}</>;
}
