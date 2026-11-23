'use client';
import { useSelectedLayoutSegments } from 'next/navigation';
import { Suspense } from 'react';
import PageMetadata from '../../components/PageMetadata';
import ProjectShell from './ProjectShell';
export default function Layout({ children }: { children: React.ReactNode }) {
  const [section = 'overview', child] = useSelectedLayoutSegments();
  const history = section === 'technical' && child === 'analysis-history';
  return (
    <>
      <PageMetadata
        titleKey={history ? 'analysis.history' : `nav.${section}`}
        descriptionKey={
          history
            ? 'analysis.historyHelp'
            : section === 'overview'
              ? undefined
              : `ui.${section}Help`
        }
      />
      <Suspense>
        <ProjectShell>{children}</ProjectShell>
      </Suspense>
    </>
  );
}
