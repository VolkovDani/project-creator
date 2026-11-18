import assert from 'node:assert/strict';
import test from 'node:test';
import { generate } from '../codegen.ts';

test('local codegen rejects operations that do not match the supplied schema', async () => {
  await assert.rejects(
    generate('type Query { ok: Boolean }', [
      { location: 'bad.graphql', text: 'query Bad { missing }' },
    ]),
    /Cannot query field "missing"/,
  );
});
