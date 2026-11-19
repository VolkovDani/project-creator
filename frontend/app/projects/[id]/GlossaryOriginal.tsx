'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import {
  RequirementGlossaryDocument,
  type GlossaryFieldsFragment as Term,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import { GlossaryText, useGlossaryEditor } from './GlossaryActions';

type Props = {
  projectId: string;
  requirementId: string;
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
function SourceText({ projectId, requirementId, version, original, locale }: Props) {
  const tag = `project:${projectId}:glossary`;
  const load = useCallback(
    async () =>
      (
        await query(
          RequirementGlossaryDocument,
          { projectId, requirementId, version },
          { tags: [tag, `project:${projectId}:requirements`] },
        )
      ).requirementGlossary,
    [projectId, requirementId, version, tag],
  );
  const result = useResource(load, [tag, `project:${projectId}:requirements`]);
  const [active, setActive] = useState<number>(),
    [chosen, setChosen] = useState<string>();
  const setDraft = useGlossaryEditor();
  const chars = Array.from(original),
    nodes: React.ReactNode[] = [];
  let pos = 0;
  const matches = result.data?.matches ?? [];
  for (const m of matches) {
    nodes.push(chars.slice(pos, m.start).join(''));
    nodes.push(
      <button
        className="glossary-match"
        key={m.start}
        type="button"
        title={m.terms.map((term) => term.definition || term.name).join(' / ')}
        onClick={() => {
          setActive(m.start);
          setChosen(undefined);
        }}
      >
        {m.text}
      </button>,
    );
    pos = m.end;
  }
  nodes.push(chars.slice(pos).join(''));
  const options = matches.find((m) => m.start === active)?.terms ?? [];
  const meaning: Term | undefined =
    options.length === 1 ? options[0] : options.find((v) => v.id === chosen);
  return (
    <>
      <GlossaryText className="user-content" sourceId={requirementId} version={version}>
        {nodes}
      </GlossaryText>
      {options.length > 1 && (
        <label>
          {t(locale, 'glossary.ambiguous')}
          <select value={chosen ?? ''} onChange={(e) => setChosen(e.target.value)}>
            <option value="">{t(locale, 'glossary.choose')}</option>
            {options.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
              </option>
            ))}
          </select>
        </label>
      )}
      {meaning && (
        <div>
          <strong>{meaning.name}</strong>
          <p>{meaning.definition}</p>
          <p>{meaning.context}</p>
          <button type="button" onClick={() => setDraft({ name: meaning.name, term: meaning })}>
            {t(locale, 'glossary.edit')}
          </button>
        </div>
      )}
    </>
  );
}
