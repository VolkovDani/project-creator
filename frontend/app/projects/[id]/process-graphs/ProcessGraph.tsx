import Link from 'next/link';
import { useId } from 'react';
import type { ProcessGraphMode } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import { requirementURL } from '../../../../lib/rag';
import { ProcessGraphDiagram } from './ProcessGraphDiagram';
import styles from './process-graphs.module.css';
import {
  processGraphMermaid,
  requirementGraphMarkdown,
  requirementGraphMermaid,
} from './processGraphMermaid';
import { type GraphDraft, type GraphNode, groupProcessGraph } from './processGraphView';

function download(text: string, filename: string, type = 'text/plain') {
  const url = URL.createObjectURL(new Blob([text], { type: `${type};charset=utf-8` }));
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

export function ProcessGraph({
  draft,
  mode,
  locale,
  projectId,
}: {
  draft: GraphDraft;
  mode: ProcessGraphMode;
  locale: Locale;
  projectId: string;
}) {
  const heading = useId();
  const { groups, isolated } = groupProcessGraph(draft.nodes, draft.edges);
  const requirements = mode === 'REQUIREMENTS';
  const kinds = requirements ? [] : [...new Set(draft.nodes.map((node) => node.kind))];
  const title = t(locale, requirements ? 'processGraph.requirementsDraft' : 'processGraph.draft');
  const connections = t(locale, 'processGraph.connections');
  const partial = draft.partial ? t(locale, 'processGraph.partial') : undefined;
  const relation = (kind: string) => t(locale, `processGraph.edge.${kind}`);

  function downloadMermaid() {
    const origin = window.location.origin;
    if (requirements) {
      download(requirementGraphMermaid(draft, projectId, origin, title), 'requirement-diagram.mmd');
    } else {
      download(processGraphMermaid(draft, projectId, origin, relation), 'process-graph.mmd');
    }
  }

  function downloadMarkdown() {
    const text = requirementGraphMarkdown(draft, projectId, window.location.origin, {
      heading: title,
      connections,
      relation,
      partial,
    });
    download(text, 'requirement-diagram.md', 'text/markdown');
  }

  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>{title}</h2>
      <p>{t(locale, 'processGraph.sourceCount', { count: draft.sourceCount })}</p>
      {partial && <p className={styles.partial}>{partial}</p>}
      <div className={styles.downloads}>
        <button type="button" onClick={downloadMermaid}>
          {t(locale, 'processGraph.downloadMermaid')}
        </button>
        {requirements && (
          <button type="button" onClick={downloadMarkdown}>
            {t(locale, 'processGraph.downloadMarkdown')}
          </button>
        )}
      </div>
      {kinds.length > 0 && (
        <ul className={styles.legend} aria-label={t(locale, 'processGraph.legend')}>
          {kinds.map((kind) => (
            <li className={styles.kind} data-kind={kind} key={kind}>
              <span className={styles.legendSwatch} aria-hidden="true" />
              {t(locale, `processGraph.node.${kind}`)}
            </li>
          ))}
        </ul>
      )}
      <ProcessGraphDiagram draft={draft} locale={locale} mode={mode} projectId={projectId} />
      <h3>{connections}</h3>
      <ul className={styles.graph}>
        {groups.map(({ from, branches }) => (
          <li className={styles.group} key={from.id}>
            <Node node={from} locale={locale} projectId={projectId} />
            <ul className={styles.branches}>
              {branches.map(({ edge, to }) => (
                <li
                  key={JSON.stringify([edge.to, edge.kind, edge.label])}
                  className={styles.branch}
                >
                  <div className={styles.relation}>
                    <span>{relation(edge.kind)}</span>
                    {edge.label && <strong>{edge.label}</strong>}
                  </div>
                  <Node node={to} locale={locale} projectId={projectId} />
                  <details className={styles.evidence}>
                    <summary>{t(locale, 'processGraph.evidence')}</summary>
                    <ul>
                      {edge.evidence.map((source) => (
                        <li
                          key={JSON.stringify([
                            source.requirementId,
                            source.version,
                            source.start,
                            source.end,
                          ])}
                        >
                          <blockquote>{source.quote}</blockquote>
                          <Link
                            href={requirementURL(projectId, source.requirementId, {
                              sourceVersion: source.version,
                              start: source.start,
                              end: source.end,
                            })}
                          >
                            {t(locale, 'processGraph.requirement', {
                              id: source.requirementId,
                              version: source.version,
                            })}
                          </Link>
                        </li>
                      ))}
                    </ul>
                  </details>
                </li>
              ))}
            </ul>
          </li>
        ))}
        {isolated.map((node) => (
          <li key={node.id}>
            <Node node={node} locale={locale} projectId={projectId} />
          </li>
        ))}
      </ul>
    </section>
  );
}

function Node({ node, locale, projectId }: { node: GraphNode; locale: Locale; projectId: string }) {
  const { requirementId, version } = node;
  return (
    <div className={`${styles.node} ${styles.kind}`} data-kind={node.kind}>
      {requirementId && version ? (
        <Link href={requirementURL(projectId, requirementId, { sourceVersion: version })}>
          {t(locale, 'processGraph.requirement', { id: requirementId, version })}
        </Link>
      ) : (
        <span>{t(locale, `processGraph.node.${node.kind}`)}</span>
      )}
      <strong>{node.label}</strong>
    </div>
  );
}
