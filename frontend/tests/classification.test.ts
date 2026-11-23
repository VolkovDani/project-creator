import assert from 'node:assert/strict';
import { test } from 'node:test';
import rules from '../../contracts/classification.json' with { type: 'json' };
import en from '../../contracts/i18n/en.json' with { type: 'json' };
import ru from '../../contracts/i18n/ru.json' with { type: 'json' };

test('test cases are a representation with a label in each locale', () => {
  assert.ok(rules.representation.includes('test_case'));
  assert.ok(!rules.semantic.includes('test_case'));
  for (const dictionary of [en, ru]) {
    for (const code of [...rules.semantic, ...rules.representation]) {
      const labels: Record<string, string> = dictionary;
      assert.ok(labels[`classification.${code}`]?.trim(), code);
    }
  }
});
