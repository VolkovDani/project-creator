import { requirementURL } from '../../../../lib/rag.ts';
import { type GraphDraft, requirementNodeURL } from './processGraphView.ts';

type Evidence = GraphDraft['edges'][number]['evidence'][number];
const requirementRelations = new Set([
  'contains',
  'copies',
  'derives',
  'satisfies',
  'verifies',
  'refines',
  'traces',
]);

function mermaidEntity(character: string) {
  return `#${character.charCodeAt(0)};`;
}

function mermaidLabel(label: string) {
  return label.replace(/[[\]"#&;<>|%\\\x60\p{Cc}\u2028\u2029]/gu, mermaidEntity);
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

function validVersion(version: number | null): version is number {
  return version !== null && Number.isSafeInteger(version) && version > 0;
}

function validEvidence({ requirementId, version, start, end }: Evidence) {
  return (
    !!requirementId &&
    validVersion(version) &&
    Number.isSafeInteger(start) &&
    Number.isSafeInteger(end) &&
    start >= 0 &&
    end > start
  );
}

function webOrigin(origin: string) {
  const url = URL.parse(origin);
  return url?.protocol === 'http:' || url?.protocol === 'https:' ? url.origin : undefined;
}

function sourcePath(projectId: string, source: Evidence) {
  return requirementURL(projectId, source.requirementId, {
    sourceVersion: source.version,
    start: source.start,
    end: source.end,
  });
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
      if (!validEvidence(source)) continue;
      from.citations.push(source);
      if (from !== to) to.citations.push(source);
    }
  }

  const base = webOrigin(origin);
  if (base) {
    for (const node of nodes) {
      const source = nodeEvidence(node.label, node.citations);
      if (!source) continue;
      const url = new URL(sourcePath(projectId, source), base);
      lines.push(`  click ${node.mermaidId} href "${url.href}"`);
    }
  }

  return `${lines.join('\n')}\n`;
}

function requirementGraphExport(draft: GraphDraft, projectId: string, origin: string) {
  const base = webOrigin(origin);
  const absolute = (path: string) => (base ? new URL(path, base).href : undefined);
  const nodes = new Map(
    draft.nodes.flatMap(({ id, kind, label, requirementId, version }, index) => {
      if (kind !== 'requirement' || !requirementId || !validVersion(version)) return [];
      return [
        [
          id,
          {
            alias: `r${index + 1}`,
            label,
            requirementId,
            version,
            reference: `${requirementId} · v${version}`,
            href: absolute(requirementURL(projectId, requirementId, { sourceVersion: version })),
          },
        ] as const,
      ];
    }),
  );
  const edges = draft.edges.flatMap((edge) => {
    const from = nodes.get(edge.from)?.alias;
    const to = nodes.get(edge.to)?.alias;
    if (!from || !to || !requirementRelations.has(edge.kind)) return [];
    return [
      {
        from,
        to,
        kind: edge.kind,
        evidence: edge.evidence.filter(validEvidence).map((source) => ({
          quote: source.quote,
          href: absolute(sourcePath(projectId, source)),
        })),
      },
    ];
  });
  return { nodes, edges };
}

function requirementField(text: string) {
  return (
    text
      .replace(/[\p{Cc}\u2028\u2029]+/gu, ' ')
      .replace(/["#%&<>\\[\]*_~`!]/g, mermaidEntity)
      // This grammar recognizes direction directives even inside a quoted field.
      .replace(
        /direction(?=\s+(?:tb|bt|rl|lr))/gi,
        (word) => mermaidEntity(word[0]) + word.slice(1),
      )
  );
}

function requirementDiagramText(graph: ReturnType<typeof requirementGraphExport>, heading: string) {
  const lines = ['requirementDiagram'];
  if (heading) lines.push(`%% ${heading}`);
  for (const { alias, label, reference, href } of graph.nodes.values()) {
    lines.push(
      `  requirement ${alias} {`,
      `    id: "${requirementField(reference)}"`,
      `    text: "${requirementField(label)}"`,
      '  }',
    );
    if (href) lines.push(`  %% ${alias} ${href}`);
  }
  for (const { from, to, kind, evidence } of graph.edges) {
    lines.push(`  ${from} - ${kind} -> ${to}`);
    for (const { href } of evidence) {
      if (href) lines.push(`  %% ${from} - ${kind} -> ${to} ${href}`);
    }
  }
  return `${lines.join('\n')}\n`;
}

export function requirementGraphPreview(draft: GraphDraft, projectId: string) {
  const graph = requirementGraphExport(draft, projectId, '');
  const evidence = draft.edges.flatMap((edge) => edge.evidence.filter(validEvidence));
  return {
    source: requirementDiagramText(graph, ''),
    nodes: Array.from(graph.nodes.values(), ({ alias, label, requirementId, version }) => ({
      alias,
      label,
      requirementId,
      version,
      href: requirementNodeURL(projectId, requirementId, version, evidence),
    })),
  };
}

export function requirementGraphMermaid(
  draft: GraphDraft,
  projectId: string,
  origin: string,
  heading: string,
) {
  return requirementDiagramText(requirementGraphExport(draft, projectId, origin), heading);
}

function markdownLink(text: string, href: string | undefined) {
  const label = text.replace(/[\r\n\u2028\u2029]+/g, ' ').replace(/[\\[\]<>*_`~&]/g, '\\$&');
  return href ? `[${label}](<${href}>)` : label;
}

export function requirementGraphMarkdown(
  draft: GraphDraft,
  projectId: string,
  origin: string,
  labels: {
    heading: string;
    connections: string;
    relation: (kind: string) => string;
    partial?: string;
  },
) {
  const graph = requirementGraphExport(draft, projectId, origin);
  const lines = [`# ${labels.heading}`, ''];
  if (labels.partial) lines.push(labels.partial, '');
  lines.push('```mermaid', requirementDiagramText(graph, labels.heading).trimEnd(), '```', '');
  for (const { alias, reference, label, href } of graph.nodes.values()) {
    lines.push(`- ${alias} · ${markdownLink(`${reference} · ${label}`, href)}`);
  }
  lines.push('', `## ${labels.connections}`, '');
  for (const { from, to, kind, evidence } of graph.edges) {
    lines.push(`- ${from} · ${labels.relation(kind)} → ${to}`);
    for (const { quote, href } of evidence) lines.push(`  - ${markdownLink(quote, href)}`);
  }
  return `${lines.join('\n')}\n`;
}
