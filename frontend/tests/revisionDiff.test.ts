import assert from 'node:assert/strict';
import { test } from 'node:test';
import { revisionDiff } from '../lib/revisionDiff.ts';

test('keeps unchanged text and handles missing text on either side', () => {
  const text = 'Статус: готово.\n';
  assert.deepEqual(revisionDiff(text, text), [{ kind: 'equal', text }]);
  assert.deepEqual(revisionDiff('', text), [{ kind: 'added', text }]);
  assert.deepEqual(revisionDiff(text, ''), [{ kind: 'removed', text }]);
  assert.deepEqual(revisionDiff('', ''), []);
});

test('marks two word replacements while leaving the shared words and punctuation intact', () => {
  assert.deepEqual(revisionDiff('Open окно, then close!', 'Закрыть окно, then reopen!'), [
    { kind: 'removed', text: 'Open' },
    { kind: 'added', text: 'Закрыть' },
    { kind: 'equal', text: ' окно, then ' },
    { kind: 'removed', text: 'close' },
    { kind: 'added', text: 'reopen' },
    { kind: 'equal', text: '!' },
  ]);
});

test('preserves line endings, spaces, combining marks and joined emoji', () => {
  assert.deepEqual(revisionDiff('cafe\u0301?\r\n\t👩🏽‍💻 ', 'café!\r\n\t👩🏽‍🔬 '), [
    { kind: 'removed', text: 'cafe\u0301?' },
    { kind: 'added', text: 'café!' },
    { kind: 'equal', text: '\r\n\t' },
    { kind: 'removed', text: '👩🏽‍💻' },
    { kind: 'added', text: '👩🏽‍🔬' },
    { kind: 'equal', text: ' ' },
  ]);
  assert.deepEqual(revisionDiff('A B', 'A  B'), [
    { kind: 'equal', text: 'A' },
    { kind: 'removed', text: ' ' },
    { kind: 'added', text: '  ' },
    { kind: 'equal', text: 'B' },
  ]);
});

test('isolates insertions and deletions separated by a long repeated passage', () => {
  const common = 'go '.repeat(3200);
  assert.deepEqual(revisionDiff(`old ${common}end.`, `${common}[new]end.`), [
    { kind: 'removed', text: 'old ' },
    { kind: 'equal', text: common },
    { kind: 'added', text: '[new]' },
    { kind: 'equal', text: 'end.' },
  ]);
});

test('preserves both complete 10,000-code-point revisions when edits are too numerous', () => {
  const start = '📁 ';
  const end = '\n.';
  const oldMiddle = 'а?'.repeat(4998);
  const newMiddle = 'b!'.repeat(4998);
  assert.deepEqual(revisionDiff(start + oldMiddle + end, start + newMiddle + end), [
    { kind: 'equal', text: start },
    { kind: 'removed', text: oldMiddle },
    { kind: 'added', text: newMiddle },
    { kind: 'equal', text: end },
  ]);
});
