export const mermaidLimits = { maxTextSize: 80_000, maxEdges: 512, maxNodes: 256 };

type MermaidSource = {
  kind: 'flowchart' | 'requirementDiagram';
  nodeIds: string[];
};

// The viewer accepts the subset produced by the application serializers.
const alias = '([a-z][a-z0-9_]{0,63})';
const quoted = '"(?:#[0-9]{1,5};|[^"#<>&%`\\\\\\u0000-\\u001f\\u007f\\u2028\\u2029])*"';
const flowNode = new RegExp(`^${alias}\\[${quoted}\\]$`);
const flowEdge = new RegExp(`^${alias} -->\\|${quoted}\\| ${alias}$`);
const requirementNode = new RegExp(`^requirement ${alias} \\{$`);
const requirementId = new RegExp(`^id: ${quoted}$`);
const requirementText = new RegExp(`^text: ${quoted}$`);
const requirementEdge = new RegExp(
  `^${alias} - (?:contains|copies|derives|satisfies|verifies|refines|traces) -> ${alias}$`,
);

function invalid(): never {
  throw new Error('Invalid Mermaid source');
}

export function mermaidSource(source: string): MermaidSource | null {
  if (source.length > mermaidLimits.maxTextSize) invalid();
  const [header, ...lines] = source
    .split(/\r?\n/u)
    .map((line) => line.trim())
    .filter(Boolean);
  if (!header) return null;
  const kind = /^flowchart (?:TD|TB|BT|LR|RL)$/u.test(header)
    ? 'flowchart'
    : header === 'requirementDiagram'
      ? 'requirementDiagram'
      : invalid();
  const nodeIds = new Set<string>();
  const edges: [string, string][] = [];
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    const node = (kind === 'flowchart' ? flowNode : requirementNode).exec(line);
    if (node) {
      if (nodeIds.has(node[1]) || nodeIds.size >= mermaidLimits.maxNodes) invalid();
      if (kind === 'requirementDiagram') {
        if (
          !requirementId.test(lines[i + 1] ?? '') ||
          !requirementText.test(lines[i + 2] ?? '') ||
          lines[i + 3] !== '}'
        )
          invalid();
        i += 3;
      }
      nodeIds.add(node[1]);
      continue;
    }
    const edge = (kind === 'flowchart' ? flowEdge : requirementEdge).exec(line);
    if (!edge || edges.length >= mermaidLimits.maxEdges) invalid();
    edges.push([edge[1], edge[2]]);
  }
  if (edges.some(([from, to]) => !nodeIds.has(from) || !nodeIds.has(to))) invalid();
  return nodeIds.size ? { kind, nodeIds: [...nodeIds] } : null;
}
