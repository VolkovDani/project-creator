'use client';
import Link from 'next/link';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  AnalysisPairConnectionDocument,
  AnalysisPairSourcesDocument,
  AnalysisPairTechnicalDocument,
  AnalysisRunMetadataDocument,
  CancelProjectAnalysisDocument,
  ProjectAnalysisSummaryDocument,
  ReplaceProjectAnalysisDocument,
  RetryProjectAnalysisDocument,
  StartProjectAnalysisDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { offsetConnection } from '../../../lib/infinite';
import { Latest } from '../../../lib/latest';
import { useAnalysisProgress, useInvalidation } from '../../../lib/live';
import { requirementURL } from '../../../lib/rag';
import { useInfinite } from '../../../lib/useInfinite';
import { useResource } from '../../../lib/useResource';
import { DesignIcon } from '../../components/DesignIcon';
import Group from '../../components/Group';
import { LoadMore } from '../../components/ListControls';
import rowStyles from '../../components/RecordIcons.module.css';
import ResourceState from '../../components/ResourceState';
import { VirtualList } from '../../components/VirtualList';
import AnalysisHistoryActions from './AnalysisHistoryActions';
import { GlossaryText } from './GlossaryActions';
import { SourceText } from './GlossaryOriginal';
import { MatchedText } from './MatchedText';

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
const stages = new Set(['checking_index', 'finding_pairs', 'comparing']);
const analysisErrorMessages: Record<string, string> = {
  TIMEOUT: 'analysis.error.TIMEOUT',
  NPU_UNAVAILABLE: 'analysis.error.NPU_UNAVAILABLE',
  NPU_RECOVERY_FAILED: 'analysis.error.NPU_RECOVERY_FAILED',
  NPU_WORKER_EXIT: 'analysis.error.NPU_WORKER_EXIT',
  INVALID_OUTPUT: 'analysis.error.INVALID_OUTPUT',
  PROFILE_CHANGED: 'analysis.error.PROFILE_CHANGED',
  SOURCE_CHANGED: 'analysis.error.SOURCE_CHANGED',
  GLOSSARY_CHANGED: 'analysis.error.GLOSSARY_CHANGED',
};
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
  analysisId,
}: {
  source: Source;
  locale: Locale;
  onSource?: SourceHandler;
  projectId: string;
  analysisId: string;
}) {
  return (
    <div className="analysis-source">
      <p className="muted">
        {source.id} · {t(locale, 'requirements.version', { version: source.version })}
      </p>
      <SourceText
        key={JSON.stringify([analysisId, source.id, source.version])}
        analysisId={analysisId}
        projectId={projectId}
        requirementId={source.id}
        version={source.version}
        original={source.text}
        locale={locale}
      />
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

function PairSources({
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
          AnalysisPairSourcesDocument,
          { projectId, analysisId: runId, pairId },
          { tags: [`project:${projectId}:analysis:${runId}:pair:${pairId}`], version },
        )
      ).analysisPair,
    [projectId, runId, pairId, version],
  );
  const resource = useResource(load, [`project:${projectId}:analysis:${runId}:pair:${pairId}`]);
  const pair = resource.data;
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
          <SavedSource
            analysisId={runId}
            projectId={projectId}
            source={pair.left}
            locale={locale}
            onSource={onSource}
          />
          <SavedSource
            analysisId={runId}
            projectId={projectId}
            source={pair.right}
            locale={locale}
            onSource={onSource}
          />
        </>
      )}
    </>
  );
}

