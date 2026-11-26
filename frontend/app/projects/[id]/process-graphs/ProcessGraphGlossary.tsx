'use client';

import { useCallback, useRef, useState } from 'react';
import { query } from '../../../../lib/client';
import { GlossaryTermPagesDocument } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import { DesignIcon } from '../../../components/DesignIcon';
import rowStyles from '../../../components/RecordIcons.module.css';
import { GlossaryListView } from '../glossary/GlossaryListView';
import styles from './process-graphs.module.css';

type Term = { id: string; name: string };

export function ProcessGraphGlossary({
  projectId,
  locale,
  tag,
  selected,
  select,
}: {
  projectId: string;
  locale: Locale;
  tag: string;
  selected?: Term;
  select: (term: Term) => void;
}) {
  const [search, setSearch] = useState('');
  const picker = useRef<HTMLDetailsElement>(null);
  const summary = useRef<HTMLElement>(null);
  const load = useCallback(
    async (after: string | null) =>
      (await query(GlossaryTermPagesDocument, { projectId, search, after }, { tags: [tag] }))
        .glossaryTermsConnection,
    [projectId, search, tag],
  );

  return (
    <details ref={picker} open={!selected}>
      <summary ref={summary}>
        {selected
          ? t(locale, 'processGraph.selected', { term: selected.name })
          : t(locale, 'processGraph.chooseTerm')}
      </summary>
      <label>
        {t(locale, 'glossary.search')}
        <span className={rowStyles.searchField}>
          <DesignIcon name="search" className={rowStyles.searchIcon} />
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') event.preventDefault();
            }}
          />
        </span>
      </label>
      <GlossaryListView
        key={search}
        locale={locale}
        load={load}
        identity={JSON.stringify(['process-graph-glossary', projectId, search])}
        tags={[tag]}
        empty={t(locale, 'glossary.empty')}
        render={(term) => (
          <button
            type="button"
            className={styles.term}
            aria-pressed={selected?.id === term.id}
            onClick={() => {
              select({ id: term.id, name: term.name });
              if (picker.current) picker.current.open = false;
              summary.current?.focus();
            }}
          >
            {term.name}
          </button>
        )}
      />
    </details>
  );
}
