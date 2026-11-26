'use client';

import { type FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { clientCache, clientSessionVersion, query } from '../../../../lib/client';
import {
  LatestProcessGraphRunDocument,
  type ProcessGraphRunFieldsFragment,
  StartBackgroundProcessGraphDocument,
} from '../../../../lib/generated';
import { fieldErrorMessage, type Locale, t } from '../../../../lib/i18n';
import { Latest } from '../../../../lib/latest';
import { useInvalidation } from '../../../../lib/live';
import { useProject } from '../ProjectShell';
import { ProcessGraph } from './ProcessGraph';
import { ProcessGraphGlossary } from './ProcessGraphGlossary';
import styles from './process-graphs.module.css';

type Result =
  | { status: 'preparing' | 'stale' | 'unavailable' | 'load_failed' }
  | ProcessGraphRunFieldsFragment;
type Term = { id: string; name: string };

const pendingStarts = new Map<string, Promise<ProcessGraphRunFieldsFragment>>();
const pollInterval = 2000;
const statusKeys: Record<string, string> = {
  preparing: 'processGraph.preparing',
  planning: 'processGraph.planning',
  load_failed: 'processGraph.loadFailed',
  ready: 'processGraph.activeBatch',
  insufficient: 'processGraph.insufficient',
  no_sources: 'processGraph.noSources',
  unavailable: 'processGraph.unavailable',
  stale: 'processGraph.stale',
  invalid: 'processGraph.invalidAnchor',
  not_found: 'processGraph.notFound',
};

function isActiveRun(status: string | undefined) {
  return status === 'planning' || status === 'ready';
}

export default function ProcessGraphsPage() {
  const { project, locale } = useProject();
  return <ProcessGraphBuilder key={project.id} projectId={project.id} locale={locale} />;
}

function ProcessGraphBuilder({ projectId, locale }: { projectId: string; locale: Locale }) {
  const [mode, setMode] = useState<'keyword' | 'term'>('keyword');
  const [keyword, setKeyword] = useState('');
  const [term, setTerm] = useState<Term>();
  const [result, setResult] = useState<Result | null>();
  const reader = useRef<AbortController | null>(null);
  const submission = useRef(new Latest());
  const startKey = JSON.stringify([clientSessionVersion(), projectId]);
  const glossaryTag = `project:${projectId}:glossary`;
  const tags = useMemo(
    () => [`project:${projectId}`, `project:${projectId}:requirements`, glossaryTag],
    [projectId, glossaryTag],
  );
  const glossarySnapshot = useRef({
    generation: clientCache.generation,
    revision: clientCache.revision,
  });
  const anchor = mode === 'keyword' ? keyword.trim() : term?.id;
  const invalid = result?.status === 'invalid';
  const running = result?.status === 'preparing' || isActiveRun(result?.status);

  const readLatest = useCallback(() => {
    reader.current?.abort();
    const controller = new AbortController();
    reader.current = controller;
    let timer: ReturnType<typeof setTimeout>;
    controller.signal.addEventListener('abort', () => clearTimeout(timer), { once: true });
    async function poll() {
      if (controller.signal.aborted) return;
      const { generation, revision } = clientCache;
      try {
        const { latestProcessGraphRun } = await query(
          LatestProcessGraphRunDocument,
          { projectId },
          { cache: false, signal: controller.signal },
        );
        if (controller.signal.aborted) return;
        if (
          generation === clientCache.generation &&
          !clientCache.invalidatedSince(revision, tags)
        ) {
          setResult(
            (previous) =>
              latestProcessGraphRun ??
              (previous?.status === 'unavailable' && !('run' in previous) ? previous : null),
          );
          if (!isActiveRun(latestProcessGraphRun?.status)) return;
        }
      } catch {
        if (controller.signal.aborted) return;
        setResult((previous) =>
          !previous || previous.status === 'preparing' ? { status: 'load_failed' } : previous,
        );
      }
      timer = setTimeout(poll, pollInterval);
    }

    // A route opened during source selection can observe the same in-flight start.
    const pending = pendingStarts.get(startKey);
    if (pending) {
      setResult({ status: 'preparing' });
      void pending.then(poll, poll);
    } else {
      void poll();
    }
  }, [projectId, startKey, tags]);

  useInvalidation(
    tags,
    () => {
      submission.current.invalidate();
      const next = { generation: clientCache.generation, revision: clientCache.revision };
      if (
        next.generation !== glossarySnapshot.current.generation ||
        clientCache.invalidatedSince(glossarySnapshot.current.revision, [glossaryTag])
      ) {
        setTerm(undefined);
      }
      glossarySnapshot.current = next;
      setResult((previous) => previous && { status: 'stale' });
      readLatest();
    },
    () => {
      setResult(undefined);
      setKeyword('');
      setTerm(undefined);
      setMode('keyword');
    },
  );

  useEffect(() => {
    readLatest();
    const current = submission.current;
    return () => {
      reader.current?.abort();
      current.invalidate();
    };
  }, [readLatest]);

  function clearFeedback() {
    setResult((previous) =>
      previous && 'fieldErrors' in previous && !previous.run ? null : previous,
    );
  }

  async function build(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!anchor || result === undefined || running || pendingStarts.has(startKey)) return;
    reader.current?.abort();
    const requestVersion = submission.current.begin();
    const { generation, revision } = clientCache;
    const changed = () =>
      generation !== clientCache.generation || clientCache.invalidatedSince(revision, tags);
    setResult({ status: 'preparing' });
    const input = {
      projectId,
      ...(mode === 'keyword' ? { keyword: anchor } : { termId: anchor }),
    };
    // The server owns the run. Keep the dispatched start alive across route changes.
    const pending = query(
      StartBackgroundProcessGraphDocument,
      { input },
      { tags: [], notify: false },
    )
      .then(({ startBackgroundProcessGraph }) => startBackgroundProcessGraph)
      .finally(() => pendingStarts.delete(startKey));
    pendingStarts.set(startKey, pending);
    try {
      const response = await pending;
      if (!submission.current.isCurrent(requestVersion)) return;
      const stale = changed();
      setResult(stale ? { status: 'stale' } : response);
      if (stale || isActiveRun(response.status)) readLatest();
    } catch {
      if (!submission.current.isCurrent(requestVersion)) return;
      setResult({ status: changed() ? 'stale' : 'unavailable' });
      readLatest();
    }
  }

  const draft = result?.status === 'suggested' && 'draft' in result && result.draft;
  const fieldError = invalid && 'fieldErrors' in result ? result.fieldErrors[0] : undefined;
  const statusKey =
    result &&
    (draft ? 'processGraph.ready' : (statusKeys[result.status] ?? 'processGraph.unavailable'));
  const progress = result && result.status !== 'stale' && 'run' in result ? result.run : undefined;
  const activeBatch = result?.status === 'ready' && progress;

  return (
    <>
      <section>
        <h2>{t(locale, 'nav.process-graphs')}</h2>
        <form onSubmit={build}>
          <fieldset className={styles.modes}>
            <legend>{t(locale, 'processGraph.anchor')}</legend>
            <label>
              <input
                type="radio"
                name="process-graph-anchor"
                checked={mode === 'keyword'}
                onChange={() => {
                  clearFeedback();
                  setMode('keyword');
                }}
              />
              {t(locale, 'processGraph.keywordMode')}
            </label>
            <label>
              <input
                type="radio"
                name="process-graph-anchor"
                checked={mode === 'term'}
                onChange={() => {
                  clearFeedback();
                  setMode('term');
                }}
              />
              {t(locale, 'processGraph.termMode')}
            </label>
          </fieldset>
          {mode === 'keyword' ? (
            <label>
              {t(locale, 'processGraph.keyword')}
              <input
                value={keyword}
                aria-invalid={invalid || undefined}
                aria-describedby={invalid ? 'process-graph-status' : undefined}
                onChange={(event) => {
                  clearFeedback();
                  setKeyword(event.target.value);
                }}
              />
            </label>
          ) : (
            <ProcessGraphGlossary
              projectId={projectId}
              locale={locale}
              tag={glossaryTag}
              selected={term}
              select={(next) => {
                clearFeedback();
                setTerm(next);
              }}
            />
          )}
          <button type="submit" disabled={!anchor || result === undefined || running}>
            {t(locale, 'processGraph.build')}
          </button>
          <output id="process-graph-status" className={styles.status}>
            {progress?.topic && (
              <span className={styles.topic}>
                {t(locale, 'processGraph.topic', { topic: progress.topic })}
              </span>
            )}
            {fieldError
              ? fieldErrorMessage(locale, fieldError)
              : statusKey &&
                t(
                  locale,
                  statusKey,
                  activeBatch
                    ? { batch: activeBatch.completed + 1, total: activeBatch.total }
                    : undefined,
                )}
            {!!progress?.total && (
              <span>
                {' '}
                {t(locale, 'processGraph.completedBatches', {
                  completed: progress.completed,
                  total: progress.total,
                })}
              </span>
            )}
            {!!progress?.failed && (
              <span> {t(locale, 'processGraph.failedBatches', { failed: progress.failed })}</span>
            )}
          </output>
        </form>
      </section>
      {draft && <ProcessGraph draft={draft} projectId={projectId} locale={locale} />}
    </>
  );
}
