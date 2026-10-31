'use client';
import { useProject } from '../ProjectShell';
import ProjectSearch from '../RagTools';
export default function SearchPage() {
  const { project, locale } = useProject();
  return <ProjectSearch key={project.id} projectId={project.id} locale={locale} />;
}
