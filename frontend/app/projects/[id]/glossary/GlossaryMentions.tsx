'use client';
import Link from 'next/link';
import { useCallback, useEffect, useRef } from 'react';
import { query } from '../../../../lib/client';
import { GlossaryMentionsDocument } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import { requirementURL } from '../../../../lib/rag';
import { GlossaryListView } from './GlossaryListView';

export default function GlossaryMentions({
  projectId,
  term,
  locale,
  onClose,
}: {
  projectId: string;
  term: { id: string; name: string };
  locale: Locale;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const element = ref.current;
    element?.showModal();
    return () => element?.close();
  }, []);
  const load = useCallback(
    async (after: string | null) =>
      (
        await query(
          GlossaryMentionsDocument,
          { projectId, termId: term.id, after },
          { tags: [`project:${projectId}:requirements`, `project:${projectId}:glossary`] },
        )
      ).glossaryMentions,
    [projectId, term.id],
  );
  return (
    <dialog ref={ref} aria-labelledby="glossary-mentions-title" onCancel={onClose}>
      <h2 id="glossary-mentions-title">
        {t(locale, 'glossary.mentionsTitle', { name: term.name })}
      </h2>
      <GlossaryListView
        locale={locale}
        identity={JSON.stringify(['mentions', projectId, term.id])}
        tags={[`project:${projectId}:requirements`, `project:${projectId}:glossary`]}
        load={load}
        empty={t(locale, 'glossary.noMentions')}
        render={(row) => (
          <p key={row.id}>
            <Link href={requirementURL(projectId, row.id)}>{row.text}</Link>
          </p>
        )}
      />
      <button type="button" onClick={onClose}>
        {t(locale, 'app.close')}
      </button>
    </dialog>
  );
}
