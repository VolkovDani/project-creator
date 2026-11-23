'use client';
import Analysis from '../../Analysis';
import { useProject } from '../../ProjectShell';
export default function AnalysisHistoryPage() {
  const { project, locale, analysisDefaultThreshold } = useProject();
  return (
    <Analysis
      key={project.id}
      projectId={project.id}
      locale={locale}
      defaultThreshold={analysisDefaultThreshold}
      historyView
    />
  );
}
