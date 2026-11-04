import assert from 'node:assert/strict';
import { test } from 'node:test';
import { proxyGraphQL } from '../lib/graphqlProxy.ts';

test('proxy lets analysis finish after 10 seconds and forwards cancellation', async () => {
  const originalFetch = globalThis.fetch,
    originalTimeout = AbortSignal.timeout;
  const oldToken = process.env.PROJECT_CREATOR_TOKEN;
  process.env.PROJECT_CREATOR_TOKEN = 'test-token';
  const controller = new AbortController();
  let signal: AbortSignal | undefined;
  try {
    // Simulate 11 seconds passing without waiting: a ten-second budget expires.
    AbortSignal.timeout = (milliseconds: number) =>
      milliseconds <= 11000 ? AbortSignal.abort() : new AbortController().signal;
    globalThis.fetch = async (_input, options) => {
      signal = options?.signal as AbortSignal;
      signal.throwIfAborted();
      return Response.json({ data: { startProjectAnalysis: { status: 'completed' } } });
    };
    const response = await proxyGraphQL(
      new Request('http://localhost/api/graphql', {
        method: 'POST',
        body: JSON.stringify({ query: 'mutation{startProjectAnalysis(projectId:"p"){status}}' }),
        signal: controller.signal,
      }),
    );
    assert.equal(response.status, 200);
    assert.equal((await response.json()).data.startProjectAnalysis.status, 'completed');
    controller.abort();
    assert.equal(signal?.aborted, true);
  } finally {
    globalThis.fetch = originalFetch;
    AbortSignal.timeout = originalTimeout;
    if (oldToken === undefined) delete process.env.PROJECT_CREATOR_TOKEN;
    else process.env.PROJECT_CREATOR_TOKEN = oldToken;
  }
});

test('proxy rejects foreign origins before forwarding private data', async () => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  try {
    globalThis.fetch = async () => {
      calls++;
      return Response.json({});
    };
    const response = await proxyGraphQL(
      new Request('http://localhost/api/graphql', {
        method: 'POST',
        headers: { origin: 'http://evil.example', host: 'localhost' },
        body: '{}',
      }),
    );
    assert.equal(response.status, 403);
    assert.equal(calls, 0);
  } finally {
    globalThis.fetch = originalFetch;
  }
});
