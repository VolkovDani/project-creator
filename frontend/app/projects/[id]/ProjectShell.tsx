'use client';
import Link from 'next/link';
import { useParams, usePathname } from 'next/navigation';
import { createContext, useCallback, useContext } from 'react';
import { query } from '../../../lib/client';
import { ProjectDetailDocument, type ProjectDetailQuery } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import { InterfacePreferences, useLocale } from '../../components/LocaleProvider';
import ResourceState from '../../components/ResourceState';
import { GlossaryActions } from './GlossaryActions';

type Project = NonNullable<ProjectDetailQuery['project']>;
const Context = createContext<{
  project: Project;
  analysisDefaultThreshold: number;
  locale: Locale;
} | null>(null);
export function useProject() {
  const context = useContext(Context);
  if (!context) throw Error('Project context unavailable');
  return context;
}
export const sections = [
  'overview',
  'requirements',
  'classification',
  'indexing',
  'search',
  'analysis',
  'glossary',
] as const;
export default function ProjectShell({ children }: { children: React.ReactNode }) {
  const { id } = useParams<{ id: string }>();
  return (
    <ProjectContent key={id} id={id}>
      {children}
    </ProjectContent>
  );
}
function ProjectContent({ id, children }: { id: string; children: React.ReactNode }) {
  const { locale } = useLocale();
  const pathname = usePathname();
  const loader = useCallback(() => query(ProjectDetailDocument, { id }), [id]);
  const { data, loading, error, reload } = useResource(loader, [`project:${id}`]);
  const project = data?.project;
  const base = `/projects/${encodeURIComponent(id)}`;
  return (
    <main>
      <div className="top">
        <Link href="/">{t(locale, 'app.back')}</Link>
        <InterfacePreferences />
      </div>
      <header className="project-header">
        <h1 tabIndex={-1}>{project?.name || t(locale, 'ui.project')}</h1>
      </header>
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={data !== undefined}
        retry={() => void reload()}
      />
      {project ? (
        <Context.Provider
          value={{ project, locale, analysisDefaultThreshold: data.analysisDefaultThreshold }}
        >
          <nav className="project-nav" aria-label={t(locale, 'ui.sections')}>
            {sections.map((section) => {
              const href = section === 'overview' ? base : `${base}/${section}`;
              return (
                <Link
                  href={href}
                  key={section}
                  aria-current={pathname === href ? 'page' : undefined}
                >
                  {t(locale, `nav.${section}`)}
                </Link>
              );
            })}
          </nav>
          {['requirements', 'indexing', 'analysis', 'search'].some(
            (section) => pathname === base + '/' + section,
          ) ? (
            <GlossaryActions key={pathname} projectId={id} locale={locale}>
              {children}
            </GlossaryActions>
          ) : (
            children
          )}
        </Context.Provider>
      ) : (
        !loading && !error && <p>{t(locale, 'app.notFound')}</p>
      )}
    </main>
  );
}
