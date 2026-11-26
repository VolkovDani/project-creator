import { requirementURL } from '../../../../lib/rag.ts';
import type { GraphDraft } from './processGraphView.ts';

type Evidence = GraphDraft['edges'][number]['evidence'][number];

function mermaidLabel(label: string) {
  return label.replace(
    /[[\]"#&;<>|%\\\x60\r\n\u2028\u2029]/g,
    (character) => `#${character.charCodeAt(0)};`,
  );
}

function normalizedWords(text: string) {
  return (
    text
      .normalize('NFC')
      .toLowerCase()
      .match(/[\p{L}\p{M}\p{N}]+/gu)
      ?.join(' ') ?? ''
  );
}

function nodeEvidence(label: string, citations: Evidence[]) {
  const phrase = normalizedWords(label);
  return (
    (phrase &&
      citations.find((source) => ` ${normalizedWords(source.quote)} `.includes(` ${phrase} `))) ||
    citations[0]
  );
}

export function processGraphMermaid(
  draft: GraphDraft,
  projectId: string,
  origin: string,
  edgeKindLabel: (kind: string) => string,
) {
  const nodes = draft.nodes.map(({ id, label }, index) => ({
    id,
    label,
    mermaidId: `n${index + 1}`,
    citations: [] as Evidence[],
  }));
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const lines = ['flowchart TD'];
  for (const node of nodes) {
    lines.push(`  ${node.mermaidId}["${mermaidLabel(node.label)}"]`);
  }
  for (const edge of draft.edges) {
    const from = byId.get(edge.from);
    const to = byId.get(edge.to);
    if (!from || !to) continue;
    const label = mermaidLabel(edge.label || edgeKindLabel(edge.kind));
    lines.push(`  ${from.mermaidId} -->|"${label}"| ${to.mermaidId}`);
    for (const source of edge.evidence) {
      if (
        !source.requirementId ||
        !Number.isSafeInteger(source.version) ||
        source.version < 1 ||
        !Number.isSafeInteger(source.start) ||
        !Number.isSafeInteger(source.end) ||
        source.start < 0 ||
        source.end <= source.start
      ) {
        continue;
      }
      from.citations.push(source);
      if (from !== to) to.citations.push(source);
    }
  }

  const base = URL.parse(origin);
  if (base?.protocol === 'http:' || base?.protocol === 'https:') {
    for (const node of nodes) {
      const source = nodeEvidence(node.label, node.citations);
      if (!source) continue;
      const path = requirementURL(projectId, source.requirementId, {
        sourceVersion: source.version,
        start: source.start,
        end: source.end,
      });
      const url = new URL(path, base.origin);
      lines.push(`  click ${node.mermaidId} href "${url.href}"`);
    }
  }

  return `${lines.join('\n')}\n`;
}
