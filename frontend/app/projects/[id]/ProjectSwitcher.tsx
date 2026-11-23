'use client';
import Link from 'next/link';
import { useId, useState } from 'react';
import { type Locale, t } from '../../../lib/i18n';
import { useProjects } from '../../../lib/useProjects';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import styles from './ProjectShell.module.css';

export function ProjectSwitcher({
  name,
  base,
  sectionPath,
  locale,
}: {
  name: string;
  base: string;
  sectionPath: string;
  locale: Locale;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <div className={styles.switcher}>
      <button type="button" aria-controls={id} aria-expanded={open} onClick={() => setOpen(!open)}>
        {name}
        <small>{t(locale, 'ui.switchProject')}</small>
      </button>
      <div id={id} hidden={!open} className={styles.projectChoices}>
        {open && <ProjectChoices base={base} sectionPath={sectionPath} locale={locale} />}
      </div>
    </div>
  );
}

function ProjectChoices({
  base,
  sectionPath,
  locale,
}: {
  base: string;
  sectionPath: string;
  locale: Locale;
}) {
  const list = useProjects();
  return (
    <>
      {(list.loading || list.error) && (
        <ResourceState
          locale={locale}
          loading={list.loading}
          error={list.error}
          hasData={list.loaded}
          retry={() => void list.retry().catch(() => {})}
        />
      )}
      {list.loaded && list.rows.length === 0 && <p>{t(locale, 'app.empty')}</p>}
      {list.rows.length > 0 && (
        <ul>
          {list.rows.map((project) => {
            const href = `/projects/${encodeURIComponent(project.id)}`;
            return (
              <li key={project.id}>
                {href === base ? (
                  <strong aria-current="true">{project.name}</strong>
                ) : (
                  <Link href={href + sectionPath}>{project.name}</Link>
                )}
              </li>
            );
          })}
        </ul>
      )}
      <LoadMore
        locale={locale}
        {...list}
        observe
        loadMore={() => void list.loadMore().catch(() => {})}
      />
    </>
  );
}
