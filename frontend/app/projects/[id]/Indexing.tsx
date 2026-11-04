'use client';
import { useEffect, useState } from 'react';
import type { RequirementIndexListQuery, RequirementsQuery } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInvalidation } from '../../../lib/live';
import { summarizeIndex } from '../../../lib/requirementsView';
import { useListMode } from '../../../lib/useListMode';
import Group from '../../components/Group';
import { ListModeSelect } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import IndexGroups from './IndexGroups';
import InfiniteIndexing from './InfiniteIndexing';
import { useProjectIndex } from './RagTools';
import { useRequirementsPage } from './useRequirementsPage';

type Display = {
  page: number;
  rows: RequirementsQuery['requirements'];
  entries: RequirementIndexListQuery['requirementIndex'];
};

export default function Indexing({ projectId, locale }: { projectId: string; locale: Locale }) {
  const location = useListMode();
  return (
    <section aria-labelledby="indexing-heading">
      <h2 id="indexing-heading">{t(locale, 'ui.indexing')}</h2>
      <ListModeSelect locale={locale} mode={location.mode} onChange={location.setMode} />
      {location.mode === 'infinite' ? (
        <InfiniteIndexing key={projectId} projectId={projectId} locale={locale} />
      ) : (
        <PagedIndexing projectId={projectId} locale={locale} location={location} />
      )}
    </section>
  );
}

function PagedIndexing({
  projectId,
  locale,
  location,
}: {
  projectId: string;
  locale: Locale;
  location: ReturnType<typeof useListMode>;
}) {
  const list = useRequirementsPage(projectId, undefined, {
    enabled: true,
    page: location.page,
    setPage: location.setPage,
  });
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
    <>
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
        {display && (
          <IndexGroups
            projectId={projectId}
            locale={locale}
            rows={display.rows}
            entries={display.entries}
            busy={loading ? 'refresh' : indexing.busy}
            index={indexing.index}
          />
        )}
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
    </>
  );
}
