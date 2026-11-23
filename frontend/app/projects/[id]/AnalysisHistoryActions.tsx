'use client';
import { useState } from 'react';
import { clientCache, query } from '../../../lib/client';
import {
  DeleteMarkedAnalysisDocument,
  ScheduleAnalysisDeletionDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import ConfirmationDialog from '../../components/ConfirmationDialog';

export default function AnalysisHistoryActions({
  run,
  retentionDays,
  locale,
}: {
  run: { id: string; projectId: string; deletionScheduledFor: string | null };
  retentionDays?: number;
  locale: Locale;
}) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  async function remove(marked: boolean) {
    setConfirming(false);
    setBusy(true);
    setError(false);
    const variables = { projectId: run.projectId, analysisId: run.id };
    const tags = [`project:${run.projectId}:analysis-history`];
    try {
      const changed = marked
        ? (await query(DeleteMarkedAnalysisDocument, variables, { tags })).deleteMarkedAnalysis
        : (await query(ScheduleAnalysisDeletionDocument, variables, { tags }))
            .scheduleAnalysisDeletion;
      setError(!changed);
    } catch {
      setError(true);
      clientCache.invalidate(tags);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {run.deletionScheduledFor ? (
        <>
          <p>
            {t(locale, 'analysis.deletionDeadline')}{' '}
            <time dateTime={run.deletionScheduledFor}>
              {new Date(run.deletionScheduledFor).toLocaleString(locale)}
            </time>
          </p>
          <button
            type="button"
            className="destructive-action"
            disabled={busy}
            onClick={() => setConfirming(true)}
          >
            {t(locale, 'analysis.deletePermanently')}
          </button>
        </>
      ) : (
        retentionDays !== undefined && (
          <button
            type="button"
            className="destructive-action"
            disabled={busy}
            onClick={() => void remove(false)}
          >
            {t(locale, 'analysis.scheduleDeletion', {
              delay: new Intl.RelativeTimeFormat(locale).format(retentionDays, 'day'),
            })}
          </button>
        )
      )}
      {error && <p role="alert">{t(locale, 'analysis.deletionError')}</p>}
      {confirming && run.deletionScheduledFor && (
        <ConfirmationDialog
          title={t(locale, 'analysis.confirmDeletion')}
          confirmLabel={t(locale, 'analysis.deletePermanently')}
          destructive
          locale={locale}
          onCancel={() => setConfirming(false)}
          onConfirm={() => void remove(true)}
        >
          <p>{t(locale, 'analysis.deleteRun', { id: run.id })}</p>
        </ConfirmationDialog>
      )}
    </>
  );
}
