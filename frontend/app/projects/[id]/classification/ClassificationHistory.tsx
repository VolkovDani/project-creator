import { useCallback } from 'react';
import { query } from '../../../../lib/client';
import { ClassificationHistoryDocument } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import { offsetConnection } from '../../../../lib/infinite';
import { useInfinite } from '../../../../lib/useInfinite';
import { LoadMore } from '../../../components/ListControls';
import ResourceState from '../../../components/ResourceState';
import DecisionEvidence from './DecisionEvidence';

export default function ClassificationHistory({
  projectId,
  requirementId,
  locale,
}: {
  projectId: string;
  requirementId: string;
  locale: Locale;
}) {
  const size = 10;
  const tag = `project:${projectId}:classification:${requirementId}`;
  const load = useCallback(
    async (after: string | null) => {
      const result = await query(
        ClassificationHistoryDocument,
        { projectId, requirementId, limit: size + 1, offset: Number(after) },
        { tags: [tag] },
      );
      return offsetConnection(
        result.requirementClassificationHistory.map((entry) => ({
          ...entry,
          id: String(entry.revision),
        })),
        after,
        size,
      );
    },
    [projectId, requirementId, tag],
  );
  const list = useInfinite({
    identity: JSON.stringify(['classification-history', projectId, requirementId]),
    tags: [tag],
    load,
  });
  const loadMore = useCallback(() => void list.loadMore().catch(() => {}), [list.loadMore]);
  return (
    <>
      <ResourceState
        locale={locale}
        loading={list.loading}
        error={list.error}
        hasData={list.loaded}
        retry={() => void list.retry().catch(() => {})}
      />
      {list.loaded && !list.rows.length && <p>{t(locale, 'review.historyEmpty')}</p>}
      <ol>
        {list.rows.map((entry) => (
          <li key={entry.revision}>
            <p>
              {t(locale, 'review.historyVersion', {
                version: entry.textVersion,
                revision: entry.revision,
              })}{' '}
              · <time dateTime={entry.createdAt}>{entry.createdAt}</time>
            </p>
            <DecisionEvidence entry={entry} locale={locale} />
          </li>
        ))}
      </ol>
      <LoadMore locale={locale} {...list} observe loadMore={loadMore} />
    </>
  );
}
