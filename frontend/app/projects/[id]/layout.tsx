import { Suspense } from 'react';
import ProjectShell from './ProjectShell';
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <Suspense>
      <ProjectShell>{children}</ProjectShell>
    </Suspense>
  );
}
