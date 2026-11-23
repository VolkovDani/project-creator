import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parser } from 'markdown-to-jsx/react';
import { markdownGlossary } from '../app/projects/[id]/markdownGlossary.ts';

test('matches in Markdown syntax do not hide confirmed terms in nearby prose', () => {
  for (const [text, expected] of [
    ['**HTTPS**: https://example.org', ['HTTPS']],
    ['1. **API**', ['API']],
    ['AMP &amp; **API**', ['AMP', 'API']],
  ] as const) {
    const matches = Array.from(text.matchAll(/https|1|api|amp/gi), ({ 0: label, index }) => ({
      start: index,
      end: index + label.length,
      terms: [{ id: label.toLowerCase() }],
    }));
    const options = { forceBlock: true };
    const result = markdownGlossary(text, parser(text, options), matches, options);
    const labels = [...result.values()]
      .flat()
      .flatMap((segment) => (typeof segment === 'string' ? [] : [segment.label]));
    assert.deepEqual(labels, expected, text);
  }
});
