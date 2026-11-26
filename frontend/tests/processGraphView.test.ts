import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  groupProcessGraph,
  layoutProcessGraph,
} from '../app/projects/[id]/process-graphs/processGraphView.ts';

const decision = { id: 'check', label: 'Check payment', kind: 'condition' };
const approve = { id: 'approve', label: 'Approve order', kind: 'action' };
const retry = { id: 'retry', label: 'Retry payment', kind: 'action' };
const operator = { id: 'operator', label: 'Operator', kind: 'actor' };
const evidence = {
  requirementId: 'requirement-1',
  version: 3,
  start: 5,
  end: 15,
  quote: 'paid order',
};
const success = {
  from: 'check',
  to: 'approve',
  kind: 'leads_to',
  label: 'Payment authorized',
  evidence: [evidence],
};
const failure = {
  ...success,
  to: 'retry',
  label: 'Payment declined',
};
const loop = {
  ...success,
  from: 'retry',
  to: 'check',
  kind: 'follows',
  label: '',
};

test('shows both branches and their evidence under one origin, and keeps a retry loop finite', () => {
  const view = groupProcessGraph([decision, approve, retry, operator], [success, failure, loop]);
  assert.deepEqual(view, {
    groups: [
      {
        from: decision,
        branches: [
          { edge: success, to: approve },
          { edge: failure, to: retry },
        ],
      },
      { from: retry, branches: [{ edge: loop, to: decision }] },
    ],
    isolated: [operator],
  });
});

test('preserves different relations between the same nodes and ignores missing endpoints', () => {
  const additional = { ...success, kind: 'permits' };
  const view = groupProcessGraph(
    [decision, approve],
    [success, additional, { ...success, to: 'missing' }],
  );
  assert.deepEqual(view.groups[0].branches, [
    { edge: success, to: approve },
    { edge: additional, to: approve },
  ]);
});

test('lays out each node once, preserving branches, a back edge, and a disconnected node', () => {
  const start = { id: 'start', label: 'Open order', kind: 'event' };
  const trigger = { ...success, from: 'start', to: 'check', kind: 'triggers' };
  const layout = layoutProcessGraph(
    [start, decision, approve, retry, operator],
    [trigger, success, failure, loop],
  );
  const levels = Object.fromEntries(layout.nodes.map(({ node, level }) => [node.id, level]));
  assert.equal(layout.nodes.length, 5);
  assert.equal(levels.start, 0);
  assert.equal(levels.operator, 0);
  assert.ok(levels.check > levels.start);
  assert.ok(levels.approve > levels.check);
  assert.ok(levels.retry > levels.check);
  assert.deepEqual(layout.edges, [trigger, success, failure, loop]);
});

test('process graph translations have the same keys and interpolation parameters', () => {
  function translations(locale: string) {
    const dict = JSON.parse(
      readFileSync(new URL(`../../contracts/i18n/${locale}.json`, import.meta.url), 'utf8'),
    ) as Record<string, string>;
    return Object.fromEntries(
      Object.entries(dict)
        .filter(([key]) => key.startsWith('processGraph.') || key.endsWith('.process-graphs'))
        .map(([key, value]) => [
          key,
          [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort(),
        ]),
    );
  }
  const en = translations('en');
  assert.ok('nav.process-graphs' in en);
  assert.deepEqual(en['processGraph.requirement'], ['id', 'version']);
  assert.deepEqual(translations('ru'), en);
});
