'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import {
  FailedRequirementSummaryCountDocument,
  RetryFailedRequirementSummariesDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';

export default function RetryFailedSummaries({
  projectId,
  locale,
}: {
  projectId: string;
  locale: Locale;
}) {
  const summaryTag = `project:${projectId}:summaries`;
  const requirementsTag = `project:${projectId}:requirements`;
  const loadCount = useCallback(async () => {
    const result = await query(
      FailedRequirementSummaryCountDocument,
      { projectId },
      { tags: [summaryTag, requirementsTag] },
    );
    return result.failedRequirementSummaryCount;
  }, [projectId, requirementsTag, summaryTag]);
  const {
    data: count,
    loading,
    error,
    reload,
  } = useResource(loadCount, [summaryTag, requirementsTag]);
  const [result, setResult] = useState<number | 'pending' | 'failed'>();
  async function retry() {
    setResult('pending');
    try {
      const response = await query(
        RetryFailedRequirementSummariesDocument,
        { projectId },
        { tags: [summaryTag] },
      );
      setResult(response.retryFailedRequirementSummaries);
    } catch {
      setResult('failed');
    }
  }
  return (
    <>
      {error && (
        <p role="alert" className="error">
          {t(locale, 'summary.failedCountError')}{' '}
          <button type="button" onClick={() => void reload()}>
            {t(locale, 'summary.refresh')}
          </button>
        </p>
      )}
      {!error && count !== undefined && count > 0 && (
        <button
          type="button"
          disabled={result === 'pending' || loading}
          onClick={() => void retry()}
        >
          {t(locale, 'summary.retryFailed', { count })}
        </button>
      )}
      {result === 'failed' && (
        <p role="alert" className="error">
          {t(locale, 'summary.bulkRetryError')}
        </p>
      )}
      {typeof result === 'number' && (
        <p role="status">{t(locale, 'summary.requeued', { count: result })}</p>
      )}
    </>
  );
}
