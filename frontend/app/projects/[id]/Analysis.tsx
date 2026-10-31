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
import { analysisRules, validateAnalysisSelection } from '../../../lib/analysisValidation';
import { clientCache, query } from '../../../lib/client';
import {
  AnalysisHistoryDocument,
  type AnalysisHistoryQuery,
  AnalysisPairPageDocument,
  ProjectAnalysisStatusDocument,
  RetryProjectAnalysisDocument,
  StartProjectAnalysisDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';
import { requirementURL } from '../../../lib/rag';
import Group from '../../components/Group';
import ResourceState from '../../components/ResourceState';

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
  const [offset, setOffset] = useState(0);
  const [pairs, setPairs] = useState<AnalysisPairView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const latest = useRef(new Latest());
  const load = useCallback(async () => {
    const generation = latest.current.begin();
    setLoading(true);
    setPairs([]);
    try {
      const data = await query(AnalysisPairPageDocument, {
        projectId,
        analysisId: run.id,
        limit: 20,
        offset,
        ...groupFilter(group.key),
      });
      if (latest.current.isCurrent(generation)) {
        setPairs(data.analysisPairs);
        setError(false);
      }
    } catch {
      if (latest.current.isCurrent(generation)) {
        setPairs([]);
        setError(true);
      }
    } finally {
      if (latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, run, offset, group.key]);
  useEffect(() => {
    void load();
    return () => latest.current.invalidate();
  }, [load]);
  useEffect(() => {
    if (offset >= group.count && offset > 0)
      setOffset(Math.max(0, Math.floor((group.count - 1) / 20) * 20));
  }, [group.count, offset]);
  return (
    <Group
      id={`analysis:${projectId}:${group.key}`}
      title={t(locale, 'analysis.group.count', {
        name: t(locale, `analysis.group.${group.key}`),
        count: group.count,
      })}
    >
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={!loading && !error}
        retry={() => void load()}
      />
      {!loading && !error && group.count === 0 && (
        <p className="muted">{t(locale, 'analysis.group.empty')}</p>
      )}
      {pairs.map((pair) => (
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
                pair.errorCode === 'NOT_PROCESSED'
                  ? 'analysis.pair.notProcessed'
                  : 'analysis.pair.failed',
              )}
            </p>
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
        </article>
      ))}
      {group.count > 20 && (
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
    </Group>
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
  const [run, setRun] = useState<AnalysisRun | null>();
  const [selected, setSelected] = useState('');
  const [mode, setMode] = useState(analysisRules.defaultMode);
  const [threshold, setThreshold] = useState(String(analysisRules.defaultThreshold));
  const thresholdValue = threshold.trim() === '' ? NaN : Number(threshold);
  const selectionValidation = validateAnalysisSelection({ mode, threshold: thresholdValue });
  const validSelection = selectionValidation.errors.length === 0;
  const [historyOffset, setHistoryOffset] = useState(0);
  const [history, setHistory] = useState<AnalysisHistoryQuery['analysisHistory']>([]);
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
    try {
      const [data, past] = await Promise.all([
        query(ProjectAnalysisStatusDocument, { projectId, analysisId: selected || null }),
        query(AnalysisHistoryDocument, { projectId, limit: 20, offset: historyOffset }),
      ]);
      if (
        active.current &&
        context.current === projectId &&
        operation.current === epoch &&
        latest.current.isCurrent(generation)
      ) {
        setRun(data.projectAnalysis);
        setHistory(past.analysisHistory);
        setError(false);
      }
    } catch {
      if (active.current && operation.current === epoch && latest.current.isCurrent(generation)) {
        setError(true);
        setRun(undefined);
        setHistory([]);
      }
    } finally {
      if (active.current && latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, selected, historyOffset]);
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
  useInvalidation(
    [`project:${projectId}`, `project:${projectId}:requirements`],
    () => void load(),
    () => {
      operation.current++;
      latest.current.invalidate();
      setRun(undefined);
      setHistory([]);
      setBusy(false);
      setError(false);
    },
  );
  async function start(retry = false) {
    const epoch = ++operation.current;
    latest.current.invalidate();
    setBusy(true);
    setError(false);
    try {
      const next =
        retry && run
          ? (await query(RetryProjectAnalysisDocument, { projectId, analysisId: run.id }))
              .retryProjectAnalysis
          : (
              await query(StartProjectAnalysisDocument, {
                projectId,
                mode,
                threshold: thresholdValue,
              })
            ).startProjectAnalysis;
      if (active.current && context.current === projectId && operation.current === epoch) {
        latest.current.invalidate();
        setRun(next);
        setSelected('');
        void load();
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
        ? 'analysis.status.failed'
        : status === 'completed'
          ? 'analysis.status.completed'
          : undefined;
  const groups = run ? groupPairs(run) : [];
  const retryable = !!run && canRetryPairs(run);
  const candidatesKnown = !!run && run.totalPairs != null;
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
        onClick={() => void start()}
      >
        {t(locale, busy ? 'analysis.starting' : run ? 'analysis.newRun' : 'analysis.start')}
      </button>
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
        <button
          type="button"
          disabled={busy}
          onClick={() => clientCache.invalidate([`project:${projectId}`])}
        >
          {t(locale, 'analysis.refresh')}
        </button>
      )}
      {retryable && (
        <button type="button" disabled={busy} onClick={() => void start(true)}>
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
        <details>
          <summary>{t(locale, 'analysis.provenance')}</summary>
          <pre>{run.generatorProvenance || run.generatorProfile}</pre>
          <p>
            {t(locale, 'analysis.selectionProfile', {
              profile: run.selectionProfile,
              embedding: run.embeddingProfile,
            })}
          </p>
        </details>
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
      {run?.status === 'failed' && <p>{t(locale, 'analysis.failureHelp')}</p>}
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
