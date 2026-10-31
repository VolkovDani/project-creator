import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Latest } from '../lib/latest.ts';

test('stale results are ignored after a newer request', () => {
  const latest = new Latest();
  const old = latest.begin();
  const current = latest.begin();
  assert.equal(latest.isCurrent(old), false);
  assert.equal(latest.isCurrent(current), true);
  latest.invalidate();
  assert.equal(latest.isCurrent(current), false);
});
