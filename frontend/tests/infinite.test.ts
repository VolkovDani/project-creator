import assert from 'node:assert/strict';
import test from 'node:test';
import { InfiniteStore } from '../lib/infinite.ts';

type Row = { id: string };
const page = (ids: string[], after: string | null, more = false) => ({
  edges: ids.map((id) => ({ cursor: id, node: { id } })),
  pageInfo: { endCursor: after, hasNextPage: more },
  totalCount: 99,
});

test('loads forward pages once and deduplicates nodes by ID', async () => {
  const store = new InfiniteStore<Row>('a', ['project:a']);
  const calls: Array<string | null> = [];
  const load = async (after: string | null) => {
    calls.push(after);
    return after === null ? page(['1', '2'], '2', true) : page(['2', '3'], '3');
  };
  await store.loadFirst(load);
  await Promise.all([store.loadMore(load), store.loadMore(load)]);
  assert.deepEqual(calls, [null, '2']);
  assert.deepEqual(
    store.snapshot().rows.map((row) => row.id),
    ['1', '2', '3'],
  );
});

test('refresh reloads every loaded prefix page and commits only after success', async () => {
  const store = new InfiniteStore<Row>('b', ['project:b']);
  await store.loadFirst(async () => page(['1', '2'], '2', true));
  await store.loadMore(async () => page(['3', '4'], '4', true));
  await store.loadMore(async () => page(['5'], '5'));
  const calls: Array<string | null> = [];
  await store.refresh(async (after) => {
    calls.push(after);
    if (after === null) return page(['10', '11'], '11', true);
    if (after === '11') return page(['12', '13'], '13', true);
    return page(['14'], '14');
  });
  assert.deepEqual(calls, [null, '11', '13']);
  assert.deepEqual(
    store.snapshot().rows.map((row) => row.id),
    ['10', '11', '12', '13', '14'],
  );
});

test('failed next page retains rows and can retry', async () => {
  const store = new InfiniteStore<Row>('c', ['project:c']);
  await store.loadFirst(async () => page(['1'], '1', true));
  await assert.rejects(
    store.loadMore(async () => {
      throw Error('offline');
    }),
  );
  assert.deepEqual(
    store.snapshot().rows.map((row) => row.id),
    ['1'],
  );
  await store.loadMore(async () => page(['2'], '2'));
  assert.deepEqual(
    store.snapshot().rows.map((row) => row.id),
    ['1', '2'],
  );
});

test('reset rejects an in flight response and clears scroll state', async () => {
  const store = new InfiniteStore<Row>('reset', ['project:reset']);
  store.setScrollOffset(420);
  let resolvePage: ((value: ReturnType<typeof page>) => void) | undefined;
  const pending = store.loadFirst(
    () =>
      new Promise((resolve) => {
        resolvePage = resolve;
      }),
  );
  store.reset();
  resolvePage?.(page(['old'], 'old'));
  await pending;
  assert.deepEqual(store.snapshot().rows, []);
  assert.equal(store.snapshot().scrollOffset, 0);
});

test('failed refresh keeps the complete previously loaded prefix', async () => {
  const store = new InfiniteStore<Row>('atomic', ['project:atomic']);
  await store.loadFirst(async () => page(['1'], '1', true));
  await store.loadMore(async () => page(['2'], '2', true));
  let calls = 0;
  await assert.rejects(
    store.refresh(async () => {
      calls++;
      if (calls === 1) return page(['10'], '10', true);
      throw Error('temporary failure');
    }),
  );
  assert.deepEqual(
    store.snapshot().rows.map((row) => row.id),
    ['1', '2'],
  );
  assert.equal(store.snapshot().error, true);
});

test('invalidation during refresh rejects old pages and keeps the replacement request', async () => {
  const store = new InfiniteStore<Row>('race', ['project:race']);
  await store.loadFirst(async () => page(['1'], '1'));
  let resolveOld: ((value: ReturnType<typeof page>) => void) | undefined;
  const oldRefresh = store.refresh(
    () =>
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
  );
  const { clientCache } = await import('../lib/client.ts');
  clientCache.invalidate(['project:race']);
  let resolveCurrent: ((value: ReturnType<typeof page>) => void) | undefined;
  const replacement = store.refresh(
    () =>
      new Promise((resolve) => {
        resolveCurrent = resolve;
      }),
  );
  resolveOld?.(page(['old'], 'old'));
  await oldRefresh;
  const secondReplacement = store.refresh(async () => page(['unexpected'], 'unexpected'));
  resolveCurrent?.(page(['current'], 'current'));
  await Promise.all([replacement, secondReplacement]);
  assert.deepEqual(
    store.snapshot().rows.map((row) => row.id),
    ['current'],
  );
});

test('LRU keeps a newly created store when all older stores are active', async () => {
  const { getInfiniteStore } = await import('../lib/infinite.ts');
  const active: Array<() => void> = [];
  for (let index = 0; index < 50; index++) {
    const store = getInfiniteStore(`active:${index}`, [`active:${index}`]);
    active.push(store.subscribe(() => {}));
  }
  const newest = getInfiniteStore<Row>('newest', ['newest']);
  assert.equal(getInfiniteStore<Row>('newest', ['newest']), newest);
  for (const unsubscribe of active) unsubscribe();
});
