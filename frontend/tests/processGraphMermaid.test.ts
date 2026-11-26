import assert from 'node:assert/strict';
import test from 'node:test';
import { processGraphMermaid } from '../app/projects/[id]/process-graphs/processGraphMermaid.ts';
import type { GraphDraft } from '../app/projects/[id]/process-graphs/processGraphView.ts';

const origin = 'https://requirements.example:3100';
const edgeLabels: Record<string, string> = { follows: 'Далее', requires: 'Требует' };
const translateKind = (kind: string) => edgeLabels[kind] ?? kind;
const action = { id: 'end', kind: 'action', label: 'Проверить "поле" [#34;] <x> | &' };
const actor = { id: 'n1', kind: 'actor', label: 'Ревьюер' };
const otherActor = { id: 'x---', kind: 'actor', label: actor.label };
const firstSource = {
  requirementId: 'req/first?',
  version: 2,
  start: 14,
  end: 33,
  quote: 'Роли нужен документ',
};
const directSource = {
  requirementId: 'req/точный#1',
  version: 5,
  start: 8,
  end: 24,
  quote: 'Как РЕВЬЮЕР, хочу',
};
const needs = {
  from: actor.id,
  to: action.id,
  kind: 'requires',
  label: '',
  evidence: [firstSource, directSource],
};
const draft = (nodes: GraphDraft['nodes'], edges: GraphDraft['edges']): GraphDraft => ({
  nodes,
  edges,
  partial: true,
  sourceCount: 2,
});

test('exports each card and directed connection, with labels escaped for Mermaid', () => {
  const diagram = processGraphMermaid(
    draft(
      [action, actor, otherActor],
      [
        needs,
        { ...needs, label: 'Поле | "#34;"' },
        { ...needs, from: action.id, to: actor.id, kind: 'follows', evidence: [] },
        { ...needs, from: otherActor.id, to: 'missing' },
      ],
    ),
    'project/one',
    origin,
    translateKind,
  );
  assert.deepEqual(
    diagram
      .trimEnd()
      .split('\n')
      .filter((line) => !line.trimStart().startsWith('click ')),
    [
      'flowchart TD',
      '  n1["Проверить #34;поле#34; #91;#35;34#59;#93; #60;x#62; #124; #38;"]',
      '  n2["Ревьюер"]',
      '  n3["Ревьюер"]',
      '  n2 -->|"Требует"| n1',
      '  n2 -->|"Поле #124; #34;#35;34#59;#34;"| n1',
      '  n1 -->|"Далее"| n2',
    ],
  );
  assert.doesNotMatch(diagram, /click n3\b/);
});

test('prefers a whole-label match in an incident citation, and otherwise keeps an incident citation', () => {
  const earlierPartialMatch = { ...firstSource, quote: 'Суперревьюер меняет документ' };
  const diagram = processGraphMermaid(
    draft(
      [action, actor, otherActor],
      [
        { ...needs, evidence: [earlierPartialMatch] },
        { ...needs, to: otherActor.id, evidence: [directSource] },
      ],
    ),
    'project/one',
    origin,
    translateKind,
  );
  assert.deepEqual(diagram.match(/^\s*click .*$/gm), [
    '  click n1 href "https://requirements.example:3100/projects/project%2Fone/requirements?requirement=req%2Ffirst%3F&version=2&start=14&end=33#requirement-req%2Ffirst%3F"',
    '  click n2 href "https://requirements.example:3100/projects/project%2Fone/requirements?requirement=req%2F%D1%82%D0%BE%D1%87%D0%BD%D1%8B%D0%B9%231&version=5&start=8&end=24#requirement-req%2F%D1%82%D0%BE%D1%87%D0%BD%D1%8B%D0%B9%231"',
    '  click n3 href "https://requirements.example:3100/projects/project%2Fone/requirements?requirement=req%2F%D1%82%D0%BE%D1%87%D0%BD%D1%8B%D0%B9%231&version=5&start=8&end=24#requirement-req%2F%D1%82%D0%BE%D1%87%D0%BD%D1%8B%D0%B9%231"',
  ]);
});

test('keeps cards without links when scheme or citation coordinates are invalid', () => {
  assert.doesNotMatch(
    processGraphMermaid(draft([action, actor], [needs]), 'p', 'file:///export.mmd', translateKind),
    /click/,
  );
  assert.doesNotMatch(
    processGraphMermaid(
      draft([action, actor], [{ ...needs, evidence: [{ ...firstSource, version: 0, end: 0 }] }]),
      'p',
      origin,
      translateKind,
    ),
    /click/,
  );
});
