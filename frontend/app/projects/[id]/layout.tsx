'use client';
import { useSelectedLayoutSegment } from 'next/navigation';
import { Suspense } from 'react';
import PageMetadata from '../../components/PageMetadata';
import ProjectShell from './ProjectShell';
export default function Layout({ children }: { children: React.ReactNode }) {
  const section = useSelectedLayoutSegment() ?? 'overview';
  return (
    <>
      <PageMetadata
        titleKey={`nav.${section}`}
        descriptionKey={section === 'overview' ? undefined : `ui.${section}Help`}
      />
      <Suspense>
        <ProjectShell>{children}</ProjectShell>
      </Suspense>
    </>
  );
}
