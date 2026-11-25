'use client';
import Link from 'next/link';
import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { clientCache, clientSessionVersion, query } from '../../../lib/client';
import {
  type RequirementIndexByIdsQuery,
  RequirementMatchesDocument,
  SearchRequirementsDocument,
  type SearchRequirementsQuery,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInvalidation } from '../../../lib/live';
import { indexStatus, requirementURL } from '../../../lib/rag';
import { useInfinite } from '../../../lib/useInfinite';
import { DesignIcon } from '../../components/DesignIcon';
import { LoadMore } from '../../components/ListControls';
import rowStyles from '../../components/RecordIcons.module.css';
import ResourceState from '../../components/ResourceState';
import { GlossaryText } from './GlossaryActions';
import styles from './requirements/requirements.module.css';

type Index = RequirementIndexByIdsQuery['requirementIndexByIds'][number];
export function RequirementIndexAction({
  entry,
  requirementId,
  version,
  locale,
  busy,
  index,
}: {
  entry?: Index;
  requirementId: string;
  version: number;
  locale: Locale;
  busy?: string;
  index: (id: string) => Promise<void>;
}) {
  const status = busy === requirementId ? 'running' : indexStatus(entry, version);
  return (
    <div className="muted">
      <p>{t(locale, 'rag.indexStatus', { status: t(locale, `rag.status.${status}`) })}</p>
      {entry?.version === version && entry.errorCode && (
        <p>
          {t(
            locale,
            entry.errorCode === 'NOT_CONFIGURED' ? 'rag.notConfigured' : 'rag.indexFailed',
          )}
        </p>
      )}
      <button
        type="button"
        disabled={!!busy || status === 'running'}
        onClick={() => void index(requirementId)}
      >
        {t(
          locale,
          busy === requirementId ? 'rag.indexing' : status === 'failed' ? 'rag.retry' : 'rag.index',
        )}
      </button>
    </div>
  );
}
function LiteralResults({
  projectId,
  locale,
  match,
  session,
  onSource,
}: {
  projectId: string;
  locale: Locale;
  match: string;
  session: number;
  onSource?: (id: string) => void;
}) {
  const id = useId();
  const load = useCallback(
    async (after: string | null) => {
      if (clientSessionVersion() !== session) throw Error('session changed');
      const data = await query(
        RequirementMatchesDocument,
        { projectId, match, first: 40, after },
        { cache: false },
      );
      return data.requirementsConnection;
    },
    [projectId, match, session],
  );
  const list = useInfinite({
    identity: `requirement-matches:${projectId}:${id}`,
    tags: [`project:${projectId}`, `project:${projectId}:requirements`],
    load,
  });
  const more = useCallback(() => {
    void list.loadMore().catch(() => {});
  }, [list.loadMore]);
  return (
    <section aria-labelledby={id}>
      <h4 id={id}>{t(locale, 'rag.literalTitle')}</h4>
      <ResourceState
        locale={locale}
        loading={list.loading}
        error={list.error}
        hasData={list.loaded}
        retry={() => void list.retry().catch(() => {})}
      />
      <div aria-live="polite">
        {list.loaded && !list.error && (
          <p className="muted">
            {list.rows.length === 0
              ? t(locale, 'rag.literalEmpty')
              : t(locale, 'ui.searchResults', { count: list.totalCount })}
          </p>
        )}
      </div>
      {list.rows.map((hit) => (
        <article className="card" key={hit.id}>
          <GlossaryText sourceId={hit.id} version={hit.version}>
            {hit.text}
          </GlossaryText>
          <p className="muted">
            {t(locale, 'requirements.metaId')} <code>{hit.id}</code> ·{' '}
            {t(locale, 'requirements.version', { version: hit.version })}
            {hit.archivedAt && <> · {t(locale, 'requirements.statusArchived')}</>}
          </p>
          <Link
            href={requirementURL(projectId, hit.id)}
            onNavigate={() => onSource?.(hit.id)}
            prefetch={false}
            scroll={false}
          >
            {t(locale, 'rag.source')}
          </Link>
        </article>
      ))}
      <LoadMore locale={locale} {...list} observe loadMore={more} />
    </section>
  );
}

type SemanticSearch =
  | { status: 'idle' | 'loading' | 'error' | 'stale' }
  | { status: 'ready'; hits: SearchRequirementsQuery['searchRequirements'] };

