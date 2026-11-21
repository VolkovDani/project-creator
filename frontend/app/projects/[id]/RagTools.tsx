'use client';
import { useEffect, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import {
  type RequirementIndexByIdsQuery,
  SearchRequirementsDocument,
  type SearchRequirementsQuery,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';
import { indexStatus, requirementURL } from '../../../lib/rag';
import Group from '../../components/Group';
import ResourceState from '../../components/ResourceState';
import { GlossaryText } from './GlossaryActions';

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
export default function ProjectSearch({
  projectId,
  locale,
  onSource,
}: {
  projectId: string;
  locale: Locale;
  onSource?: (id: string) => Promise<void>;
}) {
  const [text, setText] = useState('');
  const [results, setResults] = useState<SearchRequirementsQuery['searchRequirements']>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [stale, setStale] = useState(false);
  const latest = useRef(new Latest());
  useEffect(() => {
    const state = latest.current;
    void projectId;
    setResults(undefined);
    return () => state.invalidate();
  }, [projectId]);
  useInvalidation(
    [`project:${projectId}`, `project:${projectId}:requirements`],
    () => {
      latest.current.invalidate();
      setStale(true);
      setBusy(false);
    },
    () => {
      latest.current.invalidate();
      setText('');
      setResults(undefined);
      setError(false);
      setStale(false);
      setBusy(false);
    },
  );
  async function search(event: React.FormEvent) {
    event.preventDefault();
    const generation = latest.current.begin();
    setBusy(true);
    setError(false);
    try {
      const data = await query(SearchRequirementsDocument, { projectId, text, limit: 10 });
      if (latest.current.isCurrent(generation)) {
        setResults(data.searchRequirements);
        setStale(false);
      }
    } catch {
      if (latest.current.isCurrent(generation)) setError(true);
    } finally {
      if (latest.current.isCurrent(generation)) setBusy(false);
    }
  }
  return (
    <section>
      <h2>{t(locale, 'rag.searchTitle')}</h2>
      <p>{t(locale, 'rag.searchHelp')}</p>
      <form id="semantic-search-form" onSubmit={search}>
        <label>
          {t(locale, 'rag.query')}
          <textarea value={text} onChange={(event) => setText(event.target.value)} />
        </label>
        <button type="submit" disabled={busy || !text.trim()}>
          {t(locale, busy ? 'rag.searching' : 'rag.search')}
        </button>
      </form>
      <ResourceState
        locale={locale}
        loading={busy}
        error={error}
        hasData={results !== undefined}
        retry={() => {
          (
            document.getElementById('semantic-search-form') as HTMLFormElement | null
          )?.requestSubmit();
        }}
      />
      {stale && <p>{t(locale, 'rag.stale')}</p>}
      {results?.length === 0 && <p>{t(locale, 'rag.noResults')}</p>}
      {results && (
        <Group
          id={`search:${projectId}`}
          title={t(locale, 'ui.searchResults', { count: results.length })}
        >
          {results.map((hit) => (
            <article className="card" key={`${hit.requirementId}:${hit.start}:${hit.end}`}>
              <GlossaryText sourceId={hit.requirementId} version={hit.version}>
                {hit.text}
              </GlossaryText>
              <p className="muted">{t(locale, 'requirements.version', { version: hit.version })}</p>
              <a
                href={requirementURL(projectId, hit.requirementId)}
                onClick={
                  onSource
                    ? (event) => {
                        event.preventDefault();
                        void onSource(hit.requirementId);
                      }
                    : undefined
                }
              >
                {t(locale, 'rag.source')}
              </a>
              <details>
                <summary>{t(locale, 'rag.sourceText')}</summary>
                <GlossaryText sourceId={hit.requirementId} version={hit.version}>
                  {hit.sourceText}
                </GlossaryText>
              </details>
            </article>
          ))}
        </Group>
      )}
    </section>
  );
}
