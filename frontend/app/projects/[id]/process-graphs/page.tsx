'use client';

import { type FormEvent, useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { clientCache, clientSessionVersion, query } from '../../../../lib/client';
import {
  LatestProcessGraphRunDocument,
  type ProcessGraphRunFieldsFragment,
  StartBackgroundProcessGraphDocument,
} from '../../../../lib/generated';
import { fieldErrorMessage, type Locale, t } from '../../../../lib/i18n';
import { Latest } from '../../../../lib/latest';
import {
  cachedProcessGraph,
  cacheProcessGraph,
  forgetProcessGraph,
  type ProcessGraphProgress,
  processGraphTag,
  useInvalidation,
  useProcessGraphProgress,
} from '../../../../lib/live';
import { useProject } from '../ProjectShell';
import { ProcessGraph } from './ProcessGraph';
import { ProcessGraphGlossary } from './ProcessGraphGlossary';
import styles from './process-graphs.module.css';

type Result =
  | { status: 'preparing' | 'stale' | 'unavailable' | 'load_failed' }
  | ProcessGraphRunFieldsFragment;
type Term = { id: string; name: string };

const pendingStarts = new Map<string, Promise<ProcessGraphRunFieldsFragment>>();
const startKeyForProject = (projectId: string) =>
  JSON.stringify([clientSessionVersion(), projectId]);
const statusKeys: Record<string, string> = {
  preparing: 'processGraph.preparing',
  planning: 'processGraph.planning',
  load_failed: 'processGraph.loadFailed',
  ready: 'processGraph.activeBatch',
  suggested: 'processGraph.ready',
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

type RunOrder = { current?: string; revisions: Map<string, number>; pushes: number };

function rememberRun(order: RunOrder, run: ProcessGraphProgress['run'], pushed = false) {
  const revision = order.revisions.get(run.id);
  if (
    revision !== undefined &&
    (run.id !== order.current || run.revision < revision || (pushed && revision === run.revision))
  )
    return false;
  order.current = run.id;
  order.revisions.set(run.id, run.revision);
  if (order.revisions.size > 32) {
    const oldest = order.revisions.keys().next().value;
    if (oldest !== undefined) order.revisions.delete(oldest);
  }
  if (pushed) order.pushes++;
  return true;
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
  const [showBuildHelp, setShowBuildHelp] = useState(false);
  const buildHelpId = useId();
  const reader = useRef<AbortController | null>(null);
  const submission = useRef(new Latest());
  const order = useRef<RunOrder>({ revisions: new Map(), pushes: 0 });
  const glossaryTag = `project:${projectId}:glossary`;
  const tags = useMemo(
    () => [`project:${projectId}`, `project:${projectId}:requirements`, glossaryTag],
    [projectId, glossaryTag],
  );
  const readTags = useMemo(() => [...tags, processGraphTag(projectId)], [projectId, tags]);
  const sourceSnapshot = useRef({
    generation: clientCache.generation,
    revision: clientCache.revision,
  });
  const anchor = mode === 'keyword' ? keyword.trim() : term?.id;
  const invalid = result?.status === 'invalid';
  const loadingDraft = result?.status === 'suggested' && 'draft' in result && !result.draft;
  const running = result?.status === 'preparing' || isActiveRun(result?.status) || loadingDraft;

  useEffect(() => {
    if (!running || !showBuildHelp) return;
    const dismiss = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setShowBuildHelp(false);
    };
    window.addEventListener('keydown', dismiss);
    return () => window.removeEventListener('keydown', dismiss);
  }, [running, showBuildHelp]);

  const readLatest = useCallback(() => {
    reader.current?.abort();
    const controller = new AbortController();
    reader.current = controller;
    let retried = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    controller.signal.addEventListener('abort', () => clearTimeout(retry), { once: true });
    async function load() {
      if (controller.signal.aborted) return;
      const cached = cachedProcessGraph(projectId);
      if (cached?.run && rememberRun(order.current, cached.run)) {
        setResult(cached);
        return;
      }
      const { generation, revision } = clientCache;
      const pushes = order.current.pushes;
      const isCurrent = () =>
        !controller.signal.aborted &&
        generation === clientCache.generation &&
        !clientCache.invalidatedSince(revision, readTags);
      try {
        const { latestProcessGraphRun } = await query(
          LatestProcessGraphRunDocument,
          { projectId },
          { cache: false, signal: controller.signal },
        );
        if (!isCurrent()) return;
        const run = latestProcessGraphRun?.run;
        if (pushes !== order.current.pushes && order.current.current !== run?.id) return;
        if (run && !rememberRun(order.current, run)) return;
        if (latestProcessGraphRun)
          cacheProcessGraph(projectId, latestProcessGraphRun, tags, revision);
        setResult(
          (previous) =>
            latestProcessGraphRun ??
            (previous?.status === 'unavailable' && !('run' in previous) ? previous : null),
        );
      } catch (error) {
        if (!isCurrent() || (error instanceof Error && error.message === 'unauthorized')) return;
        if (!retried) {
          retried = true;
          retry = setTimeout(() => {
            if (isCurrent()) void load();
          }, 400);
          return;
        }
        setResult((previous) =>
          !previous ||
          previous.status === 'preparing' ||
          (previous.status === 'suggested' && 'draft' in previous && !previous.draft)
            ? { status: 'load_failed' }
            : previous,
        );
      }
    }

    // A route opened during source selection can observe the same in-flight start.
    const pending = pendingStarts.get(startKeyForProject(projectId));
    if (pending) {
      setResult((previous) => previous ?? { status: 'preparing' });
      void pending.then(load, load);
    } else {
      void load();
    }
  }, [projectId, readTags, tags]);

  useInvalidation(
    readTags,
    () => {
      const next = { generation: clientCache.generation, revision: clientCache.revision };
      const previous = sourceSnapshot.current;
      const reset = next.generation !== previous.generation;
      if (reset || clientCache.invalidatedSince(previous.revision, [glossaryTag])) {
        setTerm(undefined);
      }
      sourceSnapshot.current = next;
      if (reset || clientCache.invalidatedSince(previous.revision, tags)) {
        submission.current.invalidate();
        setResult((result) => result && { status: 'stale' });
      } else {
        setResult((result) => (result?.status === 'suggested' ? null : result));
      }
      readLatest();
    },
    () => {
      reader.current?.abort();
      order.current = { revisions: new Map(), pushes: 0 };
      setResult(undefined);
      setKeyword('');
      setTerm(undefined);
      setMode('keyword');
    },
  );

  useProcessGraphProgress(
    projectId,
    tags,
    (progress) => {
      if (!rememberRun(order.current, progress.run, true)) return;
      setResult({ ...progress, draft: null, fieldErrors: [] });
      if (progress.status === 'suggested') readLatest();
    },
    readLatest,
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
    const startKey = startKeyForProject(projectId);
    if (!anchor || result === undefined || running || pendingStarts.has(startKey)) return;
    reader.current?.abort();
    forgetProcessGraph(projectId);
    const requestVersion = submission.current.begin();
    const { generation, revision } = clientCache;
    const pushes = order.current.pushes;
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
      if (changed()) {
        setResult({ status: 'stale' });
        readLatest();
        return;
      }
      if (pushes !== order.current.pushes && order.current.current !== response.run?.id) return;
      if (response.run && !rememberRun(order.current, response.run)) return;
      setResult(response);
    } catch {
      if (!submission.current.isCurrent(requestVersion)) return;
      if (changed() || pushes === order.current.pushes) {
        setResult({ status: changed() ? 'stale' : 'unavailable' });
        readLatest();
      }
    }
  }

  const draft = result?.status === 'suggested' && 'draft' in result && result.draft;
  const fieldError = invalid && 'fieldErrors' in result ? result.fieldErrors[0] : undefined;
  const statusKey =
    result &&
    (loadingDraft ? 'app.loading' : (statusKeys[result.status] ?? 'processGraph.unavailable'));
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
          <span
            className={styles.buildAction}
            onPointerEnter={(event) => {
              if (event.pointerType !== 'touch') setShowBuildHelp(true);
            }}
            onPointerLeave={(event) => {
              if (!event.currentTarget.matches(':focus-within')) setShowBuildHelp(false);
            }}
          >
            <button
              type="submit"
              disabled={!running && (!anchor || result === undefined)}
              aria-disabled={running || undefined}
              aria-describedby={running ? buildHelpId : undefined}
              onFocus={() => setShowBuildHelp(true)}
              onBlur={(event) => {
                if (!event.currentTarget.parentElement?.matches(':hover')) setShowBuildHelp(false);
              }}
            >
              {t(locale, 'processGraph.build')}
            </button>
            {running && (
              <span
                id={buildHelpId}
                role="tooltip"
                className={styles.buildHelp}
                hidden={!showBuildHelp}
              >
                {t(locale, 'processGraph.buildBusy')}
              </span>
            )}
          </span>
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
