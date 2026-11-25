'use client';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import {
  CancelRequirementSummaryDocument,
  RequirementSummariesByIdsDocument,
  type RequirementSummariesByIdsQuery,
  RetryRequirementSummaryDocument,
} from '../../../lib/generated';
import { type Locale, summaryErrorMessage, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';
import GlossaryOriginal from './GlossaryOriginal';
import { RequirementMarkdown } from './RequirementMarkdown';

type Summary = RequirementSummariesByIdsQuery['requirementSummariesByIds'][number];

export function useRequirementSummaries(
  projectId: string,
  requirements: Array<{ id: string; version: number }>,
) {
  const sourcesKey = JSON.stringify(
    [...new Map(requirements.map((row) => [row.id, row.version])).entries()].sort(
      ([left], [right]) => left.localeCompare(right),
    ),
  );
  const sources = useMemo(() => JSON.parse(sourcesKey) as Array<[string, number]>, [sourcesKey]);
  const ids = useMemo(() => sources.map(([id]) => id), [sources]);
  const tags = useMemo(
    () => [
      `project:${projectId}:summaries`,
      `project:${projectId}:requirements`,
      `project:${projectId}:glossary`,
    ],
    [projectId],
  );
  const identity = `${projectId}:${sourcesKey}`;
  const latest = useRef(new Latest());
  const [snapshot, setSnapshot] = useState<{ identity: string; values: Map<string, Summary> }>();
  const [failedIdentity, setFailedIdentity] = useState<string>();
  const load = useCallback(async () => {
    const generation = latest.current.begin();
    if (!ids.length) {
      setSnapshot({ identity, values: new Map() });
      setFailedIdentity(undefined);
      return;
    }
    try {
      const values = new Map<string, Summary>();
      for (let offset = 0; offset < ids.length; offset += 100) {
        if (!latest.current.isCurrent(generation)) return;
        const result = await query(
          RequirementSummariesByIdsDocument,
          {
            projectId,
            requirementIds: ids.slice(offset, offset + 100),
          },
          { version: identity, tags },
        );
        if (!latest.current.isCurrent(generation)) return;
        for (const summary of result.requirementSummariesByIds)
          values.set(summary.requirementId, summary);
      }
      setSnapshot({ identity, values });
      setFailedIdentity(undefined);
    } catch {
      if (latest.current.isCurrent(generation)) setFailedIdentity(identity);
    }
  }, [identity, projectId, ids, tags]);
  useEffect(() => {
    void load();
    const state = latest.current;
    return () => state.invalidate();
  }, [load]);
  useInvalidation(
    tags,
    () => void load(),
    () => {
      latest.current.invalidate();
      setSnapshot(undefined);
      setFailedIdentity(undefined);
    },
  );
  const failure = failedIdentity === identity;
  const hasSnapshot = snapshot?.identity === identity;
  return {
    summaries: hasSnapshot ? snapshot.values : new Map<string, Summary>(),
    loading: !hasSnapshot && !failure,
    failure,
    retryLoad: load,
  };
}

export default function RequirementSummary({
  projectId,
  requirementId,
  requirementVersion,
  original,
  highlight,
  showOriginal,
  locale,
  summary,
  loadFailure,
  loading,
  refresh,
}: {
  projectId: string;
  requirementId: string;
  requirementVersion: number;
  original: string;
  showOriginal?: boolean;
  highlight?: { start: number; end: number };
  locale: Locale;
  summary?: Summary;
  loadFailure: boolean;
  loading: boolean;
  refresh: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [actionFailure, setActionFailure] = useState(false);
  const latest = useRef(new Latest());
  useEffect(() => () => latest.current.invalidate(), []);
  const versionMatches = summary?.version === requirementVersion;
  const displayedSummary = versionMatches ? summary : undefined;
  const status = summary && !versionMatches ? 'stale' : (summary?.status ?? 'missing');
  const retryable = ['missing', 'failed', 'stale', 'cancelled'].includes(status);
  async function act(action: 'retry' | 'cancel') {
    const generation = latest.current.begin();
    setBusy(true);
    setActionFailure(false);
    try {
      if (action === 'retry')
        await query(RetryRequirementSummaryDocument, { projectId, requirementId });
      else await query(CancelRequirementSummaryDocument, { projectId, requirementId });
      if (latest.current.isCurrent(generation)) await refresh();
    } catch {
      if (latest.current.isCurrent(generation)) setActionFailure(true);
    } finally {
      if (latest.current.isCurrent(generation)) setBusy(false);
    }
  }
  const hasText = versionMatches && summary?.status === 'ready' && !!summary.text;
  const source =
    loading && !showOriginal ? (
      <RequirementMarkdown
        text={original}
        locale={locale}
        sourceId={requirementId}
        version={requirementVersion}
        projectId={projectId}
      />
    ) : (
      <GlossaryOriginal
        highlight={highlight}
        collapsible={hasText && !showOriginal}
        projectId={projectId}
        requirementId={requirementId}
        version={requirementVersion}
        original={original}
        locale={locale}
      />
    );
  const sourceContent = showOriginal ? (
    <section aria-label={t(locale, 'summary.original')}>{source}</section>
  ) : (
    source
  );
  return (
    <section aria-label={hasText ? t(locale, 'summary.section') : undefined}>
      {hasText ? <p className="muted">{t(locale, 'summary.suggestion')}</p> : sourceContent}
      {hasText && displayedSummary?.text ? (
        <RequirementMarkdown
          text={displayedSummary.text}
          locale={locale}
          projectId={projectId}
          matches={displayedSummary.glossaryMatches}
        />
      ) : (
        <p
          role={
            status === 'stale'
              ? 'status'
              : status === 'failed' || status === 'not_configured' || loadFailure
                ? 'alert'
                : undefined
          }
          className={status === 'failed' || loadFailure ? 'error' : 'muted'}
        >
          {t(locale, loadFailure ? 'summary.loadError' : `summary.status.${status}`)}
        </p>
      )}
      {(summary?.errorCode || actionFailure) && (
        <p role="alert" className="error">
          {actionFailure
            ? t(locale, 'summary.actionError')
            : summaryErrorMessage(locale, summary?.errorCode ?? '')}
        </p>
      )}
      {displayedSummary && hasText && (
        <details>
          <summary>{t(locale, 'summary.provenance')}</summary>
          <dl>
            <dt>{t(locale, 'summary.version')}</dt>
            <dd>{displayedSummary.version}</dd>
            <dt>{t(locale, 'summary.profile')}</dt>
            <dd>{displayedSummary.profile || t(locale, 'summary.unknown')}</dd>
            <dt>{t(locale, 'summary.origin')}</dt>
            <dd>{displayedSummary.provenance || t(locale, 'summary.unknown')}</dd>
            <dt>{t(locale, 'summary.createdAt')}</dt>
            <dd>{displayedSummary.createdAt}</dd>
          </dl>
        </details>
      )}
      {loadFailure && (
        <button type="button" disabled={busy} onClick={() => void refresh()}>
          {t(locale, 'summary.refresh')}
        </button>
      )}
      {retryable && !loadFailure && (
        <button type="button" disabled={busy} onClick={() => void act('retry')}>
          {t(locale, status === 'missing' ? 'summary.generate' : 'summary.retry')}
        </button>
      )}
      {(status === 'queued' || status === 'running') && (
        <button type="button" disabled={busy} onClick={() => void act('cancel')}>
          {t(locale, 'summary.cancel')}
        </button>
      )}

      {hasText && sourceContent}
    </section>
  );
}
