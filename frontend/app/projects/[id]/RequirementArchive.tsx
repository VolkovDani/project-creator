'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import {
  ArchivedRequirementsDocument,
  type ArchivedRequirementsQuery,
  RemoveRequirementDocument,
  RestoreRequirementDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInvalidation } from '../../../lib/live';
import { useInfinite } from '../../../lib/useInfinite';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import RequirementConfirmation from './RequirementConfirmation';

type Entry = ArchivedRequirementsQuery['requirementsConnection']['edges'][number]['node'];

export default function RequirementArchive({
  projectId,
  locale,
}: {
  projectId: string;
  locale: Locale;
}) {
  const tag = `project:${projectId}:requirements`;
  const load = useCallback(
    async (after: string | null) =>
      (await query(ArchivedRequirementsDocument, { projectId, first: 40, after }, { tags: [tag] }))
        .requirementsConnection,
    [projectId, tag],
  );
  const entries = useInfinite({
    identity: `archived-requirements:${projectId}`,
    tags: [tag],
    load,
  });
  const [pending, setPending] = useState<Entry | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<'validation.conflict' | 'requirements.error' | null>(null);
  useInvalidation(
    [tag],
    () => {},
    () => {
      setPending(null);
      setError(null);
    },
  );
  async function change(entry: Entry, action: 'delete' | 'restore') {
    setPending(null);
    if (busy || !entry.archivedAt) return;
    setBusy(true);
    setError(null);
    const vars = { id: entry.id, version: entry.version, archivedAt: entry.archivedAt, projectId };
    try {
      const changed =
        action === 'restore'
          ? (await query(RestoreRequirementDocument, vars)).restoreRequirement
          : (await query(RemoveRequirementDocument, vars)).deleteRequirement.deleted;
      if (!changed) setError('validation.conflict');
      await entries.refresh();
    } catch {
      setError('requirements.error');
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <ResourceState
        locale={locale}
        loading={entries.loading}
        error={entries.error}
        hasData={entries.loaded}
        retry={() => void entries.retry().catch(() => {})}
      />
      {error && <p role="alert">{t(locale, error)}</p>}
      {entries.loaded && entries.rows.length === 0 && (
        <p>{t(locale, 'requirements.archiveEmpty')}</p>
      )}
      {entries.rows.map((entry) => (
        <article className="card" key={entry.id}>
          <p className="user-content">{entry.text}</p>
          <p className="muted">{t(locale, 'requirements.version', { version: entry.version })}</p>
          <button type="button" disabled={busy} onClick={() => void change(entry, 'restore')}>
            {t(locale, 'requirements.restore')}
          </button>
          <button
            type="button"
            className="destructive-action"
            disabled={busy}
            onClick={() => setPending(entry)}
          >
            {t(locale, 'requirements.purge')}
          </button>
        </article>
      ))}
      <LoadMore
        locale={locale}
        {...entries}
        observe
        loadMore={() => void entries.loadMore().catch(() => {})}
      />
      {pending && (
        <RequirementConfirmation
          action="delete"
          text={pending.text}
          locale={locale}
          onCancel={() => setPending(null)}
          onConfirm={() => void change(pending, 'delete')}
        />
      )}
    </>
  );
}
