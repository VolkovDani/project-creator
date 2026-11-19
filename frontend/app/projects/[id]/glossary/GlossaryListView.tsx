'use client';
import { type ReactNode, useCallback, useEffect, useState } from 'react';
import { type Locale, t } from '../../../../lib/i18n';
import type { ListMode } from '../../../../lib/listMode';
import { useInfinite } from '../../../../lib/useInfinite';
import { useResource } from '../../../../lib/useResource';
import { LoadMore } from '../../../components/ListControls';
import ResourceState from '../../../components/ResourceState';
import { VirtualList } from '../../../components/VirtualList';

type ListProps<T> = {
  identity: string;
  tags: string[];
  locale: Locale;
  empty: string;
  render: (item: T) => ReactNode;
  load: (
    after: string | null,
  ) => Promise<{ items: T[]; pageInfo: { endCursor?: string | null; hasNextPage: boolean } }>;
};
export function GlossaryListView<T extends { id: string }>(
  props: ListProps<T> & { mode: ListMode },
) {
  return props.mode === 'pages' ? <Paged {...props} /> : <Infinite {...props} />;
}
function Paged<T extends { id: string }>({ load, render, tags, locale, empty }: ListProps<T>) {
  const [history, setHistory] = useState<(string | null)[]>([null]);
  const after = history.at(-1) ?? null;
  const read = useCallback(async () => ({ after, ...(await load(after)) }), [load, after]);
  const resource = useResource(read, tags);
  const page = resource.data?.after === after ? resource.data : undefined;
  useEffect(() => {
    if (page && !page.items.length && after) setHistory((h) => h.slice(0, -1));
  }, [page, after]);
  return (
    <>
      <ResourceState
        locale={locale}
        loading={resource.loading}
        error={resource.error}
        hasData={!!page}
        retry={resource.reload}
      />
      {page?.items.length === 0 && !after && <p>{empty}</p>}
      {page?.items.map(render)}
      <div className="pagination">
        <button
          type="button"
          disabled={!after || resource.loading}
          onClick={() => setHistory((h) => h.slice(0, -1))}
        >
          {t(locale, 'app.previous')}
        </button>
        <span>{t(locale, 'app.page', { page: history.length })}</span>
        <button
          type="button"
          disabled={resource.loading || !page?.pageInfo.hasNextPage}
          onClick={() => {
            const cursor = page?.pageInfo.endCursor;
            if (cursor) setHistory((h) => [...h, cursor]);
          }}
        >
          {t(locale, 'app.next')}
        </button>
      </div>
    </>
  );
}
function Infinite<T extends { id: string }>({
  load,
  render,
  tags,
  identity,
  locale,
  empty,
}: ListProps<T>) {
  const read = useCallback(
    async (after: string | null) => {
      const { items, pageInfo } = await load(after);
      return { edges: items.map((node) => ({ node, cursor: node.id })), pageInfo };
    },
    [load],
  );
  const list = useInfinite({ identity, tags, load: read });
  const more = useCallback(() => {
    if (!list.loading && !list.error && list.hasNextPage) void list.loadMore().catch(() => {});
  }, [list.loading, list.error, list.hasNextPage, list.loadMore]);
  return (
    <>
      <ResourceState
        locale={locale}
        loading={list.loading}
        error={list.error}
        hasData={list.loaded}
        retry={() => void list.retry().catch(() => {})}
      />
      {list.loaded && !list.rows.length && <p>{empty}</p>}
      <VirtualList
        identity={identity}
        items={list.rows}
        keyOf={(item) => item.id}
        renderItem={render}
        onEndReached={more}
        restoreScroll={false}
      />
      <LoadMore locale={locale} {...list} loadMore={more} />
    </>
  );
}
