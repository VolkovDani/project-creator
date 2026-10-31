'use client';
import Link from 'next/link';
import { useCallback, useState } from 'react';
import { query } from '../lib/client';
import { ProjectListDocument } from '../lib/generated';
import { t } from '../lib/i18n';
import { useResource } from '../lib/useResource';
import { Language, useLocale } from './components/LocaleProvider';
import ResourceState from './components/ResourceState';
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
  const [page, setPage] = useState(0);
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
          rows?.map((project) => (
            <article className="card" key={project.id}>
              <h2>
                <Link href={`/projects/${encodeURIComponent(project.id)}`}>{project.name}</Link>
              </h2>
              <p>{project.goal}</p>
            </article>
          ))
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
