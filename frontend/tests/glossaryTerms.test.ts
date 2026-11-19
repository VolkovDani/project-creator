import assert from 'node:assert/strict';
import { test } from 'node:test';
import { clearClientSession, clientCache } from '../lib/client.ts';
import { loadGlossaryTerms } from '../lib/glossaryTerms.ts';

type Batch = { projectId: string; ids: string[] };

function transport() {
  const originalFetch = globalThis.fetch;
  const requests: Array<
    Batch & { respond: (version?: number, omitted?: string[]) => void; fail: () => void }
  > = [];
  globalThis.fetch = async (_, init) => {
    const batch = JSON.parse(String(init?.body)).variables as Batch;
    return new Promise((resolve) => {
      requests.push({
        ...batch,
        respond: (version = 1, omitted = []) =>
          resolve(
            Response.json({
              data: {
                glossaryTermsByIds: batch.ids
                  .filter((id) => !omitted.includes(id))
                  .map((id) => ({ id, name: batch.projectId, version })),
              },
            }),
          ),
        fail: () => resolve(new Response(null, { status: 500 })),
      });
    });
  };
  return {
    requests,
    batches: () => requests.map(({ projectId, ids }) => ({ projectId, ids })),
    restore: () => {
      globalThis.fetch = originalFetch;
      clearClientSession();
    },
  };
}

const nextTurn = () => new Promise<void>((resolve) => setImmediate(resolve));
const tag = 'project:alpha:glossary';

test('batches simultaneous consumers per project and reuses overlapping cards and misses', async (t) => {
  const api = transport();
  t.after(api.restore);
  assert.deepEqual(await loadGlossaryTerms('alpha', []), []);
  const a = loadGlossaryTerms('alpha', ['2', '1', '2']);
  const b = loadGlossaryTerms('alpha', ['1', '3']);
  const isolated = loadGlossaryTerms('beta', ['1']);
  await nextTurn();
  assert.deepEqual(api.batches(), [
    { projectId: 'alpha', ids: ['2', '1', '3'] },
    { projectId: 'beta', ids: ['1'] },
  ]);
  const c = loadGlossaryTerms('alpha', ['3', '4']);
  await nextTurn();
  assert.deepEqual(api.batches()[2], { projectId: 'alpha', ids: ['4'] });
  api.requests[0].respond(1, ['3']);
  api.requests[1].respond();
  api.requests[2].respond();
  assert.deepEqual(
    (await a).map((term) => term.id),
    ['2', '1'],
  );
  assert.deepEqual(
    (await b).map((term) => term.id),
    ['1'],
  );
  assert.deepEqual(
    (await c).map((term) => term.id),
    ['4'],
  );
  assert.deepEqual(
    (await isolated).map((term) => term.name),
    ['beta'],
  );
  assert.deepEqual(
    (await loadGlossaryTerms('alpha', ['4', '3', '1'])).map((term) => term.id),
    ['4', '1'],
  );
  assert.equal(api.requests.length, 3);
});

test('chunks a combined batch at the server limit', async (t) => {
  const api = transport();
  t.after(api.restore);
  const ids = Array.from({ length: 105 }, (_, i) => String(i));
  const result = loadGlossaryTerms('alpha', ids);
  await nextTurn();
  assert.deepEqual(
    api.requests.map(({ ids }) => ids.length),
    [100, 5],
  );
  api.requests[1].respond();
  api.requests[0].respond();
  assert.deepEqual(
    (await result).map((term) => term.id),
    ids,
  );
});

test('eviction invalidates cached misses and retries overlapping reads in flight', async (t) => {
  const api = transport();
  t.after(api.restore);
  const initial = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  api.requests[0].respond(1, ['1']);
  assert.deepEqual(await initial, []);
  const pending = loadGlossaryTerms('alpha', ['2', '3']);
  await nextTurn();
  clientCache.evict([tag]);
  const next = loadGlossaryTerms('alpha', ['1', '3']);
  await nextTurn();
  api.requests[1].respond();
  await nextTurn();
  assert.deepEqual(
    api.batches().map(({ ids }) => ids),
    [['1'], ['2', '3'], ['1'], ['2', '3']],
  );
  api.requests[2].respond(2);
  api.requests[3].respond(2);
  assert.deepEqual(
    (await pending).map((term) => term.version),
    [2, 2],
  );
  assert.deepEqual(
    (await next).map((term) => term.version),
    [2, 2],
  );
  clientCache.invalidate([tag]);
  const refreshed = loadGlossaryTerms('alpha', ['3', '1']);
  await nextTurn();
  api.requests[4].respond(3);
  assert.deepEqual(
    (await refreshed).map((term) => term.version),
    [3, 3],
  );
});

test('a failed batch can be retried', async (t) => {
  const api = transport();
  t.after(api.restore);
  const failed = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  api.requests[0].fail();
  await assert.rejects(failed, /request failed/);
  const retried = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  api.requests[1].respond();
  assert.equal((await retried)[0].id, '1');
});

test('session reset rejects old in-flight reads and discards queued old work', async (t) => {
  const api = transport();
  t.after(api.restore);
  const old = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  const queued = loadGlossaryTerms('alpha', ['2']);
  clearClientSession();
  const fresh = loadGlossaryTerms('alpha', ['1', '2']);
  const rejectedQueue = assert.rejects(queued, /session changed/);
  await nextTurn();
  assert.deepEqual(
    api.batches().map(({ ids }) => ids),
    [['1'], ['1', '2']],
  );
  api.requests[1].respond(2);
  assert.deepEqual(
    (await fresh).map((term) => term.version),
    [2, 2],
  );
  api.requests[0].respond();
  await assert.rejects(old, /session changed/);
  await rejectedQueue;
  assert.equal((await loadGlossaryTerms('alpha', ['1']))[0].version, 2);
  assert.equal(api.requests.length, 2);
});

test('an SSE cache reset discards stale responses and rejoins the refreshed reads', async (t) => {
  const api = transport();
  t.after(api.restore);
  const old = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  clientCache.reset(false);
  const fresh = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  api.requests[0].respond();
  await nextTurn();
  api.requests[1].respond(2);
  assert.equal((await old)[0].version, 2);
  assert.equal((await fresh)[0].version, 2);
  assert.equal(api.requests.length, 2);
});

test('an eviction refreshes a previously cached card while other cards are loading', async (t) => {
  const api = transport();
  t.after(api.restore);
  const initial = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  api.requests[0].respond();
  await initial;
  const mixed = loadGlossaryTerms('alpha', ['1', '2']);
  await nextTurn();
  clientCache.evict([tag]);
  api.requests[1].respond();
  await nextTurn();
  api.requests[2].respond(2);
  await nextTurn();
  assert.deepEqual(
    api.batches().map(({ ids }) => ids),
    [['1'], ['2'], ['2'], ['1']],
  );
  api.requests[3].respond(2);
  assert.deepEqual(
    (await mixed).map((card) => card.version),
    [2, 2],
  );
});

test('unrelated requirement changes preserve cached glossary cards', async (t) => {
  const api = transport();
  t.after(api.restore);
  const initial = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  api.requests[0].respond();
  await initial;
  clientCache.invalidate(['projects', 'project:alpha', 'project:alpha:requirements']);
  const cached = loadGlossaryTerms('alpha', ['1']);
  await nextTurn();
  assert.equal(api.requests.length, 1);
  assert.equal((await cached)[0].version, 1);
});
