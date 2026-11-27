import type { ProcessGraphRunFieldsFragment } from '../../../../lib/generated';
import { requirementURL } from '../../../../lib/rag.ts';

export type GraphDraft = NonNullable<ProcessGraphRunFieldsFragment['draft']>;
export type GraphNode = GraphDraft['nodes'][number];
type GraphEdge = GraphDraft['edges'][number];

type Branch = { edge: GraphEdge; to: GraphNode };
type Group = { from: GraphNode; branches: Branch[] };

export function requirementNodeURL(
  projectId: string,
  requirementId: string,
  version: number,
  evidence: GraphEdge['evidence'],
) {
  const source = evidence.find(
    (item) => item.requirementId === requirementId && item.version === version,
  );
  return requirementURL(
    projectId,
    requirementId,
    source
      ? { sourceVersion: version, start: source.start, end: source.end }
      : { sourceVersion: version },
  );
}

export function groupProcessGraph(nodes: GraphNode[], edges: GraphEdge[]) {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const byOrigin = new Map<string, Group>();
  const connected = new Set<string>();
  for (const edge of edges) {
    const from = byId.get(edge.from);
    const to = byId.get(edge.to);
    if (!from || !to) continue;
    let group = byOrigin.get(from.id);
    if (!group) {
      group = { from, branches: [] };
      byOrigin.set(from.id, group);
    }
    group.branches.push({ edge, to });
    connected.add(from.id);
    connected.add(to.id);
  }
  return {
    groups: [...byOrigin.values()],
    isolated: nodes.filter((node) => !connected.has(node.id)),
  };
}

export function layoutProcessGraph(nodes: GraphNode[], edges: GraphEdge[]) {
  const remaining = new Set(nodes.map((node) => node.id));
  const incoming = new Map(nodes.map((node) => [node.id, 0]));
  const levels = new Map(nodes.map((node) => [node.id, 0]));
  const validEdges = edges.filter((edge) => remaining.has(edge.from) && remaining.has(edge.to));
  for (const edge of validEdges) {
    incoming.set(edge.to, (incoming.get(edge.to) ?? 0) + 1);
  }

  while (remaining.size) {
    const candidates = [...remaining];
    const id =
      candidates.find((node) => incoming.get(node) === 0) ??
      candidates.reduce((best, node) =>
        (levels.get(node) ?? 0) > (levels.get(best) ?? 0) ? node : best,
      );
    remaining.delete(id);
    const nextLevel = (levels.get(id) ?? 0) + 1;
    for (const edge of validEdges) {
      if (edge.from !== id || !remaining.has(edge.to)) continue;
      incoming.set(edge.to, (incoming.get(edge.to) ?? 0) - 1);
      levels.set(edge.to, Math.max(levels.get(edge.to) ?? 0, nextLevel));
    }
  }

  return {
    nodes: nodes.map((node) => ({ node, level: levels.get(node.id) ?? 0 })),
    edges: validEdges,
  };
}
