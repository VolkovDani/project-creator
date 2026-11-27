import assert from 'node:assert/strict';
import test from 'node:test';
import { mermaidSource } from '../lib/mermaidSource.ts';

test('accepts generated flowcharts and requirement diagrams with quoted multilingual text', () => {
  assert.deepEqual(
    mermaidSource('flowchart TD\n  n1["Заявка #34;A#34;"]\n  n2["Accept"]\n  n1 -->|"Да"| n2\n'),
    { kind: 'flowchart', nodeIds: ['n1', 'n2'] },
  );
  assert.deepEqual(
    mermaidSource(
      'requirementDiagram\n  requirement r1 {\n    id: "req-1 · v2"\n    text: "Данные"\n  }\n' +
        '  requirement r2 {\n    id: "req-2 · v1"\n    text: "Review"\n  }\n  r1 - refines -> r2',
    ),
    { kind: 'requirementDiagram', nodeIds: ['r1', 'r2'] },
  );
  assert.equal(mermaidSource(' \n  '), null);
});

test('rejects configuration, links, markup and invalid graph structure', () => {
  for (const suffix of [
    '%%{init: {"theme": "dark"}}%%',
    '---\nconfig:\n  theme: dark\n---',
    'click n1 href "https://example.com/"',
    'n2["<b>Label</b>"]',
    'n1["Duplicate"]',
    'n1 -->|"Missing"| n2',
    'n2["Unclosed',
  ]) {
    assert.throws(() => mermaidSource(`flowchart TD\n  n1["Start"]\n  ${suffix}`), Error, suffix);
  }
  assert.throws(() => mermaidSource('requirementDiagram\nrequirement r1 {\ntext: "Missing ID"\n}'));
  assert.throws(() => mermaidSource('sequenceDiagram\nAlice->>Bob: Hello'));
});

test('rejects oversized diagrams before layout', () => {
  assert.throws(() => mermaidSource(`flowchart TD\nn1["${'x'.repeat(100_000)}"]`));
  const nodes = Array.from({ length: 257 }, (_, i) => `n${i}["A"]`).join('\n');
  assert.throws(() => mermaidSource(`flowchart TD\n${nodes}`));
  const edges = 'n1 -->|"Link"| n2\n'.repeat(513);
  assert.throws(() => mermaidSource(`flowchart TD\nn1["A"]\nn2["B"]\n${edges}`));
});
