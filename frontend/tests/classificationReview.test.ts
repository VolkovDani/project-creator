import assert from 'node:assert/strict';
import { test } from 'node:test';
import rules from '../../contracts/classification.json' with { type: 'json' };
import en from '../../contracts/i18n/en.json' with { type: 'json' };
import ru from '../../contracts/i18n/ru.json' with { type: 'json' };
import {
  classificationCounts,
  classificationDraft,
  draftIsCurrent,
  groupAlternatives,
} from '../lib/classificationReview.ts';

test('draft captures both axes and optimistic locks before a background update', () => {
  const entry = {
    requirementId: 'a',
    textVersion: 1,
    revision: 8,
    semanticType: 'functional_requirement',
    representationType: 'test_case',
  };
  const draft = classificationDraft(entry, 2);
  assert.deepEqual(draft, {
    requirementId: 'a',
    textVersion: 2,
    revision: 8,
    semanticType: 'functional_requirement',
    representationType: 'test_case',
  });
  assert.ok(draftIsCurrent(draft, { ...entry, revision: 8 }, 2));
  assert.equal(draftIsCurrent(draft, { ...entry, revision: 9 }, 2), false);
  assert.equal(draftIsCurrent(draft, entry, 3), false);
  assert.equal(draft.revision, 8);
});

test('joint group counts are summed separately for each axis, including undecided records', () => {
  const counts = classificationCounts([
    { semanticType: 'unknown', representationType: 'scenario', count: 4 },
    { semanticType: 'functional_requirement', representationType: 'scenario', count: 3 },
    { semanticType: 'unknown', representationType: 'unknown', count: 2 },
  ]);
  assert.equal(counts.semantic.get('unknown'), 6);
  assert.equal(counts.representation.get('scenario'), 7);
  assert.equal(counts.representation.get('unknown'), 2);
});

test('reviewers can look up a clue, an example and a counterexample for every class in both languages', () => {
  for (const dict of [en, ru]) {
    const labels: Record<string, string> = dict;
    for (const code of new Set([...rules.semantic, ...rules.representation])) {
      for (const kind of ['clue', 'example', 'counterexample']) {
        assert.ok(labels[`classification.guide.${code}.${kind}`]?.trim(), `${code} ${kind}`);
      }
    }
  }
});

test('ranks are comparable only inside their axis and joint legacy scores stay paired', () => {
  const semantic = { semanticType: 'business_rule', representationType: '', score: 0.4 };
  const form = { semanticType: '', representationType: 'scenario', score: 0.9 };
  const joint = { semanticType: 'business_rule', representationType: 'user_story', score: 0.3 };
  assert.deepEqual(groupAlternatives([semantic, form, joint]), {
    meaning: [semantic],
    form: [form],
    joint: [joint],
  });
});

test('each group shows its top three scores, keeping tied entries in source order', () => {
  const scores = [null, 0.2, 0.8, 0.2, 0.1, 0.9];
  const alternatives = scores.flatMap((score, i) => [
    { semanticType: String(i), representationType: '', score },
    { semanticType: '', representationType: String(i), score },
    { semanticType: String(i), representationType: String(i), score },
  ]);
  const original = structuredClone(alternatives);
  const groups = groupAlternatives(alternatives);
  for (const group of Object.values(groups)) {
    assert.deepEqual(
      group.map((item) => item.semanticType || item.representationType),
      ['5', '2', '1'],
    );
  }
  assert.deepEqual(alternatives, original);
});

test('bulk-eligible labels must both be known current model suggestions', async () => {
  const { eligibleSuggestion } = await import('../lib/classificationReview.ts');
  const entry = {
    requirementId: 'r',
    textVersion: 3,
    revision: 7,
    source: 'model',
    status: 'suggested',
    reason: '',
    semanticType: 'functional_requirement',
    representationType: 'test_case',
  };
  assert.ok(eligibleSuggestion(entry, 3));
  for (const change of [
    { status: 'needs_review' },
    { source: 'user' },
    { reason: 'uncalibrated' },
    { semanticType: 'unknown' },
    { representationType: 'unknown' },
    { semanticType: 'unsupported' },
    { textVersion: 2 },
  ]) {
    assert.equal(eligibleSuggestion({ ...entry, ...change }, 3), false);
  }
});
