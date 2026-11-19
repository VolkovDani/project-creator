'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import { RequirementGlossaryDocument } from '../../../lib/generated';
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
  const chars = Array.from(original),
    nodes: React.ReactNode[] = [];
  let pos = 0;
  const matches = result.data?.matches ?? [];
  for (const m of matches) {
    nodes.push(chars.slice(pos, m.start).join(''));
    nodes.push(<GlossaryMatch key={m.start} text={m.text} terms={m.terms} locale={locale} />);
    pos = m.end;
  }
  nodes.push(chars.slice(pos).join(''));
  return (
    <GlossaryText className="user-content" sourceId={requirementId} version={version}>
      {nodes}
    </GlossaryText>
  );
}
