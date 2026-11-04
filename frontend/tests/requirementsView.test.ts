import assert from 'node:assert/strict';
import { test } from 'node:test';
import { findRequirementPage, summarizeIndex } from '../lib/requirementsView.ts';

test('source lookup finds the correct page past the first 20 requirements', async () => {
  const calls: number[] = [];
  const result = await findRequirementPage(
    'target',
    async (offset) => {
      calls.push(offset);
      return Array.from({ length: offset === 0 ? 100 : 7 }, (_, i) => ({
        id: offset + i === 105 ? 'target' : `id-${offset + i}`,
      }));
    },
    () => true,
  );
  assert.equal(result, 5);
  assert.deepEqual(calls, [0, 100]);
});
test('source lookup stops after navigation or missing source', async () => {
  let calls = 0;
  assert.equal(
    await findRequirementPage(
      'missing',
      async () => {
        calls++;
        return [{ id: 'other' }];
      },
      () => true,
    ),
    null,
  );
  assert.equal(calls, 1);
  assert.equal(
    await findRequirementPage(
      'target',
      async () => [{ id: 'target' }],
      () => false,
    ),
    undefined,
  );
});
test('index summary is page scoped and outdated index versions stay pending', () => {
  const rows = [
    { id: 'a', version: 2 },
    { id: 'b', version: 1 },
    { id: 'c', version: 1 },
    { id: 'd', version: 1 },
  ];
  assert.deepEqual(
    summarizeIndex(rows, [
      { requirementId: 'a', version: 1, status: 'ready' },
      { requirementId: 'b', version: 1, status: 'ready' },
      { requirementId: 'c', version: 1, status: 'failed' },
    ]),
    { total: 4, ready: 1, pending: 2, failed: 1 },
  );
});
