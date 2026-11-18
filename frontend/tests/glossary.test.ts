import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { validateGlossary } from '../lib/glossaryValidation.ts';

const limits = JSON.parse(readFileSync('../contracts/glossary.json', 'utf8'));
test('shared glossary validation fixtures', () => {
  for (const c of JSON.parse(readFileSync('../contracts/glossary.fixtures.json', 'utf8'))) {
    assert.equal(validateGlossary(limits, c.term)?.code ?? '', c.code, c.name);
  }
});
