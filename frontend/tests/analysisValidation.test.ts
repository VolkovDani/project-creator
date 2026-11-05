import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import {
  analysisRules,
  validateAnalysisReplacementStrategy,
  validateAnalysisSelection,
} from '../lib/analysisValidation.ts';

test('shared analysis selection fixtures', () => {
  const fixtures = JSON.parse(
    readFileSync(new URL('../../contracts/analysis.fixtures.json', import.meta.url), 'utf8'),
  );
  for (const fixture of fixtures)
    assert.deepEqual(
      validateAnalysisSelection(fixture.input),
      { normalized: fixture.normalized, errors: fixture.errors },
      fixture.name,
    );
});
test('shared analysis replacement strategy fixtures', () => {
  const fixtures = JSON.parse(
    readFileSync(
      new URL('../../contracts/analysis-replacement.fixtures.json', import.meta.url),
      'utf8',
    ),
  );
  for (const fixture of fixtures)
    assert.deepEqual(
      validateAnalysisReplacementStrategy(fixture.strategy),
      fixture.errors,
      fixture.name,
    );
});
test('analysis selection rejects non-finite thresholds', () => {
  for (const threshold of [NaN, Infinity, -Infinity])
    assert.equal(
      validateAnalysisSelection({ mode: analysisRules.defaultMode, threshold }).errors[0].code,
      'RANGE',
    );
});
