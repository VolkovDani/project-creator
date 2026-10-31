'use client';
import Link from 'next/link';
import { useEffect, useState } from 'react';
import type { RequirementIndexListQuery, RequirementsQuery } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInvalidation } from '../../../lib/live';
import { indexStatus, requirementURL } from '../../../lib/rag';
import { summarizeIndex } from '../../../lib/requirementsView';
import Group from '../../components/Group';
import ResourceState from '../../components/ResourceState';
import { RequirementIndexAction, useProjectIndex } from './RagTools';
import { useRequirementsPage } from './useRequirementsPage';

type Display = {
  page: number;
  rows: RequirementsQuery['requirements'];
  entries: RequirementIndexListQuery['requirementIndex'];
};
const statuses = ['failed', 'pending', 'ready'] as const;

export default function Indexing({ projectId, locale }: { projectId: string; locale: Locale }) {
  const list = useRequirementsPage(projectId);
  const indexing = useProjectIndex(projectId, list.page);
  const [display, setDisplay] = useState<Display>();
  // Keep the last coherent pair of pages while either resource is being refreshed.
  useEffect(() => {
    if (
      list.loaded &&
      !list.loading &&
      indexing.loaded &&
      !indexing.loading &&
      !list.failure &&
      !indexing.error
    )
      setDisplay({ page: list.page, rows: list.rows, entries: indexing.entries });
  }, [
    list.loaded,
    list.loading,
    list.failure,
    list.page,
    list.rows,
    indexing.loaded,
    indexing.loading,
    indexing.error,
    indexing.entries,
  ]);
  useInvalidation(
    [`project:${projectId}`, `project:${projectId}:requirements`],
    () => {},
    () => setDisplay(undefined),
  );
  const loading = list.loading || indexing.loading;
  const error = list.failure || indexing.error;
  const counts = display ? summarizeIndex(display.rows, display.entries) : undefined;
  const page = display?.page ?? list.page;
  const retry = () => {
    void list.load();
    void indexing.reload();
  };
  return (
    <section aria-labelledby="indexing-heading">
      <h2 id="indexing-heading">{t(locale, 'ui.indexing')}</h2>
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={!!display}
        retry={retry}
      />
      <Group
        id={`${projectId}:indexing`}
        title={t(locale, 'ui.indexing')}
        summary={
          counts
            ? `${t(locale, 'ui.currentPage')} · ${t(locale, 'ui.indexSummary', counts)}`
            : undefined
        }
        defaultOpen
      >
        {display && display.rows.length === 0 && <p>{t(locale, 'requirements.empty')}</p>}
        {display &&
          statuses.map((status) => {
            const rows = display.rows.filter(
              (row) =>
                indexStatus(
                  display.entries.find((entry) => entry.requirementId === row.id),
                  row.version,
                ) === status,
            );
            return (
              <Group
                id={`${projectId}:indexing:${status}`}
                key={status}
                title={t(locale, `rag.status.${status}`)}
                summary={String(rows.length)}
                defaultOpen={status !== 'ready'}
              >
                {rows.length === 0 && <p className="muted">{t(locale, 'ui.groupEmpty')}</p>}
                {rows.map((row) => (
                  <article className="card" key={row.id}>
                    <p>{row.text}</p>
                    <p className="muted">
                      {t(locale, 'requirements.version', { version: row.version })}
                    </p>
                    <Link href={requirementURL(projectId, row.id)}>{t(locale, 'rag.source')}</Link>
                    <RequirementIndexAction
                      entry={display.entries.find((entry) => entry.requirementId === row.id)}
                      requirementId={row.id}
                      version={row.version}
                      locale={locale}
                      busy={loading ? 'refresh' : indexing.busy}
                      index={indexing.index}
                    />
                  </article>
                ))}
              </Group>
            );
          })}
      </Group>
      <div className="pagination">
        <button
          type="button"
          disabled={page === 0 || loading || !!indexing.busy}
          onClick={() => list.setPage(page - 1)}
        >
          {t(locale, 'app.previous')}
        </button>
        <span>{t(locale, 'app.page', { page: page + 1 })}</span>
        <button
          type="button"
          disabled={!display || display.rows.length < 20 || loading || !!indexing.busy}
          onClick={() => list.setPage(page + 1)}
        >
          {t(locale, 'app.next')}
        </button>
      </div>
    </section>
  );
}
