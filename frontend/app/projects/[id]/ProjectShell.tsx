'use client';
import { useParams, usePathname } from 'next/navigation';
import { createContext, useCallback, useContext } from 'react';
import { query } from '../../../lib/client';
import { ProjectDetailDocument, type ProjectDetailQuery } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import { useLocale } from '../../components/LocaleProvider';
import ResourceState from '../../components/ResourceState';
import { GlossaryActions } from './GlossaryActions';
import { ProjectNavigation } from './ProjectNavigation';

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
    <ProjectNavigation name={project?.name} locale={locale} base={base} pathname={pathname}>
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
          {['requirements', 'indexing', 'analysis', 'search', 'technical/analysis-history'].some(
            (section) => pathname === `${base}/${section}`,
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
    </ProjectNavigation>
  );
}
