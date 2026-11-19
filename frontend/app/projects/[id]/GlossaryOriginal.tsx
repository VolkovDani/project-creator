'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import { RequirementGlossaryDocument, type RequirementGlossaryQuery } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import { GlossaryText } from './GlossaryActions';
import GlossaryMatch from './GlossaryMatch';

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
    <GlossaryText className="user-content" sourceId={requirementId} version={version}>
      <MatchedText
        highlight={highlight}
        text={original}
        matches={result.data?.matches ?? []}
        locale={locale}
      />
    </GlossaryText>
  );
}

export function MatchedText({
  text,
  matches,
  highlight,
  locale,
}: {
  text: string;
  highlight?: Props['highlight'];
  matches: NonNullable<RequirementGlossaryQuery['requirementGlossary']>['matches'];
  locale: Locale;
}) {
  const chars = Array.from(text),
    nodes: React.ReactNode[] = [];
  function segment(start: number, end: number) {
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
  let pos = 0;
  for (const m of matches) {
    nodes.push(segment(pos, m.start));
    nodes.push(
      <GlossaryMatch
        key={m.start}
        text={segment(m.start, m.end)}
        terms={m.terms}
        locale={locale}
      />,
    );
    pos = m.end;
  }
  nodes.push(segment(pos, chars.length));
  return <>{nodes}</>;
}
