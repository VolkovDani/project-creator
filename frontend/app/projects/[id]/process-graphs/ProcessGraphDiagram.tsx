import { useId } from 'react';
import type { ProcessGraphMode } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import styles from './process-graphs.module.css';
import { type GraphDraft, layoutProcessGraph, requirementNodeURL } from './processGraphView';

const nodeWidth = 196;
const nodeHeight = 74;
const columnGap = 40;
const lateralPadding = 34;

function shortText(text: string, limit: number) {
  const characters = Array.from(text);
  return characters.length > limit ? `${characters.slice(0, limit - 1).join('')}…` : text;
}

export function ProcessGraphDiagram({
  draft,
  locale,
  mode,
  projectId,
}: {
  draft: GraphDraft;
  locale: Locale;
  mode: ProcessGraphMode;
  projectId: string;
}) {
  const id = useId();
  const marker = `${id}-arrow`;
  const { nodes, edges } = layoutProcessGraph(draft.nodes, draft.edges);
  const requirements = mode === 'REQUIREMENTS';
  const diagramLabel = t(
    locale,
    requirements ? 'processGraph.requirementsDiagram' : 'processGraph.diagram',
  );
  const evidence = requirements ? edges.flatMap((edge) => edge.evidence) : [];
  const pairs = new Map<string, typeof edges>();
  const pairKey = ({ from, to }: (typeof edges)[number]) => JSON.stringify([from, to]);
  for (const edge of edges) {
    const key = pairKey(edge);
    const parallels = pairs.get(key) ?? [];
    parallels.push(edge);
    pairs.set(key, parallels);
  }
  const maxParallel = Math.max(1, ...Array.from(pairs.values(), (pair) => pair.length));
  const rowGap = Math.max(86, maxParallel * 20 + 16);
  const verticalPadding = Math.max(34, maxParallel * 20);
  const rows = new Map<number, typeof nodes>();
  for (const node of nodes) {
    const row = rows.get(node.level) ?? [];
    row.push(node);
    rows.set(node.level, row);
  }
  const columns = Math.max(1, ...Array.from(rows.values(), (row) => row.length));
  const rowCount = Math.max(0, ...rows.keys()) + 1;
  const width = lateralPadding * 2 + columns * nodeWidth + (columns - 1) * columnGap;
  const height = verticalPadding * 2 + rowCount * nodeHeight + (rowCount - 1) * rowGap;
  const positions = new Map(
    [...rows].flatMap(([level, row]) =>
      row.map(
        ({ node }, column) =>
          [
            node.id,
            {
              node,
              x:
                (width - row.length * nodeWidth - (row.length - 1) * columnGap) / 2 +
                column * (nodeWidth + columnGap),
              y: verticalPadding + level * (nodeHeight + rowGap),
              level,
            },
          ] as const,
      ),
    ),
  );

  return (
    <section
      className={styles.diagram}
      aria-label={diagramLabel}
      // biome-ignore lint/a11y/noNoninteractiveTabindex: Keyboard users need to scroll the diagram.
      tabIndex={0}
    >
      <svg
        width={width}
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        aria-hidden={!requirements || undefined}
      >
        <title>{diagramLabel}</title>
        <defs>
          <marker
            id={marker}
            viewBox="0 0 10 10"
            refX="9"
            refY="5"
            markerWidth="7"
            markerHeight="7"
            orient="auto"
          >
            <path d="M 0 0 L 10 5 L 0 10 z" />
          </marker>
        </defs>
        {edges.map((edge, index) => {
          const from = positions.get(edge.from);
          const to = positions.get(edge.to);
          if (!from || !to) return null;
          const parallels = pairs.get(pairKey(edge)) ?? [edge];
          const slot = parallels.indexOf(edge);
          const portShift = (slot - (parallels.length - 1) / 2) * 18;
          const x1 = from.x + nodeWidth / 2 + portShift;
          const y1 = from.y + nodeHeight;
          const x2 = to.x + nodeWidth / 2 + portShift;
          const y2 = to.y;
          const middleY = (y1 + y2) / 2;
          const adjacent = to.level === from.level + 1;
          const laneOffset = 8 + index * 3;
          const lane = index % 2 === 0 ? laneOffset : width - laneOffset;
          const bend = 16 + slot * 18;
          const line = adjacent
            ? `M ${x1} ${y1} C ${x1} ${middleY}, ${x2} ${middleY}, ${x2} ${y2}`
            : `M ${x1} ${y1} V ${y1 + bend} H ${lane} V ${y2 - bend} H ${x2} V ${y2}`;
          const labelX = (x1 + (adjacent ? x2 : lane)) / 2;
          const labelY = adjacent
            ? middleY - 8 + (slot - (parallels.length - 1) / 2) * 18
            : y1 + bend - 4;
          const kind = t(locale, `processGraph.edge.${edge.kind}`);
          const label = requirements ? kind : edge.label || kind;
          return (
            <g key={JSON.stringify([edge.from, edge.to, edge.kind, edge.label])}>
              <title>{label}</title>
              <path d={line} markerEnd={`url(#${marker})`} className={styles.diagramEdge} />
              <text x={labelX} y={labelY} className={styles.diagramEdgeLabel}>
                {shortText(label, 18)}
              </text>
            </g>
          );
        })}
        {[...positions.values()].map(({ node, x, y }) => {
          const { requirementId, version } = node;
          const card = (
            <g
              key={node.id}
              transform={`translate(${x} ${y})`}
              className={`${styles.diagramNode} ${styles.kind}`}
              data-kind={node.kind}
            >
              <title>{node.label}</title>
              <rect width={nodeWidth} height={nodeHeight} rx="8" />
              <text x="12" y="24" className={styles.diagramKind}>
                {requirementId && version
                  ? t(locale, 'processGraph.requirementVersion', { version })
                  : t(locale, `processGraph.node.${node.kind}`)}
              </text>
              <text x="12" y="48">
                {shortText(node.label, 22)}
              </text>
            </g>
          );
          if (!requirements || !requirementId || !version) return card;
          const ref = t(locale, 'processGraph.requirement', { id: requirementId, version });
          return (
            <a
              key={node.id}
              className={styles.diagramNodeLink}
              aria-label={`${node.label} · ${ref}`}
              href={requirementNodeURL(projectId, requirementId, version, evidence)}
            >
              {card}
            </a>
          );
        })}
      </svg>
    </section>
  );
}
