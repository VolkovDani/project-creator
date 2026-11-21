'use client';
import Image from 'next/image';
import Link from 'next/link';
import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { type Locale, t } from '../../../lib/i18n';
import { InterfacePreferences } from '../../components/LocaleProvider';
import styles from './ProjectShell.module.css';

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
  const technical = pathname === `${base}/technical`;

  useEffect(() => {
    if (!mobile || !expanded) return;
    const first =
      sidebar.current?.querySelector<HTMLAnchorElement>('[aria-current="page"]') ??
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
          <Link href="/">{t(locale, 'app.back')}</Link>
          {name && pathname !== base && (
            <>
              <span aria-hidden="true">/</span>
              <Link href={base} onNavigate={closeMobileMenu}>
                {name}
              </Link>
            </>
          )}
          {indexing && (
            <>
              <span aria-hidden="true">/</span>
              <Link href={`${base}/technical`} onNavigate={closeMobileMenu}>
                {t(locale, 'nav.technical')}
              </Link>
            </>
          )}
        </div>
        <InterfacePreferences />
      </div>
      {name && (
        <aside id="project-sidebar" ref={sidebar} hidden={!expanded} className={styles.sidebar}>
          <div className={styles.brand}>
            <Image src="/favicon-48x48.png" alt="" width={36} height={36} unoptimized />
            <strong>{t(locale, 'app.brand')}</strong>
          </div>
          <p className={styles.projectName}>{name}</p>
          <nav className={styles.navigation} aria-label={t(locale, 'ui.sections')}>
            <ul>
              {sections.map((section) => {
                const href = section === 'overview' ? base : `${base}/${section}`;
                return (
                  <li key={section}>
                    <Link
                      href={href}
                      aria-current={pathname === href ? 'page' : undefined}
                      onNavigate={closeMobileMenu}
                    >
                      {sectionLabel(locale, section)}
                    </Link>
                  </li>
                );
              })}
              <li className={styles.technical}>
                <Link
                  href={`${base}/technical`}
                  aria-current={technical ? 'page' : indexing ? 'location' : undefined}
                  onNavigate={closeMobileMenu}
                >
                  {t(locale, 'nav.technical')}
                </Link>
                {(technical || indexing) && (
                  <ul>
                    <li>
                      <Link
                        href={`${base}/indexing`}
                        aria-current={indexing ? 'page' : undefined}
                        onNavigate={closeMobileMenu}
                      >
                        {t(locale, 'nav.indexing')}
                      </Link>
                    </li>
                  </ul>
                )}
              </li>
            </ul>
          </nav>
        </aside>
      )}
      <main className={styles.document}>{children}</main>
    </div>
  );
}
