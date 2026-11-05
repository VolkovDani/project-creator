'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import {
  RequirementIndexProgressDocument,
  type RequirementIndexProgressQuery,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';

export default function IndexProgress({
  projectId,
  locale,
}: {
  projectId: string;
  locale: Locale;
}) {
  const [progress, setProgress] =
    useState<RequirementIndexProgressQuery['requirementIndexProgress']>();
  const [failed, setFailed] = useState(false);
  const latest = useRef(new Latest());
  const load = useCallback(async () => {
    const generation = latest.current.begin();
    try {
      const data = await query(RequirementIndexProgressDocument, { projectId });
      if (!data.requirementIndexProgress) throw Error('missing index progress');
      if (latest.current.isCurrent(generation)) {
        setProgress(data.requirementIndexProgress);
        setFailed(false);
      }
    } catch {
      if (latest.current.isCurrent(generation)) setFailed(true);
    }
  }, [projectId]);
  useEffect(() => {
    void load();
    const state = latest.current;
    return () => state.invalidate();
  }, [load]);
  useInvalidation(
    [`project:${projectId}`, `project:${projectId}:requirements`],
    () => void load(),
    () => {
      latest.current.invalidate();
      setProgress(undefined);
      setFailed(false);
    },
  );
  return (
    <section aria-label={t(locale, 'rag.progressTitle')}>
      <h3>{t(locale, 'rag.progressTitle')}</h3>
      <p>{t(locale, 'rag.autoIndexHelp')}</p>
      {progress && (
        <div aria-live="polite">
          {!progress.configured && <p>{t(locale, 'rag.notConfigured')}</p>}
          <p>
            {t(locale, 'rag.progressCounts', {
              total: progress.total,
              queued: progress.queued,
              running: progress.running,
              ready: progress.ready,
              failed: progress.failed,
            })}
          </p>
          <progress
            style={{
              width: '100%',
            }}
            max={Math.max(1, progress.total)}
            value={progress.ready}
            aria-label={t(locale, 'rag.progressTitle')}
          />
        </div>
      )}
      {failed && (
        <p role="alert" className="error">
          {t(locale, 'rag.progressFailed')}{' '}
          <button type="button" onClick={() => void load()}>
            {t(locale, 'rag.progressRetry')}
          </button>
        </p>
      )}
    </section>
  );
}
