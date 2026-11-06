import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { validateSummary } from '../lib/summaryValidation.ts';

test('shared summary validation fixtures', () => {
  const contract = JSON.parse(
    readFileSync(new URL('../../contracts/summary.json', import.meta.url), 'utf8'),
  );
  const fixtures = JSON.parse(
    readFileSync(new URL('../../contracts/summary.fixtures.json', import.meta.url), 'utf8'),
  );
  for (const fixture of fixtures)
    assert.equal(validateSummary(fixture.text, contract).valid, fixture.valid, fixture.name);
});

test('summary validation rejects malformed UTF-16 and counts encoded bytes', () => {
  const contract = { version: 1, lengthUnit: 'utf8-bytes', maxBytes: 350, required: true };
  assert.equal(validateSummary('\ud800', contract).valid, false);
  assert.equal(validateSummary('я'.repeat(175), contract).byteLength, 350);
});
