'use client';
import Link from 'next/link';
import { useCallback, useRef, useState } from 'react';
import { query } from '../../../../lib/client';
import {
  GlossaryCandidatePagesDocument,
  GlossaryTermPagesDocument,
} from '../../../../lib/generated';
import { t } from '../../../../lib/i18n';
import { requirementURL } from '../../../../lib/rag';
import { DesignIcon } from '../../../components/DesignIcon';
import Group from '../../../components/Group';
import rowStyles from '../../../components/RecordIcons.module.css';
import GlossaryEditor, { type GlossaryDraft } from '../GlossaryEditor';
import { useProject } from '../ProjectShell';
import layout from '../requirements/requirements.module.css';
import { useNarrowWorkspace } from '../useNarrowWorkspace';
import CandidateExcerpt from './CandidateExcerpt';
import { GlossaryDetails } from './GlossaryDetails';
import { GlossaryListRows, GlossaryListView, useGlossaryList } from './GlossaryListView';
import styles from './glossary.module.css';

export default function GlossaryPage() {
  const { project } = useProject();
  return <Glossary key={project.id} />;
}

function Glossary() {
  const { project, locale } = useProject();
  const projectId = project.id;
  const tag = `project:${projectId}:glossary`;
  const reqTag = `project:${projectId}:requirements`;
  const narrow = useNarrowWorkspace();
  const [search, setSearch] = useState('');
  const [draft, setDraft] = useState<GlossaryDraft>();
  const [selectedId, setSelectedId] = useState<string>();
  const rowTrigger = useRef<HTMLButtonElement>(null);
  const searchInput = useRef<HTMLInputElement>(null);
  const load = useCallback(
    async (after: string | null) =>
      (await query(GlossaryTermPagesDocument, { projectId, search, after }, { tags: [tag] }))
        .glossaryTermsConnection,
    [projectId, search, tag],
  );
  const scan = useCallback(
    async (after: string | null) => {
      const page = (
        await query(GlossaryCandidatePagesDocument, { projectId, after }, { tags: [tag, reqTag] })
      ).glossaryCandidatesConnection;
      return {
        ...page,
        items: page.items.map((item) => ({
          ...item,
          id: JSON.stringify([item.sourceId, item.name, item.abbreviation]),
        })),
      };
    },
    [projectId, tag, reqTag],
  );
  const identity = JSON.stringify(['glossary', projectId, search]);
  const terms = useGlossaryList({ identity, tags: [tag], load });
  const selected = terms.rows.find((term) => term.id === selectedId);

  return (
    <div className={layout.layout}>
      <nav className={layout.contents} aria-label={t(locale, 'ui.onThisPage')}>
        <details open>
          <summary>{t(locale, 'ui.onThisPage')}</summary>
          <a href="#glossary-search">{t(locale, 'glossary.search')}</a>
          <a href="#glossary-concepts">{t(locale, 'glossary.concepts')}</a>
          <a href="#glossary-candidates">{t(locale, 'glossary.candidates')}</a>
        </details>
      </nav>
      <section aria-labelledby="glossary-heading">
        <div className={layout.heading}>
          <h2 id="glossary-heading">{t(locale, 'nav.glossary')}</h2>
          <button type="button" className="primary-action" onClick={() => setDraft({ name: '' })}>
            {t(locale, 'glossary.new')}
          </button>
        </div>
        <label className={styles.search}>
          {t(locale, 'glossary.search')}
          <span className={rowStyles.searchField}>
            <DesignIcon name="search" className={rowStyles.searchIcon} />
            <input
              id="glossary-search"
              ref={searchInput}
              type="search"
              value={search}
              onChange={(event) => {
                setSelectedId(undefined);
                setSearch(event.target.value);
              }}
            />
          </span>
        </label>
        <div className={layout.workspace}>
          <div className={layout.listPane}>
            <Group
              id={`${projectId}:glossary:concepts`}
              anchorId="glossary-concepts"
              title={t(locale, 'glossary.concepts')}
            >
              <GlossaryListRows
                key={identity}
                locale={locale}
                list={terms}
                empty={t(locale, 'glossary.empty')}
                focusedId={selected?.id}
                estimatedRowHeight={60}
                render={(term) => (
                  <button
                    type="button"
                    className={layout.row}
                    aria-pressed={selected?.id === term.id}
                    onClick={(event) => {
                      rowTrigger.current = event.currentTarget;
                      setSelectedId(term.id);
                    }}
                  >
                    <DesignIcon name="glossary" className={rowStyles.desktopIcon} />
                    {term.name}
                  </button>
                )}
              />
            </Group>
          </div>
          {selected && (
            <GlossaryDetails
              key={selected.id}
              projectId={projectId}
              locale={locale}
              term={selected}
              narrow={narrow}
              onClose={() => {
                setSelectedId(undefined);
                rowTrigger.current?.focus({ preventScroll: true });
              }}
              onDeleted={() => {
                setSelectedId(undefined);
                requestAnimationFrame(() => searchInput.current?.focus({ preventScroll: true }));
              }}
            />
          )}
        </div>
        <Group
          id={`${projectId}:glossary:candidates`}
          anchorId="glossary-candidates"
          title={t(locale, 'glossary.candidates')}
        >
          <GlossaryListView
            locale={locale}
            identity={JSON.stringify(['glossaryCandidates', projectId])}
            tags={[reqTag, tag]}
            load={scan}
            empty={t(locale, 'glossary.noCandidates')}
            render={(c) => (
              <article className="card" key={c.id}>
                <button
                  type="button"
                  onClick={() => setDraft({ name: c.name, variant: c.abbreviation, candidate: c })}
                >
                  {c.name ? `${c.name} (${c.abbreviation})` : c.abbreviation}
                </button>
                <p className="user-content">
                  <CandidateExcerpt candidate={c} />
                </p>
                <Link href={requirementURL(projectId, c.sourceId, c)}>
                  {t(locale, 'glossary.openMention')}
                </Link>
              </article>
            )}
          />
        </Group>
        {draft && (
          <GlossaryEditor
            draft={draft}
            projectId={projectId}
            locale={locale}
            onClose={() => setDraft(undefined)}
          />
        )}
      </section>
    </div>
  );
}
