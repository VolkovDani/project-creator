import { clientCache } from './client.ts';

export type Connection<T extends { id: string }> = {
  edges: Array<{ cursor: string; node: T }>;
  pageInfo: { endCursor?: string | null; hasNextPage: boolean };
  totalCount?: number;
};

// Offset-based queries request one extra row to detect the next batch.
export function offsetConnection<T extends { id: string }>(
  rows: readonly T[],
  after: string | null,
  size: number,
): Connection<T> {
  const offset = Number(after);
  const edges = rows.slice(0, size).map((node, index) => ({
    node,
    cursor: String(offset + index + 1),
  }));
  return {
    edges,
    pageInfo: { endCursor: edges.at(-1)?.cursor ?? null, hasNextPage: rows.length > size },
  };
}

export type InfiniteSnapshot<T> = {
  rows: T[];
  totalCount: number;
  hasNextPage: boolean;
  loading: boolean;
  error: boolean;
  loaded: boolean;
  scrollOffset: number;
};
type Loader<T extends { id: string }> = (after: string | null) => Promise<Connection<T>>;
const stores = new Map<string, InfiniteStore<{ id: string }>>();
const MAX_STORES = 50;

export class InfiniteStore<T extends { id: string }> {
  private rows: T[] = [];
  private totalCount = 0;
  private endCursor: string | null = null;
  private hasNextPage = false;
  private pageCount = 0;
  private loaded = false;
  private loading = false;
  private error = false;
  private errorKind: 'first' | 'next' | 'refresh' = 'first';
  private stale = false;
  private scrollOffset = 0;
  private epoch = 0;
  private nextPromise?: Promise<void>;
  private refreshPromise?: Promise<void>;
  private listeners = new Set<() => void>();
  private snapshotValue!: InfiniteSnapshot<T>;
  private unsubscribeCache: () => void;

  readonly identity: string;
  readonly tags: readonly string[];
  constructor(identity: string, tags: readonly string[]) {
    this.identity = identity;
    this.tags = tags;
    this.snapshotValue = this.makeSnapshot();
    this.unsubscribeCache = clientCache.subscribe([...tags], (reset) => {
      if (reset) this.clear();
      else {
        this.epoch++;
        this.loading = false;
        this.nextPromise = undefined;
        if (this.refreshPromise) this.refreshPromise = undefined;
        this.stale = true;
        this.error = false;
        this.emit();
      }
    });
  }

