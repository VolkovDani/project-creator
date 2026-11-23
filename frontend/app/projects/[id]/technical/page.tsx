'use client';
import Link from 'next/link';
import { t } from '../../../../lib/i18n';
import { DesignIcon } from '../../../components/DesignIcon';
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
          <Link
            className={styles.destination}
            href={`/projects/${encodeURIComponent(project.id)}/indexing`}
          >
            <DesignIcon name="indexing" />
            {t(locale, 'nav.indexing')}
            <DesignIcon name="next" />
          </Link>
        </h3>
        <p>{t(locale, 'ui.indexingBackground')}</p>
        <p>{t(locale, 'ui.indexingFailureHelp')}</p>
      </div>
      <div className={styles.indexingCard}>
        <h3>
          <Link
            className={styles.destination}
            href={`/projects/${encodeURIComponent(project.id)}/technical/analysis-history`}
          >
            <DesignIcon name="analysis" />
            {t(locale, 'analysis.history')}
            <DesignIcon name="next" />
          </Link>
        </h3>
        <p>{t(locale, 'analysis.historyHelp')}</p>
      </div>
    </section>
  );
}
