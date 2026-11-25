'use client';
import { type ReactNode, useCallback } from 'react';
import type { Locale } from '../../../../lib/i18n';
import { useInfinite } from '../../../../lib/useInfinite';
import { LoadMore } from '../../../components/ListControls';
import ResourceState from '../../../components/ResourceState';
import { VirtualList } from '../../../components/VirtualList';

type ListProps<T> = {
  identity: string;
  tags: string[];
  load: (
    after: string | null,
  ) => Promise<{ items: T[]; pageInfo: { endCursor?: string | null; hasNextPage: boolean } }>;
};
export function useGlossaryList<T extends { id: string }>({ load, tags, identity }: ListProps<T>) {
  const read = useCallback(
    async (after: string | null) => {
      const { items, pageInfo } = await load(after);
      return { edges: items.map((node) => ({ node, cursor: node.id })), pageInfo };
    },
    [load],
  );
  return useInfinite({ identity, tags, load: read });
}

export function GlossaryListView<T extends { id: string }>({
  list,
  identity,
  locale,
  empty,
  render,
  focusedId,
  estimatedRowHeight,
}: {
  list: ReturnType<typeof useGlossaryList<T>>;
  identity: string;
  locale: Locale;
  empty: string;
  render: (item: T) => ReactNode;
  focusedId?: string;
  estimatedRowHeight?: number;
}) {
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
        focusedId={focusedId}
        estimatedRowHeight={estimatedRowHeight}
        restoreScroll={false}
      />
      <LoadMore locale={locale} {...list} loadMore={more} />
    </>
  );
}
