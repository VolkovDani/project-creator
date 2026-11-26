'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../lib/client';
import {
  type RequirementSummariesByIdsQuery,
  RequirementSummaryProvenanceDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import ResourceState from '../../components/ResourceState';

type Props = {
  summary: Pick<
    RequirementSummariesByIdsQuery['requirementSummariesByIds'][number],
    'projectId' | 'requirementId' | 'version' | 'profile' | 'createdAt'
  >;
  locale: Locale;
};

export default function RequirementSummaryProvenance({ summary, locale }: Props) {
  const [open, setOpen] = useState(false);
  const { projectId, requirementId, version, profile, createdAt } = summary;
  const identity = JSON.stringify([projectId, requirementId, version, profile, createdAt]);
  return (
    <details className="data-group" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary>
        <h3>{t(locale, 'summary.provenance')}</h3>
      </summary>
      {open && <ProvenanceData key={identity} summary={summary} locale={locale} />}
    </details>
  );
}

function ProvenanceData({ summary, locale }: Props) {
  const { projectId, requirementId, version, profile, createdAt } = summary;
  const summariesTag = `project:${projectId}:summaries`;
  const requirementsTag = `project:${projectId}:requirements`;
  const load = useCallback(async () => {
    const result = await query(
      RequirementSummaryProvenanceDocument,
      { projectId, requirementIds: [requirementId] },
      {
        tags: [summariesTag, requirementsTag],
        version: JSON.stringify([version, profile, createdAt]),
      },
    );
    const [current] = result.requirementSummariesByIds;
    if (
      current?.projectId !== projectId ||
      current.requirementId !== requirementId ||
      current.status !== 'ready' ||
      current.version !== version ||
      current.profile !== profile ||
      current.createdAt !== createdAt
    )
      return null;
    return current;
  }, [projectId, requirementId, version, profile, createdAt, summariesTag, requirementsTag]);
  const { data, loading, error, reload } = useResource(load, [summariesTag, requirementsTag]);
  if (loading || error)
    return (
      <ResourceState
        locale={locale}
        loading={loading}
        error={error}
        hasData={false}
        retry={() => void reload()}
      />
    );
  if (!data) return <p role="status">{t(locale, 'summary.provenanceUnavailable')}</p>;
  return (
    <dl>
      <dt>{t(locale, 'summary.version')}</dt>
      <dd>{data.version}</dd>
      <dt>{t(locale, 'summary.profile')}</dt>
      <dd>{data.profile || t(locale, 'summary.unknown')}</dd>
      <dt>{t(locale, 'summary.origin')}</dt>
      <dd>
        <pre>{data.provenance || t(locale, 'summary.unknown')}</pre>
      </dd>
      <dt>{t(locale, 'summary.createdAt')}</dt>
      <dd>{data.createdAt}</dd>
    </dl>
  );
}
