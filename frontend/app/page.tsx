'use client';
import Link from 'next/link';
import { useCallback } from 'react';
import { query } from '../lib/client';
import {
  ProjectConnectionDocument,
  ProjectListDocument,
  type ProjectListQuery,
} from '../lib/generated';
import { t } from '../lib/i18n';
import { useInfinite } from '../lib/useInfinite';
import { useListMode } from '../lib/useListMode';
import { useResource } from '../lib/useResource';
import { ListModeSelect, LoadMore } from './components/ListControls';
import { Language, useLocale } from './components/LocaleProvider';
import ResourceState from './components/ResourceState';
import { VirtualList } from './components/VirtualList';
export default function Home() {
  const { locale } = useLocale();
  return (
    <main>
      <div className="top">
        <h1>{t(locale, 'app.title')}</h1>
        <Language />
      </div>
      <Link className="button-link" href="/projects/new">
        {t(locale, 'app.create')}
      </Link>
      <ProjectList />
    </main>
  );
}
function ProjectList() {
  const { locale } = useLocale();
  const { mode, setMode, page, setPage } = useListMode();
  return (
    <>
      <ListModeSelect locale={locale} mode={mode} onChange={setMode} />
      {mode === 'infinite' ? <InfiniteProjects /> : <PagedProjects page={page} setPage={setPage} />}
    </>
  );
}
function ProjectCard({ project }: { project: ProjectListQuery['projects'][number] }) {
  return (
    <article className="card">
      <h2>
        <Link href={`/projects/${encodeURIComponent(project.id)}`}>{project.name}</Link>
      </h2>
      <p>{project.goal}</p>
    </article>
  );
}
function InfiniteProjects() {
  const { locale } = useLocale();
  const load = useCallback(
    async (after: string | null) =>
      (await query(ProjectConnectionDocument, { first: 40, after })).projectsConnection,
    [],
  );
  const list = useInfinite({ identity: 'projects', tags: ['projects'], load });
  const more = useCallback(() => {
    if (!list.loading && !list.error && list.hasNextPage) void list.loadMore().catch(() => {});
  }, [list.loading, list.error, list.hasNextPage, list.loadMore]);
  return (
    <section aria-label={t(locale, 'app.title')} aria-busy={list.loading}>
      <ResourceState
        locale={locale}
        loading={list.loading}
        error={list.error}
        hasData={list.loaded}
        retry={() => void list.retry().catch(() => {})}
      />
      {list.loaded && (
        <p className="muted">{t(locale, 'list.total', { total: list.totalCount })}</p>
      )}
      {list.loaded && list.rows.length === 0 && <p>{t(locale, 'app.empty')}</p>}
      <VirtualList
        identity="projects"
        items={list.rows}
        keyOf={(row) => row.id}
        renderItem={(project) => <ProjectCard project={project} />}
        onEndReached={more}
      />
      <LoadMore locale={locale} {...list} loadMore={more} />
    </section>
  );
}
function PagedProjects({ page, setPage }: { page: number; setPage: (page: number) => void }) {
  const { locale } = useLocale();
  const loader = useCallback(
    async () => ({
      page,
      rows: (await query(ProjectListDocument, { limit: 20, offset: page * 20 })).projects,
    }),
    [page],
  );
  const { data: snapshot, loading, error, reload } = useResource(loader, ['projects']);
  const rows = snapshot?.rows;
  const displayedPage = snapshot?.page ?? page;
  return (
    <section aria-label={t(locale, 'app.title')} aria-busy={loading}>
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={rows !== undefined}
        retry={() => void reload()}
      />
      <div className="record-list">
        {rows?.length === 0 ? (
          <p>{t(locale, 'app.empty')}</p>
        ) : (
          rows?.map((project) => <ProjectCard key={project.id} project={project} />)
        )}
      </div>
      <div className="pagination">
        <button
          type="button"
          disabled={displayedPage === 0 || loading}
          onClick={() => setPage(displayedPage - 1)}
        >
          {t(locale, 'app.previous')}
        </button>
        <span>{t(locale, 'app.page', { page: displayedPage + 1 })}</span>
        <button
          type="button"
          disabled={!rows || rows.length < 20 || loading}
          onClick={() => setPage(displayedPage + 1)}
        >
          {t(locale, 'app.next')}
        </button>
      </div>
    </section>
  );
}
