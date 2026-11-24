'use client';
import Image from 'next/image';
import Link from 'next/link';
import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { type Locale, t } from '../../../lib/i18n';
import brandIcon from '../../../public/favicon-48x48.png';
import { DesignIcon } from '../../components/DesignIcon';
import { InterfacePreferences } from '../../components/LocaleProvider';
import styles from './ProjectShell.module.css';
import { ProjectSwitcher } from './ProjectSwitcher';

export const sections = [
  'overview',
  'requirements',
  'search',
  'classification',
  'glossary',
  'analysis',
] as const;
export function sectionLabel(locale: Locale, section: (typeof sections)[number]) {
  return t(locale, section === 'glossary' ? 'nav.glossaryShort' : `nav.${section}`);
}

const mobileQuery = '(max-width: 48rem)';
function listenViewport(listener: () => void) {
  const query = window.matchMedia(mobileQuery);
  query.addEventListener('change', listener);
  return () => query.removeEventListener('change', listener);
}
const mobileViewport = () => window.matchMedia(mobileQuery).matches;
const serverViewport = () => false;

export function ProjectNavigation({
  children,
  locale,
  name,
  base,
  pathname,
}: {
  children: React.ReactNode;
  locale: Locale;
  name?: string;
  base: string;
  pathname: string;
}) {
  const mobile = useSyncExternalStore(listenViewport, mobileViewport, serverViewport);
  const [desktopOpen, setDesktopOpen] = useState(true);
  const [mobilePath, setMobilePath] = useState<string | null>(null);
  const expanded = mobile ? mobilePath === pathname : desktopOpen;
  const toggle = useRef<HTMLButtonElement>(null);
  const sidebar = useRef<HTMLElement>(null);
  const indexing = pathname === `${base}/indexing`;
  const history = pathname === `${base}/technical/analysis-history`;
  const technical = pathname === `${base}/technical`;
  const externalProposals = pathname === `${base}/analysis/external-proposals`;

  useEffect(() => {
    if (!mobile || !expanded) return;
    const first =
      sidebar.current?.querySelector<HTMLAnchorElement>('[aria-current="page"]') ??
      sidebar.current?.querySelector<HTMLAnchorElement>('[aria-current="location"]') ??
      sidebar.current?.querySelector<HTMLAnchorElement>('nav a');
    first?.focus();
    const dismiss = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || document.querySelector('dialog[open]')) return;
      setMobilePath(null);
      toggle.current?.focus({ preventScroll: true });
    };
    document.addEventListener('keydown', dismiss);
    return () => document.removeEventListener('keydown', dismiss);
  }, [mobile, expanded]);
  function closeMobileMenu() {
    if (mobile && expanded) {
      setMobilePath(null);
      toggle.current?.focus({ preventScroll: true });
    }
  }

  return (
    <div className={styles.workspace} data-expanded={expanded && !!name}>
      <div className={styles.toolbar}>
        {name && (
          <button
            type="button"
            ref={toggle}
            className={styles.toggle}
            aria-label={t(locale, expanded ? 'ui.closeNavigation' : 'ui.openNavigation')}
            aria-expanded={expanded}
            aria-controls="project-sidebar"
            onClick={() =>
              mobile ? setMobilePath(expanded ? null : pathname) : setDesktopOpen(!desktopOpen)
            }
          >
            <span aria-hidden="true" />
          </button>
        )}
        <div className={styles.breadcrumb}>
          <Link href="/">
            <DesignIcon name="projects" />
            {t(locale, 'app.back')}
          </Link>
          {name && pathname !== base && (
            <>
              <span aria-hidden="true">/</span>
              <Link href={base} onNavigate={closeMobileMenu}>
                {name}
              </Link>
            </>
          )}
          {(indexing || history) && (
            <>
              <span aria-hidden="true">/</span>
              <Link href={`${base}/technical`} onNavigate={closeMobileMenu}>
                {t(locale, 'nav.technical')}
              </Link>
            </>
          )}
        </div>
        {!name && <InterfacePreferences />}
      </div>
      {name && (
        <aside id="project-sidebar" ref={sidebar} hidden={!expanded} className={styles.sidebar}>
          <div className={styles.brand}>
            <Image src={brandIcon} alt="" width={36} height={36} unoptimized />
            <strong>{t(locale, 'app.brand')}</strong>
          </div>
          {expanded && (
            <ProjectSwitcher
              key={pathname}
              name={name}
              base={base}
              sectionPath={pathname.slice(base.length)}
              locale={locale}
            />
          )}
          <nav className={styles.navigation} aria-label={t(locale, 'ui.sections')}>
            <ul>
              {sections.map((section) => {
                const href = section === 'overview' ? base : `${base}/${section}`;
                return (
                  <li key={section}>
                    <Link
                      href={href}
                      aria-current={
                        pathname === href
                          ? 'page'
                          : section === 'analysis' && externalProposals
                            ? 'location'
                            : undefined
                      }
                      onNavigate={closeMobileMenu}
                    >
                      <DesignIcon name={section} />
                      {sectionLabel(locale, section)}
                      {mobile && <DesignIcon name="next" />}
                    </Link>
                  </li>
                );
              })}
              <li className={styles.technical}>
                <Link
                  href={`${base}/technical`}
                  aria-current={technical ? 'page' : indexing || history ? 'location' : undefined}
                  onNavigate={closeMobileMenu}
                >
                  <DesignIcon name="technical" />
                  {t(locale, 'nav.technical')}
                  {mobile && <DesignIcon name="next" />}
                </Link>
                {(technical || indexing || history) && (
                  <ul>
                    <li>
                      <Link
                        href={`${base}/indexing`}
                        aria-current={indexing ? 'page' : undefined}
                        onNavigate={closeMobileMenu}
                      >
                        <DesignIcon name="indexing" />
                        {t(locale, 'nav.indexing')}
                        {mobile && <DesignIcon name="next" />}
                      </Link>
                    </li>
                    <li>
                      <Link
                        href={`${base}/technical/analysis-history`}
                        aria-current={history ? 'page' : undefined}
                        onNavigate={closeMobileMenu}
                      >
                        <DesignIcon name="analysis" />
                        {t(locale, 'analysis.history')}
                        {mobile && <DesignIcon name="next" />}
                      </Link>
                    </li>
                  </ul>
                )}
              </li>
            </ul>
          </nav>
          <InterfacePreferences />
        </aside>
      )}
      <main className={styles.document}>{children}</main>
    </div>
  );
}
