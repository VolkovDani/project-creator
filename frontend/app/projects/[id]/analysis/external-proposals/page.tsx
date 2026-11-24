'use client';
import Link from 'next/link';
import { useCallback } from 'react';
import { query } from '../../../../../lib/client';
import {
  ExternalProposalsDocument,
  type ExternalProposalsQuery,
} from '../../../../../lib/generated';
import { type Locale, t } from '../../../../../lib/i18n';
import { requirementURL } from '../../../../../lib/rag';
import { useInfinite } from '../../../../../lib/useInfinite';
import { LoadMore } from '../../../../components/ListControls';
import ResourceState from '../../../../components/ResourceState';
import { useProject } from '../../ProjectShell';
import styles from './proposals.module.css';

type Proposal = ExternalProposalsQuery['externalProposals']['items'][number];

function ProposalCard({
  proposal,
  date,
  projectId,
  locale,
}: {
  proposal: Proposal;
  date: Intl.DateTimeFormat;
  projectId: string;
  locale: Locale;
}) {
  return (
    <article className="card">
      <h3>{t(locale, 'externalProposals.analysis')}</h3>
      <dl className={styles.metadata}>
        <div>
          <dt>{t(locale, 'externalProposals.status')}</dt>
          <dd>{t(locale, `externalProposals.status.${proposal.status}`)}</dd>
        </div>
        <div>
          <dt>{t(locale, 'externalProposals.client')}</dt>
          <dd>{proposal.client}</dd>
        </div>
        <div>
          <dt>{t(locale, 'externalProposals.model')}</dt>
          <dd>{proposal.model}</dd>
        </div>
        <div>
          <dt>{t(locale, 'externalProposals.sharing')}</dt>
          <dd>{t(locale, `externalProposals.sharing.${proposal.sharing}`)}</dd>
        </div>
        <div>
          <dt>{t(locale, 'externalProposals.created')}</dt>
          <dd>
            <time dateTime={proposal.createdAt}>{date.format(new Date(proposal.createdAt))}</time>
          </dd>
        </div>
      </dl>
      <p className="user-content">{proposal.analysis}</p>
      <h4>{t(locale, 'externalProposals.limitations')}</h4>
      <p className="user-content">{proposal.limitations}</p>
      <h4>{t(locale, 'externalProposals.sources')}</h4>
      <ul className={styles.sources}>
        {proposal.sources.map((source) => (
          <li key={source.requirementId}>
            {source.status === 'current' || source.status === 'stale' ? (
              <Link href={requirementURL(projectId, source.requirementId)}>
                {source.requirementId}
              </Link>
            ) : (
              <span>{source.requirementId}</span>
            )}
            {' · '}
            <span>{t(locale, 'requirements.version', { version: source.version })}</span>
            {' · '}
            <span>{t(locale, `externalProposals.source.${source.status}`)}</span>
          </li>
        ))}
      </ul>
    </article>
  );
}

export default function ExternalProposalsPage() {
  const { project, locale } = useProject();
  const projectId = project.id;
  const proposalTag = `project:${projectId}:external-proposals`;
  const requirementTag = `project:${projectId}:requirements`;
  const load = useCallback(
    async (after: string | null) => {
      const {
        externalProposals: { items, pageInfo },
      } = await query(
        ExternalProposalsDocument,
        { projectId, after },
        { tags: [proposalTag, requirementTag] },
      );
      return { edges: items.map((node) => ({ node, cursor: node.id })), pageInfo };
    },
    [projectId, proposalTag, requirementTag],
  );
  const { rows, loaded, loading, error, hasNextPage, loadMore, retry } = useInfinite({
    identity: JSON.stringify(['externalProposals', projectId]),
    tags: [proposalTag, requirementTag],
    load,
  });
  const date = new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' });

  return (
    <section aria-labelledby="external-proposals-heading">
      <h2 id="external-proposals-heading">{t(locale, 'externalProposals.title')}</h2>
      <p>{t(locale, 'externalProposals.help')}</p>
      <p>
        <Link href={`/projects/${encodeURIComponent(projectId)}/analysis`}>
          {t(locale, 'externalProposals.back')}
        </Link>
      </p>
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={loaded}
        retry={() => void retry().catch(() => {})}
      />
      {loaded && !error && rows.length === 0 && <p>{t(locale, 'externalProposals.empty')}</p>}
      {rows.map((proposal) => (
        <ProposalCard
          key={proposal.id}
          proposal={proposal}
          date={date}
          projectId={projectId}
          locale={locale}
        />
      ))}
      <LoadMore
        locale={locale}
        hasNextPage={hasNextPage}
        loading={loading}
        error={error}
        loadMore={() => void loadMore().catch(() => {})}
        observe
      />
    </section>
  );
}