export default function ProjectSearch({
  projectId,
  locale,
  onSource,
}: {
  projectId: string;
  locale: Locale;
  onSource?: (id: string) => void;
}) {
  const title = useId();
  const semanticTitle = useId();
  const [text, setText] = useState('');
  const [submitted, setSubmitted] = useState<{ match: string; attempt: number; session: number }>();
  const [semantic, setSemantic] = useState<SemanticSearch>({ status: 'idle' });
  const request = useRef<AbortController>(null);
  const attempt = useRef(0);
  const tags = [`project:${projectId}`, `project:${projectId}:requirements`];
  useEffect(() => () => request.current?.abort(), []);
  function clearResults() {
    request.current?.abort();
    setSubmitted(undefined);
    setSemantic({ status: 'idle' });
  }
  useInvalidation(
    tags,
    () => {
      request.current?.abort();
      setSemantic((current) => (current.status === 'idle' ? current : { status: 'stale' }));
    },
    () => {
      setText('');
      clearResults();
    },
  );
  async function search(event: React.FormEvent) {
    event.preventDefault();
    const match = text.trim();
    if (!match) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const { signal } = controller;
    const revision = clientCache.revision;
    setSubmitted({ match, attempt: ++attempt.current, session: clientSessionVersion() });
    setSemantic({ status: 'loading' });
    try {
      const data = await query(
        SearchRequirementsDocument,
        { projectId, text: match, limit: 10 },
        { signal },
      );
      if (!signal.aborted)
        setSemantic(
          clientCache.invalidatedSince(revision, tags)
            ? { status: 'stale' }
            : { status: 'ready', hits: data.searchRequirements },
        );
    } catch {
      if (!signal.aborted) setSemantic({ status: 'error' });
    }
  }
  return (
    <section id="requirement-search" aria-labelledby={title}>
      <h3 id={title}>{t(locale, 'rag.requirementsSearchTitle')}</h3>
      <search>
        <form className={styles.searchForm} onSubmit={search}>
          <label>
            {t(locale, 'rag.query')}
            <input
              type="search"
              value={text}
              onChange={(event) => {
                const value = event.target.value;
                setText(value);
                if (!value.trim()) clearResults();
              }}
            />
          </label>
          <button type="submit" disabled={!text.trim()}>
            {t(locale, 'rag.search')}
          </button>
        </form>
      </search>
      {submitted && (
        <div className={styles.searchResults}>
          <LiteralResults
            key={submitted.attempt}
            projectId={projectId}
            locale={locale}
            match={submitted.match}
            session={submitted.session}
            onSource={onSource}
          />
          <section aria-labelledby={semanticTitle}>
            <h4 id={semanticTitle}>{t(locale, 'rag.searchTitle')}</h4>
            <div aria-live="polite">
              {semantic.status === 'loading' && <p role="status">{t(locale, 'rag.searching')}</p>}
              {semantic.status === 'error' && (
                <p className="error" role="alert">
                  {t(locale, 'rag.searchFailed')}
                </p>
              )}
              {semantic.status === 'stale' && <p>{t(locale, 'rag.stale')}</p>}
              {semantic.status === 'ready' && (
                <p className="muted">
                  {semantic.hits.length === 0
                    ? t(locale, 'rag.noResults')
                    : t(locale, 'ui.searchResults', { count: semantic.hits.length })}
                </p>
              )}
            </div>
            {semantic.status === 'ready' &&
              semantic.hits.map((hit) => (
                <article className="card" key={`${hit.requirementId}:${hit.start}:${hit.end}`}>
                  <GlossaryText sourceId={hit.requirementId} version={hit.version}>
                    {hit.text}
                  </GlossaryText>
                  <p className="muted">
                    {t(locale, 'requirements.version', { version: hit.version })}
                  </p>
                  <Link
                    href={requirementURL(projectId, hit.requirementId)}
                    onNavigate={() => onSource?.(hit.requirementId)}
                    prefetch={false}
                    scroll={false}
                  >
                    {t(locale, 'rag.source')}
                  </Link>
                  <details>
                    <summary>
                      <DesignIcon name="requirements" className={rowStyles.desktopIcon} />
                      {t(locale, 'rag.sourceText')}
                    </summary>
                    <GlossaryText sourceId={hit.requirementId} version={hit.version}>
                      {hit.sourceText}
                    </GlossaryText>
                  </details>
                </article>
              ))}
          </section>
        </div>
      )}
    </section>
  );
}
