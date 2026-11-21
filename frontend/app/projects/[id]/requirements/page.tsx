'use client';
import { Suspense } from 'react';
import { t } from '../../../../lib/i18n';
import { useProject } from '../ProjectShell';
import Requirements from '../Requirements';
import styles from './requirements.module.css';
export default function RequirementsPage() {
  const { project, locale } = useProject();
  return (
    <div className={styles.layout}>
      <nav className={styles.contents} aria-label={t(locale, 'ui.onThisPage')}>
        <details open>
          <summary>{t(locale, 'ui.onThisPage')}</summary>
          <a href="#requirement-entry">{t(locale, 'requirements.add')}</a>
          <a href="#requirements-list">{t(locale, 'ui.requirementsGroup')}</a>
        </details>
      </nav>
      <Suspense>
        <Requirements key={project.id} projectId={project.id} locale={locale} />
      </Suspense>
    </div>
  );
}
