import { type ReactNode, useCallback, useMemo } from 'react';
import { query } from '../../../lib/client';
import { RequirementRevisionHistoryDocument } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { revisionDiff } from '../../../lib/revisionDiff';
import { useInfinite } from '../../../lib/useInfinite';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';

const pageSize = 10;

function RevisionWording({
  previousText,
  text,
  version,
  locale,
}: {
  previousText: string | null;
  text: string;
  version: number;
  locale: Locale;
}) {
  const wording = useMemo(() => {
    if (previousText === null || previousText === text)
      return { previous: previousText, next: text };
    const previous: ReactNode[] = [];
    const next: ReactNode[] = [];
    let offset = 0;
    for (const part of revisionDiff(previousText, text)) {
      switch (part.kind) {
        case 'equal':
          previous.push(part.text);
          next.push(part.text);
          break;
        case 'removed':
          previous.push(
            <del key={offset} className="revision-removed">
              {part.text}
            </del>,
          );
          break;
        case 'added':
          next.push(
            <ins key={offset} className="revision-added">
              {part.text}
            </ins>,
          );
          break;
      }
      offset += part.text.length;
    }
    return { previous, next };
  }, [previousText, text]);

  return (
    <>
      {version > 1 && (
        <>
          <dt>{t(locale, 'requirements.previousText')}</dt>
          <dd className="user-content">
            {wording.previous ?? t(locale, 'requirements.previousUnavailable')}
          </dd>
        </>
      )}
      <dt>{t(locale, version === 1 ? 'requirements.text' : 'requirements.newText')}</dt>
      <dd className="user-content">{wording.next}</dd>
    </>
  );
}

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
                </>
              )}
              <RevisionWording
                previousText={entry.previousText}
                text={entry.text}
                version={entry.version}
                locale={locale}
              />
            </dl>
          </li>
        ))}
      </ol>
      <LoadMore locale={locale} {...list} observe loadMore={loadMore} />
    </>
  );
}
