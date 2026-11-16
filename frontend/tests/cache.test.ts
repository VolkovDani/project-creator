import assert from 'node:assert/strict';
import { test } from 'node:test';
import { TaggedCache } from '../lib/cache.ts';
import { nextCursor, SSEParser } from '../lib/sse.ts';

test('tag invalidation clears matching entries and duplicate ids are ignored', async () => {
  const c = new TaggedCache();
  let loads = 0,
    events = 0;
  c.subscribe(['projects'], () => events++);
  const load = async () => ({ data: ++loads, tags: ['projects'] });
  assert.equal(await c.read('list', load), 1);
  assert.equal(await c.read('list', load), 1);
  c.invalidate(['projects'], 'a');
  assert.equal(events, 1);
  assert.equal(await c.read('list', load), 2);
  c.invalidate(['projects'], 'a');
  assert.equal(events, 1);
  assert.equal(await c.read('list', load), 2);
  c.invalidate(['other'], 'b');
  assert.equal(events, 1);
});
test('invalidation during fetch repeats it before returning', async () => {
  const c = new TaggedCache();
  let release!: () => void;
  const pending = new Promise<void>((r) => (release = r));
  let loads = 0;
  const result = c.read('q', async () => {
    const data = ++loads;
    if (data === 1) await pending;
    return { data, tags: ['projects'] };
  });
  c.invalidate(['projects'], 'one');
  release();
  assert.equal(await result, 2);
});
test('reset clears all entries, cursor dedup and notifies subscribers', async () => {
  const c = new TaggedCache();
  let n = 0;
  c.subscribe(['x'], () => n++);
  await c.read('q', async () => ({ data: 1, tags: ['x'] }));
  c.invalidate(['x'], 'old');
  c.reset();
  c.invalidate(['x'], 'old');
  assert.equal(n, 3);
});
test('SSE parser handles split CRLF, comments, multiline and cursor', () => {
  const frames: unknown[] = [];
  const parser = new SSEParser((frame) => frames.push(frame));
  parser.push(': heartbeat\r');
  parser.push(
    '\nid: cursor-1\r\nevent: cache.invalidate\r\ndata: {"keys":\r\ndata: ["projects"]}\r\n\r\n',
  );
  assert.deepEqual(frames, [
    { id: 'cursor-1', event: 'cache.invalidate', data: '{"keys":\n["projects"]}' },
  ]);
});
test('unrelated cache entries survive tagged invalidation', async () => {
  const c = new TaggedCache();
  let n = 0;
  const load = async () => ({ data: ++n, tags: ['other'] });
  await c.read('other', load);
  c.invalidate(['projects'], 'x');
  assert.equal(await c.read('other', load), 1);
});
test('unsubscribe stops callbacks', () => {
  const c = new TaggedCache();
  let count = 0;
  const stop = c.subscribe(['x'], () => count++);
  stop();
  c.reset();
  assert.equal(count, 0);
});

test('empty SSE id clears reconnect cursor', () => {
  assert.equal(nextCursor('old', ''), undefined);
  assert.equal(nextCursor('old', undefined), 'old');
  assert.equal(nextCursor('old', 'new'), 'new');
});
test('cache reset refreshes subscriptions without clearing unsaved UI state', () => {
  const c = new TaggedCache();
  const modes: boolean[] = [];
  c.subscribe(['projects'], (clear) => modes.push(clear));
  c.reset(false);
  c.reset();
  assert.deepEqual(modes, [false, true]);
});

test('concurrent reads for the same key share one request', async () => {
  const c = new TaggedCache();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let loads = 0;
  const load = async () => {
    loads++;
    await gate;
    return { data: 42, tags: ['projects'] };
  };
  const a = c.read('same', load);
  const b = c.read('same', load);
  assert.equal(loads, 1);
  release();
  assert.deepEqual(await Promise.all([a, b]), [42, 42]);
});

test('unrelated invalidation does not repeat an in-flight read', async () => {
  const c = new TaggedCache();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let loads = 0;
  const pending = c.read('same', async () => {
    const data = ++loads;
    if (data === 1) await gate;
    return { data, tags: ['projects'] };
  });
  c.invalidate(['settings']);
  release();
  assert.equal(await pending, 1);
  assert.equal(loads, 1);
});

test('repeated relevant invalidations are bounded and finish with a fresh read', async () => {
  const c = new TaggedCache();
  let loads = 0;
  const pending = c.read('same', async () => {
    const data = ++loads;
    if (data <= 3) c.invalidate(['projects']);
    return { data, tags: ['projects'] };
  });
  assert.equal(await pending, 4);
  assert.equal(loads, 4);
});

test('reset prevents an older read from repopulating the cache', async () => {
  const c = new TaggedCache();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  const oldRead = c.read('same', async () => {
    await gate;
    return { data: 'old', tags: ['projects'] };
  });
  c.reset();
  release();
  assert.equal(await oldRead, 'old');
  let loads = 0;
  assert.equal(await c.read('same', async () => ({ data: ++loads, tags: ['projects'] })), 1);
});

test('more than three relevant invalidations still cache the final fresh result sequentially', async () => {
  const c = new TaggedCache();
  let loads = 0;
  let active = 0;
  let maxActive = 0;
  const result = await c.read('same', async () => {
    active++;
    maxActive = Math.max(maxActive, active);
    const data = ++loads;
    await Promise.resolve();
    if (data <= 5) c.invalidate(['projects']);
    active--;
    return { data, tags: ['projects'] };
  });
  assert.equal(result, 6);
  assert.equal(maxActive, 1);
  assert.equal(await c.read('same', async () => ({ data: 99, tags: ['projects'] })), 6);
});

test('silent eviction clears matching entries and refreshes in-flight reads without notifying', async () => {
  const c = new TaggedCache();
  let notifications = 0;
  c.subscribe(['projects'], () => notifications++);
  let loads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  const pending = c.read('same', async () => {
    const data = ++loads;
    if (data === 1) await gate;
    return { data, tags: ['projects'] };
  });
  c.evict(['projects']);
  release();
  assert.equal(await pending, 2);
  assert.equal(notifications, 0);
  let cachedLoad = 0;
  assert.equal(await c.read('same', async () => ({ data: ++cachedLoad, tags: ['projects'] })), 2);
  c.evict(['projects']);
  assert.equal(await c.read('same', async () => ({ data: ++cachedLoad, tags: ['projects'] })), 1);
  assert.equal(notifications, 0);
});
