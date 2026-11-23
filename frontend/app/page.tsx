'use client';
import Link from 'next/link';
import { useCallback } from 'react';
import { query } from '../lib/client';
import { ProjectConnectionDocument, type ProjectConnectionQuery } from '../lib/generated';
import { t } from '../lib/i18n';
import { useInfinite } from '../lib/useInfinite';
import { DesignIcon } from './components/DesignIcon';
import { LoadMore } from './components/ListControls';
import { InterfacePreferences, useLocale } from './components/LocaleProvider';
import PageMetadata from './components/PageMetadata';
import styles from './components/RecordIcons.module.css';
import ResourceState from './components/ResourceState';
import { VirtualList } from './components/VirtualList';
export default function Home() {
  const { locale } = useLocale();
  return (
    <main>
      <PageMetadata titleKey="app.title" />
      <div className="top">
        <h1>{t(locale, 'app.title')}</h1>
        <InterfacePreferences />
      </div>
      <Link className="button-link" href="/projects/new">
        {t(locale, 'app.create')}
      </Link>
      <ProjectList />
    </main>
  );
}
function ProjectCard({
  project,
}: {
  project: ProjectConnectionQuery['projectsConnection']['edges'][number]['node'];
}) {
  return (
    <article className="card">
      <h2>
        <Link className={styles.projectLink} href={`/projects/${encodeURIComponent(project.id)}`}>
          <DesignIcon name="projects" />
          <span>{project.name}</span>
          <DesignIcon name="next" />
        </Link>
      </h2>
      <p>{project.goal}</p>
    </article>
  );
}
function ProjectList() {
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
