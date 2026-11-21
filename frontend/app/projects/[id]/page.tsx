'use client';
import Link from 'next/link';
import { t } from '../../../lib/i18n';
import { DesignIcon } from '../../components/DesignIcon';
import { sectionLabel, sections } from './ProjectNavigation';
import { useProject } from './ProjectShell';
import styles from './ProjectShell.module.css';
export default function ProjectPage() {
  const { project, locale } = useProject();
  return (
    <section>
      <h2>{t(locale, 'nav.overview')}</h2>
      <h3>{t(locale, 'app.goal')}</h3>
      <p className="user-content">{project.goal}</p>
      <h3>{t(locale, 'app.description')}</h3>
      <p className="user-content">{project.description}</p>
      <div className={`overview-links ${styles.cards}`}>
        {sections
          .filter((key) => key !== 'overview')
          .map((key) => (
            <Link
              className="card"
              key={key}
              href={`/projects/${encodeURIComponent(project.id)}/${key}`}
            >
              <div className={styles.destination}>
                <DesignIcon name={key} />
                <strong>{sectionLabel(locale, key)}</strong>
                <DesignIcon name="next" />
              </div>
              <p>{t(locale, `ui.${key}Help`)}</p>
            </Link>
          ))}
      </div>
    </section>
  );
}