  snapshot = (): InfiniteSnapshot<T> => this.snapshotValue;
  private makeSnapshot = (): InfiniteSnapshot<T> => ({
    rows: this.rows,
    totalCount: this.totalCount,
    hasNextPage: this.hasNextPage,
    loading: this.loading,
    error: this.error,
    loaded: this.loaded,
    scrollOffset: this.scrollOffset,
  });
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };
  setScrollOffset(value: number) {
    this.scrollOffset = Math.max(0, value);
    this.emit();
  }
  get isActive() {
    return this.listeners.size > 0;
  }
  dispose() {
    this.unsubscribeCache();
    this.clear();
    this.listeners.clear();
  }
  private emit() {
    this.snapshotValue = this.makeSnapshot();
    for (const listener of this.listeners) listener();
  }
  reset() {
    this.clear();
  }
  private clear() {
    this.epoch++;
    this.rows = [];
    this.totalCount = 0;
    this.endCursor = null;
    this.hasNextPage = false;
    this.pageCount = 0;
    this.loaded = false;
    this.loading = false;
    this.error = false;
    this.errorKind = 'first';
    this.stale = false;
    this.scrollOffset = 0;
    this.nextPromise = undefined;
    this.refreshPromise = undefined;
    this.pageConnections = [];
    this.emit();
  }
  async loadFirst(load: Loader<T>) {
    if (this.loading || (this.loaded && !this.stale)) return;
    const epoch = this.epoch;
    this.loading = true;
    this.error = false;
    this.errorKind = 'first';
    this.emit();
    try {
      const page = await load(null);
      if (epoch !== this.epoch) return;
      this.commit([page]);
      this.stale = false;
    } catch {
      if (epoch === this.epoch) this.error = true;
      throw new Error('infinite page load failed');
    } finally {
      if (epoch === this.epoch) {
        this.loading = false;
        this.emit();
      }
    }
  }
  async loadMore(load: Loader<T>) {
    if (this.refreshPromise) return this.refreshPromise;
    if (this.nextPromise) return this.nextPromise;
    if (!this.hasNextPage || !this.loaded) return;
    const epoch = this.epoch;
    this.loading = true;
    this.error = false;
    this.errorKind = 'next';
    this.emit();
    this.nextPromise = (async () => {
      try {
        const page = await load(this.endCursor);
        if (epoch !== this.epoch) return;
        this.commit([...this.pageConnections, page]);
      } catch {
        if (epoch === this.epoch) this.error = true;
        throw new Error('infinite page load failed');
      } finally {
        if (epoch === this.epoch) {
          this.loading = false;
          this.nextPromise = undefined;
          this.emit();
        }
      }
    })();
    return this.nextPromise;
  }
  private pageConnections: Connection<T>[] = [];
  private commit(pages: Connection<T>[]) {
    const byId = new Map<string, T>();
    for (const page of pages) for (const edge of page.edges) byId.set(edge.node.id, edge.node);
    const last = pages.at(-1);
    this.rows = [...byId.values()];
    this.totalCount = last?.totalCount ?? 0;
    this.endCursor = last?.pageInfo.endCursor ?? null;
    this.hasNextPage = last?.pageInfo.hasNextPage ?? false;
    this.pageCount = pages.length;
    this.loaded = true;
    this.pageConnections = pages;
    this.stale = false;
    this.emit();
  }
  refresh(load: Loader<T>): Promise<void> {
    if (this.nextPromise) return this.nextPromise.then(() => this.refresh(load));
    if (this.refreshPromise) return this.refreshPromise;
    const request = this.performRefresh(load);
    const tracked = request.finally(() => {
      if (this.refreshPromise === tracked) this.refreshPromise = undefined;
    });
    this.refreshPromise = tracked;
    return tracked;
  }
  private async performRefresh(load: Loader<T>) {
    if (!this.loaded) return this.loadFirst(load);
    const epoch = this.epoch;
    const oldPageCount = this.pageCount;
    this.loading = true;
    this.error = false;
    this.errorKind = 'refresh';
    this.emit();
    try {
      const pages: Connection<T>[] = [];
      for (let index = 0; index < oldPageCount; index++) {
        if (index > 0 && !pages[index - 1]?.pageInfo.hasNextPage) break;
        pages.push(await load(index === 0 ? null : (pages[index - 1]?.pageInfo.endCursor ?? null)));
        if (epoch !== this.epoch) return;
      }
      this.commit(pages);
    } catch {
      if (epoch === this.epoch) this.error = true;
      throw new Error('infinite page refresh failed');
    } finally {
      if (epoch === this.epoch) {
        this.loading = false;
        this.stale = false;
        this.emit();
      }
    }
  }
  retry(load: Loader<T>) {
    if (this.errorKind === 'next') return this.loadMore(load);
    if (this.errorKind === 'refresh') return this.refresh(load);
    return this.loadFirst(load);
  }
  refreshIfStale(load: Loader<T>) {
    if (!this.stale) return Promise.resolve();
    return this.refresh(load);
  }
}

export function getInfiniteStore<T extends { id: string }>(
  identity: string,
  tags: readonly string[],
) {
  const key = JSON.stringify([identity, [...tags].sort()]);
  let store = stores.get(key) as InfiniteStore<T> | undefined;
  if (!store) {
    store = new InfiniteStore<T>(identity, tags);
    stores.set(key, store as InfiniteStore<{ id: string }>);
  } else {
    stores.delete(key);
    stores.set(key, store as InfiniteStore<{ id: string }>);
  }
  while (stores.size > MAX_STORES) {
    const oldest = [...stores.entries()].find(
      ([candidateKey, candidate]) => candidateKey !== key && !candidate.isActive,
    )?.[0];
    if (oldest === undefined) break;
    stores.get(oldest)?.dispose();
    stores.delete(oldest);
  }
  return store;
}
export function clearInfiniteStores() {
  for (const store of stores.values()) {
    store.reset();
    store.dispose();
  }
  stores.clear();
}
