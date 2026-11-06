'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  type AnalysisGroup,
  type AnalysisPairView,
  type AnalysisRun,
  canRetryPairs,
  groupFilter,
  groupPairs,
} from '../../../lib/analysis';
import {
  getLayaPools,
  layaPairErrorMessageKey,
  parseLayaPreclassification,
} from '../../../lib/analysisLaya';
import { acceptSnapshot } from '../../../lib/analysisLive';
import type { AnalysisSnapshot } from '../../../lib/analysisSnapshot';
import { analysisRules, validateAnalysisSelection } from '../../../lib/analysisValidation';
import { clientCache, query } from '../../../lib/client';
import {
  AnalysisHistoryDocument,
  type AnalysisHistoryQuery,
  AnalysisPairConnectionDocument,
  type AnalysisPairConnectionQuery,
  AnalysisPairDetailDocument,
  AnalysisPairPageDocument,
  AnalysisRunMetadataDocument,
  CancelProjectAnalysisDocument,
  ProjectAnalysisSummaryDocument,
  ReplaceProjectAnalysisDocument,
  RetryProjectAnalysisDocument,
  StartProjectAnalysisDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useAnalysisProgress, useInvalidation } from '../../../lib/live';
import { requirementURL } from '../../../lib/rag';
import { useInfinite } from '../../../lib/useInfinite';
import { useListMode } from '../../../lib/useListMode';
import { useResource } from '../../../lib/useResource';
import Group from '../../components/Group';
import { ListModeSelect, LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import { VirtualList } from '../../components/VirtualList';

const outcomes: Record<string, string> = {
  possible_collision: 'analysis.outcome.collision',
  insufficient: 'analysis.outcome.insufficient',
  no_collisions_found: 'analysis.outcome.none',
};
const reasonCodes = new Set([
  'INDEX_INCOMPLETE',
  'TOO_FEW_REQUIREMENTS',
  'REQUIREMENT_LIMIT',
  'ANALYSIS_LIMIT',
  'NO_CANDIDATES',
  'MODEL_INSUFFICIENT',
]);
const verdicts: Record<string, string> = {
  collision: 'analysis.verdict.collision',
  insufficient: 'analysis.verdict.insufficient',
  compatible: 'analysis.verdict.compatible',
  duplicate: 'analysis.verdict.duplicate',
};
const stages = new Set(['checking_index', 'finding_pairs', 'comparing']);
const analysisErrorMessages: Record<string, string> = {
  TIMEOUT: 'analysis.error.TIMEOUT',
  NPU_UNAVAILABLE: 'analysis.error.NPU_UNAVAILABLE',
  NPU_RECOVERY_FAILED: 'analysis.error.NPU_RECOVERY_FAILED',
  NPU_WORKER_EXIT: 'analysis.error.NPU_WORKER_EXIT',
  INVALID_OUTPUT: 'analysis.error.INVALID_OUTPUT',
  PROFILE_CHANGED: 'analysis.error.PROFILE_CHANGED',
  SOURCE_CHANGED: 'analysis.error.SOURCE_CHANGED',
};
type PairSummary = AnalysisPairConnectionQuery['analysisPairsConnection']['edges'][number]['node'];
type LiveRun = AnalysisRun & { revision: string };
function liveRun(snapshot: AnalysisSnapshot): LiveRun {
  return {
    ...snapshot,
    revision: snapshot.revision || '0',
    layaPendingPairs: snapshot.layaPendingPairs ?? 0,
    layaRunningPairs: snapshot.layaRunningPairs ?? 0,
    layaCompletedPairs: snapshot.layaCompletedPairs ?? 0,
    layaFailedPairs: snapshot.layaFailedPairs ?? 0,
    classifierProfile: snapshot.classifierProfile ?? '',
    generatorProvenance: '',
    generatorProfile: '',
    embeddingProfile: '',
    selectionProfile: '',
    sources: [],
    pairs: [],
    findings: [],
  };
}
type Source = AnalysisPairView['left'];
type SourceHandler = (id: string) => Promise<void>;

function SavedSource({
  source,
  locale,
  onSource,
  projectId,
}: {
  source: Source;
  locale: Locale;
  onSource?: SourceHandler;
  projectId: string;
}) {
  return (
    <div className="analysis-source">
      <p className="muted">
        {source.id} · {t(locale, 'requirements.version', { version: source.version })}
      </p>
      <p>{source.text}</p>
      <a
        href={requirementURL(projectId, source.id)}
        onClick={
          onSource
            ? (event) => {
                event.preventDefault();
                void onSource(source.id);
              }
            : undefined
        }
      >
        {t(locale, 'rag.source')}
      </a>
    </div>
  );
}

function PairGroup({
  group,
  run,
  projectId,
  locale,
  onSource,
}: {
  group: AnalysisGroup;
  run: AnalysisRun;
  projectId: string;
  locale: Locale;
  onSource?: SourceHandler;
}) {
  return (
    <Group
      id={`analysis:${projectId}:${group.key}`}
      title={t(locale, 'analysis.group.count', {
        name: t(locale, `analysis.group.${group.key}`),
        count: group.count,
      })}
      lazy
      defaultOpen={false}
    >
      <PairList group={group} run={run} projectId={projectId} locale={locale} onSource={onSource} />
    </Group>
  );
}

function AnalysisProvenance({
  runId,
  projectId,
  locale,
}: {
  runId: string;
  projectId: string;
  locale: Locale;
}) {
  const load = useCallback(
    async () =>
      (
        await query(
          AnalysisRunMetadataDocument,
          { projectId, analysisId: runId },
          { tags: [`project:${projectId}:analysis:${runId}:metadata`] },
        )
      ).projectAnalysis,
    [projectId, runId],
  );
  const resource = useResource(load, [`project:${projectId}:analysis:${runId}:metadata`]);
  return (
    <>
      <ResourceState
        locale={locale}
        loading={resource.loading}
        error={resource.error}
        hasData={resource.data !== undefined}
        retry={() => void resource.reload()}
      />
      {resource.data && (
        <>
          <pre>{resource.data.generatorProvenance || resource.data.generatorProfile}</pre>
          <p>
            {t(locale, 'analysis.selectionProfile', {
              profile: resource.data.selectionProfile,
              embedding: resource.data.embeddingProfile,
            })}
          </p>
        </>
      )}
    </>
  );
}

function PairDetails({
  pairId,
  runId,
  projectId,
  locale,
  version,
  onSource,
}: {
  pairId: string;
  runId: string;
  projectId: string;
  locale: Locale;
  version: string;
  onSource?: SourceHandler;
}) {
  const load = useCallback(
    async () =>
      (
        await query(
          AnalysisPairDetailDocument,
          { projectId, analysisId: runId, pairId },
          { tags: [`project:${projectId}:analysis:${runId}:pair:${pairId}`], version },
        )
      ).analysisPair,
    [projectId, runId, pairId, version],
  );
  const resource = useResource(load, [`project:${projectId}:analysis:${runId}:pair:${pairId}`]);
  const pair = resource.data;
  const preclassification = parseLayaPreclassification(pair?.preclassification);
  return (
    <>
      <ResourceState
        locale={locale}
        loading={resource.loading}
        error={resource.error}
        hasData={pair !== undefined}
        retry={() => void resource.reload()}
      />
      {pair && (
        <>
          {pair.layaStatus && (
            <p className="muted">
              {t(locale, 'analysis.laya.status', {
                status: t(locale, `analysis.laya.status.${pair.layaStatus}`),
              })}
              {pair.layaCheckedAt &&
                ` · ${t(locale, 'analysis.laya.checkedAt', { time: pair.layaCheckedAt })}`}
            </p>
          )}
          {preclassification && (
            <div>
              <p>{t(locale, 'analysis.laya.suggestion')}</p>
              <p>{t(locale, `analysis.laya.decision.${preclassification.decision}`)}</p>
              <p className="muted">
                {t(locale, 'analysis.laya.scores', {
                  related: preclassification.scores.related.toFixed(3),
                  unrelated: preclassification.scores.unrelated.toFixed(3),
                  uncertain: preclassification.scores.uncertain.toFixed(3),
                })}
              </p>
            </div>
          )}
          <SavedSource
            projectId={projectId}
            source={pair.left}
            locale={locale}
            onSource={onSource}
          />
          <SavedSource
            projectId={projectId}
            source={pair.right}
            locale={locale}
            onSource={onSource}
          />
          {pair.verdict === 'duplicate' && (
            <p>
              {t(
                locale,
                pair.resultOrigin === 'exact_text_v1'
                  ? 'analysis.duplicate.exact'
                  : 'analysis.duplicate.possible',
              )}
            </p>
          )}
          {pair.explanation && (
            <p>
              {pair.resultOrigin === 'exact_text_v1'
                ? t(locale, 'analysis.exactDuplicateExplanation')
                : pair.explanation}
            </p>
          )}
          {pair.selectionReasons.length > 0 && (
            <p>
              {t(locale, 'analysis.selectionReasons')}{' '}
              {pair.selectionReasons
                .map((reason) =>
                  t(
                    locale,
                    ['exact_text', 'semantic_similarity', 'shared_terms', 'all_pairs'].includes(
                      reason,
                    )
                      ? `analysis.selection.${reason}`
                      : 'analysis.selection.unknown',
                  ),
                )
                .join('; ')}
            </p>
          )}
          {pair.similarityScore != null && (
            <p>
              {t(locale, 'analysis.similarityScore', { score: pair.similarityScore.toFixed(4) })}
            </p>
          )}
          {pair.resultOrigin && (
            <p className="muted">
              {t(
                locale,
                pair.resultOrigin === 'exact_text_v1'
                  ? 'analysis.origin.exact'
                  : 'analysis.origin.model',
              )}
            </p>
          )}
          {pair.checkedAt && (
            <p className="muted">{t(locale, 'analysis.checkedAt', { time: pair.checkedAt })}</p>
          )}
          {pair.reusedFrom && (
            <p className="muted">{t(locale, 'analysis.reusedFrom', { id: pair.reusedFrom })}</p>
          )}
        </>
      )}
    </>
  );
}

function PairList({
  group,
  run,
  projectId,
  locale,
  onSource,
}: {
  group: AnalysisGroup;
  run: AnalysisRun;
  projectId: string;
  locale: Locale;
  onSource?: SourceHandler;
}) {
  const { mode, setMode } = useListMode();
  const loader = useCallback(
    async (after: string | null) =>
      (
        await query(
          AnalysisPairConnectionDocument,
          {
            projectId,
            analysisId: run.id,
            first: 40,
            after,
            ...groupFilter(group.key),
          },
          {
            tags: [
              `project:${projectId}:analysis:${run.id}:${group.key}`,
              `project:${projectId}:requirements`,
            ],
          },
        )
      ).analysisPairsConnection,
    [projectId, run.id, group.key],
  );
  const infinite = useInfinite({
    identity: `pairs:${projectId}:${run.id}:${group.key}`,
    tags: [
      `project:${projectId}:analysis:${run.id}:${group.key}`,
      `project:${projectId}:requirements`,
    ],
    load: loader,
    enabled: mode === 'infinite',
  });
  const more = useCallback(() => {
    if (!infinite.loading && !infinite.error && infinite.hasNextPage)
      void infinite.loadMore().catch(() => {});
  }, [infinite.loading, infinite.error, infinite.hasNextPage, infinite.loadMore]);
  const [offset, setOffset] = useState(0);
  const [pagedPairs, setPairs] = useState<PairSummary[]>([]);
  const [pageLoading, setLoading] = useState(true);
  const [pageError, setError] = useState(false);
  const latest = useRef(new Latest());
  const load = useCallback(async () => {
    if (mode !== 'pages') return;
    const generation = latest.current.begin();
    setLoading(true);
    try {
      const data = await query(
        AnalysisPairPageDocument,
        {
          projectId,
          analysisId: run.id,
          limit: 20,
          offset,
          ...groupFilter(group.key),
        },
        {
          tags: [
            `project:${projectId}:analysis:${run.id}:${group.key}`,
            `project:${projectId}:requirements`,
          ],
        },
      );
      if (latest.current.isCurrent(generation)) {
        setPairs(data.analysisPairs);
        setError(false);
      }
    } catch {
      if (latest.current.isCurrent(generation)) {
        setError(true);
      }
    } finally {
      if (latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, run.id, offset, group.key, mode]);
  useInvalidation(
    [`project:${projectId}:analysis:${run.id}:${group.key}`, `project:${projectId}:requirements`],
    () => void load(),
    () => {
      latest.current.invalidate();
      setPairs([]);
    },
  );
  useEffect(() => {
    void load();
    return () => latest.current.invalidate();
  }, [load]);
  useEffect(() => {
    if (offset >= group.count && offset > 0)
      setOffset(Math.max(0, Math.floor((group.count - 1) / 20) * 20));
  }, [group.count, offset]);
  const pairs = mode === 'pages' ? pagedPairs : infinite.rows;
  const loading = mode === 'pages' ? pageLoading : infinite.loading;
  const error = mode === 'pages' ? pageError : infinite.error;
  return (
    <>
      <ListModeSelect locale={locale} mode={mode} onChange={setMode} />
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={mode === 'pages' ? pagedPairs.length > 0 || !pageLoading : infinite.loaded}
        retry={() => {
          if (mode === 'pages') void load();
          else void infinite.retry().catch(() => {});
        }}
      />
      {!loading && !error && group.count === 0 && (
        <p className="muted">{t(locale, 'analysis.group.empty')}</p>
      )}
      <VirtualList
        identity={`pairs:${projectId}:${run.id}:${group.key}:${mode}:${offset}`}
        overscan={mode === 'pages' ? 20 : 4}
        items={pairs}
        keyOf={(pair) => pair.id}
        onEndReached={mode === 'infinite' ? more : undefined}
        restoreScroll={false}
        estimatedRowHeight={650}
        renderItem={(pair) => (
          <article
            className={`card analysis-pair${pair.status === 'running' ? ' analysis-pair-current' : ''}`}
            key={pair.id}
            data-pair-id={pair.id}
            aria-label={t(locale, 'analysis.pair.position', { number: pair.position + 1 })}
          >
            <h4>{t(locale, 'analysis.pair.position', { number: pair.position + 1 })}</h4>
            {pair.status === 'completed' && (
              <p>{t(locale, verdicts[pair.verdict] || 'analysis.verdict.insufficient')}</p>
            )}
            {(pair.status === 'pending' || pair.status === 'running') && (
              <p role="status">{t(locale, `analysis.pair.${pair.status}`)}</p>
            )}
            {pair.status === 'failed' && (
              <p className="error">
                {t(
                  locale,
                  layaPairErrorMessageKey(pair.errorCode) ||
                    (pair.errorCode === 'CANCELLED'
                      ? 'analysis.pair.cancelled'
                      : pair.errorCode === 'NOT_PROCESSED'
                        ? 'analysis.pair.notProcessed'
                        : 'analysis.pair.failed'),
                )}
              </p>
            )}
            <Group
              id={`analysis-pair:${run.id}:${pair.id}`}
              title={t(locale, 'analysis.details')}
              lazy
              defaultOpen={false}
            >
              <PairDetails
                pairId={pair.id}
                runId={run.id}
                projectId={projectId}
                locale={locale}
                version={`${pair.status}:${pair.checkedAt}:${pair.errorCode}:${pair.layaStatus}:${pair.layaCheckedAt}`}
                onSource={onSource}
              />
            </Group>
          </article>
        )}
      />
      {mode === 'infinite' && <LoadMore locale={locale} {...infinite} loadMore={more} />}
      {mode === 'pages' && group.count > 20 && (
        <nav
          aria-label={t(locale, 'analysis.pages', {
            name: t(locale, `analysis.group.${group.key}`),
          })}
        >
          <button
            type="button"
            disabled={loading || offset === 0}
            onClick={() => setOffset((value) => Math.max(0, value - 20))}
          >
            {t(locale, 'analysis.previous')}
          </button>
          <span>
            {t(locale, 'analysis.pageRange', {
              from: offset + 1,
              to: Math.min(offset + 20, group.count),
              total: group.count,
            })}
          </span>
          <button
            type="button"
            disabled={loading || offset + 20 >= group.count}
            onClick={() => setOffset((value) => value + 20)}
          >
            {t(locale, 'analysis.next')}
          </button>
        </nav>
      )}
    </>
  );
}

export default function Analysis({
  projectId,
  locale,
  onSource,
}: {
  projectId: string;
  locale: Locale;
  onSource?: SourceHandler;
}) {
  const [run, setRun] = useState<LiveRun | null>();
  const [selected, setSelected] = useState('');
  const [mode, setMode] = useState(analysisRules.defaultMode);
  const [threshold, setThreshold] = useState(String(analysisRules.defaultThreshold));
  const [replacementStrategy, setReplacementStrategy] = useState<'wait' | 'cancel'>('wait');
  const thresholdValue = threshold.trim() === '' ? NaN : Number(threshold);
  const selectionValidation = validateAnalysisSelection({ mode, threshold: thresholdValue });
  const validSelection = selectionValidation.errors.length === 0;
  const [historyOffset, setHistoryOffset] = useState(0);
  const [history, setHistory] = useState<AnalysisHistoryQuery['analysisHistory']>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(true);
  const latest = useRef(new Latest());
  const historyLatest = useRef(new Latest());
  const operation = useRef(0);
  const context = useRef(projectId);
  const active = useRef(true);
  if (context.current !== projectId) {
    context.current = projectId;
    operation.current++;
    latest.current.invalidate();
  }
  const load = useCallback(async () => {
    const generation = latest.current.begin();
    setLoading(true);
    const epoch = operation.current;
    try {
      const data = await query(
        ProjectAnalysisSummaryDocument,
        { projectId, analysisId: selected || null },
        { tags: [`project:${projectId}:analysis`, `project:${projectId}:requirements`] },
      );
      if (
        active.current &&
        context.current === projectId &&
        operation.current === epoch &&
        latest.current.isCurrent(generation)
      ) {
        setRun((current) =>
          data.projectAnalysis
            ? acceptSnapshot(current, liveRun(data.projectAnalysis))
            : (current ?? null),
        );
        setError(false);
      }
    } catch {
      if (active.current && operation.current === epoch && latest.current.isCurrent(generation)) {
        setError(true);
      }
    } finally {
      if (active.current && latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, selected]);
  useEffect(() => {
    active.current = true;
    operation.current++;
    setRun(undefined);
    setBusy(false);
    setError(false);
    void load();
    return () => {
      active.current = false;
      operation.current++;
      latest.current.invalidate();
    };
  }, [load]);
  const loadHistory = useCallback(async () => {
    const generation = historyLatest.current.begin();
    const epoch = operation.current;
    try {
      const data = await query(
        AnalysisHistoryDocument,
        { projectId, limit: 20, offset: historyOffset },
        { tags: [`project:${projectId}:analysis`, `project:${projectId}:requirements`] },
      );
      if (
        active.current &&
        context.current === projectId &&
        operation.current === epoch &&
        historyLatest.current.isCurrent(generation)
      )
        setHistory(data.analysisHistory);
    } catch {
      if (active.current && operation.current === epoch) setError(true);
    }
  }, [projectId, historyOffset]);
  useEffect(() => {
    void loadHistory();
    return () => historyLatest.current.invalidate();
  }, [loadHistory]);
  useInvalidation(
    [`project:${projectId}:analysis`, `project:${projectId}:requirements`],
    () => {
      void load();
      void loadHistory();
    },
    () => {
      operation.current++;
      latest.current.invalidate();
      setRun(undefined);
      setHistory([]);
      setBusy(false);
      setError(false);
    },
  );
  const runRef = useRef(run);
  runRef.current = run;
  useAnalysisProgress(projectId, (snapshot) => {
    if (selected && selected !== snapshot.id) return;
    const previous = runRef.current;
    const next = acceptSnapshot(previous, liveRun(snapshot));
    if (next === previous) return;
    if (previous?.id === next.id) {
      for (const group of groupPairs(next)) {
        const old = groupPairs(previous).find((item) => item.key === group.key);
        if (
          old?.count !== group.count ||
          (group.key === 'pending' && previous.activePairId !== next.activePairId)
        )
          clientCache.invalidate([`project:${projectId}:analysis:${next.id}:${group.key}`]);
      }
    }
    if (previous && previous.id !== next.id) void loadHistory();
    runRef.current = next;
    setRun(next);
    setLoading(false);
  });
  async function changeAnalysis(action: 'start' | 'retry' | 'cancel' | 'replace' = 'start') {
    const epoch = ++operation.current;
    latest.current.invalidate();
    setBusy(true);
    setError(false);
    try {
      const next =
        action === 'replace' && run
          ? (
              await query(
                ReplaceProjectAnalysisDocument,
                {
                  projectId,
                  analysisId: run.id,
                  mode,
                  threshold: thresholdValue,
                  strategy: replacementStrategy,
                },
                { notify: false },
              )
            ).replaceProjectAnalysis
          : action === 'cancel' && run
            ? (
                await query(
                  CancelProjectAnalysisDocument,
                  { projectId, analysisId: run.id },
                  { notify: false },
                )
              ).cancelProjectAnalysis
            : action === 'retry' && run
              ? (
                  await query(
                    RetryProjectAnalysisDocument,
                    { projectId, analysisId: run.id },
                    { notify: false },
                  )
                ).retryProjectAnalysis
              : (
                  await query(
                    StartProjectAnalysisDocument,
                    {
                      projectId,
                      mode,
                      threshold: thresholdValue,
                    },
                    { notify: false },
                  )
                ).startProjectAnalysis;
      if (active.current && context.current === projectId && operation.current === epoch) {
        latest.current.invalidate();
        const current = runRef.current;
        const confirmed = acceptSnapshot(current, liveRun(next));
        if (confirmed !== current) {
          runRef.current = confirmed;
          setRun(confirmed);
          if (current?.id === confirmed.id)
            for (const group of groupPairs(confirmed))
              clientCache.invalidate([
                `project:${projectId}:analysis:${confirmed.id}:${group.key}`,
              ]);
        }
        setSelected('');
        setLoading(false);
        void loadHistory();
      }
    } catch {
      if (active.current && context.current === projectId && operation.current === epoch)
        setError(true);
    } finally {
      if (active.current && context.current === projectId && operation.current === epoch)
        setBusy(false);
    }
  }
  const heading = `analysis-heading-${encodeURIComponent(projectId)}`;
  const status = busy ? 'running' : run?.status;
  const statusKey =
    status === 'running'
      ? 'analysis.status.running'
      : status === 'failed'
        ? run?.errorCode === 'CANCELLED'
          ? 'analysis.status.cancelled'
          : 'analysis.status.failed'
        : status === 'completed'
          ? 'analysis.status.completed'
          : undefined;
  const groups = run ? groupPairs(run) : [];
  const parametersChanged =
    !!run && (mode !== run.mode || thresholdValue !== run.similarityThreshold);
  const retryable = !selected && !!run && canRetryPairs(run);
  const candidatesKnown = !!run && run.totalPairs != null;
  const layaPools = run ? getLayaPools(run) : null;
  const hasLayaPipeline =
    !!run &&
    (!!run.classifierProfile ||
      run.layaPendingPairs + run.layaRunningPairs + run.layaCompletedPairs + run.layaFailedPairs >
        0);
  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>{t(locale, 'analysis.title')}</h2>
      <p>{t(locale, 'analysis.help')}</p>
      <p>{t(locale, 'analysis.disclaimer')}</p>
      <label>
        {t(locale, 'analysis.mode')}{' '}
        <select value={mode} onChange={(event) => setMode(event.target.value)} disabled={busy}>
          <option value="related">{t(locale, 'analysis.mode.related')}</option>
          <option value="all">{t(locale, 'analysis.mode.all')}</option>
        </select>
      </label>
      {mode === 'related' && (
        <label>
          {t(locale, 'analysis.threshold')}{' '}
          <input
            type="number"
            min={analysisRules.minThreshold}
            max={analysisRules.maxThreshold}
            step="0.01"
            value={threshold}
            onChange={(event) => setThreshold(event.target.value)}
            disabled={busy}
          />
        </label>
      )}
      <p className="muted">
        {t(locale, 'analysis.thresholdHelp', {
          min: analysisRules.minThreshold,
          max: analysisRules.maxThreshold,
        })}
      </p>
      {selectionValidation.errors.map((error) => (
        <p role="alert" key={error.path.join('.')}>
          {t(locale, error.messageKey, error.params)}
        </p>
      ))}
      <button
        type="button"
        disabled={!validSelection || busy || run?.status === 'running'}
        onClick={() => void changeAnalysis()}
      >
        {t(locale, busy ? 'analysis.starting' : run ? 'analysis.newRun' : 'analysis.start')}
      </button>
      {run?.status === 'running' && parametersChanged && !selected && (
        <fieldset>
          <legend>{t(locale, 'analysis.replacement.strategy')}</legend>
          <label>
            <input
              type="radio"
              name={`analysis-replacement-${projectId}`}
              value="wait"
              checked={replacementStrategy === 'wait'}
              onChange={() => setReplacementStrategy('wait')}
              disabled={busy}
            />
            {t(locale, 'analysis.replacement.wait')}
          </label>
          <label>
            <input
              type="radio"
              name={`analysis-replacement-${projectId}`}
              value="cancel"
              checked={replacementStrategy === 'cancel'}
              onChange={() => setReplacementStrategy('cancel')}
              disabled={busy}
            />
            {t(locale, 'analysis.replacement.cancel')}
          </label>
          <button
            type="button"
            disabled={!validSelection || busy}
            onClick={() => void changeAnalysis('replace')}
          >
            {t(locale, 'analysis.replacement.submit')}
          </button>
        </fieldset>
      )}
      {(history.length > 0 || historyOffset > 0) && (
        <div>
          <label>
            {t(locale, 'analysis.history')}{' '}
            <select value={selected} onChange={(event) => setSelected(event.target.value)}>
              <option value="">{t(locale, 'analysis.latest')}</option>
              {selected && run && !history.some((item) => item.id === selected) && (
                <option value={selected}>
                  {run.createdAt} · {run.generatorProfile}
                </option>
              )}
              {history.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.createdAt} · {item.generatorProfile}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            disabled={historyOffset === 0}
            onClick={() => setHistoryOffset((value) => Math.max(0, value - 20))}
          >
            {t(locale, 'analysis.previous')}
          </button>
          <button
            type="button"
            disabled={history.length < 20}
            onClick={() => setHistoryOffset((value) => value + 20)}
          >
            {t(locale, 'analysis.next')}
          </button>
        </div>
      )}
      {run?.status === 'running' && (
        <button type="button" disabled={busy} onClick={() => void changeAnalysis('cancel')}>
          {t(locale, 'analysis.cancel')}
        </button>
      )}
      {run?.status === 'running' && (
        <button
          type="button"
          disabled={busy}
          onClick={() => clientCache.invalidate([`project:${projectId}:analysis`])}
        >
          {t(locale, 'analysis.refresh')}
        </button>
      )}
      {retryable && (
        <button type="button" disabled={busy} onClick={() => void changeAnalysis('retry')}>
          {t(locale, 'analysis.retryFailed')}
        </button>
      )}
      {statusKey && <p role="status">{t(locale, statusKey)}</p>}
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={run !== undefined}
        retry={() => void load()}
      />
      {run?.stale && <p role="alert">{t(locale, 'analysis.stale')}</p>}
      {run?.status === 'running' && stages.has(run.stage) && (
        <p role="status">{t(locale, `analysis.stage.${run.stage}`)}</p>
      )}
      {run?.stage === 'waiting_current_pair' && (
        <p role="status">{t(locale, 'analysis.stage.waiting_current_pair')}</p>
      )}
      {run && run.totalPairs != null && (
        <p role="status">
          {t(locale, 'analysis.progress', { processed: run.processedPairs, total: run.totalPairs })}
        </p>
      )}
      {run && run.totalPairs == null && <p role="status">{t(locale, 'analysis.totalUnknown')}</p>}
      {run && (
        <p>
          {t(locale, 'analysis.coverage', {
            reused: run.reusedPairs,
            fresh: run.newPairs,
            pending: run.pendingPairs,
            running: run.runningPairs,
            failed: run.failedPairs,
          })}
        </p>
      )}
      {layaPools && hasLayaPipeline && (
        <section aria-label={t(locale, 'analysis.laya.pools')}>
          <h3>{t(locale, 'analysis.laya.pools')}</h3>
          <ul>
            <li>
              {t(locale, 'analysis.laya.pool.classifying', { count: layaPools.awaitingLaya })}
              {' · '}
              {t(locale, 'analysis.laya.running', { count: layaPools.layaRunning })}
            </li>
            <li>
              {t(locale, 'analysis.laya.pool.awaitingModel', { count: layaPools.awaitingModel })}
              {' · '}
              {t(locale, 'analysis.laya.modelRunning', { count: layaPools.modelRunning })}
            </li>
            <li>{t(locale, 'analysis.laya.pool.completed', { count: layaPools.completed })}</li>
          </ul>
          <p className="muted">
            {t(locale, 'analysis.laya.failures', {
              laya: layaPools.layaFailed,
              model: layaPools.modelFailed,
            })}
          </p>
          <p className="muted">{t(locale, 'analysis.laya.suggestionDisclaimer')}</p>
        </section>
      )}
      {run && (
        <p>
          {t(locale, 'analysis.savedMode', {
            mode: t(locale, run.mode === 'all' ? 'analysis.mode.all' : 'analysis.mode.related'),
            threshold: run.similarityThreshold,
          })}
        </p>
      )}
      {run && run.possiblePairs != null && (
        <p>{t(locale, 'analysis.possiblePairs', { total: run.possiblePairs })}</p>
      )}
      {run && run.totalPairs != null && (
        <p>{t(locale, 'analysis.selectedPairs', { selected: run.totalPairs })}</p>
      )}
      {run && run.notSelectedPairs != null && (
        <p>{t(locale, 'analysis.notSelectedPairs', { count: run.notSelectedPairs })}</p>
      )}
      {run && (
        <Group
          id={`analysis-provenance:${run.id}`}
          title={t(locale, 'analysis.provenance')}
          lazy
          defaultOpen={false}
        >
          <AnalysisProvenance runId={run.id} projectId={projectId} locale={locale} />
        </Group>
      )}
      {run?.status === 'completed' && (
        <h3>{t(locale, outcomes[run.outcome] || 'analysis.outcome.insufficient')}</h3>
      )}
      {run && run.status !== 'running' && reasonCodes.has(run.reasonCode) && (
        <p role="status">{t(locale, `analysis.reason.${run.reasonCode}`)}</p>
      )}
      {run &&
        run.status !== 'running' &&
        run.reasonCode === 'INDEX_INCOMPLETE' &&
        run.missingRequirementIds.length > 0 && (
          <fieldset
            aria-label={t(locale, 'analysis.missingIndexes', {
              count: run.missingRequirementIds.length,
            })}
          >
            <h3>
              {t(locale, 'analysis.missingIndexes', { count: run.missingRequirementIds.length })}
            </h3>
            <ul>
              {run.missingRequirementIds.map((id) => (
                <li key={id}>
                  <a
                    href={requirementURL(projectId, id)}
                    onClick={
                      onSource
                        ? (event) => {
                            event.preventDefault();
                            void onSource(id);
                          }
                        : undefined
                    }
                  >
                    {id}
                  </a>
                </li>
              ))}
            </ul>
          </fieldset>
        )}
      {run?.errorCode === 'CANCELLED' && <p>{t(locale, 'analysis.cancelledHelp')}</p>}
      {run?.errorCode === 'REPLACED' && <p>{t(locale, 'analysis.replaced')}</p>}
      {run?.status === 'failed' && analysisErrorMessages[run.errorCode] && (
        <p>{t(locale, analysisErrorMessages[run.errorCode])}</p>
      )}
      {run?.status === 'failed' &&
        !['CANCELLED', 'REPLACED', ...Object.keys(analysisErrorMessages)].includes(
          run.errorCode,
        ) && <p>{t(locale, 'analysis.failureHelp')}</p>}
      {retryable && <p>{t(locale, 'analysis.retryFailedHelp')}</p>}
      {run && (
        <>
          {run.sources.length > 0 && <h3>{t(locale, 'analysis.sources')}</h3>}
          {candidatesKnown &&
            groups.map((group) => (
              <PairGroup
                key={`${run.id}:${group.key}`}
                group={group}
                run={run}
                projectId={projectId}
                locale={locale}
                onSource={onSource}
              />
            ))}
          {!candidatesKnown &&
            run.sources.map((source) => (
              <SavedSource
                projectId={projectId}
                key={`${source.id}:${source.version}`}
                source={source}
                locale={locale}
                onSource={onSource}
              />
            ))}
        </>
      )}
    </section>
  );
}
