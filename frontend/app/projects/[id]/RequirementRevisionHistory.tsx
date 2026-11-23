import { useCallback } from 'react';
import { query } from '../../../lib/client';
import { RequirementRevisionHistoryDocument } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInfinite } from '../../../lib/useInfinite';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';

const pageSize = 10;

export default function RequirementRevisionHistory({
  projectId,
  requirementId,
  version,
  locale,
}: {
  projectId: string;
  requirementId: string;
  version: number;
  locale: Locale;
}) {
  const tag = `project:${projectId}:requirements`;
  const load = useCallback(
    async (after: string | null) => {
      const { requirementRevisionHistory } = await query(
        RequirementRevisionHistoryDocument,
        {
          projectId,
          requirementId,
          first: pageSize + 1,
          beforeVersion: after === null ? null : Number(after),
        },
        { tags: [tag], version: String(version) },
      );
      const edges = requirementRevisionHistory.slice(0, pageSize).map((entry) => {
        const cursor = String(entry.version);
        return { cursor, node: { ...entry, id: cursor } };
      });
      return {
        edges,
        pageInfo: {
          endCursor: edges.at(-1)?.cursor,
          hasNextPage: requirementRevisionHistory.length > pageSize,
        },
      };
    },
    [projectId, requirementId, version, tag],
  );
  const list = useInfinite({
    identity: JSON.stringify(['requirement-revisions', projectId, requirementId, version]),
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
      {list.loaded && !list.rows.length && <p>{t(locale, 'requirements.historyEmpty')}</p>}
      <ol>
        {list.rows.map((entry) => (
          <li key={entry.id}>
            <h4>
              {entry.version === 1
                ? t(locale, 'requirements.initialVersion')
                : t(locale, 'requirements.transition', {
                    previous: entry.version - 1,
                    version: entry.version,
                  })}
            </h4>
            <time dateTime={entry.createdAt}>{entry.createdAt}</time>
            {entry.author && (
              <p>
                {t(locale, 'requirements.author')}:{' '}
                <span className="user-content">{entry.author}</span>
              </p>
            )}
            <dl>
              {entry.version > 1 && (
                <>
                  <dt>{t(locale, 'requirements.changeReason')}</dt>
                  <dd className="user-content">
                    {entry.changeReason ?? t(locale, 'requirements.reasonUnavailable')}
                  </dd>
                  <dt>{t(locale, 'requirements.previousText')}</dt>
                  <dd className="user-content">
                    {entry.previousText ?? t(locale, 'requirements.previousUnavailable')}
                  </dd>
                </>
              )}
              <dt>
                {t(locale, entry.version === 1 ? 'requirements.text' : 'requirements.newText')}
              </dt>
              <dd className="user-content">{entry.text}</dd>
            </dl>
          </li>
        ))}
      </ol>
      <LoadMore locale={locale} {...list} observe loadMore={loadMore} />
    </>
  );
}
