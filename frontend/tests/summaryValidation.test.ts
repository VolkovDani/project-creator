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
    assert.deepEqual(
      validateSummary(fixture.text, contract),
      {
        valid: fixture.valid,
        layaEligible: fixture.layaEligible,
        byteLength: new TextEncoder().encode(fixture.text).length,
      },
      fixture.name,
    );
});

test('summary validation preserves long previews while limiting only Laya eligibility', () => {
  const contract = { version: 2, lengthUnit: 'utf8-bytes', layaMaxBytes: 350, required: true };
  assert.deepEqual(validateSummary('я'.repeat(175), contract), {
    valid: true,
    layaEligible: true,
    byteLength: 350,
  });
  assert.deepEqual(validateSummary('я'.repeat(176), contract), {
    valid: true,
    layaEligible: false,
    byteLength: 352,
  });
  assert.equal(validateSummary('\ud800', contract).valid, false);
  assert.equal(validateSummary('preview\0text', contract).valid, false);
});
