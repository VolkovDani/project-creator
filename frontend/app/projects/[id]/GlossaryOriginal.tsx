'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import { RequirementGlossaryDocument } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
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
      highlight={highlight}
      projectId={projectId}
      matches={result.data?.matches}
    />
  );
}
