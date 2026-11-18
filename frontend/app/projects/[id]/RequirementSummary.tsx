'use client';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import {
  CancelRequirementSummaryDocument,
  RequirementSummariesByIdsDocument,
  type RequirementSummariesByIdsQuery,
  RetryRequirementSummaryDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';
import GlossaryOriginal from './GlossaryOriginal';

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
  const identity = `${projectId}:${sourcesKey}`;
  const latest = useRef(new Latest());
  const [snapshot, setSnapshot] = useState<{ identity: string; values: Map<string, Summary> }>();
  const [failure, setFailure] = useState(false);
  const load = useCallback(async () => {
    const generation = latest.current.begin();
    if (!ids.length) {
      setSnapshot({ identity, values: new Map() });
      setFailure(false);
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
          { version: identity },
        );
        if (!latest.current.isCurrent(generation)) return;
        for (const summary of result.requirementSummariesByIds)
          values.set(summary.requirementId, summary);
      }
      setSnapshot({ identity, values });
      setFailure(false);
    } catch {
      if (latest.current.isCurrent(generation)) setFailure(true);
    }
  }, [identity, projectId, ids]);
  useEffect(() => {
    void load();
    const state = latest.current;
    return () => state.invalidate();
  }, [load]);
  useInvalidation(
    [`project:${projectId}:summaries`, `project:${projectId}:requirements`],
    () => void load(),
    () => {
      latest.current.invalidate();
      setSnapshot(undefined);
      setFailure(false);
    },
  );
  return {
    summaries: snapshot?.identity === identity ? snapshot.values : new Map<string, Summary>(),
    failure,
    retryLoad: load,
  };
}

export default function RequirementSummary({
  projectId,
  requirementId,
  requirementVersion,
  original,
  locale,
  summary,
  loadFailure,
  refresh,
}: {
  projectId: string;
  requirementId: string;
  requirementVersion: number;
  original: string;
  locale: Locale;
  summary?: Summary;
  loadFailure: boolean;
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
  return (
    <section aria-label={t(locale, 'summary.section')}>
      <p className="muted">{t(locale, 'summary.suggestion')}</p>
      {hasText && displayedSummary?.text ? (
        <p>{displayedSummary.text}</p>
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
      {hasText && displayedSummary && !displayedSummary.layaEligible && (
        <p role="status" className="muted">
          {t(locale, 'summary.longPreview', { bytes: displayedSummary.byteLength })}
        </p>
      )}
      {(summary?.errorCode || actionFailure) && (
        <p role="alert" className="error">
          {t(locale, actionFailure ? 'summary.actionError' : 'summary.error', {
            code: summary?.errorCode ?? '',
          })}
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
      <GlossaryOriginal
        projectId={projectId}
        requirementId={requirementId}
        version={requirementVersion}
        original={original}
        locale={locale}
      />
    </section>
  );
}