function PairTechnical({
  pairId,
  runId,
  projectId,
  locale,
  version,
}: {
  pairId: string;
  runId: string;
  projectId: string;
  locale: Locale;
  version: string;
}) {
  const load = useCallback(
    async () =>
      (
        await query(
          AnalysisPairTechnicalDocument,
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
          {pair.layaErrorCode && (
            <p role="status" className="muted">
              {t(
                locale,
                layaPairErrorMessageKey(pair.layaErrorCode) || 'analysis.pair.layaSkippedGeneric',
              )}
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
          {pair.generatorProfile && (
            <p className="muted">
              {t(locale, 'analysis.pair.modelProfile', { profile: pair.generatorProfile })}
            </p>
          )}
          {pair.reusedFrom && (
            <p className="muted">{t(locale, 'analysis.reusedFrom', { id: pair.reusedFrom })}</p>
          )}
          {!!pair.glossaryContext?.length && (
            <section>
              <h5>{t(locale, 'analysis.pair.glossaryContext')}</h5>
              <dl>
                {pair.glossaryContext.map((hint) => (
                  <div key={hint.spelling}>
                    <dt>
                      {hint.spelling}
                      {hint.name && hint.name !== hint.spelling && ` · ${hint.name}`}
                      {hint.version != null &&
                        ` · ${t(locale, 'requirements.version', { version: hint.version })}`}
                    </dt>
                    <dd>
                      {hint.ambiguous ? (
                        <p>{t(locale, 'analysis.pair.ambiguousGlossary')}</p>
                      ) : (
                        <>
                          {hint.definition && <p className="user-content">{hint.definition}</p>}
                          {hint.context && <p className="user-content">{hint.context}</p>}
                        </>
                      )}
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
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
  const tags = useMemo(
    () => [
      `project:${projectId}:analysis:${run.id}:${group.key}`,
      `project:${projectId}:requirements`,
      `project:${projectId}:glossary`,
    ],
    [projectId, run.id, group.key],
  );
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
          { tags },
        )
      ).analysisPairsConnection,
    [projectId, run.id, group.key, tags],
  );
  const infinite = useInfinite({
    identity: `pairs:${projectId}:${run.id}:${group.key}`,
    tags,
    load: loader,
  });
  const more = useCallback(() => {
    if (!infinite.loading && !infinite.error && infinite.hasNextPage)
      void infinite.loadMore().catch(() => {});
  }, [infinite.loading, infinite.error, infinite.hasNextPage, infinite.loadMore]);
  const { rows: pairs, loading, error } = infinite;
  return (
    <>
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={infinite.loaded}
        retry={() => void infinite.retry().catch(() => {})}
      />
      {!loading && !error && group.count === 0 && (
        <p className="muted">{t(locale, 'analysis.group.empty')}</p>
      )}
      <VirtualList
        identity={`pairs:${projectId}:${run.id}:${group.key}`}
        items={pairs}
        keyOf={(pair) => pair.id}
        onEndReached={more}
        restoreScroll={false}
        estimatedRowHeight={650}
        renderItem={(pair) => (
          <article
            className={`card analysis-pair${pair.status === 'running' ? ' analysis-pair-current' : ''}`}
            key={pair.id}
            data-pair-id={pair.id}
            aria-label={t(locale, 'analysis.pair.position', { number: pair.position + 1 })}
          >
            <h4>
              <DesignIcon name="classification" className={rowStyles.desktopIcon} />
              {t(locale, 'analysis.pair.position', { number: pair.position + 1 })}
            </h4>
            {pair.status === 'completed' && (
              <>
                {pair.explanation && !pair.isExactDuplicate ? (
                  <GlossaryText className="user-content">
                    <MatchedText
                      projectId={projectId}
                      text={pair.explanation}
                      matches={pair.explanationMatches ?? []}
                      locale={locale}
                    />
                  </GlossaryText>
                ) : (
                  <p>
                    {pair.isExactDuplicate
                      ? t(locale, 'analysis.exactDuplicateExplanation')
                      : t(locale, 'analysis.pair.noExplanation')}
                  </p>
                )}
                {!pair.isExactDuplicate && (
                  <p className="muted">{t(locale, 'analysis.pair.modelSuggestion')}</p>
                )}
              </>
            )}
            {pair.glossaryStale && (
              <p role="status" className="muted">
                {t(locale, 'analysis.pair.glossaryStale')}
              </p>
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
              <PairSources
                key={pair.detailsVersion}
                pairId={pair.id}
                runId={run.id}
                projectId={projectId}
                locale={locale}
                version={pair.detailsVersion}
                onSource={onSource}
              />
            </Group>
            <Group
              id={`analysis-pair-technical:${run.id}:${pair.id}`}
              title={t(locale, 'analysis.technicalInfo')}
              lazy
              defaultOpen={false}
            >
              <PairTechnical
                key={pair.detailsVersion}
                pairId={pair.id}
                runId={run.id}
                projectId={projectId}
                locale={locale}
                version={pair.detailsVersion}
              />
            </Group>
          </article>
        )}
      />
      <LoadMore locale={locale} {...infinite} loadMore={more} />
    </>
  );
}

export default function Analysis({
  projectId,
  defaultThreshold,
  locale,
  onSource,
  historyView = false,
}: {
  projectId: string;
  defaultThreshold: number;
  locale: Locale;
  onSource?: SourceHandler;
  historyView?: boolean;
}) {
  const [run, setRun] = useState<LiveRun | null>();
  const runRef = useRef(run);
  runRef.current = run;
  const [retentionDays, setRetentionDays] = useState<number>();
  const tags = useMemo(
    () => [
      `project:${projectId}:analysis`,
      `project:${projectId}:analysis-history`,
      `project:${projectId}:requirements`,
    ],
    [projectId],
  );
  const [selected, setSelected] = useState('');
  const [mode, setMode] = useState(analysisRules.defaultMode);
  const [thresholdOverride, setThresholdOverride] = useState<string>();
  const threshold = thresholdOverride ?? String(defaultThreshold);
  const [replacementStrategy, setReplacementStrategy] = useState<'wait' | 'cancel'>('wait');
  const thresholdValue = threshold.trim() === '' ? NaN : Number(threshold);
  const selectionValidation = validateAnalysisSelection({ mode, threshold: thresholdValue });
  const validSelection = selectionValidation.errors.length === 0;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(true);
  const latest = useRef(new Latest());
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
    const runAtStart = runRef.current;
    try {
      const data = await query(
        ProjectAnalysisSummaryDocument,
        { projectId, analysisId: selected || null, withRetention: historyView },
        { tags },
      );
      if (
        active.current &&
        context.current === projectId &&
        operation.current === epoch &&
        latest.current.isCurrent(generation)
      ) {
        setRetentionDays(data.analysisDeletionRetentionDays);
        const snapshot = data.projectAnalysis ? liveRun(data.projectAnalysis) : null;
        // A confirmed latest run can be older or absent after retention cleanup.
        // Preserve progress that arrived while the query was in flight.
        setRun((current) => {
          if (current === runAtStart && current?.id !== snapshot?.id) return snapshot;
          return snapshot ? acceptSnapshot(current, snapshot) : (current ?? null);
        });
        if (selected && !snapshot) setSelected('');
        setError(false);
      }
    } catch {
      if (active.current && operation.current === epoch && latest.current.isCurrent(generation)) {
        setError(true);
      }
    } finally {
      if (active.current && latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, selected, tags, historyView]);
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
  const historyLoader = useCallback(
    async (after: string | null) => {
      const size = 20;
      const data = await query(
        AnalysisHistoryDocument,
        { projectId, limit: size + 1, offset: Number(after) },
        { tags },
      );
      return offsetConnection(data.analysisHistory, after, size);
    },
    [projectId, tags],
  );
  const historyList = useInfinite({
    identity: `analysis-history:${projectId}`,
    tags,
    enabled: historyView,
    load: historyLoader,
  });
  const history = historyList.rows;
  function moreHistory() {
    if (historyList.hasNextPage && !historyList.loading && !historyList.error)
      void historyList.loadMore().catch(() => {});
  }
  useInvalidation(
    tags,
    () => {
      void load();
    },
    () => {
      operation.current++;
      latest.current.invalidate();
      setRun(undefined);
      setBusy(false);
      setError(false);
    },
  );
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
    if (historyView && previous && previous.id !== next.id)
      void historyList.refresh().catch(() => {});
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
      }
    } catch {
      if (active.current && context.current === projectId && operation.current === epoch)
        setError(true);
    } finally {
      if (active.current && context.current === projectId && operation.current === epoch)
        setBusy(false);
    }
  }
  function historyLabel(
    item: Pick<AnalysisRun, 'createdAt' | 'generatorProfile' | 'deletionScheduledFor'>,
  ) {
    return [
      item.createdAt,
      item.generatorProfile,
      item.deletionScheduledFor &&
        `${t(locale, 'analysis.deletionDeadline')} ${new Date(item.deletionScheduledFor).toLocaleString(locale)}`,
    ]
      .filter(Boolean)
      .join(' · ');
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
  const retryable = !historyView && !selected && !!run && canRetryPairs(run);
  const candidatesKnown = !!run && run.totalPairs != null;
  const layaPools = run ? getLayaPools(run) : null;
  const hasLayaPipeline =
    !!run &&
    (!!run.classifierProfile ||
      run.layaPendingPairs +
        run.layaRunningPairs +
        run.layaCompletedPairs +
        run.layaFailedPairs +
        run.layaSkippedPairs >
        0);
  return (
    <section className="ui-panel" aria-labelledby={heading}>
      <h2 id={heading}>{t(locale, historyView ? 'analysis.history' : 'analysis.title')}</h2>
      {!historyView && (
        <>
          <p>{t(locale, 'analysis.help')}</p>
          <p>
            <Link href={`/projects/${encodeURIComponent(projectId)}/analysis/external-proposals`}>
              {t(locale, 'externalProposals.title')}
            </Link>
          </p>
        </>
      )}
      {(!historyView || run) && <p>{t(locale, 'analysis.disclaimer')}</p>}
      {!historyView && (
        <>
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
                step={Number(defaultThreshold.toFixed(2)) === defaultThreshold ? 0.01 : 'any'}
                value={threshold}
                onChange={(event) => setThresholdOverride(event.target.value)}
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
            className="primary-action"
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
        </>
      )}
      {historyView && historyList.error && (
        <ResourceState
          locale={locale}
          loading={false}
          error
          hasData={history.length > 0}
          retry={() => void historyList.retry().catch(() => {})}
        />
      )}
      {historyView && historyList.loaded && !historyList.error && !history.length && (
        <p>{t(locale, 'analysis.historyEmpty')}</p>
      )}
      {historyView && history.length > 0 && (
        <div>
          <label>
            {t(locale, 'analysis.history')}{' '}
            <select
              value={selected}
              onChange={(event) => setSelected(event.target.value)}
              size={historyList.hasNextPage || history.length > 20 ? 6 : undefined}
              onScroll={(event) => {
                const list = event.currentTarget;
                if (list.scrollTop + list.clientHeight >= list.scrollHeight - 2) moreHistory();
              }}
            >
              <option value="">{t(locale, 'analysis.latest')}</option>
              {selected && run && !history.some((item) => item.id === selected) && (
                <option value={selected}>{historyLabel(run)}</option>
              )}
              {history.map((item) => (
                <option key={item.id} value={item.id}>
                  {historyLabel(item)}
                </option>
              ))}
            </select>
          </label>
          <LoadMore locale={locale} {...historyList} loadMore={moreHistory} />
        </div>
      )}
      {!historyView && run?.status === 'running' && (
        <button type="button" disabled={busy} onClick={() => void changeAnalysis('cancel')}>
          {t(locale, 'analysis.cancel')}
        </button>
      )}
      {!historyView && run?.status === 'running' && (
        <button
          type="button"
          disabled={busy}
          onClick={() => clientCache.invalidate([`project:${projectId}:analysis`])}
        >
          {t(locale, 'analysis.refresh')}
        </button>
      )}
      {historyView &&
        run &&
        (!selected || run.id === selected) &&
        (run.status === 'completed' || run.status === 'failed') && (
          <AnalysisHistoryActions
            key={run.id}
            run={run}
            retentionDays={retentionDays}
            locale={locale}
          />
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
            <li>{t(locale, 'analysis.laya.pool.skipped', { count: layaPools.layaSkipped })}</li>
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
                analysisId={run.id}
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
