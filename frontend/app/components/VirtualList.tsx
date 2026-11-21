'use client';
import { type ReactNode, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { clientCache } from '../../lib/client.ts';

type Props<T> = {
  identity: string;
  items: readonly T[];
  keyOf: (item: T) => string;
  renderItem: (item: T, index: number) => ReactNode;
  onEndReached?: () => void;
  focusedId?: string;
  scrollToId?: string;
  estimatedRowHeight?: number;
  overscan?: number;
  restoreScroll?: boolean;
};
const savedOffsets = new Map<string, number>();
const savedHeights = new Map<string, Map<string, number>>();
const MAX_SAVED_IDENTITIES = 50;
const useBrowserLayoutEffect = typeof window === 'undefined' ? useEffect : useLayoutEffect;
clientCache.subscribe([], (reset) => {
  if (!reset) return;
  savedOffsets.clear();
  savedHeights.clear();
});

export function VirtualList<T>({
  identity,
  items,
  keyOf,
  renderItem,
  onEndReached,
  focusedId,
  scrollToId,
  estimatedRowHeight = 220,
  overscan = 4,
  restoreScroll = true,
}: Props<T>) {
  const host = useRef<HTMLDivElement>(null);
  const rowRefs = useRef(new Map<string, HTMLElement>());
  const heights = useRef(new Map(savedHeights.get(identity)));
  const heightsIdentity = useRef(identity);
  if (heightsIdentity.current !== identity) {
    heights.current = new Map(savedHeights.get(identity));
    heightsIdentity.current = identity;
  }
  const observer = useRef<ResizeObserver | null>(null);
  const [scrollY, setScrollY] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(800);
  const [, setRevision] = useState(0);
  const [actualFocusedId, setActualFocusedId] = useState<string>();
  useEffect(() => {
    observer.current = new ResizeObserver((entries) => {
      let changed = false;
      for (const entry of entries) {
        const id = (entry.target as HTMLElement).dataset.virtualId;
        if (id && heights.current.get(id) !== entry.contentRect.height) {
          heights.current.set(id, entry.contentRect.height);
          const identityHeights = savedHeights.get(identity) ?? new Map<string, number>();
          identityHeights.set(id, entry.contentRect.height);
          savedHeights.delete(identity);
          savedHeights.set(identity, identityHeights);
          while (savedHeights.size > MAX_SAVED_IDENTITIES) {
            const oldest = savedHeights.keys().next().value;
            if (oldest === undefined) break;
            savedHeights.delete(oldest);
            savedOffsets.delete(oldest);
          }
          changed = true;
        }
      }
      if (changed) setRevision((n) => n + 1);
    });
    return () => observer.current?.disconnect();
  }, [identity]);
  const saveHeight = useCallback((id: string, element: HTMLElement | null) => {
    const previous = rowRefs.current.get(id);
    if (previous) observer.current?.unobserve(previous);
    if (element) {
      rowRefs.current.set(id, element);
      observer.current?.observe(element);
    } else {
      rowRefs.current.delete(id);
    }
  }, []);
  useBrowserLayoutEffect(() => {
    const dialog = host.current?.closest('dialog');
    const scroller = dialog ?? window;
    const update = () => {
      const offset = dialog?.scrollTop ?? window.scrollY;
      setScrollY(offset);
      setViewportHeight(dialog?.clientHeight ?? window.innerHeight);
      savedOffsets.set(identity, offset);
    };
    const unsubscribe = clientCache.subscribe([], (reset) => {
      if (!reset) return;
      savedOffsets.clear();
      savedHeights.clear();
      heights.current.clear();
      setActualFocusedId(undefined);
      setRevision((n) => n + 1);
    });
    scroller.addEventListener('scroll', update, { passive: true });
    const resize = dialog ? new ResizeObserver(update) : undefined;
    if (dialog) resize?.observe(dialog);
    window.addEventListener('resize', update);
    const restore = savedOffsets.get(identity);
    if (restoreScroll && restore !== undefined) scroller.scrollTo({ top: restore });
    update();
    return () => {
      unsubscribe();
      resize?.disconnect();
      scroller.removeEventListener('scroll', update);
      window.removeEventListener('resize', update);
    };
  }, [identity, restoreScroll]);

  const offsets: number[] = [];
  let total = 0;
  for (const item of items) {
    offsets.push(total);
    total += heights.current.get(keyOf(item)) ?? estimatedRowHeight;
  }
  const dialog = host.current?.closest('dialog');
  const currentScrollY =
    dialog?.scrollTop ?? (typeof window === 'undefined' ? scrollY : window.scrollY);
  const viewportTop = dialog ? dialog.getBoundingClientRect().top + dialog.clientTop : 0;
  const hostTop = (host.current?.getBoundingClientRect().top ?? 0) - viewportTop + currentScrollY;
  const localStart = Math.max(0, currentScrollY - hostTop);
  let first = offsets.findIndex((offset, index) => {
    const item = items[index];
    return !item || offset + (heights.current.get(keyOf(item)) ?? estimatedRowHeight) >= localStart;
  });
  if (first < 0) first = 0;
  let last = first;
  const localEnd = localStart + viewportHeight;
  while (last < items.length && (offsets[last] ?? 0) < localEnd) last++;
  first = Math.max(0, first - overscan);
  last = Math.min(items.length, last + overscan);
  const selected = new Set<number>();
  for (let index = first; index < last; index++) selected.add(index);
  for (const id of [focusedId, actualFocusedId, scrollToId]) {
    if (!id) continue;
    const index = items.findIndex((item) => keyOf(item) === id);
    if (index >= 0) selected.add(index);
  }
  const visible = [...selected].sort((a, b) => a - b);
  const visibleKeys = visible.map((index) => {
    const item = items[index];
    return item ? keyOf(item) : '';
  });
  const visibleKeyString = visibleKeys.join('\0');
  useEffect(() => {
    const observed = visibleKeyString
      .split('\0')
      .map((id) => rowRefs.current.get(id))
      .filter((el) => el !== undefined);
    for (const element of observed) observer.current?.observe(element);
    return () => {
      for (const element of observed) observer.current?.unobserve(element);
    };
  }, [visibleKeyString]);

  useEffect(() => {
    if (!scrollToId || !visibleKeyString.includes(scrollToId)) return;
    rowRefs.current.get(scrollToId)?.scrollIntoView({ block: 'center' });
  }, [scrollToId, visibleKeyString]);
  useEffect(() => {
    const distanceToEnd = hostTop + total - (currentScrollY + viewportHeight);
    if (items.length && distanceToEnd <= 500) onEndReached?.();
  }, [hostTop, total, currentScrollY, viewportHeight, items.length, onEndReached]);

  let previousBottom = 0;
  return (
    <div
      ref={host}
      onFocusCapture={(event) => {
        const row = (event.target as HTMLElement).closest<HTMLElement>('[data-virtual-id]');
        if (row?.dataset.virtualId) setActualFocusedId(row.dataset.virtualId);
      }}
    >
      {visible.map((index) => {
        const item = items[index];
        if (!item) return null;
        const id = keyOf(item);
        const top = offsets[index] ?? previousBottom;
        const gap = Math.max(0, top - previousBottom);
        const height = heights.current.get(id) ?? estimatedRowHeight;
        previousBottom = top + height;
        return (
          <div key={id}>
            {gap > 0 && <div aria-hidden="true" style={{ height: gap }} />}
            <div
              ref={(element) => saveHeight(id, element)}
              style={{ display: 'flow-root' }}
              data-virtual-id={id}
              tabIndex={focusedId === id ? -1 : undefined}
            >
              {renderItem(item, index)}
            </div>
          </div>
        );
      })}
      <div
        aria-hidden="true"
        style={{
          height: Math.max(0, total - previousBottom),
        }}
      />
    </div>
  );
}
