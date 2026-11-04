'use client';
import { useCallback, useEffect, useSyncExternalStore } from 'react';
import { type Connection, getInfiniteStore } from './infinite.ts';
import { useInvalidation } from './live.ts';

type Options<T extends { id: string }> = {
  identity: string;
  tags: readonly string[];
  load: (after: string | null) => Promise<Connection<T>>;
  enabled?: boolean;
};
export function useInfinite<T extends { id: string }>({
  identity,
  tags,
  load,
  enabled = true,
}: Options<T>) {
  const tagKey = JSON.stringify([...tags].sort());
  const stableTags = JSON.parse(tagKey) as string[];
  const store = getInfiniteStore<T>(identity, stableTags);
  const snapshot = useSyncExternalStore(store.subscribe, store.snapshot, store.snapshot);
  const loadMore = useCallback(() => store.loadMore(load), [store, load]);
  const refresh = useCallback(() => store.refresh(load), [store, load]);
  const retry = useCallback(() => store.retry(load), [store, load]);
  useInvalidation(
    stableTags,
    () => {
      if (!enabled) return;
      const state = store.snapshot();
      void (state.loaded ? store.refreshIfStale(load) : store.loadFirst(load)).catch(() => {});
    },
    () => {},
  );
  useEffect(() => {
    if (!enabled) return;
    if (!store.snapshot().loaded) void store.loadFirst(load).catch(() => {});
    else void store.refreshIfStale(load).catch(() => {});
  }, [store, load, enabled]);
  return {
    ...snapshot,
    loadMore,
    refresh,
    retry,
    scrollOffset: snapshot.scrollOffset,
    setScrollOffset: (value: number) => store.setScrollOffset(value),
  };
}
