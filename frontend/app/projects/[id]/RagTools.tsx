'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import {
  IndexRequirementDocument,
  RequirementIndexListDocument,
  type RequirementIndexListQuery,
  SearchRequirementsDocument,
  type SearchRequirementsQuery,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';
import { indexStatus, requirementURL } from '../../../lib/rag';
import Group from '../../components/Group';
import ResourceState from '../../components/ResourceState';

type Index = RequirementIndexListQuery['requirementIndex'][number];
export function useProjectIndex(projectId: string, page: number) {
  const [entries, setEntries] = useState<Index[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedContext, setLoadedContext] = useState('');
  const [busy, setBusy] = useState<string>();
  const [error, setError] = useState(false);
  const latest = useRef(new Latest());
  const active = useRef(true);
  const context = useRef(`${projectId}:${page}`);
  context.current = `${projectId}:${page}`;
  const load = useCallback(async () => {
    const generation = latest.current.begin();
    setLoading(true);
    try {
      const data = await query(RequirementIndexListDocument, {
        projectId,
        limit: 20,
        offset: page * 20,
      });
      if (latest.current.isCurrent(generation)) {
        setEntries(data.requirementIndex);
        setLoaded(true);
        setLoadedContext(`${projectId}:${page}`);
        setError(false);
      }
    } catch {
      if (latest.current.isCurrent(generation)) setError(true);
    } finally {
      if (latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, page]);
  useEffect(() => {
    active.current = true;
    setEntries([]);
    setLoaded(false);
    setBusy(undefined);
    void load();
    const state = latest.current;
    return () => {
      active.current = false;
      state.invalidate();
    };
  }, [load]);
  useInvalidation(
    [`project:${projectId}`, `project:${projectId}:requirements`],
    () => void load(),
    () => {
      latest.current.invalidate();
      setEntries([]);
      setLoaded(false);
      setBusy(undefined);
      setError(false);
    },
  );
  const index = async (requirementId: string) => {
    const requestContext = context.current;
    setBusy(requirementId);
    setError(false);
    try {
      const data = await query(IndexRequirementDocument, { projectId, requirementId });
      if (active.current && context.current === requestContext)
        setEntries((items) => [
          ...items.filter((item) => item.requirementId !== requirementId),
          data.indexRequirement,
        ]);
    } catch {
      if (active.current && context.current === requestContext) setError(true);
    } finally {
      if (active.current && context.current === requestContext) setBusy(undefined);
    }
  };
  return {
    entries,
    busy,
    error,
    index,
    loading,
    loaded: loaded && loadedContext === `${projectId}:${page}`,
    reload: load,
  };
}
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
  const status = busy === requirementId ? 'pending' : indexStatus(entry, version);
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
      <button type="button" disabled={!!busy} onClick={() => void index(requirementId)}>
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
        <button type="button" disabled={busy || !text.trim()}>
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
              <p>{hit.text}</p>
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
                <p>{hit.sourceText}</p>
              </details>
            </article>
          ))}
        </Group>
      )}
    </section>
  );
}
