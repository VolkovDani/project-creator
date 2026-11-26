import Link from 'next/link';
import { useId } from 'react';
import { type Locale, t } from '../../../../lib/i18n';
import { requirementURL } from '../../../../lib/rag';
import { ProcessGraphDiagram } from './ProcessGraphDiagram';
import styles from './process-graphs.module.css';
import { type GraphDraft, type GraphNode, groupProcessGraph } from './processGraphView';

export function ProcessGraph({
  draft,
  locale,
  projectId,
}: {
  draft: GraphDraft;
  locale: Locale;
  projectId: string;
}) {
  const heading = useId();
  const { groups, isolated } = groupProcessGraph(draft.nodes, draft.edges);
  const kinds = [...new Set(draft.nodes.map((node) => node.kind))];
  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>{t(locale, 'processGraph.draft')}</h2>
      <p>{t(locale, 'processGraph.sourceCount', { count: draft.sourceCount })}</p>
      {draft.partial && <p className={styles.partial}>{t(locale, 'processGraph.partial')}</p>}
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
      <ProcessGraphDiagram draft={draft} locale={locale} />
      <h3>{t(locale, 'processGraph.connections')}</h3>
      <ul className={styles.graph}>
        {groups.map(({ from, branches }) => (
          <li className={styles.group} key={from.id}>
            <Node node={from} locale={locale} />
            <ul className={styles.branches}>
              {branches.map(({ edge, to }) => (
                <li
                  key={JSON.stringify([edge.to, edge.kind, edge.label])}
                  className={styles.branch}
                >
                  <div className={styles.relation}>
                    <span>{t(locale, `processGraph.edge.${edge.kind}`)}</span>
                    {edge.label && <strong>{edge.label}</strong>}
                  </div>
                  <Node node={to} locale={locale} />
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
            <Node node={node} locale={locale} />
          </li>
        ))}
      </ul>
    </section>
  );
}

function Node({ node, locale }: { node: GraphNode; locale: Locale }) {
  return (
    <div className={`${styles.node} ${styles.kind}`} data-kind={node.kind}>
      <span>{t(locale, `processGraph.node.${node.kind}`)}</span>
      <strong>{node.label}</strong>
    </div>
  );
}
