import { useCallback, useState } from 'react';
import { query } from '../../../../lib/client';
import { ClassificationHistoryDocument } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import { useResource } from '../../../../lib/useResource';
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
  const [page, setPage] = useState(0);
  const size = 10;
  const tag = `project:${projectId}:classification:${requirementId}`;
  const load = useCallback(
    async () => ({
      page,
      entries: (
        await query(
          ClassificationHistoryDocument,
          { projectId, requirementId, limit: size + 1, offset: page * size },
          { tags: [tag] },
        )
      ).requirementClassificationHistory,
    }),
    [projectId, requirementId, page, tag],
  );
  const resource = useResource(load, [tag]);
  const entries = resource.data?.page === page ? resource.data.entries : undefined;
  return (
    <>
      <ResourceState
        locale={locale}
        loading={resource.loading && !entries}
        error={resource.error}
        hasData={!!entries}
        retry={resource.reload}
      />
      {entries?.length === 0 && <p>{t(locale, 'review.historyEmpty')}</p>}
      <ol>
        {entries?.slice(0, size).map((entry) => (
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
      {(page > 0 || (entries?.length ?? 0) > size) && (
        <div className="pagination">
          <button type="button" disabled={!page} onClick={() => setPage(page - 1)}>
            {t(locale, 'review.previous')}
          </button>
          <button
            type="button"
            disabled={(entries?.length ?? 0) <= size}
            onClick={() => setPage(page + 1)}
          >
            {t(locale, 'review.next')}
          </button>
        </div>
      )}
    </>
  );
}
