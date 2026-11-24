'use client';
import { useSelectedLayoutSegments } from 'next/navigation';
import { Suspense } from 'react';
import PageMetadata from '../../components/PageMetadata';
import ProjectShell from './ProjectShell';
export default function Layout({ children }: { children: React.ReactNode }) {
  const [section = 'overview', child] = useSelectedLayoutSegments();
  const history = section === 'technical' && child === 'analysis-history';
  const externalProposals = section === 'analysis' && child === 'external-proposals';
  const titleKey = history
    ? 'analysis.history'
    : externalProposals
      ? 'externalProposals.title'
      : `nav.${section}`;
  const descriptionKey = history
    ? 'analysis.historyHelp'
    : externalProposals
      ? 'externalProposals.help'
      : section === 'overview'
        ? undefined
        : `ui.${section}Help`;
  return (
    <>
      <PageMetadata titleKey={titleKey} descriptionKey={descriptionKey} />
      <Suspense>
        <ProjectShell>{children}</ProjectShell>
      </Suspense>
    </>
  );
}
