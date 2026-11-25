'use client';
import Image from 'next/image';
import Link from 'next/link';
import {
  type ComponentProps,
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import { type Locale, t } from '../../../lib/i18n';
import brandIcon from '../../../public/favicon-48x48.png';
import { DesignIcon } from '../../components/DesignIcon';
import { InterfacePreferences } from '../../components/LocaleProvider';
import styles from './ProjectShell.module.css';
import { ProjectSwitcher } from './ProjectSwitcher';

export const sections = [
  'overview',
  'requirements',
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
  const [help, setHelp] = useState<string | null>(null);
  const helpPrefix = useId();
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
      if (
        event.defaultPrevented ||
        event.key !== 'Escape' ||
        document.querySelector('dialog[open]')
      )
        return;
      setMobilePath(null);
      toggle.current?.focus({ preventScroll: true });
    };
    document.addEventListener('keydown', dismiss);
    return () => document.removeEventListener('keydown', dismiss);
  }, [mobile, expanded]);
  useEffect(() => {
    if (!expanded || !help) return;
    function dismissHelp(event: KeyboardEvent) {
      if (event.key !== 'Escape' || document.querySelector('dialog[open]')) return;
      event.preventDefault();
      setHelp(null);
    }
    document.addEventListener('keydown', dismissHelp, true);
    return () => document.removeEventListener('keydown', dismissHelp, true);
  }, [expanded, help]);

  function onNavigate() {
    setHelp(null);
    if (mobile && expanded) {
      setMobilePath(null);
      toggle.current?.focus({ preventScroll: true });
    }
  }
  function sectionEntry(
    href: string,
    name: string,
    description: string,
    icon: ComponentProps<typeof DesignIcon>['name'],
    current?: 'page' | 'location',
  ) {
    const tooltip = `${helpPrefix}-${href.slice(base.length)}`;
    const dismiss = () => setHelp((active) => (active === href ? null : active));
    return (
      <div className={styles.navRow}>
        <Link href={href} aria-current={current} onNavigate={onNavigate}>
          <DesignIcon name={icon} />
          {name}
          {mobile && <DesignIcon name="next" />}
        </Link>
        <div
          className={styles.help}
          onPointerEnter={(event) => {
            if (event.pointerType !== 'touch') setHelp(href);
          }}
          onPointerLeave={(event) => {
            if (!event.currentTarget.matches(':focus-within')) dismiss();
          }}
        >
          <button
            type="button"
            className={styles.helpIcon}
            aria-label={t(locale, 'ui.sectionHelpLabel', { section: name })}
            aria-describedby={tooltip}
            onFocus={() => setHelp(href)}
            onBlur={(event) => {
              if (!event.currentTarget.parentElement?.matches(':hover')) dismiss();
            }}
          >
            <DesignIcon name="help" />
          </button>
          <span id={tooltip} role="tooltip" className={styles.helpText} hidden={help !== href}>
            {description}
          </span>
        </div>
      </div>
    );
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
            onClick={() => {
              setHelp(null);
              if (mobile) setMobilePath(expanded ? null : pathname);
              else setDesktopOpen(!desktopOpen);
            }}
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
              <Link href={base} onNavigate={onNavigate}>
                {name}
              </Link>
            </>
          )}
          {(indexing || history) && (
            <>
              <span aria-hidden="true">/</span>
              <Link href={`${base}/technical`} onNavigate={onNavigate}>
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
                    {sectionEntry(
                      href,
                      sectionLabel(locale, section),
                      t(locale, `ui.${section}Help`),
                      section,
                      pathname === href
                        ? 'page'
                        : section === 'analysis' && externalProposals
                          ? 'location'
                          : undefined,
                    )}
                  </li>
                );
              })}
              <li className={styles.technical}>
                {sectionEntry(
                  `${base}/technical`,
                  t(locale, 'nav.technical'),
                  t(locale, 'ui.technicalHelp'),
                  'technical',
                  technical ? 'page' : indexing || history ? 'location' : undefined,
                )}
                {(technical || indexing || history) && (
                  <ul>
                    <li>
                      {sectionEntry(
                        `${base}/indexing`,
                        t(locale, 'nav.indexing'),
                        t(locale, 'ui.indexingHelp'),
                        'indexing',
                        indexing ? 'page' : undefined,
                      )}
                    </li>
                    <li>
                      {sectionEntry(
                        `${base}/technical/analysis-history`,
                        t(locale, 'analysis.history'),
                        t(locale, 'analysis.historyHelp'),
                        'analysis',
                        history ? 'page' : undefined,
                      )}
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
