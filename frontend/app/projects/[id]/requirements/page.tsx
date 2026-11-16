'use client';
import { Suspense } from 'react';
import { useProject } from '../ProjectShell';
import Requirements from '../Requirements';
export default function RequirementsPage() {
  const { project, locale } = useProject();
  return (
    <Suspense>
      <Requirements key={project.id} projectId={project.id} locale={locale} />
    </Suspense>
  );
}
