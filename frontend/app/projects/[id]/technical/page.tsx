'use client';
import Link from 'next/link';
import { t } from '../../../../lib/i18n';
import { useProject } from '../ProjectShell';
import styles from '../ProjectShell.module.css';

export default function TechnicalPage() {
  const { project, locale } = useProject();
  return (
    <section aria-labelledby="technical-heading">
      <h2 id="technical-heading">{t(locale, 'nav.technical')}</h2>
      <p>{t(locale, 'ui.technicalHelp')}</p>
      <div className={styles.indexingCard}>
        <h3>
          <Link href={`/projects/${encodeURIComponent(project.id)}/indexing`}>
            {t(locale, 'nav.indexing')}
          </Link>
        </h3>
        <p>{t(locale, 'ui.indexingBackground')}</p>
        <p>{t(locale, 'ui.indexingFailureHelp')}</p>
      </div>
    </section>
  );
}
