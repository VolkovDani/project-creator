import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parse } from 'graphql';
import { clearClientSession, clientCache, query } from '../lib/client.ts';

test('unauthorized responses clear state once instead of recursive refetch resets', async () => {
  const original = globalThis.fetch;
  let resets = 0;
  const stop = clientCache.subscribe(['projects'], (clear) => {
    if (clear) resets++;
  });
  try {
    globalThis.fetch = async () => new Response(null, { status: 401 });
    const document = parse('{ projects(limit:20,offset:0){id} }');
    await assert.rejects(query(document, {}), /unauthorized/);
    await assert.rejects(query(document, {}), /unauthorized/);
    assert.equal(resets, 1);
  } finally {
    globalThis.fetch = original;
    stop();
  }
});
test('mutation response from an earlier session is rejected after session clear', async () => {
  const original = globalThis.fetch;
  let resolve!: (response: Response) => void;
  try {
    globalThis.fetch = () =>
      new Promise<Response>((r) => {
        resolve = r;
      });
    const result = query(
      parse(
        'mutation { createProject(input:{name:"Old",goal:"Old",description:""}){project{id}} }',
      ),
      {},
    );
    clearClientSession();
    resolve(
      new Response(
        JSON.stringify({ data: { createProject: { project: { id: 'old-session' } } } }),
        { status: 200 },
      ),
    );
    await assert.rejects(result, /session changed/);
  } finally {
    globalThis.fetch = original;
  }
});
test('old unauthorized response cannot clear a new session', async () => {
  const original = globalThis.fetch;
  let resolve!: (response: Response) => void;
  let resets = 0;
  try {
    globalThis.fetch = () =>
      new Promise<Response>((r) => {
        resolve = r;
      });
    const result = query(parse('{ projects(limit:20,offset:0){id} }'), {});
    clearClientSession();
    const stop = clientCache.subscribe(['projects'], () => resets++);
    resolve(new Response(null, { status: 401 }));
    await assert.rejects(result, /session changed/);
    assert.equal(resets, 0);
    stop();
  } finally {
    globalThis.fetch = original;
  }
});
